# 31. Require owner review for AI soil calibration changes

Date: 2026-08-30

## Status

Accepted.

## Context and Problem Statement

Relative soil moisture depends on probe-specific dry and wet baselines, and those baselines can drift as probes or soil move.
The AI sees actual and relative readings and can identify suspicious calibration, but silently changing baselines would rewrite the meaning of every watering threshold.
Repeated suggestions would create the same alert fatigue the incident changes are intended to remove.

## Considered Options

1. Let the AI update calibration immediately
2. Let the AI propose calibration changes with explicit owner approval
3. Keep calibration entirely manual

## Decision Outcome

Chosen: **option 2**.

The assessment schema may return an optional calibration proposal for a calibrated soil sensor in the current plant evidence. Planty stores the source reading, actual value, current and proposed baselines, both resulting relative values, model, and reason. The plant dashboard exposes Approve and Deny. Only approval updates the sensor link, and approval fails if its calibration changed after the proposal. Creation requires fresh evidence, a material baseline change, and no pending or recent proposal for that probe. The cooldown is 72 hours and applies regardless of the prior proposal's outcome.

## Consequences

### Good

- The AI can help refine drifting probes without gaining calibration authority
- The owner sees exactly how the current reading and relative percentage would change
- Every accepted change retains evidence and provenance
- The cooldown prevents calibration suggestion fatigue

### Bad

- Useful changes wait for an owner to review them
- A proposal becomes invalid if somebody manually recalibrates the probe first
- The model still needs enough recorded care evidence to make a useful suggestion
- A fixed 72-hour cooldown can suppress a second legitimate correction

### Rejected because

- Automatic application lets a probabilistic model redefine watering evidence without human review.
- Manual-only calibration discards the model's ability to connect fresh readings with recorded watering history and surface a reviewable correction.
