# GPT audit findings

Audited on 2026-08-19 against `main` at `f7ef503`.

This is a working backlog, not a victory lap.
An item is checked only after the implementation, regression test, commit, and push are complete.

## Baseline

- `go test -race ./...` passes, but most Postgres-backed tests skip unless `PLANTY_TEST_DATABASE_URL` is set.
- `go vet ./...` passes.
- The iOS suite passed all 282 tests in 41 suites on the iPhone 17 simulator during the audit.
- The live LAN service loads 13 plants in the simulator and the Today, unconfigured, library, and Snap states were visually inspected.
- Default Go coverage is misleadingly thin in the risky packages: `cmd/planty` 0%, `internal/api` 2.6%, `internal/job` 11.4%, and `internal/store` 0.8% without the integration database.

## P0: safety and core-product failures

- [ ] **PNT-001 — Closed-loop watering verifies before a sensor can report.**
  `internal/job/water.go:22-23` defines a 45-minute `SettleWindow`, but `internal/job/water.go:163-169` waits only for the pump duration and immediately verifies without ingesting fresh Home Assistant state.
  The normal missing post-watering reading becomes `store.ErrNotFound`, which `internal/job/water.go:198-201` silently treats as success.
  The right fix is a durable watering-attempt record that a later ingest verifies after `verify_after`; sleeping in the pump process would still lose verification on restart.

- [x] **PNT-002 — Stale moisture readings can turn the pump on.**
  `internal/job/water.go:82-121` uses the latest value without checking `TakenAt`.
  A disconnected probe whose last reading was dry can therefore trigger watering indefinitely, while a stale wet reading can suppress needed water.
  Fixed by treating readings older than two normal ingest cycles as blind, refusing the shared line, and testing both the freshness boundary and that stale dry evidence calls no Home Assistant service.

- [ ] **PNT-003 — A hard process or node failure can leave the pump on indefinitely.**
  `internal/job/water.go:145-161` relies on a Go `defer` to turn the switch off, which does not run after SIGKILL, OOM, node loss, or power failure.
  `PumpSensor` is configured in `cmd/planty/main.go:265-272` but never read.
  Home Assistant or the device must own an independent maximum-on failsafe, with Planty verifying both on and off.

- [ ] **PNT-004 — Failed or unknown water delivery is recorded as successful watering.**
  `internal/job/water.go:185-207` writes `ObservedWatered` before verification and still writes it when verification has no reading.
  That resets last-watered state, suppresses reminders, and feeds false evidence into later judgments for the exact clogged-dripper case Planty exists to catch.

- [x] **PNT-005 — Historical open verdicts remain actionable after newer judgments supersede them.**
  `internal/store/verdicts.go:25-35` creates a row per day, while digest and escalation queries select every unacknowledged actionable row at `internal/store/verdicts.go:106-116` and `internal/store/escalate.go:15-28`.
  Yesterday's water instruction can remain visible and escalating after today's judgment says none or something different.
  Fixed by transactionally superseding prior verdicts, serializing concurrent judgments per plant, and enforcing one open verdict per plant with a partial unique index documented in ADR 0004.

- [ ] **PNT-006 — A partially failed daily run can look fresh, complete, and all clear.**
  `internal/job/daily.go:42-70` tolerates any partial success, while `Digest` reports every live plant as checked and derives freshness from the global newest verdict at `internal/store/verdicts.go:88-103`.
  A single successful `none` result can therefore claim the entire garden was checked while every other judgment failed.
  This needs an ADR and a persisted judgment-run model with expected, succeeded, failed, and completion state.

- [x] **PNT-007 — Retrying a failed first-plant creation is a dead button.**
  `CaptureStore.createPlant` records a generic failed capture without the original operation at `ios/Planty/State/CaptureStore.swift:186-209`.
  The visible retry calls `retrySave`, which silently requires `selectedPlant` at `ios/Planty/State/CaptureStore.swift:107-109,174-177`; first-plant creation has none.
  Fixed by retaining the exact failed operation and creation payload, retrying it directly, refreshing the library after success, and covering the regression in the now-283-test iOS suite.

## P1: correctness, security, and data integrity

- [x] **PNT-008 — Photo capture time is sent by iOS and discarded by Go.**
  `ios/Planty/Networking/PlantyClient.swift:220-238` sends multipart `taken_at`, but `internal/api/photos.go:46-53,118-140` stores `time.Now()` and never reads it.
  Imported historical photos are placed at upload time, corrupting the timeline used for longitudinal comparisons.
  Fixed by accepting and validating RFC3339 `taken_at` on both multipart and raw upload shapes, defaulting only omitted timestamps to now, and adding regression tests for preservation and rejection.

- [x] **PNT-009 — Multipart image endpoints do not enforce their advertised request limits.**
  `internal/api/identify.go:158-175` bounds a local reader but parses the original `r.Body`, and `ParseMultipartForm` in `internal/api/photos.go:127` is a memory threshold rather than a request-size limit.
  Oversized unauthenticated requests can consume pod memory or the 1 GiB temporary volume.
  Fixed by bounding the actual multipart request before parsing, independently bounding each file, returning 413 for size failures, and testing raw and multipart limits.

- [ ] **PNT-010 — Conversation IDs can cross-contaminate plants and scratch chats.**
  `internal/api/consult.go:118-133`, `internal/api/ask.go:95-112`, and `internal/store/conversation.go:49-55` load by conversation UUID without asserting the plant or scratch owner.
  A stale conversation ID can replay one plant's context into another and save subsequent turns under the wrong record.

- [ ] **PNT-011 — The unauthenticated LAN API is writable through CSRF and DNS rebinding.**
  The server binds all interfaces, has no authentication or Origin/Host protection, and JSON handlers do not consistently require `application/json`.
  A hostile website opened by a LAN client can send simple blind writes even though CORS prevents it from reading responses.

- [x] **PNT-012 — Malformed shelter JSON can still execute a bulk move.**
  `internal/api/shelter.go:27-46` discards the decoder error and then inspects a partially populated request.
  A truncated body that decoded `"all": true` first can mutate every eligible plant.
  Fixed by rejecting decode failures before inspecting any request fields, with an integration test proving a partially decoded bulk request leaves the plant outside.

- [ ] **PNT-013 — Plant PATCH bypasses aggregate validation.**
  Creation calls `Plant.Valid`, but `internal/store/update.go:39-132` writes patch fields directly.
  Empty names can persist, and invalid enums become misleading 500s because SQLSTATE class 22 is not classified as caller error.

- [x] **PNT-014 — Archive accepts contradictory and invalid statuses.**
  `internal/api/plants.go:218-229` accepts any query status, unlike the agent CLI's `dead|gone` restriction.
  `DELETE ...?status=alive` creates an archived plant that still claims to be alive, while unknown enums can surface as 500.
  Fixed with one domain validator shared by HTTP, agent CLI, and store boundaries, plus unit and Postgres-backed regression tests.

- [x] **PNT-015 — Verification reads only the first soil probe.**
  Watering considers every calibrated soil link and lets the driest drive the line, but `internal/job/ingest.go:75-88` returns after the first soil link.
  Probe creation order can therefore decide whether delivery is called successful or clogged.
  Fixed by evaluating every calibrated soil probe: any rise confirms delivery, every measured probe staying flat fails it, and no measurable result remains unknown.

- [ ] **PNT-016 — Photo deduplication races under concurrent retries.**
  `internal/store/photos.go:27-41` does a separate lookup and insert despite unique indexes.
  Racing identical uploads can both miss, one then fails rather than returning the existing photo, and the losing request can leave an orphaned object.

- [ ] **PNT-017 — Automatic slug allocation races.**
  `internal/store/plants.go:78-122,251-287` chooses a free slug before a separate insert.
  Concurrent same-name creates can pick the same slug and one fails instead of receiving the next suffix.

- [ ] **PNT-018 — Completing a verdict is a non-atomic, non-idempotent two-call workflow.**
  `TodayStore` and `CaptureStore` separately post an observation and then acknowledge the verdict.
  Failure between calls either duplicates care observations on retry or leaves completed work escalating.
  The server needs one transactional completion command with an idempotency key.

- [ ] **PNT-019 — Long-lived iOS stores accept stale out-of-order responses.**
  `TodayStore`, `PlantsStore`, and `PlantStoryStore` allow overlapping loads and publish whichever response finishes last.
  `AppSession.updateConfiguration` swaps clients without invalidating in-flight operations, so old-server data can repopulate a newly configured session.

- [ ] **PNT-020 — Overlapping identification can show results for the wrong photo.**
  `SnapScreen` launches unowned tasks and `IdentificationStore.identify` publishes every completion without cancellation or a photo-generation check.
  A slower result for photo A can overwrite the candidates displayed beside photo B.

- [ ] **PNT-021 — Owner-question answer failures discard the typed answer.**
  `TodayStore.answer` absorbs the failure, while `AnswerSheet` always dismisses after awaiting it.
  The error appears only after the exact answer text has been destroyed.

- [ ] **PNT-022 — Note-save failures are invisible inside the editor.**
  `NotesStore` records the error behind the presented sheet, while `NoteSheet` receives only a false Boolean and renders no failure.
  Save appears to do nothing even though the draft remains open.

- [ ] **PNT-023 — Photo viewer claims a save succeeded without observing the result.**
  `ios/Planty/Screens/Story/PhotoViewer.swift:73-107` calls `UIImageWriteToSavedPhotosAlbum` with no completion callback and immediately displays `Saved to Photos`.
  Denied permission and write failures therefore produce a false data-safety claim.

- [ ] **PNT-024 — Cancelling a consultation leaves an unrecoverable dangling question.**
  `ConsultStore.ask` optimistically appends the question, but cancellation neither removes it nor marks it retryable.
  The transcript can retain a question with no answer, failure state, or recovery action.

- [ ] **PNT-025 — Object-storage writes are not compensated after database failure.**
  Photo and scratch attachment handlers upload the object before saving the row and do not delete it when persistence fails.
  Database outages, uniqueness races, and cancellations can leak blobs no query can discover.

## P2: missing capabilities and usability debt

- [ ] **PNT-026 — Harvests are write-only.**
  The service, CLI, and iOS can record yield but cannot list, aggregate, correct, or delete it.
  This contradicts the data model's stated purpose of answering yield-per-plant-per-season questions.

- [ ] **PNT-027 — Harvest writes accept meaningless data.**
  There is no domain or database validation for positive finite quantity, a non-empty unit, plausible timestamps, or applicable plant domains, and no correction path exists.

- [ ] **PNT-028 — Away periods cannot be viewed, changed, or cancelled.**
  HTTP and agent CLI expose create only, iOS exposes nothing, and overlap behavior is undefined.
  A typo in dates or backup contact is permanent and the user cannot confirm what is active.

- [ ] **PNT-029 — iOS does not have the server and agent's advertised powers.**
  `PlantyAPI` lacks away planning, cold-watch query, full question list/create, postmortem history, toxicity editing, and harvest history.
  This directly contradicts the README claim that the app and plugin have the same powers.

- [ ] **PNT-030 — Questions after the fourth are unreachable in iOS.**
  `OpenQuestionsCard` renders only `prefix(4)` and displays the remaining count as inert text.
  There is no full queue screen or `GET /v1/questions` client method.

- [ ] **PNT-031 — Archived plants cannot be inspected or restored in iOS.**
  The client models `include_archived`, but `PlantsStore` always loads live plants and there is no archive surface.
  A mistaken death/archive cannot be repaired from the phone.

- [ ] **PNT-032 — Timeline and history routes silently truncate.**
  Plant detail caps observations at 20 and timeline caps photos at 24 without cursors or a partial-result signal.
  Older care events and photos disappear from the app as a plant ages.

- [ ] **PNT-033 — URL validation accepts unusable schemes and hostless values.**
  Settings accepts any URL with any scheme and defers the failure to URLSession.
  It should require an HTTP or HTTPS URL with a host and explain whether LAN HTTP is intentionally allowed.

- [ ] **PNT-034 — Camera and photo-import failures are silent.**
  Snap and consultation attachment flows both use `try?` and return without visible state on capture/import errors.

- [ ] **PNT-035 — Core controls do not reflow at Accessibility Dynamic Type sizes.**
  Plant actions are forced into a single row with labels shrunk to 75%, and photo comparison forces two 200-point panes side by side.
  These should use dynamic-type-aware vertical alternatives rather than shrinking meaningful labels.

- [ ] **PNT-036 — Note edit/delete actions are hidden behind long press.**
  Note cards expose their only edit/delete affordances through a context menu, which is poorly discoverable and weaker for Voice Control and switch access.

- [ ] **PNT-037 — iPad layouts stretch phone compositions edge to edge.**
  The target supports iPad, but primary content stacks use fixed phone padding and unlimited width.
  A reusable adaptive content container and deliberate tablet navigation are missing.

- [ ] **PNT-038 — The app globally overrides the user's appearance choice.**
  `PlantyApp` forces dark mode, ignoring system preference and removing a high-ambient-light option.

- [ ] **PNT-039 — Notifications are still missing.**
  `CHECKLIST.md` already acknowledges that the iOS app has no push payload contract or notification implementation.

- [ ] **PNT-040 — Species-identification cache entries never expire or follow configuration.**
  Disk cache keys contain only the Photos asset ID, so switching servers or improving the backend still returns an old result forever.

## P3: DRY, maintainability, and contract drift

- [ ] **PNT-041 — Async sheet save/error behavior is duplicated and inconsistent.**
  Six sheets independently implement saving, dismissal, and error propagation; some preserve and explain failures, some preserve silently, and one destroys input.
  Standardize one async result/error contract without forcing unrelated form layouts into one component.

- [ ] **PNT-042 — Photo acquisition is duplicated and already diverging.**
  Snap and consultation attachments separately own camera state, picker state, lifecycle, and the same silent-failure behavior.
  Extract a small photo-acquisition state object and keep screen-specific guidance in the views.

- [ ] **PNT-043 — `CaptureStore.upload()` appears dead.**
  No production or test caller references the method, yet it describes a second diagnosis upload flow that the current consultation path does not use.

- [ ] **PNT-044 — API contracts are manually duplicated across Go, Swift, tests, and prose.**
  Literal routes, enums, coding keys, and risk logic are repeated across several sources of truth.
  A machine-readable API contract should generate Swift wire models and client paths, while presentation logic remains handwritten.

- [ ] **PNT-045 — Defaults and conversation plumbing are duplicated inside Go.**
  API and agent plant defaults mirror one another, and plant/scratch conversation reconstruction is duplicated with different ownership and photo behavior.

- [ ] **PNT-046 — The API contract advertises a nonexistent diagnosis route.**
  `docs/DATA-MODEL.md` still lists `POST /v1/plants/{slug}/diagnosis`, while the server folded that behavior into consultation and registers no such route.

- [ ] **PNT-047 — Safety-critical pump duration configuration fails open.**
  Invalid, zero, or negative `PLANTY_PUMP_SECONDS` silently falls back to two minutes.
  Invalid safety configuration should prevent watering and report the bad value.

- [ ] **PNT-048 — Internal server errors are returned verbatim.**
  Every 5xx response serializes `err.Error()`, including Postgres, object storage, Home Assistant, and Claude CLI errors.
  Return stable public error codes with a request ID and keep wrapped details only in structured logs.

## Deliberately rejected audit claim

The first iOS pass claimed Settings validates an unauthenticated health route while real routes require a token.
That is not true of the current service: `internal/api/server.go` installs no authentication middleware and the README explicitly says there is no authentication.
The actual problem is PNT-011: the token field suggests protection that the service does not provide.
