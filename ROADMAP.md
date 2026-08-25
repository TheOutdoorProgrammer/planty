# Roadmap

Planty's priority order is safety, operational truthfulness, recoverability, field usability, and then expansion.
This is the only active project backlog in the repository.
Shipped behavior is recorded in [CHECKLIST.md](CHECKLIST.md), architectural trade-offs live in [adr/](adr/), and operational knowledge lives in Dusk.

## Now: make failure visible and recoverable

### Self-healing photograph storage

Planty currently attempts MinIO initialization once when `serve` starts.
If MinIO is still starting, the API stays healthy but photograph routes remain disabled for the lifetime of that pod, leaving every iOS timeline image as a placeholder.

- Retry object-storage initialization with a bounded backoff after startup.
- Report photo-storage readiness separately from process liveness.
- Keep existing photo metadata readable while storage is temporarily unavailable, and restore signed URLs without restarting Planty.
- Add an integration test that starts Planty before its object store and proves recovery.

### Reliable daily judgments

A model can return malformed or schema-invalid JSON for one plant while the garden-wide run correctly records a partial failure.
That is truthful, but it leaves the user with a stale warning and no recovery beyond waiting for the next scheduled run.

- Add a bounded repair or retry path for invalid structured output.
- Preserve the original failure and model provenance for diagnosis.
- Expose which plants failed in the latest run instead of only aggregate counts.
- Add a safe operator-triggered rerun for failed plants without duplicating successful verdicts.

### Provider capability parity

The OpenAI-compatible harness can use Planty commands and trusted web pages, but it cannot selectively open the historical photographs offered to a consultation.
The catalogue currently treats vision and tool support as sufficient, so an interactive job can be assigned to a model that sees an attached current photo but not the timeline evidence the product promises.

- Add a bounded historical-photo tool or another on-demand transport that does not attach every image up front.
- Represent offered-photo access separately in model capabilities and assignment validation.
- Make the consultation UI disclose when its selected provider cannot inspect historical images.
- Decide whether the direct Anthropic API backend should gain the shared acting loop or be explicitly ineligible for acting jobs.

### Push registration health

The iOS app currently swallows `didFailToRegisterForRemoteNotificationsWithError`, while Settings only tests the HTTP service.
That made a correctly reachable service and a broken signed APNs entitlement look healthy.

- Show notification permission, APNs registration, token upload, environment, and last server acceptance as separate states.
- Add a real test-notification action instead of treating `/healthz` as a push test.
- Preserve the last registration error in diagnostics and provide a useful recovery action.
- Verify device-token refresh after reinstall, environment changes, and service URL changes.

### Safe manual watering foundation

`planty water` is manual-only, but the current process-held timer cannot turn the pump off after SIGKILL, OOM, node loss, or a network partition.
No scheduled watering is acceptable until water movement is independently bounded and verified.

- Persist a watering attempt before turning the pump on.
- Record pump start, stop, post-watering sensor evidence, and the final verified outcome separately.
- Do not record `watered` until delivery is verified.
- Add an independent Home Assistant or hardware maximum-on backstop that survives the Planty process.
- Alert when the pump reports activity without a moisture rise, and distinguish a clogged line from an unknown sensor result.

## Next: close data-integrity and recovery gaps

### Atomic photograph writes

- Make duplicate-photo handling safe under concurrent uploads instead of relying on a preflight lookup.
- Compensate object storage when database insertion fails so bytes and metadata cannot orphan each other.
- Make concurrent slug allocation deterministic when identical plants are created together.

### Recoverable records

- Add archive and restore controls to the iOS app.
- Add explicit photo-retention and deletion behavior, including scratch consultation photos.
- Validate external and presigned URLs before the client attempts to load them.

### Complete harvest history

- Add garden-wide aggregation by plant, unit, and season.
- Reject or explicitly represent non-positive quantities.
- Support correction and deletion without pretending an immutable bad record is useful history.

## Later: finish the field app

- Add a first-run setup flow for service URL, connectivity, notifications, and optional embedded configuration.
- Support Dynamic Type throughout action cards and settings.
- Replace the forced dark appearance with tested light, dark, and system modes.
- Build an intentional iPad layout instead of stretching the phone hierarchy.
- Make note editing and deletion discoverable.
- Route notification taps to the relevant plant, reminder, cold action, or owner update.
- Scope identification caching to the service configuration and invalidate it when photo content changes.
- Explore consistent-viewpoint capture guidance only after photo metadata can make the overlay trustworthy.

## Maintenance

- Give every async sheet the same loading, error, retry, and dismissal contract.
- Remove the dead `CaptureStore.upload` path or make it the single upload implementation.
- Consolidate duplicated default values and conversation-building logic across the service and app.
- Decide and publish the repository's license.

## Deliberately not planned

- Scheduled automatic watering without the independent safety work above.
- Home Assistant notifications, mobile-app fallbacks, or speaker escalation.
- Relative-humidity-triggered mushroom misting.
- A sensor dashboard, care streaks, global health score, fixed watering calendar, community feed, achievements, or general chat tab.
- A second source of garden state in the iOS app or Dusk plugin.
