# Separate fresh assessments from evidence windows

## Context and problem statement

Planty's photo recheck wording made an evidence window look like a request for the AI to analyze the newest photograph again.
An evidence window actually measures what happened after a real care intervention, so treating a photograph as the intervention would corrupt the record and make later conclusions meaningless.

## Considered options

* Add `photo` as an intervention kind.
* Broaden evidence windows to include observation-only checks.
* Keep evidence windows tied to care actions and add a separate fresh-assessment action.

## Decision outcome

Planty will keep evidence windows tied to real care interventions and present them as before-and-after care tracking.
Planty will provide a separate on-demand assessment that gathers the same current evidence as the daily job, including the newest available photograph, and writes a fresh verdict without sending a garden-wide notification.

### Consequences

* A user can ask for an immediate new opinion without inventing an intervention or waiting for a scheduled job.
* Evidence windows retain a defensible causal boundary between a care action and later review evidence.
* The interface has two explicit actions instead of one ambiguous recheck action.
* An on-demand assessment still updates today's verdict and health history, so repeated requests are real judgments rather than disposable previews.
