# Domain model and contract rationale

The service owns all of it.
The iOS app and the Dusk plugin are two clients of the same HTTP API with the same powers, and neither keeps its own copy.

This document explains the stable domain choices and behavior that are easy to misunderstand.
It is not an exhaustive schema reference: SQL migrations in `internal/store/migrations/` are canonical for storage, and `api/openapi.json` is canonical for the HTTP wire contract.

## Three domains, one table

Planty covers three kinds of growing, and they want different things:

| Domain | Goal | What matters |
| --- | --- | --- |
| `houseplant` | Keep it alive | Moisture, light, not overwatering, cold |
| `edible_indoor` | Get fruit | Feeding, pollination, much more light, harvest timing |
| `edible_outdoor` | Work with the season | Frost dates, sowing calendar, succession, harvest |

They are one table because they share almost everything: a thing in a place, with soil, that needs water, that you photograph and observe over time.
Splitting them would mean three of every query and three of every screen for one differing column group.

What differs is the **care profile**, which is where the domain-specific knowledge lives.

## What is typed and what is JSON

The split matters, so it is stated once.

**Typed columns for anything an automation or a query drives on.** If a rule needs to answer "which plants come in tonight" or "which plants do I water by hand", that field is a column with a type and an index. Burying it in JSON means the cold-snap automation does a full scan and a JSON extract to find five plants, and a typo in a key fails silently.

**JSONB for the descriptive care knowledge a model reads.** Days to maturity, feeding notes, the owner's remarks, species quirks. The consumer is a prompt, not a `WHERE` clause. Adding a fourth domain should not be a migration.

## plants

| Column | Type | Notes |
| --- | --- | --- |
| `id` | uuid | |
| `slug` | text unique | Stable, human readable, what a Dusk ref carries |
| `common_name` | text | |
| `botanical_name` | text null | Drives the care knowledge lookup. Null is common and fine |
| `variety` | text null | Cultivar. Matters enormously for edibles, rarely for houseplants |
| `domain` | enum | The three above |
| `steward` | text | Who it belongs to. `self`, or a person's name |
| `status` | enum | `alive`, `struggling`, `dormant`, `dead`, `gone` |
| `location` | text | Free text, because "top shelf, greenhouse cabinet" is more useful than an area id |
| `ha_area` | text null | The Home Assistant area, when there is one |
| `accessibility` | enum | `easy`, `awkward`, `hard` |
| `watering_method` | enum | `letpot`, `hand` |
| `letpot_dripper` | int null | Which dripper, when on the line |
| `pot_size_in` | numeric null | |
| `pot_material` | text null | Terracotta dries far faster than plastic, which changes every threshold |
| `has_drainage` | bool null | A pot with no drainage hole is the single most common way a plant is drowned |
| `soil_mix` | text null | |
| `light_exposure` | enum null | `direct`, `bright_indirect`, `medium`, `low` |
| `min_temp_f` | numeric null | Below this it needs protecting. Typed because the cold automation queries it |
| `sheltered_at` | timestamptz null | Set while it is indoors for cold, so the warning stops and it becomes eligible to go back out. A client that does not read it cannot say which plants are inside |
| `care_profile` | jsonb | Domain specific, below |
| `acquired_at` | date null | |
| `archived_at` | timestamptz null | Set instead of deleting; restore clears it and returns the plant to `alive` |

### Why `accessibility` is a first-class column

Your friend's rule is to touch the top inch of soil.
That does not work for a plant you cannot reach, and a plant you cannot reach is a plant that gets forgotten.

So it drives two real behaviours: sensors are bought for `hard` plants first regardless of what the plant is worth, and the nag escalates faster for them, because "I'll check it later" is how they die.

### Why `watering_method` splits the product in half

For a `letpot` plant Planty can act: sensor reads dry, pump runs, sensor confirms water arrived, and you only hear about it when something broke.

For a `hand` plant Planty can only nag, and it must keep nagging until you confirm you did it.

The escalation is therefore **more** aggressive for hand-watered plants, not less. Automated plants fail rarely and only from a clog. Hand-watered plants fail every time you get busy, which is the actual thing this project exists to prevent.

## sensor_links

A plant is watched by zero or more Home Assistant entities.

| Column | Type | Notes |
| --- | --- | --- |
| `id` | uuid | |
| `plant_id` | uuid null | Null for an ambient sensor serving a whole zone rather than one plant |
| `zone` | text null | Set when `plant_id` is null |
| `ha_entity_id` | text | |
| `role` | enum | `soil_moisture`, `ambient_temp`, `ambient_humidity`, `illuminance` |
| `dry_baseline` | numeric null | |
| `wet_baseline` | numeric null | |
| `calibrated_at` | timestamptz null | |

**Calibration belongs here, not on the plant.** A raw percentage is meaningless until dry and saturated are recorded for that probe in that soil in that pot. Two sensors' absolute numbers are never comparable, so every threshold is expressed relative to that link's own baselines.

An uncalibrated link produces readings but must never produce an automated watering decision. Confident wrong alerts are worse than no alerts.

## Plant-dedicated actuators

`plant_actuators` is the explicit allowlist of Home Assistant `fan` and `switch` entities Planty may control.
Discovery only offers those two domains for selection and never registers or actuates an entity by guessing from its name.
Every start and stop route takes the Planty actuator UUID, never a caller-supplied Home Assistant entity ID.
`plant_actuator_plants` assigns each registration to one or more living plants, and registration is refused without that explicit relationship.
A shared room fan remains one actuator assigned to several plants rather than several registrations for one Home Assistant entity.

`plant_actuator_leases` persists the requested duration and absolute shutdown deadline before Home Assistant receives `turn_on`.
Durations are bounded to one hour, only one unfinished lease may exist per actuator, and command idempotency keys make request retries incapable of extending or repeating a run.
The server reconciles overdue leases independently of the initiating request, and `planty reconcile-actuators` provides the same recovery pass as a standalone job after a restart.
A failed `turn_off` leaves the lease unfinished so later reconciliation retries it.
A successful `turn_on` and its `started` audit event are committed with one `airflow` observation for every current plant assignment.
The observation describes the requested duration as an upper bound because an explicit stop can end the run early, and an idempotent retry cannot duplicate those plant records.

`plant_actuator_events` is the append-only command ledger: request, successful start, failed start, requested stop, successful stop, failed stop, and already-stopped outcomes retain actor, source, lease, detail, and idempotency provenance.
Deleting an actuator only removes it from the active allowlist; historical leases and events remain.
An active actuator cannot be removed until it has been stopped.

Planty owns bounded ad hoc runs only.
Recurring fan schedules remain Home Assistant automations because duplicating schedule ownership would create two controllers that can disagree about whether a device should be on.

## readings

Time series. Keyed on the sensor link, not the plant, because the plant a probe serves can change when it is moved.

`id`, `sensor_link_id`, `value`, `unit`, `taken_at`.

## observations

Everything a human or an agent recorded. This is the write path the app and the plugin share.

`id`, `plant_id`, `kind`, `body`, `occurred_at`, `source`, `actor`.

`kind` is one of `watered`, `airflow`, `misted`, `repotted`, `fertilized`, `pruned`, `harvested`, `moved`, `symptom`, `note`, `died`.

`source` is `app`, `agent`, or `automation`, and it is not cosmetic: it is how you later tell what Planty did from what you did, which is the first question asked when something went wrong.

### Watering claims get verified

When an observation says `watered`, the soil reading afterwards either rises or it does not.

If it does not, something is wrong and it is worth saying so. The usual cause is that bone dry potting soil goes hydrophobic: water channels down the gap between soil and pot and straight out the drainage hole without wetting the root ball. The pot feels watered, the plant is still dying, and nobody catches it by hand.

That check is only possible because the claim and the sensor are in the same system.

## photos

`id`, `plant_id`, `storage_key`, `taken_at`, `caption`, `vision_findings` jsonb, `analyzed_at`, `deletion_requested_at`.

Blobs go to MinIO, which already runs here. Postgres holds the key, never the bytes.

`vision_findings` is the model's reading of the image, kept separately from anything a human wrote, so a wrong machine finding is never mistaken later for something you observed yourself.
Explicit deletion first hides the metadata, then deletes object bytes, then removes the row; a failed object call leaves a retryable pending record for `prune-photos`.
Unowned scratch consultation photos follow the same path after 30 days, while archived plants retain their owned photos until somebody explicitly deletes them.

## verdicts

One row per plant per day, the output of the daily judgment run.

`id`, `plant_id`, `for_date`, `action`, `reasoning`, `evidence` jsonb, `confidence`, `created_at`, `acknowledged_at`.

`action` is `none`, `water`, `check`, `urgent`, or `harvest`.

**`evidence` is not optional.** It records which readings, observations and photos the verdict was built from. Without it a wrong verdict cannot be debugged and a right one cannot be trusted, and the whole thing degrades into a horoscope.

**A missing verdict is not a calm verdict.** If the daily run fails, that must be visibly distinct from "nothing needs doing". Silence that looks like reassurance is exactly how this class of system kills things.

## Garden incidents

`garden_incidents` is a lifecycle record for a suspected shared factor across otherwise independent plant verdicts.
The deterministic detector runs only after a judgment run completed successfully for every expected plant.
It may open or refresh an incident only for at least two affected plants, or for one affected plant plus an independent environmental or actuator failure record.

Typed factor membership names a shared Home Assistant area, current location, registered actuator, tightly timed common-care batch, or environmental failure.
`garden_incident_plants` keeps plant membership normalized while each row retains its verdict and action evidence.
`garden_incident_detections` append-only records every complete judgment run that refreshed the candidate, so a later update cannot erase the evidence that originally opened it.

An incident says “shared factor worth checking,” never that the factor caused the symptoms.
It does not acknowledge or hide the plants' individual verdicts, and it cannot actuate equipment, quarantine plants, spray anything, or change care.
Acknowledgement only records that a person saw the candidate.
Resolution preserves the actor, conclusion, and one of `confirmed_common_cause`, `unrelated`, `contained`, or `inconclusive`.

Current-location grouping is deliberately limited to the current `plants.location` value.
Planty does not infer historical pest exposure until movement history records both sides of a move.
Registered-actuator correlation follows `plant_actuator_plants`, so a failed unrelated smart plug is never evidence about a plant.

## Evidence windows, guardrails, and experiments

`evidence_windows` is the shared lifecycle for a single-plant visual recheck and a multi-plant household experiment.
Each window normalizes its participating plants, baseline and review references, required evidence, one intervention observation, bounded review dates, conclusion, and actor provenance.
The lifecycle is `proposed`, `active`, `ready`, then `completed` or `cancelled`; scheduled automation may propose a window but cannot start one.

Every evidence reference resolves to an owned photo, observation, or reading in Planty's ledger.
Review evidence must be newer than the recorded intervention, fall inside the review window, and satisfy every expected plant-and-kind pair.
The iOS app can therefore show a real baseline/review overlay and must say when a referenced image is unavailable instead of silently substituting another photo.

Code-owned Do-Not-Disturb guardrails name intervention conflicts and safety red flags.
An override never erases the guardrail: it appends who overrode which conflict and why, then marks the result confounded so the conclusion options narrow honestly.

A household experiment names exactly one changed variable, explicit hold-constant rules, success criteria, and review bounds.
It records evidence and interpretation but does not schedule or actuate care.

`GET /v1/evidence-coverage` ranks the single next input that would most improve the garden's evidence.
The app deliberately displays one recommendation with its reason rather than turning missing data into a generic completion checklist.

## plant_health_events

Plant health is an append-only, evidence-backed ledger rather than a mutable number on `plants`.
The current health score is unknown until an explicit baseline is recorded; Planty never invents a neutral starting value.

Each event records the resulting `score`, an optional requested and applied signed delta, a rationale, structured evidence, source, actor, optional judgment run, idempotency key, and creation time.
A baseline supplies an absolute score, while every later event supplies an arbitrary nonzero signed delta.
The applied delta records boundary clamping, so a request to add 30 to a score of 90 is auditable as requested `+30`, applied `+10`, resulting `100`.
Scores are always serialized in the closed range 0 through 100.

Automated changes must name typed reading, observation, or photo evidence belonging to the plant, and at least one referenced item must be newer than the current health event.
One automated health change is permitted per plant per judgment run, making a retried daily assessment idempotent without hiding a later independent human correction.
Manual and agent writes require their own idempotency key so a lost response can be retried without duplicating history.

A score of zero means the latest evidence supports zero health; it does not archive the plant or rewrite its lifecycle status.
Lifecycle state remains an explicit human-visible decision because a low confidence assessment must never make a plant disappear.

## harvests

For the edible domains. `id`, `plant_id`, `occurred_at`, `quantity`, `unit`, `notes`, `created_at`, `updated_at`.

Kept apart from observations because harvests aggregate: yield per plant per season is a real question, and burying it in a free text observation body makes it unanswerable.
Quantity must be positive and the unit must be present.
Records can be corrected or deleted, and the service aggregates by plant, unit, meteorological season, and season year.

## Conversations, notes, reminders, and questions

`diagnosis_turns` stores both plant consultations and scratch conversations, distinguished by kind and conversation id.
A turn may refer to a plant and a photograph, but both can be absent for a question about something not yet in the garden.

`notes` stores editable human prose either against one plant or against the household.
Household notes are included in every consultation because facts such as “there is a cat here” affect advice without belonging to one pot.

`reminders` stores standing intent, while `reminder_completions` stores the completed or missed disposition of each scheduled occurrence.
A completed disposition links to the care observation it created; a missed disposition deliberately has no observation.
A notification does not advance the care schedule by itself.

Owner questions are durable records rather than conversation-only prompts, so uncertainty can be answered later and remain attached to the plant.

## Reliability and configuration records

`judgment_runs` records the expected, succeeded, failed, and completed counts for a whole daily attempt.
That aggregate is what prevents one fresh verdict from making a partial run look like a current garden-wide all-clear.

`care_completions` binds idempotency keys to the observations it creates.
`reminder_completions` binds a key and exact due slot to one immutable outcome, and only the completed outcome links to an observation.
`plant_health_events` binds manual retries to idempotency keys and daily changes to judgment runs while preserving every accepted score transition.
A phone can retry after losing a response without duplicating care history or closing the wrong occurrence.

`push_devices` stores APNs tokens by production or sandbox environment.
`model_assignments` stores only jobs moved away from their environment default, so an empty table preserves deployment behavior.
`prompt_instructions` stores only user-edited job overlays, so an empty table preserves every code-owned prompt exactly.
An overlay may refine household context, priorities, and writing style, but it is appended inside a marked boundary and cannot replace Planty's safety, evidence, schema, or tool-authority instructions.

Toxicity lives on the plant as one explanatory JSON document with generated, constrained ratings for cats, dogs, and people.
Unknown remains a first-class rating and never defaults to safe.

## The care_profile, by domain

**`houseplant`**: target moisture band, humidity preference, dormancy needs, species quirks, and whatever the owner said.

**`edible_indoor`**: days to maturity, sow and transplant dates, feeding schedule, `needs_pollination`, and expected first harvest.

Pollination is called out because indoor tomatoes do not set fruit without physical agitation of the flowers. Outdoors that is wind and bees. In a still cabinet the flowers yellow and drop and the yield is zero, which reads like a mystery failure rather than a missing input.

**`edible_outdoor`**: hardiness zone, frost sensitivity, last and first frost dates, succession interval, days to maturity.

## The HTTP contract

Both clients call the same thing. Anything the app can do by tapping, an agent can do by asking.

The canonical shipped route inventory and closed wire enums live in `api/openapi.json`. `cmd/contractgen` generates the Go mux patterns and Swift client paths/enums from that contract, and CI fails if the checked-in generated output drifts. This document explains behavior and rationale instead of copying the route table into another source of truth.

**Prompt settings are overlays, not replacement system prompts.** `GET /v1/prompt-instructions` returns all six model jobs in the same stable order as model assignments, with an empty `instructions` value when a job uses only its code-owned prompt.
`PUT /v1/prompt-instructions/{job}` creates or replaces one trimmed overlay, and `DELETE` clears it.
The service reads the current value for every model request, so a settings change takes effect without restarting a pod.
Prompt settings cannot add model tools, expand trusted web hosts, weaken structured output validation, or alter physical-action authority because none of those boundaries live in the editable row.

**Health reads and writes expose the ledger, not a mutable field.** `GET /v1/plants/{slug}/health` returns the nullable current event and newest-first history, so unknown is different from zero.
`POST /v1/plants/{slug}/health-events` accepts either a baseline or a signed delta with rationale, evidence, actor, and idempotency key.
The agent exposes the same operations through `health` and `healthchange`; neither client can bypass the store's evidence, ownership, clamping, and retry rules.

**Actuator discovery and actuation are separate capabilities.** `/v1/home-assistant/actuators` returns only fan and switch candidates, while `/v1/actuators` manages the persistent allowlist.
Start, stop, and event-history routes are nested beneath `/v1/actuators/{id}` so no actuation request accepts an arbitrary Home Assistant entity ID.

**`POST /v1/identify` belongs to no plant, deliberately.** Nobody knows which plant it is yet, and it may not be one on record. It takes a `photo` multipart part or raw bytes, and answers `{candidates: [{common_name, scientific_name, confidence}], count}` with at most three, most likely first. An empty list is a valid answer and a better one than a guessed name.

The app sends a Vision cutout rather than the raw frame, and only after its own on-device classifier agrees the subject is a plant, so this is never spent on a photo of a dog. With no judge configured it answers 503 and the app falls back to showing those on-device labels.

**`POST /v1/plants/from-photo` is the same identification, kept.** `/v1/identify` answers a question and throws the photograph away; this one names the plant, creates it, and keeps the photograph as the first frame of its timeline, which is the whole point of a timeline starting on day one.

It takes the image the same two ways upload does, and any of `common_name`, `botanical_name`, `location`, `steward` and `domain` as query overrides.
A given `common_name` wins outright, because somebody holding the plant knows better than a model holding a picture of it.
It answers `{plant, candidates, photo}` so the app can show what else was considered and let you correct it, and 422 when nothing was recognised and no name was supplied, because inventing a species is the one thing the identify prompt forbids.

Owning three pothos is ordinary and slugs are unique, so the second one becomes `golden-pothos-2`.
If the photograph fails to store the plant still exists, and the reply carries `photo_error` rather than unwinding a record you can see was created.

**`/ask` needs no photograph, and that is the point.** Most questions are not about a picture at all, and requiring one to ask anything is what made this awkward to use. When seeing the plant would change the answer, a plant consultation can inspect recent timeline photographs or accept a photo attached to the conversation.
It reads the last 45 days of the record: what was done, what the probes saw, what earlier photographs were found to show.
Long enough for a season to turn and a watering rhythm to be visible, short enough that a plant's whole life is not re-read to answer "are these leaves normal".

**Photographs are offered to `/ask`, not attached up front.** The recent ones are put where the model can open them, with a line per photo saying when it was taken, and it opens one only if seeing it would change the answer.
The reply carries `looked_at` so you can tell whether it did.
Asked "when did I last water this", it answers from the log in about seven seconds and reports looking at nothing; asked what colour the leaves are, it opens the photo and takes twice as long.
Only the Claude Code CLI can genuinely offer historical photographs today because it stages them as files the model may choose to read.
The direct Anthropic API and OpenAI-compatible paths receive their names and are required to say they have not seen them; a current photograph explicitly attached to the question still reaches any verified vision model.

**A Claude CLI follow-up resumes rather than re-reading.** `conversation_id` is the CLI session id as well as the durable conversation id, so its second question can send only itself while the record and earlier turns remain in the session.
Measured over four turns against a plant with sixty observations, billed input per turn was `3110, 3133, 3156, 3177` replaying and `3104, 10, 10, 10` resuming.
Replaying is marginally cheaper for a two-turn exchange and breaks even at three; past that resuming is roughly half the cost per turn, and the gap widens because replaying grows with the conversation and resuming does not.

Sessions live in the service's own scratch space, which does not survive a restart, so a conversation can outlive the session it was using.
A resume that finds nothing falls back to replaying the transcript: slower, still correct, and invisible from the app.
Backends without resumable sessions always receive the stored transcript, which is why every request carries the complete conversation even when the CLI can optimize it away.

**A consultation can write, through one command and nothing else.** Told "I watered it this morning", "move it to the entryway" or "remind me to mist this twice a day", it does that rather than telling you to.
Writes go through `planty agent`, which covers the whole service except `autopsy`, and they land with source `agent` so the record always says who claimed a thing happened.
Its reference is `planty agent help`, and the same text is handed to the model in its system prompt so it never has to explore to find a flag.
The daily verdict may start or stop only an explicitly assigned, durably bounded fan when current plant evidence justifies airflow.
It receives the same complete `planty agent` reference as a consultation, must inspect every plant sharing an actuator, and cannot use scheduled assessment authority for watering or general record maintenance.
A successful physical start writes its own `airflow` records after Home Assistant accepts the command, while identification, postmortems, and owner updates remain read-only.
`PLANTY_JUDGE_CAN_ACT=false` turns it off. `adr/0002` records what holds it shut and what was rejected.

**Reminders are two fields because misting is not watering.** `every_days` says how often a day qualifies and `at_hours` says when on that day, so a mushroom kit is `{every_days: 1, at_hours: [8, 20]}` and a pothos is `{every_days: 10, at_hours: [8]}`.
An interval expressed only in days cannot say the first one, and misting twice a day is the common case rather than the exotic one.
Omit both and you get daily at 08:00, matching the digest.

**A reminder is measured against observations, never against itself.** Due is computed from the last time that kind was actually recorded, so a notification nobody acted on stays due instead of quietly rescheduling itself.
`last_sent_at` exists only to stop the hourly job sending the same slot twice, and the morning misting does not suppress the evening one.
The cadence counts whole days from the day it was done, so watering at 08:30 does not push an 08:00 slot into tomorrow and turn a weekly chore into an eight-day one.

**The cold warning has to be answerable.** Without `/v1/shelter` the afternoon warning repeats forever and no plant ever becomes eligible to go back out, so the notification itself asks you to reply. `{"all":true}` is the honest default: the real interaction happens at dusk with an armful of pots, not with a list of slugs.

**Photo upload accepts either shape.** Post raw bytes with a `Content-Type` and `?caption=`, or a `multipart/form-data` body with a `photo` part and an optional `caption` field. `URLSession` builds multipart naturally and curl posts raw bytes naturally; rejecting either would be arbitrary.

**Timeline returns `{photos: [...], count}`.** Each photo carries a presigned `url` valid for 30 minutes, so a client renders images straight from object storage rather than proxying every byte through the service.

**`POST /v1/questions` defaults `asked_of` to the plant's steward.** An agent asking a question about a plant should not have to know who owns it.

The Dusk plugin maps its actions onto exactly these endpoints and stores nothing of its own.
Dusk reflects what this service says, and when this service cannot be reached the plugin reports `partial` rather than an empty catalogue, because an unreachable service and a garden with no plants in it must never look the same.
