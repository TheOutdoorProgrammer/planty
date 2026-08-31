# 32. Model actuator schedules as ordered daily windows

Date: 2026-08-31

## Status

Accepted. Supersedes the one-window schedule shape in [ADR-0030](0030-let-planty-own-explicit-daily-fan-schedules.md).

## Context and Problem Statement

Planty currently stores exactly one daily on/off window for each scheduled fan or grow light.
The owner needs split duty cycles, such as one hour on, several hours off, then another on period.
The existing production iOS client and API must keep working during the rollout.

## Considered Options

1. Keep one daily window per actuator
2. Store an arbitrary windows array in JSON on the schedule row
3. Store a bounded ordered set of windows in a child table

## Decision Outcome

Chosen: **option 3**.

Each actuator schedule owns one timezone, one enabled flag, and up to 12 ordered daily windows. Windows use local start and end minutes, may cross midnight, and may touch but may not overlap. Store windows in a relational child table with database bounds and duration checks. Backfill every existing schedule as its first window. The API returns the windows collection while retaining top-level start and end minutes as a compatibility projection of the first window. A legacy request without windows replaces the schedule with its single start and end window. Reconciliation turns the actuator on when any window contains the current local minute. Existing fan lease precedence and exclusive Planty schedule ownership do not change.

## Consequences

### Good

- Owners can express split fan duty cycles and multiple light periods without competing schedulers
- Relational rows keep minute bounds, ordering, and cascading deletion enforceable in PostgreSQL
- Existing installations and the previous iOS build continue to read and edit one-window schedules
- One reconciliation rule applies identically to fans and lights

### Bad

- Schedule reads and writes now include a child table and transactional replacement
- The API carries temporary compatibility fields in addition to the canonical windows array
- An older client that edits a multi-window schedule intentionally replaces it with one window
- Overlap validation must account for windows that cross midnight

### Rejected because

- One window cannot represent the requested on, off, then on duty cycle.
- A JSON array weakens database constraints, ordering integrity, and queryability for a small relational structure.
