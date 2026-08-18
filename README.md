# Planty

Keeps houseplants alive when the person responsible for them has no idea what he is doing.

Planty watches soil moisture and cabinet humidity through Home Assistant, remembers what every plant is and who it belongs to, looks at photographs of them over time, and once a day says one short thing: water that one, ignore the rest.
Most days it says nothing needs doing, which is the point.

It exists because an automatic waterer running on a timer is a drowning machine.
It waters on a schedule and has no idea whether water reached soil, so a clogged line waters nothing, a stuck one floods, an already wet pot gets watered anyway, and all three report success.

## The loop

1. A soil sensor reads dry.
2. Home Assistant runs the pump.
3. The pump reports that it ran.
4. The soil sensor confirms water actually reached the soil.
5. If step 3 succeeded and step 4 did not, that dripper is clogged, and you are told.

Step 5 is the whole product.
A timer cannot do it, and neither can a person who is not standing there at the time.

## Three surfaces, one brain

The Go service is the only thing that owns state.
Everything else is a way of talking to it.

| Surface | What it is for |
| --- | --- |
| **Service** (Go, Kubernetes, Postgres) | Ingests readings, stores observations and photos, calls Claude for judgment, generates the daily verdict |
| **iOS app** (SwiftUI, `ios/`) | For when you are standing in front of the plant holding a watering can. Photograph it, log what you did, ask what is wrong |
| **Dusk plugin** ([dusk-plugin-planty](https://github.com/NerdsWhoFish/dusk-plugin-planty)) | The same operations, for agents. Add a plant by describing it, log that you watered something, ask what needs doing |

The app and the plugin call the same API and have the same powers.
Anything you can do by tapping, an agent can do by asking, and the other way round.
The plugin stores nothing in Dusk: it hands everything to this service and reflects back what the service says.

## Running it

```sh
export PLANTY_DATABASE_URL=postgres://planty:...@localhost:5432/planty
planty migrate    # apply the schema
planty seed       # load the sabbatical plants and their open questions
planty serve      # the HTTP API on :8080
```

Scheduled work, one command each, wired as CronJobs in `deploy/`:

| Command | Cadence | What it does |
| --- | --- | --- |
| `planty ingest` | every 20 min | Pull current sensor values from Home Assistant |
| `planty daily` | 08:00 | Judge every plant, sweep for autopsies, send one digest |
| `planty away` | 08:30 | Pre-departure watering pass, or the briefing on return |
| `planty cold` | 15:00 | Tonight's forecast: what comes in, and what can go back out |
| `planty autopsy <slug>` | on demand | Work out what killed a plant |

## What the photographs are for

Sensors measure water.
They cannot see spider mites, root rot, light burn, or new growth.

Planty keeps a photo timeline per plant and reads it with a vision model, so the useful finding is not "soil is at 34%" but "the yellowing on the lower leaves has progressed since July 20th, and that is overwatering, not light".
That comparison over time is the single most valuable thing in here and the reason a phone app exists at all.

## Three states, never two

"Nothing needs doing", "the data is stale", and "the judgment has never run" are three different answers and the API returns them as three different fields.
Collapsing them is how this class of system kills things: silence that looks like reassurance.

## Things it deliberately does not do

**It does not ask you to take readings.**
Sensors do that.
If the design ever requires typing in a number, the design is wrong.
Manual entry is for exceptions: repotted this, spotted mites, watered by hand.

**It does not automate the mushroom kit.**
Kits live four to eight weeks, the variable that matters is airflow rather than humidity, and the correct misting trigger is "the surface looks dry", which is not a number any sensor produces.
An RH threshold misting automation over mists, and over misting is precisely how bacterial blotch happens.
A daily reminder and a fan on a plug is the entire correct implementation, and it lives in `docs/home-assistant.md`.

**It does not nag.**
One digest a day, usually saying nothing needs doing.
The house speakers are reserved for a plant actually at risk, because a system that cries wolf gets ignored, and then it is worse than nothing.

**It has no authentication, on purpose.**
Keep it on the LAN.
The plant data is dull but the pod holds a Home Assistant token and an Anthropic key.

## Layout

```text
cmd/planty/          The binary: serve plus the scheduled jobs
internal/plant/      Domain types shared by every surface
internal/store/      Postgres, and the only thing that touches it
internal/api/        HTTP, 22 routes
internal/judge/      Claude: daily verdicts, vision diagnosis, autopsies
internal/job/        Scheduled work
internal/photos/     S3/MinIO object storage
internal/seed/       The sabbatical plants, as shipped data
docs/                The data model, the friend's care sheet, the HA side
deploy/              Kubernetes manifests, not yet applied
design/              UI concept, screen specs, SwiftUI prototype, logo
ios/                 The app
```

## Tests

```sh
go test -race ./...                                   # unit
PLANTY_TEST_DATABASE_URL=postgres://... go test ./internal/store/...   # integration
```

The integration tests exist because a digest query once compiled cleanly, passed every unit test, and was rejected outright by Postgres.
Anything touching SQL gets an integration test.

## License

TBD.
