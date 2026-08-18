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
- [x] **Sensor calibration writes.** An uncalibrated probe reports and is then ignored, which looks identical to a working one, so the state is on every row and tapping it fixes it. The sheet spends its room on when to take each reading rather than asking for two numbers, and refuses backwards baselines with the reason: the other way round, a soaked pot reads as bone dry and gets watered again
- [x] **Photos render.** `PlantPhotoView` loads the timeline's presigned link, keeping bytes just captured ahead of the network so a new photo appears instantly, and falling back to the stand-in when a link has expired
- [x] **Every write asserts its own method and path**, which is the check that would have caught the harvest route on either side
- [x] **Photo comparison scrubber.** The first photo stays put and the scrubber moves the second through time, because the question is whether it is better than when it arrived, not better than last Tuesday. Side by side rather than a wipe: handheld shots weeks apart never line up, so a sliding divider would be comparing backgrounds. The gap is said in words, and the same day says so outright rather than reading "0 days apart"
- [ ] Notifications, which need a push payload contract that does not exist yet

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
- [x] **Closed eyes.** Joey picked closed over the recommended open: "open looks like a maniac". The app already shipped the closed artwork, so nothing needed redrawing

## 10. Hardening added after the checklist was done

- [x] **19 API handler tests**, run against a real Postgres. The package had 23 routes and no tests at all
- [x] **Errors say whose fault they are.** Every write handler used to pass 400 as its default, so a database outage during a PATCH reported "Bad Request" and sent whoever was debugging it to look at the request. `plant.ErrInvalid` now marks caller mistakes, Postgres constraint violations are classified into it, and `fail` does the mapping so no call site has to guess
- [x] **Constraint names are translated.** `dripper_needs_letpot` tells a person nothing; the reply now says a dripper number only means something on the LetPot line
- [x] **Migrations are serialised.** Every command migrates on start, so a deployment and a CronJob landing together raced on a fresh database. Fixed with a Postgres advisory lock across processes and a mutex within one, because goose keeps its dialect and filesystem in package globals and the race detector caught it racing inside the library
- [x] Server faults are logged before being returned, so a 500 leaves a trace
- [x] **Judge and Home Assistant packages tested.** `replay` rebuilds a diagnosis conversation with index arithmetic nobody had checked; the API rejects two messages in a row from the same role, so a slip there would make every follow-up fail outright. Now proven to alternate for 1, 2, 3 and 6 prior turns, to send the photographs only once, and to pair each answer with the question it answered
- [x] **A pot with no drainage hole was only reported if somebody had also recorded the material.** It is the most common way a plant drowns, and it was being dropped from both the daily judgment and the autopsy. Now reported on its own, covered by a table over every combination
- [x] **A stale weather forecast is rejected.** Today's daily entry is past-dated by the afternoon yet carries tonight's low, so it has to survive; anything older than a day describes a night that has been and gone
- [x] **The cold watch is tested end to end**, against a fake Home Assistant that records what was sent: who gets named, that the margin warns early, that a sheltered plant is not warned about twice, that the tenderest plant gates everyone going back out, and that the warning follows you to your backup while you are away
- [x] **Each test package gets its own database.** `go test ./...` runs packages in parallel, so tests that read the whole garden were being decided by another package's rows
- [x] **The cold warning can be answered from the app's own world, not just curl.** The Dusk plugin gained `shelter_plants` and `unshelter_plants`, so the answer to a notification on a phone is a sentence to an agent. An empty form is refused rather than treated as "all", because a half-filled one would otherwise move the whole garden

## 11. Bugs found while building

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
- [x] **Half the cold feature was unreachable.** `Shelter` and `Unshelter` existed in the store and were tested, but nothing could call them: no route, no client, nothing. The warning to bring plants in could never be answered, so it would have repeated every afternoon forever and no plant would ever have become eligible to go back out. Now `POST /v1/shelter` and `POST /v1/unshelter`, and the warning itself says to reply
- [x] **Both clients were blind to whether a plant is indoors.** `sheltered_at` has always been on the wire and neither the app nor the Dusk plugin decoded it, because the data model never documented it. So an agent asked which plants are inside could not answer and would have offered to put out ones that never came in. Documented now, read by both, and the plugin says it in the description an agent reads first
- [x] **The cold warning can be answered from the phone it arrives on.** A plant with a cold threshold carries a row saying where it is and offering the other side. The local state moves on success only, so a refused write never leaves the button claiming a plant is inside when the service never heard about it
- [x] **Nothing waters a plant on a timer, and now the reporting half exists.** I reported `water` having no CronJob as an oversight; it was not. `deploy/README.md` said plainly that a job which moves water does not get scheduled, and I had not read it. Joey then confirmed the rule outright: he wants to be told, and he turns the pump on himself. So `water` stays manual and is recorded as deliberately manual in the coverage test, and a new `planty thirst` does the part that was actually missing: twice a day it reads every calibrated probe and names the dry plants, covering hand-watered ones as well as the LetPot line, since most are watered by hand and those are the ones that get forgotten. It needs no pump and no API key
- [x] **Nothing checked that a command anybody can run is a command something runs.** Now two tests do, in both directions: a documented command must be scheduled or say in `manual` why it is run by hand, and a scheduled command must still exist. Verified by unscheduling `water` and watching it fail
- [x] **An unconfigured pump is no longer a fault.** The check ran before the plant query, so an hourly job on a garden with no LetPot line at all would have failed every hour forever, and a failure that fires every hour is one nobody reads. Nothing on the line is now a quiet no-op; plants on the line with no pump behind them stays loud, because that is watering silently not happening
- [x] **The app could not display a photo it had not just taken.** The timeline mints a presigned `url` per photo and always had; `Photo` never decoded it and `PlantPhotoView` had no remote path, so every photo in the story rendered as the placeholder. Both fixed, and the placeholder is now the fallback it was meant to be rather than the only outcome
- [x] **The story was built from the timeline alone, which returns photos and nothing else.** Observations, readings and the current verdict all arrive on `GET /v1/plants/{slug}`, which the app already fetched alongside it and then ignored, so the story was pictures with no reason for any of them and the sensor evidence disclosure was permanently empty. `PlantTimeline.merging(_:)` joins the two
- [x] **The test suite shared one database across packages running in parallel.** The cold watch reads every plant and away period, so the API package's fixtures were quietly deciding the job package's results. It only ever failed in the full suite, never when a package was run on its own
- [x] **Every harvest ever logged would have 404d.** The iOS app and the Dusk plugin both posted to a flat `/v1/harvests`; the service has only ever served `POST /v1/plants/{slug}/harvests`, which is exactly what the data model documented. Two independent clients drifted the same way from a correct contract, because nothing on either side tested which URL it posted to. Both fixed, and the path is now pinned by a test on all three sides
- [x] `SensorByEntity` was dead: no caller, no test. Ingest reads every link once and maps them in memory, which is one query rather than one per entity
