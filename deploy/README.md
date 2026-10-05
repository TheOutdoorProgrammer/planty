# Deploying Planty

These files are public templates, not the live deployment.
Flux reconciles the completed copies in the private `flux` repository, so repository-specific URLs, credentials, and observed runtime values do not belong here.

## Manifests

| File | Resource |
| --- | --- |
| `namespace.yaml` | The `planty` namespace. |
| `postgres-cluster.yaml` | A single-instance CloudNativePG cluster. |
| `deployment.yaml` | The API deployment, ClusterIP service, and namespace-scoped scheduled-job RBAC. |
| `codex-pvc.yaml` | Shared persistent Codex subscription state for the API and daily job. |
| `cronjobs.yaml` | Ingest, watering verification, photo pruning, daily, chase, away, thirst, cold, and reminder jobs. |
| `configmap.yaml` | Public non-secret defaults and provider declarations. |
| `secret.yaml.example` | Template for API, database, Home Assistant, APNs, and MinIO credentials. |

Every CronJob imports the same `planty-secrets` Secret as the service.
Scheduled commands wait up to five minutes for a transient PostgreSQL connection failure before starting work, with ten-second connection attempts and jittered backoff. Authentication and configuration failures still fail immediately. Migrations and job side effects are never replayed by this wait; interactive commands retain their immediate connection check. Retry warnings and a `database.connect` span expose the wait, and an exhausted deadline still fails the Job.
The live deployment must override the empty Home Assistant and object-storage endpoints and must configure a weather entity that actually supports a daily forecast.

The API service account may read Planty's CronJobs and read or create Jobs in the `planty` namespace.
The manual-run API maps stable code-owned identifiers to exact CronJob names, copies their current job templates, and never accepts an arbitrary Kubernetes resource name or command.
Manual copies expire seven days after completion, while repeated requests reuse an active scheduled or manual run.

Confirmed model-provider quota exhaustion exits with code 3. The daily Job's
`podFailurePolicy` marks that failure terminal without rerunning the whole batch;
otherwise a retry would create a newer empty run that hides partial coverage.
Other failures exit with code 1 and retain the configured Kubernetes retry budget.
After provider allowance recovers, use `planty retry` to assess only failed plants.
The quota exit code requires a matching policy in the live CronJob template.

## Judge configuration

`PLANTY_PROVIDERS` declares the available model backends.
The app chooses and persists one compatible model per model job, while an unassigned job follows the service default.

These templates select `PLANTY_JUDGE=codex`, `PLANTY_JUDGE_MODEL=gpt-6-astra`, and a single `codex` provider. The image includes pinned Codex 0.154.0, with no Claude binary.
The Codex backend requires ChatGPT subscription authentication and has no metered API fallback.
Assignments are checked against verified vision, schema, and tool capabilities before they are accepted.
Existing assignments must be updated through Settings or the model-assignment API after the new provider is configured; changing the fallback does not replace them.

Create a dedicated Planty login using the [supported Codex login flow](https://developers.openai.com/codex/auth/) and file credential storage, as shown in the [project README](../README.md#codex-subscription-setup).
Do not clone a personal session that is still in use: independent refreshes can invalidate the shared login, as described in [Codex CI authentication](https://developers.openai.com/codex/auth/ci-cd-auth).
Use a secure channel to seed the dedicated home into the `planty-codex` volume once, then keep that volume writable so Codex can persist refreshed credentials. Never commit the home or copy a static `auth.json` Secret over it on every startup.

The volume's `/codex` directory must belong to UID/GID65532 with mode0700, and `auth.json` must have mode0600. Verify ownership while provisioning: `fsGroup` does not change ownership for every storage driver, including some local-path/hostPath configurations.
The backend serializes requests with a file lock across API and daily processes, including token refresh. Keep those processes on the same underlying volume; a second copy defeats the lock.
ReadWriteOnce allows these pods to share a volume on one node. Multi-node clusters need compatible placement or storage. Set the storage class for your environment and retain the volume during upgrades; its requested capacity may not be an enforced quota.
The home and `/tmp` scratch mounts remain disposable, while `/codex` is persistent. The container root filesystem stays read-only.

## Photograph storage

`PLANTY_S3_ENDPOINT` is the address Planty uses to reach MinIO.
`PLANTY_S3_PUBLIC_ENDPOINT` is the same bucket under a hostname the phone can reach.

Presigned URLs include the host in the signature, so changing a cluster-only hostname after signing invalidates the URL.
An unset or unreachable public endpoint makes successful uploads look like missing images in the app.

Planty retries photograph storage initialization with bounded backoff while keeping API liveness separate from photo readiness.
Metadata remains readable during an outage, and signed URLs return automatically after storage recovers.
The daily `prune-photos` job removes explicitly requested deletions and unowned scratch photographs older than 30 days.

## Network boundary

Planty requires `PLANTY_API_TOKEN` for every application route.
The iOS app and Dusk plugin send that deployment-scoped bearer token; `/healthz` and `/readyz` remain public for Kubernetes probes.
There is intentionally no IngressRoute in this public directory, because one shared token is appropriate for trusted LAN clients but is not a multi-user internet identity system.
Private DNS should still resolve to a private address and TLS should still protect the token in transit.

## Deployment order

1. Release an image supporting the cluster architecture and pin its tag and digest in the deployment overlay. It must contain the Codex backend before applying these provider settings.
2. Apply the namespace, securely provisioned Secret, and Postgres cluster, then wait for CloudNativePG readiness.
3. Provision `codex-pvc.yaml`, seed its dedicated subscription login, and verify permissions before starting model workloads.
4. Apply the completed ConfigMap, deployment and CronJob templates, then verify migrations and service readiness.
5. Verify MinIO readiness, the `photo storage ready` log, and one returned timeline URL from a client-reachable host.
6. Seed the initial plants and questions with `kubectl -n planty exec deploy/planty -- /planty seed` only when the database is new. Link and calibrate sensors before interpreting moisture-derived care advice.
7. Verify Astra's vision, strict schema and constrained tools before updating stored model assignments. After a failed assessment batch, run `planty retry` to preserve existing successes, and confirm a complete ledger plus correlated logs and traces.
8. For a new installation, run `planty cold`, `planty daily`, and a real APNs delivery check before relying on their schedules.

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
