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
| `planty reconcile-actuators` | Every minute | Turn off bounded fan and smart-plug leases whose deadlines elapsed. |
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

[ADR 0001](adr/0001-buy-judgments-through-the-claude-code-cli.md) explains the subscription-backed default.
[ADR 0007](adr/0007-choose-a-model-per-job.md) records per-job selection, and [ADR 0008](adr/0008-run-the-acting-loop-in-the-openai-compatible-harness.md) records the later shared tool loop.

## Integrations and boundaries

Home Assistant supplies sensor readings, a forecast, the optional LetPot watering line, and explicitly registered plant fans or smart plugs.
Planty can run a registered actuator for at most one hour from a plant page or an evidence-driven assessment, records successful shared airflow against every assigned plant, and persists the shutdown deadline before Home Assistant receives `turn_on`.
It is not a notification transport.
Planty sends scheduled alerts directly to registered iOS devices through APNs and fails the job when native delivery is unavailable.

MinIO stores photograph bytes and PostgreSQL stores their metadata and object keys.
`PLANTY_S3_PUBLIC_ENDPOINT` must name the same bucket through a hostname the phone can reach because a presigned URL cannot be rewritten after signing.

Every application route requires a deployment-scoped bearer token, while Kubernetes liveness and readiness probes remain public.
The service still belongs on the LAN because the pod holds credentials for Home Assistant, model providers, APNs, and object storage.

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
