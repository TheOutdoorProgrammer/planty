# Roadmap

Planty's reliability, recovery, watering-safety, data-integrity, provider-capability, push-health, and field-app roadmap shipped in August 2026.
The delivered inventory is maintained in [CHECKLIST.md](CHECKLIST.md), architectural trade-offs live in [adr/](adr/), and operational knowledge lives in Dusk.

## Remaining product choices

These two items were deliberately excluded from the August 2026 completion scope.

- Add a first-run setup flow for service URL, connectivity, notifications, and optional embedded configuration.
- Replace the forced dark appearance with tested light, dark, and system modes.

## Product boundaries

- Watering remains manual-only even though its actuation, verification, and independent cutoff are now durable.
- Home Assistant never carries Planty notifications or speaker escalation.
- Planty does not add a sensor dashboard, care streaks, a global health score, a fixed watering calendar, a community feed, achievements, or a general chat tab.
- The iOS app and Dusk plugin remain clients of the Go service rather than alternate sources of garden state.
