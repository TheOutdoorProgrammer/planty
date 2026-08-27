# 25. Expose independent Planty policy rules

Date: 2026-08-27

## Status

Accepted. Supersedes [ADR-0024](0024-embed-opa-behind-a-typed-policy-contract-and-physical-safety.md).

## Context and Problem Statement

The aggregate decision object makes independent conditions mutually exclusive and forces every policy author to construct unrelated empty fields.
Planty needs composable rules where omission means no result while retaining typed checks around side effects.

## Considered Options

1. Keep one aggregate `data.planty.decision` object
2. Evaluate independent top-level rules under `data.planty.v1`

## Decision Outcome

Chosen: **option 2**.

Planty evaluates the object at `data.planty.v1`.
Every materialized top-level rule is recorded.
A missing rule is inactive, a boolean rule follows its boolean value, and a present non-boolean value is active.
The extensible `needs_<thing>` care-rule family, named care rules, and their original values remain available to the app and agents.
Known health-adjustment, notification, fan, and agent-context rules are normalized into typed fields; side-effecting values fail closed when malformed.

## Consequences

### Good

- Independent Rego rules compose without mutually exclusive aggregate object construction
- New `needs_<thing>` concerns compose without changing Planty's contract
- The app and agents retain the original value and a deterministic active flag
- Physical side effects remain behind explicit typed validation and existing safety controls

### Bad

- A misspelled `needs_<thing>` suffix is still a valid owner-defined care rule
- The service must preserve arbitrary JSON values across PostgreSQL, API, and iOS
- A present empty collection or null is active because presence, not truthiness, is the contract
- Known singular and plural directive aliases add normalization code

### Rejected because

- The aggregate object couples unrelated rules, requires noisy defaults, and conflicts when multiple complete decision rules match.
