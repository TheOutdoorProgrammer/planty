# 30. Let Planty own explicit daily fan schedules

Date: 2026-08-30

## Status

Accepted. Supersedes [ADR-0014](0014-keep-physical-safety-outside-model-prompts.md).

## Context and Problem Statement

The app now needs owner-configured fan on and off times beside the existing fan controls.
Keeping the recurring schedule only in Home Assistant makes Planty unable to display, review, or safely coordinate that intent.
A Planty schedule and a Home Assistant recurring automation for the same fan would create competing controllers.

## Considered Options

1. Keep recurring fan schedules only in Home Assistant
2. Store schedules in Planty but let Home Assistant continue scheduling the same fan
3. Let Planty exclusively own the registered fan's daily schedule while Home Assistant retains an independent physical backstop

## Decision Outcome

Chosen: **option 3**.

Planty stores one optional daily schedule for each semantic fan, using local start and end minutes plus an IANA timezone. The minute reconciler reads Home Assistant state before acting and restores the desired state after restarts. An active bounded manual or agent lease takes priority, so scheduled reconciliation does not fight a timed run. Removing or disabling the schedule returns the fan to off at the next reconciliation. Any recurring Home Assistant automation for the same registered fan must be disabled. Home Assistant may retain an independent maximum-on watchdog sized beyond the intended daily window, because it is a physical backstop rather than a second scheduler.

## Consequences

### Good

- The app can display, edit, enable, disable, and audit the real fan schedule
- Restart recovery and current-state reconciliation use the same path as grow lights
- Manual and agent runs remain bounded and take explicit priority
- Only one system owns recurring intent for a registered fan

### Bad

- Planty and Postgres must be available to correct schedule drift
- Existing Home Assistant fan automations must be removed or they will compete
- The daily window does not express periodic cycles such as fifteen minutes every three hours
- The independent Home Assistant watchdog must be configured so it does not fight a legitimate schedule window

### Rejected because

- Home Assistant-only ownership cannot satisfy in-app scheduling or show Planty the schedule it is expected to explain.
- Two active schedulers can repeatedly reverse each other's commands and make state, audit history, and safety behavior unpredictable.
