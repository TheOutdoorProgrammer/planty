# Planty

Keeps houseplants alive when the person responsible for them has no idea what they are doing.

Planty watches soil moisture and cabinet humidity through Home Assistant, remembers what every plant is and who it belongs to, looks at photographs of them over time, and once a day says one short thing: water that one, ignore the rest.
Most days it says nothing needs doing, which is the point.

It exists because an automatic waterer running on a timer is a drowning machine.
It waters on a schedule and has no idea whether water reached soil, so a clogged line waters nothing, a stuck one floods, an already wet pot gets watered anyway, and all three report success.
Planty closes that loop.

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
| **Service** (Go, Kubernetes, Postgres) | Ingests readings, stores observations and photos, calls a model for judgment, generates the daily verdict, drives watering |
| **iOS app** (SwiftUI) | For when you are standing in front of the plant holding a watering can. Photograph it, log what you did, ask what is wrong |
| **Dusk plugin** ([dusk-plugin-planty](https://github.com/NerdsWhoFish/dusk-plugin-planty)) | The same operations, for agents. Add a plant by describing it in conversation, log that you watered something, ask what needs doing |

The app and the plugin call the same API and have the same powers.
Anything you can do by tapping, an agent can do by asking, and the other way round.
Neither is the source of truth, and the plugin stores nothing in Dusk: it hands everything to this service and reflects back what the service says.

## What the photographs are for

Sensors measure water. They cannot see spider mites, root rot, light burn, or new growth.

Planty keeps a photo timeline per plant and reads it with a vision model, so the useful finding is not "soil is at 34%" but "the yellowing on the lower leaves has progressed since July 20th, and that is overwatering, not light".
That comparison over time is the single most valuable thing in here and the reason a phone app exists at all.

## Things it deliberately does not do

**It does not ask you to take readings.** Sensors do that. If the design ever requires typing in a number, the design is wrong. Manual entry is for exceptions: repotted this, spotted mites, watered by hand.

**It does not automate the mushroom kit.** Kits live four to eight weeks, the variable that matters is airflow rather than humidity, and the correct misting trigger is "the surface looks dry" which is not a number any sensor produces. An RH threshold misting automation over mists, and over misting is precisely how bacterial blotch happens. A daily reminder and a fan on a plug is the entire correct implementation.

**It does not nag.** One digest a day, usually saying nothing needs doing. Escalation is reserved for a plant actually at risk, because a system that cries wolf gets ignored, and then it is worse than nothing.

## Status

Early. Nothing is deployed yet.

## Layout

```text
docs/DATA-MODEL.md   The model and the HTTP contract both clients call
adr/                 Decisions, in MADR format
design/              UI concept, screen specs, SwiftUI prototype, logo
```

## License

TBD.
