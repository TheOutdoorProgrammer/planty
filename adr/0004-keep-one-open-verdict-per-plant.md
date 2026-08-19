# Keep one open verdict per plant

## Context and Problem Statement

Planty writes one verdict per plant per day and originally left every row unacknowledged until a person handled it.
The digest and escalation queries treated every unacknowledged actionable row as current.
An old watering instruction could therefore survive after a newer judgment said the plant was fine, or appear beside a contradictory newer action.

The record must keep historical judgments, but the user must see and escalate only the current instruction.

## Considered Options

- Filter every reader to the newest verdict per plant.
- Delete older verdicts when a new judgment arrives.
- Transactionally supersede older verdicts and enforce one open verdict per plant in Postgres.

## Decision Outcome

Chosen: **transactionally supersede older verdicts and enforce one open verdict per plant in Postgres**.

Saving a verdict locks its plant row, acknowledges every prior verdict for that plant, and then inserts or updates the new open verdict in the same transaction.
A partial unique index on `plant_id` where `acknowledged_at` is null makes the invariant survive every caller and future query.
The migration closes historical duplicates while retaining the newest open row.

Filtering every reader was rejected because a new consumer could forget the filter and resurrect the same bug.
Deleting history was rejected because past reasoning is valuable evidence for diagnosis and postmortems.

### Consequences

Good:

- Digest and escalation cannot expose contradictory historical instructions.
- Historical verdicts remain available and explicitly show when they stopped being current.
- Concurrent judgments for one plant serialize on the plant row.
- The database rejects any future write path that attempts to create two open verdicts.

Bad, and accepted:

- `acknowledged_at` now means either handled by a person or superseded by a newer judgment.
- If the product later needs to distinguish those reasons, the schema will need an explicit resolution reason rather than inferring it from the timestamp.
