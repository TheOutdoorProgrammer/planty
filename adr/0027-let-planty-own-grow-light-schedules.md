# 27. Let Planty own grow-light schedules

Date: 2026-08-30

## Status

Accepted. Amended, see [Amendments](#amendments).

## Context and Problem Statement

Agents and the iOS app need one durable, auditable way to set and change recurring grow-light behavior. Existing actuator leases intentionally stop within one hour and Home Assistant-owned automations cannot be safely edited through Planty's allowlisted agent tools.

## Considered Options

1. Store schedules in Planty and reconcile explicitly registered Home Assistant lights
2. Create and mutate Home Assistant automation entities
3. Use only expiring actuator leases and require the agent to renew them

## Decision Outcome

Extend Planty's existing allowlisted actuator registration with a light kind. Persist one daily schedule per registered light, including an IANA timezone, and reconcile desired state through the Home Assistant REST API. Manual changes are explicit and the next scheduled transition resumes schedule ownership. Agents may list and update schedules only for lights assigned to the plant in scope.

## Consequences

### Good

- The app and agent share one audited source of truth
- Arbitrary Home Assistant entities remain unreachable
- Schedules survive process restarts and daylight-saving changes
- The existing Home Assistant client, actuator assignment, and reconciliation deployment are reused

### Bad

- Planty must run a minute-level reconciler
- A temporary Planty or Home Assistant outage can delay a transition
- One daily interval does not model every possible lighting program

### Rejected because

- Home Assistant automation mutation broadens credentials and splits ownership across two APIs
- Lease renewal is fragile, noisy, and violates the durable deadline model

## Amendments

### 2026-08-31: Multiple daily windows

[ADR-0032](0032-model-actuator-schedules-as-ordered-daily-windows.md) supersedes this record's one-window schedule shape for both grow lights and fans. Planty ownership, timezone-aware reconciliation, allowlisted actuation, and explicit manual control remain unchanged.
