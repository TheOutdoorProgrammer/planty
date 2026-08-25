# Separate watering actuation from verification

## Context and Problem Statement

The LetPot line moves real water through one shared pump.
Planty's original manual command held the maximum runtime in process memory, wrote a `watered` observation as soon as the pump ran, and immediately checked probes that report on a slower cadence.
A killed process could leave the switch energized, and a successful service call could become false care history even when a dripper was blocked or the probes never reported.

## Considered Options

- Keep the process timer and treat a successful Home Assistant service call as watering.
- Make Planty wait through the entire sensor-settle window before the manual command exits.
- Persist actuation separately, verify it in a later job, and give Home Assistant an independent maximum-runtime automation.
- Schedule watering automatically once the independent cutoff exists.

## Decision Outcome

Chosen: **persist actuation separately, verify it later, and cap the pump independently in Home Assistant**.

`planty water` remains manual-only.
It creates a watering attempt before turning on the switch, records start, activity, stop, and errors, and never writes a `watered` observation itself.
The scheduled `verify-water` job waits for probes to settle, stores the before-and-after evidence, and writes the observation only for a plant whose moisture rose.
Flat evidence is a likely clog, missing evidence is an unknown sensor result, and missing pump activity is a pump failure.
Failures are durable and delivered through APNs rather than Home Assistant notifications.

Home Assistant owns a separate three-minute maximum-runtime automation for the physical switch.
That automation also turns the switch off after Home Assistant restarts and sweeps for an energized switch whose original trigger timer was lost during an automation reload.

### Consequences

Good:

- A model, app, or operator cannot create positive watering history merely by asking the pump to run.
- Planty process death no longer removes the only ordinary maximum-runtime control.
- Verification survives command exit and normal job retries.
- Clogged delivery, missing sensor evidence, pump inactivity, and stop failure remain distinguishable.
- The shared line stays under human control even though its failure handling is now durable.

Bad, and accepted:

- A watering attempt is not credited until a later verification job runs.
- Home Assistant loss can still delay the software cutoff; eliminating that failure mode requires a hardware timer or normally-closed plumbing design.
- The service and Home Assistant automation must agree on the pump entity and maximum runtime.
- Failed APNs delivery remains queued on the attempt and retries when `verify-water` runs again.

Rejected:

- Scheduling automatic watering remains outside the product because a shared line can still water a plant that did not request it.
