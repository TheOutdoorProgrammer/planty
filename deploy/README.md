# Deploying Planty

These files are public templates, not the live deployment.
Flux reconciles the completed copies in the private `flux` repository, so repository-specific URLs, credentials, and observed runtime values do not belong here.

## Manifests

| File | Resource |
| --- | --- |
| `namespace.yaml` | The `planty` namespace. |
| `postgres-cluster.yaml` | A single-instance CloudNativePG cluster. |
| `deployment.yaml` | The API deployment and ClusterIP service. |
| `cronjobs.yaml` | Ingest, watering verification, photo pruning, daily, chase, away, thirst, cold, and reminder jobs. |
| `configmap.yaml` | Public non-secret defaults and provider declarations. |
| `secret.yaml.example` | Template for database, Home Assistant, model-provider, APNs, and MinIO credentials. |

Every CronJob imports the same `planty-secrets` Secret as the service.
The live deployment must override the empty Home Assistant and object-storage endpoints and must configure a weather entity that actually supports a daily forecast.

## Judge configuration

`PLANTY_PROVIDERS` declares the available model backends.
The app chooses and persists one compatible model per model job, while an unassigned job follows the service default.

For the Claude Code subscription backend, run `claude setup-token` on an already signed-in machine and store the result as `CLAUDE_CODE_OAUTH_TOKEN`.
Do not also set `ANTHROPIC_API_KEY` unless metered Anthropic API use is intentional.
The Claude CLI writes temporary state, so the deployment and model-using CronJobs mount writable `emptyDir` volumes while the root filesystem remains read-only.

An OpenAI-compatible provider declares its base URL and the environment variable containing its key.
If that key is absent, the provider is not offered by the model catalogue.
Assignments are checked against verified vision, schema, and tool capabilities before they are accepted.

## Photograph storage

`PLANTY_S3_ENDPOINT` is the address Planty uses to reach MinIO.
`PLANTY_S3_PUBLIC_ENDPOINT` is the same bucket under a hostname the phone can reach.

Presigned URLs include the host in the signature, so changing a cluster-only hostname after signing invalidates the URL.
An unset or unreachable public endpoint makes successful uploads look like missing images in the app.

Planty retries photograph storage initialization with bounded backoff while keeping API liveness separate from photo readiness.
Metadata remains readable during an outage, and signed URLs return automatically after storage recovers.
The daily `prune-photos` job removes explicitly requested deletions and unowned scratch photographs older than 30 days.

## Network boundary

Planty has no authentication and must remain LAN-only.
There is intentionally no IngressRoute in this public directory.
Any private DNS name should resolve to a private address and be reachable only wherever the app itself is trusted to run.

Making the service public requires real authentication in the same change.
The pod holds credentials for Home Assistant, model providers, APNs, and object storage, so hiding an unauthenticated API behind an obscure hostname is not a security control.

## Deployment order

1. Release a multi-architecture image containing `linux/arm64` for the Apple-silicon cluster.
2. Apply the namespace, encrypted secret, and Postgres cluster, then wait for CloudNativePG readiness.
3. Apply the deployment and wait for migrations and service readiness.
4. Verify MinIO readiness, the `photo storage ready` log, and one returned timeline URL from a client-reachable host.
5. Seed the initial plants and questions with `kubectl -n planty exec deploy/planty -- /planty seed` when the database is new.
6. Link and calibrate sensors before interpreting moisture-derived care advice.
7. Run `planty cold`, `planty daily`, and a real APNs delivery check before relying on their schedules.

## Watering boundary

No CronJob moves water.
`planty thirst` reports dry plants, and `planty water` remains an explicit manual command.

Before a manual pump run, install `HSTEP/letpot2.0-home-assistant`, disable every LetPot schedule, calibrate every probe on the line, configure `PLANTY_PUMP_SWITCH`, and watch the physical system.
A wet plant vetoes the shared line.

The command persists its attempt before energizing the switch, explicitly stops on ordinary return and cancellation, and records `watered` only after the scheduled verifier observes a moisture rise.
The live Home Assistant installation independently caps the switch at three minutes and turns it off after a Home Assistant restart.
That backstop limits process and node failures, but it does not make a shared water line safe for unattended scheduling.

## Release and Fledge

The release workflow tests the iOS app, exports and verifies a production-APNs signed IPA, then asks Quill to publish the GitHub release, Fledge build, and matching container images as one versioned release.

The user-facing install page is [fledge.theoutdoorprogrammer.com/a/zone.stout.Planty](https://fledge.theoutdoorprogrammer.com/a/zone.stout.Planty).
`fledge.stout.zone` is only an internal LAN verification path used from the cluster environment and should not appear in user instructions.
