# Authenticate every private API route

## Context and Problem Statement

Planty's HTTP service holds credentials for Home Assistant, model providers, APNs, and object storage.
The iOS app and Dusk plugin already send a bearer token, but the service did not validate it and the live deployment did not configure one.
That mismatch was already misleading for ordinary garden writes and becomes unacceptable once an API call can energize a fan.
The existing browser-origin guard prevents cross-site form attacks, but it does not authenticate native clients or another machine on the LAN.

## Considered Options

- Keep the service unauthenticated and rely on private DNS and LAN reachability.
- Authenticate only actuator routes.
- Authenticate every application route with one deployment-scoped bearer token while leaving Kubernetes health probes public.
- Introduce per-user OAuth and fine-grained scopes before shipping any other feature.

## Decision Outcome

Chosen: **authenticate every application route with one deployment-scoped bearer token while leaving liveness and readiness public**.

The production `serve` command refuses to start without `PLANTY_API_TOKEN`.
The server compares the presented bearer token in constant time and returns the same unauthorized response for a missing, malformed, or incorrect credential.
`/healthz` and `/readyz` disclose only component readiness and remain unauthenticated so Kubernetes probes do not carry application credentials.
The browser-origin guard remains defense in depth after authentication.

The token is a shared deployment credential rather than a user identity.
Audit records therefore continue to require an explicit source and actor supplied through the constrained application operation rather than pretending the bearer token identifies a person.

### Consequences

Good:

- LAN reachability is no longer equivalent to permission to read or mutate the garden.
- Physical actuator routes inherit the same authentication boundary as every other operation.
- Existing iOS and Dusk clients already have token configuration and require no protocol migration.
- Health probes remain simple and do not leak the bearer credential into pod specifications.

Bad, and accepted:

- Enabling enforcement before the live secret and clients agree on a token would cause an outage, so the deployment must update the secret before rolling the new server.
- One shared token cannot revoke one client independently or attribute actions cryptographically.
- A future multi-user or internet-facing deployment will need a new ADR and stronger identity model.

