# 23. Process every iOS model operation as durable leased work

Date: 2026-08-27

## Status

Accepted. Supersedes [ADR-0022](0022-process-ios-conversations-as-durable-leased-work.md).

## Context and Problem Statement

Plant-specific iOS chat became durable in ADR 22, but scratch chat and identification still hold HTTP requests open for model execution.
Grafana showed identification and scratch requests cancel together when the client session disappeared, followed by visible 502 responses.
The same lifecycle boundary therefore still makes ordinary iOS workflows unreliable.

## Considered Options

1. Keep identification and scratch chat synchronous and retry them in iOS
2. Create a separate queue table and worker for each model operation
3. Extend the existing PostgreSQL-leased turn state machine with operation-specific payload and result adapters

## Decision Outcome

Chosen: **option 3**.

Use client-generated operation and turn identifiers for every iOS model call. Accept and persist attachments plus the operation payload before returning HTTP 202, process them from service-owned worker contexts under the existing lease rules, and expose status reads that iOS can poll or resume. Scratch chat remains separate from plant history, while identification stores candidates rather than a conversational answer. Existing synchronous routes remain compatible for non-iOS callers.

## Consequences

### Good

- Backgrounding, navigation, network changes, and app termination cannot cancel accepted model work.
- Retries reuse stable identifiers and cannot enqueue duplicate paid model calls.
- One lease implementation governs crash recovery, retries, and stale-worker protection.
- Existing synchronous API clients remain compatible.

### Bad

- The conversation storage table now also carries identification jobs under a kind discriminator.
- iOS must preserve pending identifiers and understand accepted, processing, complete, and failed states.
- Unowned photographs remain in object storage until the existing scratch-photo sweeper removes them.
- A shared worker must dispatch operation-specific payloads and results without mixing their wire shapes.

### Rejected because

- Rejected client retries because a canceled response is ambiguous, can repeat a paid model call, and still loses work when the process terminates.
- Rejected separate queues because each would duplicate lease expiry, retry, idempotency, telemetry, and stale-worker rules that already exist.
