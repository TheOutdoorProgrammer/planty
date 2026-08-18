# Planty build checklist

Everything discussed, plus three additions. Nothing here is done until it builds and is committed.

## 1. Backend service (Go, mini-2, Postgres)

- [x] Domain model: Plant, Observation, SensorLink, Reading, Verdict, Photo, Harvest, Digest
- [x] Postgres schema with enums, constraints, partial indexes
- [x] Store layer: plants, observations, harvests
- [x] HTTP API: plants CRUD, observations, cold-watch
- [x] Archive instead of delete
- [x] Sparse creates, so an agent can add a plant from half a sentence
- [ ] Sensor links CRUD and calibration endpoints
- [ ] Readings ingest from Home Assistant
- [ ] Verdict storage and the `/v1/today` digest
- [ ] Photo upload to object storage, with the timeline endpoint
- [ ] Harvest endpoints
- [ ] Home Assistant client: read states, read history, call services, notify
- [ ] Claude client for daily judgment
- [ ] Vision analysis of photo timelines
- [ ] Daily judgment job
- [ ] Dockerfile, CI, Flux manifests

## 2. The things that stop plants dying

- [ ] **Cold snap watch.** Forecast-driven off `weather.nws_home`, not current temperature. 58F threshold, not 55F, because a porch at 3am is colder than the airport and a false positive costs a carried pot while a false negative kills someone else's plants
- [ ] **Bring them back out.** The half everyone forgets; five tropicals in a dark room for a week is its own way of killing them
- [ ] **Runaway safety.** `time_pattern` backstop, restart trigger, duration as a condition not a trigger, bounded repeat count, restored booleans. The valve that ran 14h33m past a 45 minute cap is why
- [ ] **Closed watering loop.** Sensor dry, pump runs, pump confirms, sensor confirms water arrived. If the pump ran and the soil did not change, that dripper is clogged
- [ ] **Hydrophobic soil detection.** A watering claim the sensor cannot confirm; dry soil channels water down the pot wall and out the drainage hole without wetting roots
- [ ] **Hand-watered plants nag harder.** They fail every time their owner is busy; the LetPot line only fails from a clog
- [ ] **Per-sensor calibration.** Relative to that probe's own dry and wet baselines. Never compare two sensors' absolute numbers. Uncalibrated links never drive an automated decision
- [ ] **Friend's plants are a stricter tier.** Tighter thresholds, faster escalation
- [ ] **Sensor priority by accessibility.** Hard-to-reach plants get sensors first, whatever they are worth
- [ ] **Stale data never renders as calm.** A failed run must not look like "nothing to do"

## 3. Three domains

- [ ] Houseplants: moisture, light, cold, not overwatering
- [ ] Indoor edibles: feeding, pollination, harvest windows
- [ ] Outdoor garden: frost dates, sowing calendar, succession, harvest
- [ ] **Tomato pollination.** Indoors they set no fruit without agitation; flowers drop and the plant looks healthy while yielding nothing
- [ ] **Fan control on the Shelly plug.** Airflow doubles as pollination
- [ ] Harvest logging and yield per plant per season
- [ ] Reminders

## 4. Mushroom kit: deliberately NOT automated

- [ ] Daily reminder only
- [ ] Fan on a fixed schedule for fresh air exchange
- [ ] Optional passive SNZB-02WD nearby, monitor only
- [ ] Never automate misting: the trigger is "looks dry", and an RH threshold over mists, which is exactly how bacterial blotch happens

## 5. Dusk plugin (nerdswhofish/dusk-plugin-planty)

- [ ] Mint the `plant` kind as `reference`
- [ ] Observe plants from the service into the catalog
- [ ] **Full CRUD as actions**, so an agent can do anything the app can
- [ ] Stores nothing in Dusk; everything goes to the service
- [ ] **`partial: true` when the service is unreachable**, or an outage looks identical to every plant being deleted
- [ ] Dry run on every action
- [ ] Declared views, no JavaScript
- [ ] ADRs

## 6. iOS app (SwiftUI)

- [ ] Today, Snap, Plants. Diagnosis is not a tab
- [ ] Calm state designed first; "nothing to do" is satisfying, not empty
- [ ] Camera-first capture
- [ ] Photo timeline per plant
- [ ] Diagnosis conversation from a photo
- [ ] Friend-owned badge, sorts first, no guilt chrome
- [ ] Sensor plots only under "Why Planty thinks this"
- [ ] Never claims health, only that no action is needed on current evidence
- [ ] Dynamic Type, VoiceOver, contrast that passes

## 7. Three additions

### 7a. Away mode

Joey flies constantly and his friend's plants are the ones with real stakes. Away mode changes behaviour rather than just muting: pre-water before departure, escalate to a named backup human instead of a phone nobody is holding, hold non-urgent nags, and produce a "here is what needs you" briefing on return. Can be driven from HA presence or a manual toggle.

- [ ] Away toggle and date range
- [ ] Pre-departure watering pass
- [ ] Backup contact escalation
- [ ] Return briefing

### 7b. Post-mortem

When a plant dies, generate an analysis from its whole history: what the readings did in the weeks before, what was done and when, and the most likely cause. The entire premise of this project is that Joey does not know what he is doing yet, so a death that teaches something is worth more than one that does not. This is why plants archive instead of deleting.

- [ ] Generate on status change to dead
- [ ] Reads readings, observations, photos and verdicts
- [ ] Writes a Dusk gotcha note attached to the plant

### 7c. Ask-the-owner queue

Questions needing the friend's answer collect in one place instead of being asked ad hoc, so one text goes out rather than ten. Several already exist: how many peace lilies, what species are the vines, what kind of bonsai, does the sequoia actually need to come in at 55F, is the porch covered, when is he back.

- [ ] Queue with open and answered states
- [ ] Agents and the app can both add
- [ ] Renders as a single copyable message
- [ ] Answers write back onto the plant

## 8. Seed data

- [ ] The friend's five: peace lilies, bonsai, vines, fern, sequoia sprout
- [ ] Their care rules from `docs/friends-plants.md`
- [ ] **The sequoia and the peace lilies must never share a dripper line.** Consistent moisture versus very little water is the sharpest group-by-thirst case in the collection

## 9. Brand

- [x] Mascot chosen: the seal
- [ ] Open-eyed variant, since closed eyes cost the vacant stare the joke depends on
- [ ] Separate simplified icon cropped to the head, because the full watering scene turns to mush well before 32px
- [ ] Wordmark for dark and light backgrounds
