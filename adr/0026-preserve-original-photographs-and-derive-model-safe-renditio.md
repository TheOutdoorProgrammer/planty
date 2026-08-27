# 26. Preserve original photographs and derive model-safe renditions

Date: 2026-08-27

## Status

Accepted.

## Context and Problem Statement

Planty accepts high-resolution photographs that are valid for durable storage and human viewing but exceed some model providers' image envelopes.
A 5712x4284, 4.23 MB iPhone JPEG was accepted and stored, then deterministically rejected by Console Go with HTTP 415 whenever Kimi opened it.
The same photograph auto-oriented and resized within 2048x2048 succeeded.

## Considered Options

1. Preserve durable originals and derive bounded model renditions at the model boundary
2. Resize every photograph at ingestion and store only the reduced image
3. Reject photographs that exceed the strictest configured model provider
4. Normalize only in the iOS client

## Decision Outcome

Keep the uploaded photograph unchanged as the durable record. Before image bytes cross any model boundary, validate declared pixel area, apply JPEG EXIF orientation when conversion is necessary, fit within 2048 pixels, encode as JPEG, and bound the encoded rendition to 2 MiB. Small images already inside that envelope pass through unchanged. Record normalization dimensions and byte counts on the active trace without recording image content.

Treat deterministic provider 4xx responses as permanent so durable workers do not repeat an identical rejected payload. Timeouts, conflicts, rate limits, and server failures remain retryable.

## Consequences

### Good

- Human-facing originals retain their full resolution and metadata
- Every backend and client benefits from one server-owned model envelope
- Existing oversized photographs become usable without rewriting stored objects
- Deterministic request failures stop consuming three identical model attempts
- Trace events show when normalization occurred and how much it reduced the payload

### Bad

- The service spends CPU and memory decoding large photographs when a model actually needs them
- Model renditions are recomputed rather than cached
- JPEG conversion can discard transparency and some fine detail
- The portable 2048 pixel and 2 MiB envelope may need revision as providers change

### Rejected because

- Ingestion-only resizing destroys the original evidence and couples human photo quality to current model limits.
- Rejecting large photographs makes a valid phone capture the user's problem even though Planty can adapt it safely.
- iOS-only normalization leaves API, agent, historical, and future clients able to reproduce the same failure.
