# 24. Embed OPA behind a typed policy contract and physical safety boundary

Date: 2026-08-27

## Status

Accepted. The Rego authoring and output shape is superseded by [ADR-0025](0025-expose-independent-planty-policy-rules.md).

The embedded OPA, typed enforcement, safety boundary, persistence, and idempotency decisions still stand.

## Context and Problem Statement

Planty needs user-authored rules that can make repeatable decisions for the app, scheduled work, and AI agents from plant age, care history, health, sensors, weather, incidents, and assigned actuators.
Freeform model prompts cannot provide deterministic decisions, and raw policy output must not become a path around Planty's evidence and physical-safety boundaries.
Policies need live validation, preview, decision history, explanations, telemetry, and an iOS authoring surface.

## Considered Options

1. Keep every decision hard-coded in Go and expose only fixed settings
2. Run a separate OPA service or sidecar
3. Embed the OPA Go SDK and store independently versioned Rego modules behind a typed Planty input and output contract
4. Execute arbitrary Rego output directly as Home Assistant and database commands

## Decision Outcome

Chosen: **option 3**.

Planty embeds the OPA v1 Go SDK and stores named, versioned Rego modules in PostgreSQL. Every module is compiled before it can be enabled and is evaluated independently at the fixed `data.planty.decision` entrypoint against a Planty-built, documented input snapshot.

The result must decode into a closed decision envelope. Policies may emit explainable care signals, bounded health adjustments, agent facts and denials, notifications, and actuator directives. Unknown fields, invalid enum values, unassigned actuators, unsafe durations, or malformed results fail closed and are recorded as failed evaluations.

Saved policies have advisory or enforce mode. Advisory policies are visible to the app and agents but cause no mutation. Enforced policies may record supported health decisions, send notifications, and start only an explicitly registered fan assigned to the evaluated plant through the existing durable lease controller. Watering, misting, sheltering, and recurring schedules remain recommendations because policy code is not a physical safety boundary. Water continues to require a person, and Home Assistant continues to own independent equipment cutoffs and recurring fan schedules.

Every evaluation persists the policy version, input fingerprint, idempotency key, typed result, explanation, duration, and outcome. Daily runs use the UTC date as the key, while manual and agent evaluations use the input fingerprint. Policy decisions are supplied as structured context to model jobs, but cannot alter tool authority, schemas, or immutable safety prompts.

## Consequences

### Good

- The same documented rule and input produce the same reviewable decision for the app, agents, and scheduled automation
- Rego remains fully authorable while a typed result contract keeps invalid or unsafe output from becoming an action
- Preview, stored history, input fingerprints, traces, and correlated logs make every decision explainable and debuggable
- Existing actuator assignment, durable leases, independent Home Assistant cutoffs, and manual watering requirements remain authoritative
- Advisory mode lets a policy earn trust before it is allowed to mutate anything

### Bad

- Embedding OPA adds a large dependency and policy compilation cost that must be bounded and measured
- The input and output schemas become public product contracts that require careful versioning
- A safe automation runner needs durable idempotency and per-directive validation, not merely successful Rego evaluation
- Policies can still be logically wrong, so previews, history, advisory mode, and clear ownership remain necessary
- Some desired actions such as automatic watering remain intentionally unavailable even when a policy requests them

### Rejected because

- Hard-coded Go rules are safe but do not satisfy in-app authoring or let household knowledge evolve without a release.
- A separate OPA service adds deployment, authentication, availability, and configuration drift for one application without improving the policy boundary.
- Direct execution gives arbitrary policy text the Home Assistant credential and bypasses assignments, leases, evidence rules, idempotency, and independent safety controls.
