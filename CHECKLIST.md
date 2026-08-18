# Planty build checklist

Everything discussed, plus three additions. Nothing here is done until it builds and is committed.

## 1. Backend service (Go, mini-2, Postgres)

- [x] Domain model: Plant, Observation, SensorLink, Reading, Verdict, Photo, Harvest, Digest
- [x] Postgres schema with enums, constraints, partial indexes
- [x] Store layer: plants, observations, harvests, sensors, verdicts, questions, away, postmortems
- [x] HTTP API: 18 routes across plants, observations, sensors, today, questions, away, harvests
- [x] Archive instead of delete
- [x] Sparse creates, so an agent can add a plant from half a sentence
- [x] Sensor links CRUD and calibration endpoints
- [x] Readings ingest from Home Assistant
- [x] Verdict storage and the `/v1/today` digest
- [x] Harvest endpoints
- [x] Home Assistant client: states, forecast, notify, announce
- [x] Claude client for daily judgment (Opus 5, structured output, refusal handling)
- [x] Daily judgment job
- [x] Job CLI: serve, ingest, daily, cold, seed, migrate
- [x] Dockerfile, CI, release workflow, Kubernetes manifests
- [ ] Photo upload to object storage, with the timeline endpoint
- [ ] Vision analysis of photo timelines

## 2. The things that stop plants dying

- [x] **Cold snap watch.** Forecast-driven off `weather.nws_home`, 3F margin on each plant's own threshold
- [x] **Runaway safety.** Cron-scheduled rather than duration-triggered, so no `for:` clock to reset
- [x] **Hydrophobic soil detection.** `VerifyWatering` compares moisture before and after a watering claim
- [x] **Per-sensor calibration.** `Fraction` refuses to answer for an uncalibrated probe
- [x] **Friend's plants are a stricter tier.** `Risk()` weights them, digest sorts on it
- [x] **Sensor priority by accessibility.** `Accessibility` is a typed column and feeds `Risk()`
- [x] **Stale data never renders as calm.** `Digest.AllClear()` requires fresh verdicts; tested
- [ ] **Bring them back out.** `WarmEnough` exists but is not wired to a job or a notification
- [ ] **Closed watering loop.** Needs LetPot pump control through the HSTEP component
- [ ] **Hand-watered plants nag harder.** Risk scoring is in; repeat escalation is not

## 3. Three domains

- [x] Domain enum covers houseplant, edible_indoor, edible_outdoor
- [x] Care profile carries days-to-maturity, sow/transplant dates, frost dates, succession
- [x] Harvest logging with quantity and unit
- [x] **Tomato pollination** recorded as `needs_pollination` and surfaced to the judge
- [ ] Fan control on the Shelly plug
- [ ] Reminders

## 4. Mushroom kit: deliberately NOT automated

- [ ] Daily reminder only
- [ ] Fan on a fixed schedule for fresh air exchange
- [x] Decision recorded in the README: never automate misting, an RH threshold over mists and that is exactly how bacterial blotch happens

## 5. Dusk plugin (nerdswhofish/dusk-plugin-planty)

- [x] Builds, vets clean, **79 tests passing**
- [x] Full CRUD as actions, proxied to the service
- [x] Stores nothing in Dusk
- [ ] Verify `partial: true` on unreachable service is actually implemented
- [ ] Verify dry run on every action
- [ ] ADRs
- [ ] Wire into Dusk config and confirm the `plant` kind mints

## 6. iOS app (SwiftUI)

- [x] 41 Swift files: models, networking, config, design system, screens
- [ ] Confirm it compiles against the iOS 26 simulator
- [ ] Verify calm versus stale is distinguishable in the UI
- [ ] Verify mascot never appears beside a severe alert

## 7. Three additions

### 7a. Away mode

- [x] Away period storage, `Covers`, `AwayAt`, `UpcomingAway`
- [x] Cold watch escalates to the backup contact when away
- [ ] Pre-departure watering pass
- [ ] Return briefing

### 7b. Post-mortem

- [x] Table, domain type, save and read
- [ ] Generate on status change to dead
- [ ] Write a Dusk gotcha note attached to the plant

### 7c. Ask-the-owner queue

- [x] Queue with open/answered/dropped states
- [x] API for agents and the app to add and answer
- [x] Renders as a single copyable message
- [x] Seeded with the seven real questions for the friend

## 8. Seed data

- [x] The friend's five: peace lilies, bonsai, vines, fern, sequoia sprout
- [x] Their care rules and the owner's exact words
- [x] All five carry a 55F cold threshold
- [x] Tests assert every seeded plant validates and has a threshold
- [x] The sequoia-versus-peace-lily opposite-water-needs warning recorded on the plant

## 9. Brand

- [x] Mascot chosen: the seal
- [x] Open-eyed and closed-eyed variants, hero and icon, with measured scale tests
- [ ] Joey picks open versus closed
- [ ] Wordmark for dark and light backgrounds

## 10. Found while building

- [x] `.gitignore` bug: unanchored `planty` also matched the `cmd/planty` source directory
- [x] `nerdswhofish/go.work` did not list the new plugin, breaking its tooling
