# Reuse an evidence window for rechecks, guardrails, and experiments

## Context and Problem Statement

Planty needs to verify whether consequential care helped, warn against stacking confounding interventions, and run bounded household-specific trials.
These can look like three separate features, but all three describe one interval between an intervention and the evidence expected to evaluate it.
Separate workflow engines would drift on deadlines, evidence ownership, overrides, and safety behavior.

## Considered Options

- Build independent recheck, do-not-disturb, and experiment tables with unrelated lifecycle code.
- Express every workflow as ordinary reminders and infer meaning from reminder text.
- Use one typed evidence-window state machine, with optional guardrail and experiment projections that reference it.
- Keep the workflows entirely in model prompts and notifications.

## Decision Outcome

Chosen: **use one deterministic evidence-window state machine and layer rechecks, guardrails, and experiments on it**.

An evidence window names its plants, initiating observation or actuator event, baseline evidence, one declared intervention, expected evidence, earliest and latest review times, and terminal outcome.
A guardrail attaches service-owned conflicting action kinds and red flags to that window.
An experiment adds a hypothesis, one changed variable, hold-constant rules, success criteria, and a conclusion.
All referenced photos, observations, readings, and actuator events remain in their source ledgers rather than being copied into opaque model prose.

Models may propose a window, choose a shorter review inside code-owned bounds, and interpret complete evidence.
Code owns state transitions, maximum durations, physical-actuator limits, evidence freshness, and whether an override confounded the result.
Logging what actually happened is never blocked; an override records the conflict instead of falsifying history.

Garden Incident Radar consumes completed evidence and correlates across plants only after a complete garden judgment run.
It may nominate a shared factor worth checking, but it cannot assert causation or actuate anything.

### Consequences

Good:

- One deadline and provenance model supports all three workflows.
- A conflicting action automatically marks the same evidence window confounded everywhere it appears.
- Household experiments can reuse photo comparison, reminders, and actuator audit records.
- Agents cannot bypass physical safety by hiding an experiment inside prompt text.

Bad, and accepted:

- The shared state machine has more explicit types than three small feature-specific tables would initially need.
- Some experiments end inconclusive because reality did not hold one variable constant.
- Incident correlation stays conservative until location and shared-actuator evidence is strong enough.
