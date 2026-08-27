# 22. Process iOS conversations as durable leased work

Date: 2026-08-26

## Status

Accepted.

## Context and Problem Statement

Planty's iOS chat currently holds one HTTP request open for the entire model call.
When iOS backgrounds or terminates the app, URLSession cancels that request and the request context cancels the model call.
Grafana records the resulting 502 responses as upstream `context canceled` errors, and no question or answer survives in conversation history.
The user needs to send a chat, close the app, and later reopen the stored conversation with the finished reply.

## Considered Options

1. Keep synchronous chat requests and ask iOS for background execution time
2. Detach model work into an in-memory goroutine after accepting the request
3. Persist each iOS conversation turn before returning and process it through a database-leased backend worker

## Decision Outcome

Chosen: **option 3**.

The iOS-specific message route accepts a client-generated conversation and turn identifier, validates and stores any attachment, inserts the turn as pending, and returns HTTP 202 before calling a model.
The API process runs a worker that claims pending turns through PostgreSQL leases, reconstructs completed conversation history and plant evidence, performs the model call with a service-owned context, then marks the turn complete or failed.
Expired leases are claimable after a pod dies, while active lease tokens prevent old and new pods in a rolling deployment from completing the same turn.
Conversation reads expose status so the app can render pending work and poll while open.
The existing synchronous ask routes remain for non-iOS clients whose request-response contract still requires the answer inline.

## Consequences

### Good

- Closing or backgrounding iOS no longer cancels accepted model work.
- A pod restart does not lose accepted questions, and expired work is retried.
- Client-generated turn identifiers make a retried submission idempotent.
- The existing non-iOS API contract stays compatible.

### Bad

- Conversation storage now carries a queue state machine and lease timestamps in addition to completed transcript data.
- The app must understand pending and failed turns and poll while a conversation is visible.
- A single API deployment now also owns worker capacity, so model throughput must remain bounded and observable.
- Exactly-once model side effects are not generally possible across a crash after the model acts but before the lease is completed; idempotent Planty agent commands remain required.

### Rejected because

- Rejected keeping the synchronous request because iOS background time is bounded and termination still cancels the network task; it cannot satisfy close-and-return reliability.
- Rejected an in-memory goroutine because a pod restart or rollout would erase accepted work and recreate the same failure at a different lifecycle boundary.
