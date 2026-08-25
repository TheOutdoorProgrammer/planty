# Identify push registrations by installation

## Context and Problem Statement

APNs device tokens can change after reinstall, signing-environment changes, and Apple-directed refreshes.
Planty previously keyed registrations only by environment and token, so a refresh created another row and the app could not say which registration belonged to its current install.
The Settings connection test also exercised only `/healthz`, which says nothing about notification permission, APNs registration, token upload, or APNs acceptance.

## Considered Options

- Keep tokens as identities and rely on APNs errors to prune stale rows eventually.
- Persist one global device identifier outside the app so reinstall reuses it.
- Give each app installation a UUID, scope it by APNs environment on the server, and replace that installation's token on refresh.

## Decision Outcome

Chosen: **give each app installation a UUID and scope its registration by APNs environment**.

The installation UUID lives in app defaults, so uninstalling creates a genuinely new installation.
The server uniquely identifies a registration by environment and installation UUID while retaining the environment-and-token uniqueness APNs requires.
A token refresh replaces the current installation row instead of accumulating another active token.
A service URL change uploads the current token to the new Planty service because registrations are service-local state.

Registration returns the server acceptance timestamp.
The app can recover that timestamp from a dedicated push-health route, and a separate test route sends through APNs to only that installation.
HTTP service health is never presented as a notification test.

### Consequences

Good:

- Settings can name permission, APNs registration, upload, environment, and server acceptance independently.
- Token refresh is deterministic and does not wait for a later broadcast to discover the stale token.
- Reinstall, environment change, and service change have explicit, testable behavior.
- A test notification proves APNs accepted a request for this installation rather than merely proving Planty answered HTTP.

Bad, and accepted:

- An uninstall leaves the old installation row until APNs rejects its token during a later send.
- Sandbox and production registrations for the same physical phone remain separate rows because APNs tokens are not interchangeable.
- The app and server now share an installation identifier in addition to the APNs token.
