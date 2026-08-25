# Model health and care as evidence ledgers

## Context and Problem Statement

The product now needs a visible zero-to-one-hundred health value, arbitrary signed adjustments from scheduled agents, missed reminder outcomes, visual intervention rechecks, and household experiments.
A mutable percentage or boolean completion flag would erase why a value changed, lose concurrent updates, and encourage the app to present model opinion as objective fact.
The existing design intentionally avoided a global health score for those reasons, so reversing that boundary requires an auditable replacement rather than decorative UI state.

## Considered Options

- Store one mutable `health` column on each plant and let every caller overwrite it.
- Derive a score on the phone from verdict wording and reminder history.
- Store append-only health events and occurrence/intervention outcomes with typed evidence and idempotency boundaries.
- Keep qualitative care state only and reject the health percentage requirement.

## Decision Outcome

Chosen: **store append-only evidence events and derive current projections from the latest valid event**.

A health event records the requested delta, applied delta, resulting score, rationale, evidence references, source, actor, optional job/run identity, and revision.
Automated nonzero changes require evidence newer than the preceding automated event so repeated runs cannot ratchet a score from the same photograph.
The first score is an explicit evidence-bearing baseline; an unassessed plant remains unknown rather than silently starting at fifty.
Scores clamp to the inclusive zero-to-one-hundred range under a plant-row lock, and automated run identity makes retries idempotent.

Zero means the available evidence supports a dead or unrecoverable assessment.
It opens a human confirmation and postmortem path but never archives, deletes, or instructs anyone to discard the plant automatically.
Plant lifecycle status remains authoritative.

A missed reminder is a disposition of one due occurrence and creates no care observation.
Interventions, rechecks, guardrail overrides, experiment outcomes, and incident resolutions similarly preserve their source evidence instead of rewriting history.

### Consequences

Good:

- Every displayed percentage and trend can explain who changed it and why.
- Concurrent app and agent changes serialize without lost updates.
- Retries and repeated evidence do not manufacture improvement or decline.
- Rechecks, experiments, and incident analysis can reuse the same provenance vocabulary.

Bad, and accepted:

- Some plants display unknown health until a baseline assessment has sufficient evidence.
- Correcting a mistaken score adds a labeled corrective event instead of deleting history.
- More tables and state transitions are required than a mutable integer would need.

