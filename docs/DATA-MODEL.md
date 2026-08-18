# The model

The service owns all of it.
The iOS app and the Dusk plugin are two clients of the same HTTP API with the same powers, and neither keeps its own copy.

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
| `care_profile` | jsonb | Domain specific, below |
| `acquired_at` | date null | |
| `archived_at` | timestamptz null | Set instead of deleting. See ADR-0001 |

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

## readings

Time series. Keyed on the sensor link, not the plant, because the plant a probe serves can change when it is moved.

`id`, `sensor_link_id`, `value`, `unit`, `taken_at`.

## observations

Everything a human or an agent recorded. This is the write path the app and the plugin share.

`id`, `plant_id`, `kind`, `body`, `occurred_at`, `source`, `actor`.

`kind` is one of `watered`, `repotted`, `fertilized`, `pruned`, `harvested`, `moved`, `symptom`, `note`, `died`.

`source` is `app`, `agent`, or `automation`, and it is not cosmetic: it is how you later tell what Planty did from what you did, which is the first question asked when something went wrong.

### Watering claims get verified

When an observation says `watered`, the soil reading afterwards either rises or it does not.

If it does not, something is wrong and it is worth saying so. The usual cause is that bone dry potting soil goes hydrophobic: water channels down the gap between soil and pot and straight out the drainage hole without wetting the root ball. The pot feels watered, the plant is still dying, and nobody catches it by hand.

That check is only possible because the claim and the sensor are in the same system.

## photos

`id`, `plant_id`, `storage_key`, `taken_at`, `caption`, `vision_findings` jsonb, `analyzed_at`.

Blobs go to MinIO, which already runs here. Postgres holds the key, never the bytes.

`vision_findings` is the model's reading of the image, kept separately from anything a human wrote, so a wrong machine finding is never mistaken later for something you observed yourself.

## verdicts

One row per plant per day, the output of the daily judgment run.

`id`, `plant_id`, `for_date`, `action`, `reasoning`, `evidence` jsonb, `confidence`, `created_at`, `acknowledged_at`.

`action` is `none`, `water`, `check`, `urgent`, or `harvest`.

**`evidence` is not optional.** It records which readings, observations and photos the verdict was built from. Without it a wrong verdict cannot be debugged and a right one cannot be trusted, and the whole thing degrades into a horoscope.

**A missing verdict is not a calm verdict.** If the daily run fails, that must be visibly distinct from "nothing needs doing". Silence that looks like reassurance is exactly how this class of system kills things.

## harvests

For the edible domains. `id`, `plant_id`, `occurred_at`, `quantity`, `unit`, `notes`.

Kept apart from observations because harvests aggregate: yield per plant per season is a real question, and burying it in a free text observation body makes it unanswerable.

## The care_profile, by domain

**`houseplant`**: target moisture band, humidity preference, dormancy needs, species quirks, and whatever the owner said.

**`edible_indoor`**: days to maturity, sow and transplant dates, feeding schedule, `needs_pollination`, and expected first harvest.

Pollination is called out because indoor tomatoes do not set fruit without physical agitation of the flowers. Outdoors that is wind and bees. In a still cabinet the flowers yellow and drop and the yield is zero, which reads like a mystery failure rather than a missing input.

**`edible_outdoor`**: hardiness zone, frost sensitivity, last and first frost dates, succession interval, days to maturity.

## The HTTP contract

Both clients call the same thing. Anything the app can do by tapping, an agent can do by asking.

This is the shipped surface, not a plan. Every route below exists.

```text
GET    /healthz                   liveness, and the only unauthenticated concern

GET    /v1/plants                 list; filter by domain, steward, status, watering_method, include_archived
POST   /v1/plants                 create; sparse, the service fills domain/status/steward/accessibility/watering
GET    /v1/plants/{slug}          record plus risk score, recent observations and last watered
PATCH  /v1/plants/{slug}          sparse update; omitted fields are left alone
DELETE /v1/plants/{slug}          archive, never a hard delete; ?status=dead records why

GET    /v1/plants/{slug}/observations   history, newest first
POST   /v1/plants/{slug}/observations   log watered, repotted, fertilized, pruned, moved, symptom, note, died
POST   /v1/plants/{slug}/harvests       log a harvest, with quantity and unit
POST   /v1/plants/{slug}/photos         upload raw image bytes; Content-Type sets the format
GET    /v1/plants/{slug}/timeline       photos oldest first, with short-lived links
POST   /v1/plants/{slug}/diagnose       read the photo timeline and report what changed

GET    /v1/today                  the digest; carries all_clear and stale_since separately
POST   /v1/verdicts/{id}/ack      acknowledge, which stops escalation

GET    /v1/sensors                links and calibration state
POST   /v1/sensors                link a Home Assistant entity to a plant or a zone
PATCH  /v1/sensors/{id}           record dry and wet baselines

GET    /v1/questions              the owner queue; also returns as_text, ready to send
POST   /v1/questions              queue one; asked_of defaults to the plant's steward
POST   /v1/questions/{id}/answer  record what the owner said

POST   /v1/away                   record a period with a backup contact

GET    /v1/cold-watch             which plants need bringing in for a given forecast_low_f
```

**Photo upload takes raw bytes, not multipart.** The `Content-Type` header carries the format and `?caption=` carries the note, so a phone client posts the image directly instead of assembling a multipart body.

**`POST /v1/questions` defaults `asked_of` to the plant's steward.** An agent asking a question about a plant should not have to know who owns it.

The Dusk plugin maps its actions onto exactly these endpoints and stores nothing of its own.
Dusk reflects what this service says, and when this service cannot be reached the plugin reports `partial` rather than an empty catalogue, because an unreachable service and a garden with no plants in it must never look the same.
