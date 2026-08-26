# Record shared airflow when an actuator starts

## Context and problem statement

A fan can serve several plants, but a generic actuator event belongs to the device rather than any plant's care history.
A person or scheduled agent also needs one safe control path without manually duplicating records across every plant the fan reaches.
Recording the requested duration before Home Assistant accepts `turn_on` would claim airflow that may never have happened, while waiting until the lease ends would hide a real intervention from assessments during the run.

## Considered options

* Keep airflow only in the actuator audit ledger.
* Ask each caller to write a separate plant observation before or after starting the fan.
* Atomically write one `airflow` observation for every current actuator assignment when Home Assistant accepts the start.
* Write a single shared observation and make plant history discover it through the actuator relationship.

## Decision outcome

Planty records one `airflow` observation on every plant assigned to an actuator in the same transaction that marks a successful lease start.
The observation says the named actuator started for up to its requested duration, because an explicit stop can make the actual run shorter.
Failed starts record no airflow, and an idempotent replay does not duplicate observations.
App and agent callers use the same bounded controller, while agent starts additionally prove that the named plant is assigned to the actuator under the actuator lock.

### Consequences

* A shared fan appears honestly in every affected plant's history and becomes evidence for later scheduled assessments.
* Callers cannot forget or duplicate the per-plant bookkeeping.
* A start-audit failure triggers the existing fail-safe stop, so Planty does not leave an unaudited fan running.
* The history records the maximum intended run rather than claiming the exact elapsed airflow duration.
* Reassigning a fan affects future starts only; old airflow records remain on the plants that were assigned when the run began.
