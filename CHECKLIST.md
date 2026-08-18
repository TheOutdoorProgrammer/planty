# Planty build checklist

Everything discussed, plus three additions.
Nothing is done until it builds, is tested, and is committed.

## Current state

23 unit tests plus 7 Postgres integration tests, all green under `-race`.
22 HTTP routes, 8 CLI commands, 3 migrations, verified end to end against a real Postgres.

## 1. Backend service

- [x] Domain model: Plant, Observation, SensorLink, Reading, Verdict, Photo, Harvest, Digest, Question, AwayPeriod, Postmortem
- [x] Postgres schema, three migrations, enums, check constraints, partial indexes
- [x] Store layer for every type, sparse patch update included
- [x] 22 HTTP routes
- [x] Archive instead of delete
- [x] Sparse creates and sparse patches, so an agent can work from half a sentence
- [x] Sensor links, calibration, readings ingest from Home Assistant
- [x] Verdicts and the `/v1/today` digest
- [x] Home Assistant client: states, forecast, notify, announce
- [x] Claude judgment (Opus 5, structured output, refusal handling)
- [x] Photo storage to S3/MinIO, timeline with presigned links
- [x] Vision diagnosis across a photo timeline
- [x] CLI: serve, ingest, daily, cold, away, autopsy, seed, migrate
- [x] Dockerfile, CI, release workflow, Kubernetes manifests, four CronJobs

## 2. The things that stop plants dying

- [x] **Cold snap watch**, forecast-driven with a 3F margin biased toward the cheap mistake
- [x] **Put them back out**, gated on every sheltered plant clearing its own threshold
- [x] **Runaway safety**, cron-scheduled so there is no `for:` clock to reset
- [x] **Hydrophobic soil detection**, verified by integration test
- [x] **Per-sensor calibration**, uncalibrated probes refuse to produce a fraction
- [x] **Friend's plants are a stricter tier**, weighted in `Risk()`
- [x] **Sensor priority by accessibility**, typed column feeding `Risk()`
- [x] **Calm, stale and never-run are three distinct states**, tested
- [ ] **Closed watering loop.** Needs the LetPot pump exposed through the HSTEP component
- [ ] **Hand-watered plants nag harder.** Risk scoring is in; repeat escalation is not

## 3. Three domains

- [x] houseplant, edible_indoor, edible_outdoor
- [x] Care profile: days to maturity, sow and transplant dates, frost dates, succession
- [x] Harvest logging with quantity and unit
- [x] **Tomato pollination** carried as `needs_pollination` and surfaced to the judge
- [x] Fan schedule written as a Home Assistant automation, doubling as pollination agitation
- [ ] Reminders beyond the daily digest

## 4. Mushroom kit: deliberately NOT automated

- [x] Daily reminder automation, gated on an `input_boolean` so it stops when the block is spent
- [x] Fan on a fixed schedule for fresh air exchange
- [x] The reasoning written down: an RH threshold over mists, and over misting is exactly how bacterial blotch happens

## 5. Dusk plugin (nerdswhofish/dusk-plugin-planty)

- [x] Builds, vets clean, **80 tests under `-race`**, goreleaser green on all four targets
- [x] Full CRUD as actions, proxied to the service, storing nothing in Dusk
- [x] `partial: true` on an unreachable service, tested from both sides
- [x] Dry run on every action
- [x] Four ADRs
- [x] Added to `nerdswhofish/go.work`
- [ ] Wire into Dusk config and mint the `plant` kind as `reference`

## 6. iOS app (SwiftUI)

- [x] 57 Swift files: models, networking, config, design system, screens
- [ ] Agent still running; needs compile confirmation and a review pass

## 7. Three additions

### 7a. Away mode

- [x] Away periods with a backup contact
- [x] Cold watch escalates to the backup while away
- [x] Pre-departure pass naming the hand-watered plants
- [x] Return briefing

### 7b. Post-mortem

- [x] Full history gathering, reading thinning, Claude analysis, storage
- [x] `planty autopsy <slug>`
- [ ] Trigger automatically on status change to dead
- [ ] Write a Dusk gotcha note attached to the plant

### 7c. Ask-the-owner queue

- [x] Queue, API, `as_text` rendering, seeded with the seven real questions
- [x] `asked_of` defaults to the plant's steward

## 8. Seed data

- [x] The friend's five, with the owner's exact words
- [x] All five at a 55F threshold
- [x] Tests assert every seeded plant validates and carries a threshold
- [x] Seeding runs in CI against a real Postgres

## 9. Brand

- [x] Mascot: the seal, open-eyed and closed-eyed, hero and icon, with measured scale tests
- [ ] Joey picks open versus closed
- [ ] Wordmark for dark and light backgrounds

## 10. Bugs found while building

- [x] `.gitignore`: unanchored `planty` also matched the `cmd/planty` source directory
- [x] `nerdswhofish/go.work` did not list the new plugin
- [x] **The digest join emitted invalid SQL.** Splitting a column list on commas shredded every `coalesce(x, '')` into two fragments. Compiled fine, passed every unit test, failed only against a real Postgres. Now generated rather than string-substituted, and covered by an integration test
- [x] `stale_since` used a zero-time sentinel to mean "never ran". Now an explicit `never_run`
