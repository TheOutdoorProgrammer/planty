# Planty build checklist

Everything discussed, plus three additions.
Nothing is done until it builds, is tested, and is committed.

## Current state

**Go:** 32 unit tests plus 9 Postgres integration tests, green under `-race`. 22 routes, 10 commands, 5 migrations.
**iOS:** 92 tests in 12 suites, `** TEST SUCCEEDED **`, verified independently. Release builds for simulator and arm64 device, zero warnings.
**Dusk plugin:** 80 tests under `-race`, goreleaser green on four targets.

## 1. Backend service

- [x] Domain model for every entity
- [x] Postgres schema, four migrations, enums, check constraints, partial indexes
- [x] Store layer, including sparse patch update
- [x] 22 HTTP routes
- [x] Archive instead of delete
- [x] Sparse creates and patches, so an agent can work from half a sentence
- [x] Sensor links, calibration, readings ingest
- [x] Verdicts, the `/v1/today` digest, and the escalation ladder
- [x] Home Assistant client: states, forecast, notify, announce
- [x] Claude judgment: daily verdicts, vision diagnosis, autopsies
- [x] **Diagnosis is a conversation**: turns persist, follow-ups replay the earlier answers, and the reply separates what is seen from what it means from what to do today
- [x] Photo storage to MinIO, timeline with presigned links
- [x] CLI: serve, ingest, daily, cold, away, chase, water, autopsy, seed, migrate
- [x] Dockerfile, CI, release workflow, manifests, five CronJobs

## 2. The things that stop plants dying

- [x] **Cold snap watch**, forecast-driven, 3F margin biased toward the cheap mistake
- [x] **Put them back out**, gated on every sheltered plant clearing its own threshold
- [x] **Runaway safety**, duration held by the process with a deferred pump-off
- [x] **Hydrophobic soil detection**, covered by integration test
- [x] **Per-sensor calibration**, uncalibrated probes refuse to answer
- [x] **Friend's plants are a stricter tier**
- [x] **Sensor priority by accessibility**
- [x] **Calm, stale and never-run are three distinct states**, enforced on both sides
- [x] **Hand-watered plants nag harder**, bounded three-rung ladder, speakers only on the last rung for a plant whose neglect costs something
- [x] **Closed watering loop written**, including a single wet plant vetoing the whole line
- [ ] Watering loop needs the HSTEP component installed and the pump exposed as a switch (hardware, not code)

## 3. Three domains

- [x] houseplant, edible_indoor, edible_outdoor
- [x] Care profile: maturity, sow and transplant dates, frost dates, succession
- [x] Harvest logging
- [x] **Tomato pollination** carried and surfaced to the judge
- [x] Fan schedule as a Home Assistant automation, doubling as pollination agitation
- [x] Reminders: the daily digest plus the escalation ladder

## 4. Mushroom kit: deliberately NOT automated

- [x] Daily reminder, gated on an `input_boolean` so it stops when the block is spent
- [x] Fan on a fixed schedule
- [x] The reasoning written down: an RH threshold over mists, and over misting is how bacterial blotch happens

## 5. Dusk plugin

- [x] Builds, 80 tests under `-race`, goreleaser green
- [x] Full CRUD as actions, storing nothing in Dusk
- [x] `partial: true` on an unreachable service, tested both ways
- [x] Dry run on every action, four ADRs
- [x] Added to `nerdswhofish/go.work`
- [x] Emits autopsy lessons as `gotcha` notes on the plant they are about
- [x] Repo initialised and committed; the agent had left it untracked
- [ ] Wire into Dusk config and mint the `plant` kind as `reference` (Joey's call)

## 6. iOS app

- [x] Xcode project, 48 app files, 10 test files
- [x] **92 tests in 12 suites passing**, verified independently
- [x] Release builds for simulator and arm64 device, zero warnings, Swift 6 strict concurrency
- [x] Calm versus stale as a pure function with 20+ tests; staleness removes reassurance, never an alarm
- [x] Three tabs, diagnosis from a photo, no health claims, purple badges that only change sort order
- [x] **Diagnosis is live**, talking to the real endpoint, 92 tests still green after the swap
- [ ] Photo comparison scrubber, sensor calibration writes, notifications

## 7. Three additions

### 7a. Away mode

- [x] Away periods, backup contact, cold watch and chase both escalate to the backup
- [x] Pre-departure pass, return briefing

### 7b. Post-mortem

- [x] History gathering, reading thinning, Claude analysis, storage, `planty autopsy`
- [x] Sweeps automatically with the daily job
- [x] **Writes a Dusk gotcha attached to the plant**, so a death teaches every future session and not just the one that noticed. `GET /v1/postmortems` on the service, emitted as notes by the plugin, deduped by Dusk on content hash

### 7c. Ask-the-owner queue

- [x] Queue, API, `as_text` rendering, seeded with seven real questions
- [x] `asked_of` defaults to the plant's steward

## 8. Seed data

- [x] The friend's five with his exact words, all at 55F, validated in CI

## 9. Brand

- [x] Mascot: the seal. Hero and icon, open and closed eyes, measured scale tests
- [x] Wordmarks for dark and light backgrounds
- [x] `design/MASCOT.md` resolves the drift across three older design docs
- [ ] Joey picks open versus closed

## 10. Bugs found while building

- [x] `.gitignore`: unanchored `planty` also matched the `cmd/planty` source directory, so `main.go` was never committed
- [x] `nerdswhofish/go.work` did not list the new plugin
- [x] **The digest join emitted invalid SQL.** Splitting a column list on commas shredded every `coalesce(x, '')`. Compiled, vetted, passed every unit test, failed only against real Postgres. Now generated, and covered
- [x] `stale_since` used a zero-time sentinel for "never ran". Now an explicit `never_run`
- [x] iOS `RelativeAge.phrase` ignored its `now` parameter, so "3 days ago" rendered as "last year"
- [x] iOS requested camera permission on launch, because `TabView` builds neighbouring tabs eagerly
- [x] **Client and server had drifted apart:** the app assumed multipart upload and `/diagnosis`, the service shipped raw bytes and `/diagnose`. Server now accepts both upload shapes and uses `/diagnosis`
- [x] `GET /v1/plants/{slug}` did not return what the contract promised. Now includes readings and the current verdict
- [x] The plugin's budget test asserted a magic number (two requests) rather than the invariant it cared about. Rewritten to assert the count does not scale with garden size, which is the property that actually matters
- [x] The first autopsy note was rejected by the SDK's own conformance validator: notes require `Provenance`, because Dusk never infers where a claim came from
