# Bound Home Assistant actuation with durable Planty leases

## Context and problem statement

Planty needs to run plant-dedicated fans and smart plugs on demand without turning its broad Home Assistant credential into an arbitrary-device control surface.
An in-process timer cannot provide the physical safety boundary because a deployment restart can erase it after the device was turned on.
Owning recurring schedules in both Planty and Home Assistant would also create two authorities with conflicting state.

## Considered options

- Accept a Home Assistant entity ID on every start and stop request.
- Keep an allowlist but stop devices with an in-process timer only.
- Persist an allowlisted registry, a bounded lease with an absolute deadline, and an append-only command ledger, then reconcile overdue leases independently.
- Import recurring Home Assistant schedules into Planty.

## Decision outcome

Planty persists only explicitly selected `fan` and `switch` entities and exposes actuation by Planty UUID.
It commits a lease with a hard one-hour maximum before issuing `turn_on`.
The server and standalone reconciliation job issue `turn_off` for every overdue unfinished lease, including leases left uncertain by a restart.
Failed stops remain unfinished and retryable.
Home Assistant remains the sole owner of recurring schedules.

This keeps arbitrary household entities outside the API and makes the shutdown obligation survive process failure.
The cost is that safe operation now depends on reconciliation running promptly, and Planty still cannot prove a physical device obeyed Home Assistant without a separate state or airflow sensor.
The standalone recovery command exists so deployment health can verify and repair that obligation independently of the API request path.
