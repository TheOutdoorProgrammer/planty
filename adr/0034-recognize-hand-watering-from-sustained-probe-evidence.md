# 34. Recognize hand watering from sustained probe evidence

Date: 2026-10-04

## Status

Proposed.

## Context and Problem Statement

Watering by hand currently requires a separate app tap even when a soil probe records the result.
The existing verification helper accepts any rise after a human claim; autonomous inference needs stronger evidence to avoid false care history.

## Considered Options

1. Require a manual care claim forever.
2. Accept any positive moisture delta.
3. Infer watering deterministically from calibrated, sustained probe evidence.
4. Ask a model to infer every watering event.

## Decision Outcome

Chosen: **option 3**.

During sensor ingestion, detect watering only for active hand-watered plants with an outstanding water verdict. Require an established sensor assignment, unchanged calibration, a stable baseline, and at least two independent fresh Home Assistant reports showing a sustained rise of at least 20 percent of that probe's calibrated range. Record an automation-sourced watering observation with reading references and acknowledge the verdict in one transaction, reusing the existing completion writer. Serialize competing care operations on the plant. Do not operate a pump or alter calibration.

## Consequences

### Good

- Removes the app tap when sensor evidence supports the requested care.
- Reuses existing history, reminder timing, and verdict completion without a new mobile contract.
- Freshness, assignment settling, and atomic completion reject cached reports, old-pot evidence, and duplicate detection.

### Bad

- Conservative gates miss watering without calibration, sufficient fresh history, or an open water verdict; manual logging remains available.
- Confirmation arrives on a later ingest pass, and newly assigned probes wait 24 hours.
- A moved probe that is not reassigned or recorded as moved can still mimic watering; sensor inference cannot prove who supplied water.

### Rejected because

- Manual-only logging retains the exact human bookkeeping the user wants removed.
- Any positive delta accepts sensor noise and insertion drift.
- Model inference adds cost and nondeterminism to a bounded numerical decision.
