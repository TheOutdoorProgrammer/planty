# 33. Relay bounded native diagnostics and retain private release symbols

Date: 2026-09-08

## Status

Accepted.

## Context and Problem Statement

The iOS application has no remotely available crash diagnostics, so backend traces cannot explain failures during notification routing.
The signed archive already produces dSYMs but the release handoff retains only the IPA.
Native binaries and public container images cannot hold Grafana credentials or private debug symbols.

## Considered Options

1. Use MetricKit diagnostics with a strict authenticated application relay and private dSYM retention.
2. Use a native crash reporter with its standard on-disk reports.
3. Export unrestricted OTLP directly from the mobile application.
4. Keep logs on the device and discard release symbols.

## Decision Outcome

Chosen: **option 1**.

Use Apple's MetricKit delivery through its public subscriber interface and project only release/build, bounded operation outcomes, app-image UUID, architecture and text-relative frame offsets before application persistence. The existing OpenTelemetry Swift MetricKit adapter loses the originating diagnostic build, so it cannot safely establish symbol identity for delayed reports. Reuse the existing application API bearer authentication for a narrow versioned envelope; the server converts it to OTLP and acknowledges only after the durable collector accepts both logs and traces. Retain dSYM DWARF objects privately through a separate upload endpoint authorized by GitHub's verified issuer, audience, immutable repository ID, main ref and release workflow. Resolve frames with LLVM and emit safe function names, file basenames and line numbers. Keep unresolved frame identity when symbols are unavailable.

## Consequences

### Good

- No general telemetry credential or symbol publisher privilege reaches the application.
- Crash reports retain their originating build and matching image UUID across delayed delivery and application updates.
- Existing API authentication, object storage, release archive and OTLP collector are reused.
- Privacy and replay behavior can be tested independently from OS delivery timing.

### Bad

- MetricKit delivery is controlled by iOS and may be delayed or absent; this does not guarantee every crash.
- Offline replay has finite storage and retry limits and can produce duplicate signals when an acknowledgement is lost.
- Private symbol retention adds storage and trusted publication checks; unavailable symbols leave bounded unresolved frames.
- Signed physical-device diagnostics require installation and launch and cannot be proven solely by simulator unit tests.

### Rejected because

- Standard crash reporter dumps can persist raw registers, memory and exception text before an export filter sees them, conflicting with the chosen privacy boundary.
- Unrestricted mobile OTLP would expose a broadly useful credential or allow arbitrary payload ingestion.
- Device-only logs and discarded symbols preserve the diagnostic gap this change must close.
