# Keep physical safety outside model prompts

## Status

Accepted. Ordinary recurring fan schedule ownership is superseded by [ADR-0030](0030-let-planty-own-explicit-daily-fan-schedules.md).

The immutable prompt boundary, allowlist, bounded manual leases, audit ledger, and independent physical backstop still stand.

## Context and Problem Statement

Planty needs editable scheduled-agent instructions, agent-initiated health changes, and bounded control of Home Assistant airflow devices.
A prompt can be useful product configuration, but it is not an authorization boundary, a concurrency controller, or a reliable timer.
The existing Home Assistant guidance also assigns a fan schedule to Home Assistant, while the new product requirements give Planty, the app, and agents manual control.
Two independent schedulers could fight over the same plug, and a process-local timer could leave a fan running after a restart.

## Considered Options

- Let users edit complete system prompts and let the model call Home Assistant directly.
- Move every schedule and cutoff into Planty.
- Keep the standing schedule and independent maximum-on cutoff in Home Assistant while Planty issues audited, bounded overrides through stored allowlisted actuator links.
- Make airflow read-only in Planty and leave all control in Home Assistant.

## Decision Outcome

Chosen: **keep the standing schedule and independent cutoff in Home Assistant, and let Planty issue only audited bounded overrides through allowlisted links**.

Planty stores the exact `switch` or `fan` entity once it is selected in Settings.
Callers operate on the Planty actuator ID and a closed start/stop operation; they never provide an arbitrary Home Assistant domain, service, or entity ID.
A start includes a bounded duration, persists its stop deadline before returning, and is recovered by a scheduled Planty worker after process restarts.
Home Assistant independently enforces the hard maximum runtime and remains the only owner of the ordinary recurring schedule.

Editable agent text is an instruction layer composed inside immutable service-owned safety, evidence, schema, and authority rules.
Scheduled agents may propose a physical intervention, but they can energize an actuator only through the same bounded controller and audit ledger used by the app and Dusk.

### Consequences

Good:

- Prompt edits cannot grant broader Home Assistant access or remove runtime bounds.
- App, agent, and Dusk actions share one allowlist, audit trail, state confirmation, and recovery path.
- Home Assistant protects the device even if Planty, Postgres, or the cluster is unavailable.
- One scheduler owns normal operation, avoiding competing automation loops.

Bad, and accepted:

- Planty and Home Assistant must agree on the maximum runtime and configured entity.
- A bounded override can temporarily diverge from the standing schedule and must be shown honestly in both systems.
- Adding another actuator role requires explicit service behavior and safety rules rather than a freeform Home Assistant proxy.
