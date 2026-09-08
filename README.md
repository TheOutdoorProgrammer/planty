# Planty

Planty keeps houseplants alive when the person responsible for them has no idea what he is doing.

It watches soil moisture and room conditions through Home Assistant, remembers each plant and its owner, compares photographs over time, and gives one short answer: act on this plant or leave it alone.
Most days it should confidently say that nothing needs doing.

## The product rule

A missing, stale, or partial check is never an all-clear.
Planty preserves the difference between current calm, an unfinished judgment, stale evidence, and a system that has never run.

It also refuses to turn watering into a timer.
`planty water` is manual-only, persists every attempt, and receives an independent Home Assistant maximum-runtime cutoff.
The scheduled verifier observes delivery after the probes settle, but no scheduled job starts the pump.

## Surfaces

The Go service is the only owner of state.

| Surface | Purpose |
| --- | --- |
| **Service** | Stores the garden in PostgreSQL, reads Home Assistant, stores photographs in MinIO, runs model jobs, and sends APNs notifications. |
| **iOS app** | Shows today's work, captures photographs, records care, keeps plant stories, handles consultations, and manages garden workflows. |
| **Dusk plugin** | Gives agents the same API-backed records and actions without copying Planty's state into Dusk. |

Anything important is reachable without a conversation.
The app and plugin are clients of the same generated HTTP contract in [api/openapi.json](api/openapi.json).

## Running it

```sh
export PLANTY_DATABASE_URL=postgres://planty:...@localhost:5432/planty
planty migrate
planty seed
planty serve
```

Scheduled work is one command per Kubernetes CronJob:

| Command | Cadence | Purpose |
| --- | --- | --- |
| `planty ingest` | Every 20 minutes | Import current sensor readings from Home Assistant. |
| `planty verify-water` | Every 15 minutes | Close completed manual watering attempts after sensor evidence has settled. |
| `planty prune-photos` | 03:30 | Finish requested deletions and expire unowned consultation photos after 30 days. |
| `planty reconcile-actuators` | Every minute | Turn off expired fan leases and enforce owner-configured fan and grow-light schedules. |
| `planty daily` | 08:00 | Judge active plants, optionally run an evidence-justified assigned fan, persist the garden-wide run, sweep for postmortems, and send a digest. |
| `planty away` | 08:30 | Send the pre-departure pass or return briefing. |
| `planty thirst` | 09:00 and 18:00 | Report calibrated plants that appear dry without moving water. |
| `planty chase` | 13:00 and 20:00 | Follow up on open care actions. |
| `planty cold` | 15:00 | Warn about tonight's low or tell the user when sheltered plants can go back out. |
| `planty remind` | Hourly | Send due chores that sensors cannot observe. |

The iOS app can launch these same CronJob templates from Settings.
When Today reports that Planty needs a fresh look, Run fresh check launches `planty daily`, follows the Kubernetes Job to completion, and reloads the result.
An active scheduled or manual run is reused so repeated taps do not duplicate model calls or notifications.

`planty autopsy <slug>` is an on-demand model job.
`planty water` is an on-demand actuator command and has no schedule by design.

## Model providers

Planty chooses a model per job rather than forcing assessment, identification, consultation, postmortem, and owner-update work through one model.
The iOS Settings screen persists those assignments, and the service rejects a model that lacks the vision, schema, or tool capabilities a job requires.
Each model job may also carry a user-editable instruction overlay.
The overlay can add household context, priorities, and style preferences, while safety rules, evidence requirements, response schemas, and tool authority remain immutable in code.

Providers are declared with `PLANTY_PROVIDERS`.
The configured fallback selected by `PLANTY_JUDGE` can use the Claude Code subscription or the direct Anthropic API, while declared OpenAI-compatible providers use the shared chat-completions harness.
Daily assessment and consultation are acting jobs, so they require the Claude Code CLI or OpenAI-compatible harness; the direct Anthropic API fallback remains available only to one-shot jobs that do not execute Planty tools.
Current photographs can reach any verified vision model, and acting providers must advertise offered-photo access before they can be assigned to consultations.
The Claude Code CLI and OpenAI-compatible harness can selectively open offered history; the direct Anthropic API remains explicitly ineligible.
The OpenAI-compatible harness gives the prompt and photo tool the same numbered catalogue, with explicit zero-based arguments and recoverable validation errors.
Selected images follow the complete batch of text tool replies as user image content, so providers receive the Chat Completions vision format even when several photos are opened together.
Photo access emits `model.photo.open` spans with the offered count, selected index, and outcome, without photograph bytes, labels, or conversation content.

[ADR 0001](adr/0001-buy-judgments-through-the-claude-code-cli.md) explains the subscription-backed default.
[ADR 0007](adr/0007-choose-a-model-per-job.md) records per-job selection, and [ADR 0008](adr/0008-run-the-acting-loop-in-the-openai-compatible-harness.md) records the later shared tool loop.

## Integrations and boundaries

Home Assistant supplies sensor readings, a forecast, the optional LetPot watering line, and explicitly registered plant fans, smart plugs, or grow lights.
Planty can run a registered actuator for at most one hour from a plant page or an evidence-driven assessment, records successful shared airflow against every assigned plant, and persists the shutdown deadline before Home Assistant receives `turn_on`.
Owner-configured daily fan and grow-light schedules persist up to 12 non-overlapping time windows, survive restarts, and defer to an active bounded fan run.
Grow lights instead use direct state control and a Planty-owned, timezone-aware daily schedule whose windows the app or an assigned AI agent can change.
It is not a notification transport.
Planty sends scheduled alerts directly to registered iOS devices through APNs and fails the job when native delivery is unavailable.

MinIO stores photograph bytes and PostgreSQL stores their metadata and object keys.
`PLANTY_S3_PUBLIC_ENDPOINT` must name the same bucket through a hostname the phone can reach because a presigned URL cannot be rewritten after signing.

Application routes require a deployment-scoped bearer token. Private native-symbol publication requires a verified GitHub release workflow identity instead. Kubernetes liveness and readiness probes remain public.
The service still belongs on the LAN because the pod holds credentials for Home Assistant, model providers, APNs, and object storage.

## Native diagnostics

`POST /v1/native-telemetry` accepts a versioned, bounded diagnostic envelope through the existing application authentication. It forwards sanitized logs and traces to `OTEL_EXPORTER_OTLP_ENDPOINT` and returns 204 only after both signals are accepted. Clients retain the same event IDs on transport failures, 429, or 503; duplicate delivery is possible if an acknowledgement is lost.

The envelope contains `schema_version: 1`, the originating numeric `release` and `build`, and at most 16 events within 64 KiB. Each event has a fresh UUID-v4 `id`, UTC `timestamp`, `operation`, `outcome`, and bounded `duration_ms`. Operations are `app.start`, `notification.open`, `api.request`, `app.crash`, and `app.hang`. Outcomes are `success`, `failure`, and `cancelled`. Failures require a closed `error_class`: `transport`, `timeout`, `unauthorized`, `server`, `decoding`, `crash`, `hang`, or `other`. Unknown fields are rejected. Events older than 30 days or more than five minutes in the future are rejected.

Crash and hang events include only app-image `image_uuid`, `architecture` (`arm64`, `arm64e`, or `x86_64`), and up to 64 text-relative `frames` containing integer `offset` values. The relay matches private symbols by UUID and architecture, then uses LLVM to resolve safe function names, file basenames and line numbers. Missing symbols leave explicitly unresolved frame identities. The additional `telemetry.delivery` operation reports only `queue_full` or `queue_expired` failures, so bounded replay loss can be detected after delivery recovers. Raw exception messages, notification contents, user identifiers, memory dumps, credentials, and URLs are not accepted.

`PUT /v1/native-symbols/{image_uuid}/{architecture}` retains a raw, thin dSYM DWARF object of at most 64 MiB under a private object-storage prefix. It has no download or presigned-URL route. Writes are atomic create-if-absent; identical retries succeed and conflicting replacements return 409. Configure `PLANTY_SYMBOLS_REPOSITORY` and its immutable numeric `PLANTY_SYMBOLS_REPOSITORY_ID` to enable publication. GitHub identity must have audience `planty-native-symbols`, the exact repository identity, `refs/heads/main`, and that repository's `.github/workflows/release.yml` workflow dispatched from main.

From that trusted release job, `planty publish-symbols <archive-dSYMs-directory>` reads the existing `PLANTY_BASE_URL` and GitHub's `ACTIONS_ID_TOKEN_REQUEST_URL` / `ACTIONS_ID_TOKEN_REQUEST_TOKEN`. The job needs `id-token: write`. The command validates each archived object and publishes it without placing a long-lived upload credential in CI, printing credentials, or exposing symbols in public releases or images. Symbol publication must finish before distributing the corresponding native build.

The iOS app records startup, notification opens, API outcomes and MetricKit crash or hang diagnostics. API requests carry sampled W3C trace context that matches their persisted completion event. Its bounded disk queue prioritizes failures over routine events and retains loss reports across restarts. Delivery pauses while the app is inactive and resumes with the existing app configuration. Only the approved diagnostic fields enter this queue.

Delayed diagnostics are logged at receipt time so Loki's ingestion limits do not discard old or out-of-order events. The original occurrence time remains in `event.timestamp` and the trace. Symbol lookup has a shared one-second budget per batch; a slow symbol store leaves explicit unresolved frames while the relay delivers the diagnostic.

The release workflow checks that the signed app and its dSYM have matching image UUIDs, then requires successful private symbol publication before staging the IPA for distribution. Dry runs validate that match without publishing symbols. Deploy the symbol endpoint before the first native-instrumented release.

MetricKit reports platform architecture, which can differ from the app slice. If an `arm64e` object is unavailable, symbolication may use a trusted `arm64` object with the exact same binary UUID. The object header is validated before LLVM runs; other architecture or UUID mismatches remain unresolved.

[ADR 0033](adr/0033-relay-bounded-native-diagnostics-and-retain-private-release.md) records the capture and transport choices. Install the new native build to enable capture. MetricKit delivery is controlled by iOS and may be delayed or absent; finite local replay and collector storage also prevent a guarantee of capturing every crash.

## Repository map

| Path | Contents |
| --- | --- |
| `api/` | Canonical OpenAPI contract. |
| `cmd/planty/` | Service, migrations, seed command, agent command, and scheduled jobs. |
| `internal/` | Domain, storage, API, model, job, photo, seed, notification, and constrained-agent implementations. |
| `ios/` | SwiftUI field app. |
| `deploy/` | Public Kubernetes templates mirrored and completed in the private Flux repository. |
| `docs/` | Current domain and integration behavior. |
| `design/` | Current product principles plus explicitly historical visual exploration. |
| `adr/` | Immutable architectural decisions. |

## Documentation

- [OPA policy rules](docs/OPA-POLICIES.md): authoring, inputs, outputs, examples, safety boundaries, and API.

- [Delivered capabilities](CHECKLIST.md)
- [Roadmap](ROADMAP.md)
- [Data model and API behavior](docs/DATA-MODEL.md)
- [Managed choices and constrained vocabulary](docs/managed-choices.md)
- [Friend's plant-care ground truth](docs/friends-plants.md)
- [Home Assistant boundary](docs/home-assistant.md)
- [Cold-snap behavior](docs/cold-snap-automation.md)
- [Push and owner updates](docs/push-and-owner-updates.md)
- [iOS implementation](ios/README.md)
- [Deployment](deploy/README.md)
- [Current UI concept](design/UI-CONCEPT.md)
- [Screen behavior](design/SCREENS.md)
- [Mascot and historical identity work](design/MASCOT.md)

## Tests

```sh
go test -race ./...
PLANTY_TEST_DATABASE_URL=postgres://... go test ./internal/store/...
```

SQL behavior gets integration coverage because a query can compile, satisfy every mock, and still be rejected by PostgreSQL.
The iOS test command is documented in [ios/README.md](ios/README.md).

## License

Planty is available under the [Apache License 2.0](LICENSE).
