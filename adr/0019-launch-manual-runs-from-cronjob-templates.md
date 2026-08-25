# 0019: Launch manual runs from CronJob templates

- Status: Accepted
- Date: 2026-08-25

## Context and problem statement

Planty's Today screen can detect that its latest assessment is stale or incomplete, but the phone previously offered only connection settings and a camera shortcut.
The scheduled work already exists as Kubernetes CronJobs, and a phone-triggered recovery must use the same command, image, secrets, resource limits, and writable volumes as the scheduled run.
Accepting a Kubernetes resource name or command from the app would turn a narrow recovery feature into remote command execution.

## Considered options

### Run job functions inside the API process

This would avoid Kubernetes API access.
It would also make model work share the serving pod's lifetime and resources, duplicate CronJob assembly, and lose a long-running recovery when the API pod restarts.

### Maintain a second queue and worker

A database-backed queue would make orchestration independent of Kubernetes.
It would introduce another scheduler, worker lifecycle, retry policy, and deployment contract for work Kubernetes already models durably.

### Create a Job from an allowlisted CronJob template

The API can copy the live CronJob's job template, label the manual run, and let Kubernetes own execution and status.
A code-owned ID maps to an exact CronJob name, and a namespace-scoped ServiceAccount may only read CronJobs and create or read Jobs.

## Decision outcome

Planty creates manual Jobs from a fixed allowlist of its live CronJob templates.
The API exposes stable job IDs rather than Kubernetes names or commands, reuses an already active scheduled or manual run, and gives completed manual Jobs a seven-day TTL.
The iOS app exposes every allowlisted job in Settings and makes the stale Today state run the daily assessment directly.

### Consequences

- Manual and scheduled runs use identical runtime configuration and survive API restarts.
- The app can show queued, running, succeeded, and failed state from Kubernetes instead of inventing a second run ledger.
- The API pod receives a narrow namespace Role for CronJob reads and Job creation and reads.
- The feature is unavailable outside Kubernetes unless a launcher is injected, while the rest of the API continues serving.
- Concurrency is enforced by active-run checks and the single API replica; a future multi-replica API would need a Kubernetes Lease or another cross-replica lock before scaling out.
