# Delivered capabilities

This document records what Planty ships today.
Future work belongs in [ROADMAP.md](ROADMAP.md), durable trade-offs belong in [adr/](adr/), and the canonical HTTP contract is [api/openapi.json](api/openapi.json).

## Service and state

- [x] PostgreSQL is the sole source of truth and migrations run when the service starts.
- [x] Plants cover houseplants, indoor edibles, and outdoor edibles with ownership, location, care, toxicity, and archival state.
- [x] Sensor links carry per-probe wet and dry calibration instead of treating raw percentages as comparable.
- [x] Observations, readings, photographs, notes, reminders, harvests, questions, consultations, and postmortems are durable records.
- [x] Daily verdicts include evidence and model provenance, and only one verdict per plant can remain open.
- [x] Garden-wide judgment runs record expected, successful, failed, and completed counts so a partial run cannot look like an all-clear.
- [x] Care and reminder completion writes are idempotent.
- [x] Push device registrations and per-job model assignments are persisted.
- [x] Per-job prompt instruction overlays are persisted without making safety, schema, evidence, or tool-authority rules editable.
- [x] Plant health is an append-only 0-to-100 evidence ledger with signed, clamped, attributable corrections.
- [x] Rechecks and household experiments share a durable evidence-window lifecycle with bounded review dates and audited Do-Not-Disturb overrides.
- [x] Garden incidents correlate inspectable shared factors without replacing any plant's individual action or claiming causation.
- [x] Plant fans and smart plugs require explicit plant assignments and bounded durable shutdown leases.
- [x] Photograph storage retries after startup, reports readiness separately, and recovers without a pod restart.
- [x] Photograph uploads are atomic under duplicate races and compensate object storage when metadata cannot be committed.
- [x] Archived plants can be restored without losing their story, and explicit photo deletion removes metadata and bytes through a retryable pending state.
- [x] Scratch consultation photos expire after 30 days through `prune-photos`.
- [x] Harvest quantities must be positive and harvests can be corrected, deleted, and summarized by plant, unit, season, and year.
- [x] The OpenAPI contract generates Go routes and Swift paths and enums, with CI drift checks.

## Scheduled work

- [x] `ingest` imports Home Assistant sensor readings every 20 minutes.
- [x] `thirst` reports dry calibrated plants twice daily without moving water.
- [x] `daily` judges every active plant, records the run outcome, sweeps for postmortems, and sends one digest.
- [x] `chase` follows up on unacknowledged care actions twice daily.
- [x] `away` sends departure and return care briefings.
- [x] `cold` compares the forecast against each plant's minimum temperature and tracks shelter state in Planty.
- [x] `remind` sends due non-sensor chores hourly.
- [x] `water` exists only as an explicit manual command and has no CronJob.
- [x] `verify-water` closes persisted manual watering attempts every 15 minutes after sensor evidence settles.
- [x] `prune-photos` removes requested deletions and expired scratch photographs daily.
- [x] `reconcile-actuators` independently turns off expired fan and smart-plug leases every minute.

## iOS app

- [x] Today distinguishes calm, stale, incomplete, unconfigured, loading, and actionable states.
- [x] Capture supports camera and photo-library input, plant identification, plant creation, timeline upload, and quick observations.
- [x] Plant stories merge photographs, findings, sensor evidence, observations, notes, reminders, and harvest history.
- [x] Consultations can use current and historical photographs and can write through the constrained `planty agent` interface.
- [x] More contains away mode, cold shelter state, owner questions, harvest history, postmortem lessons, owner updates, and settings.
- [x] Settings tests API reachability and allows a compatible model to be selected independently for each model job.
- [x] A stale Today state can launch a fresh daily check, and Settings can rerun any allowlisted CronJob from the phone without exposing arbitrary Kubernetes execution.
- [x] Native APNs registration and device-token upload are implemented.
- [x] Settings separates notification permission, APNs registration, token upload, environment, server acceptance, and real delivery testing.
- [x] Notification taps route to Today, Capture, Settings, or a specific plant when the payload names one.
- [x] Archived plants are visible and restorable, and photo, note, and harvest correction controls are visible without hidden gestures.
- [x] The app validates photograph URLs, scopes identification cache hits to the configured service, and guides repeatable camera framing.
- [x] Accessibility Dynamic Type collapses dense action grids, and iPad uses a sidebar-adaptable tab layout.
- [x] Room-grouped care rounds are available in the app and through a Lock Screen-capable App Intent.
- [x] Plant stories expose health history, persisted visual rechecks, honest prior-photo overlays, and printable QR labels.
- [x] More exposes bounded household experiments and exactly one backend-ranked next-best evidence input.
- [x] Incident Radar preserves individual urgent actions and labels shared timing as correlation rather than causation.
- [x] Settings edits per-job prompt overlays and explicitly registers plant assignments for discovered Home Assistant fans and switches.
- [x] Thumbnail bytes are cached on disk across launches with bounded eviction and authenticated cache identity.
- [x] Release automation tests, signs, verifies the production APNs entitlement, publishes the IPA through Fledge, and releases matching server artifacts.

## Safety and truthfulness

- [x] Stale or incomplete evidence cannot render as calm.
- [x] Unknown toxicity cannot render as safe.
- [x] Uncalibrated sensors cannot authorize watering.
- [x] A wet plant vetoes the shared LetPot line.
- [x] Manual watering persists start, activity, stop, and sensor verification independently, and Home Assistant caps the physical switch at three minutes.
- [x] Scheduled notifications use APNs only and fail loudly instead of silently falling back to Home Assistant.
- [x] Browser writes are same-origin unless a LAN host is explicitly allowed.
- [x] Every private API route requires a deployment-scoped bearer token, while liveness and readiness remain public.

## Integrations

- [x] Home Assistant supplies sensors, a daily forecast, the optional LetPot watering line, and explicitly registered plant fans or smart plugs.
- [x] MinIO stores photograph bytes while PostgreSQL stores their metadata and object keys.
- [x] The Dusk plugin exposes Planty records and actions without keeping a second copy of garden state.

## Verification

- [x] Go unit, integration, race, and live-provider tests cover the service at the appropriate boundaries.
- [x] Swift Testing covers presentation, stores, networking, camera intake, completion semantics, and generated contract behavior without requiring a live network.
- [x] CI builds both codebases and rejects generated-contract drift.
- [x] Release verification inspects the signed application entitlement rather than trusting the provisioning profile alone.
- [x] The repository is published under Apache License 2.0.
