# License the project under Apache 2.0

## Context and Problem Statement

Planty was publicly visible without a license, which granted readers no permission to copy, modify, or distribute it.
The repository includes a service, an iOS client, deployment templates, and integration code that may be useful outside this home deployment.
A published license needs to permit reuse while preserving attribution, disclaiming warranties, and making patent rights explicit.

## Considered Options

- Keep the repository source-available without granting reuse rights.
- Use the MIT License for a short permissive grant without an explicit patent license.
- Use Apache License 2.0 for a permissive grant with explicit patent rights and contribution terms.
- Use a reciprocal license that requires derivative works to publish their source.

## Decision Outcome

Chosen: **publish Planty under Apache License 2.0**.

Apache 2.0 keeps commercial and personal reuse straightforward while adding the explicit patent grant and termination language that MIT omits.
Planty does not need copyleft to protect its operational boundary, because secrets and private deployment data are absent from the public repository by design.

### Consequences

Good:

- People can use, modify, and distribute Planty under a standard OSI-approved license.
- Contributors and users receive an explicit patent grant.
- Warranty and liability limits are stated instead of implied.
- Private Flux values, credentials, and Dusk operational knowledge remain outside the licensed repository.

Bad, and accepted:

- Derivative works are not required to publish their source.
- Distributions must preserve the license and notices.
- Apache 2.0 is longer and less immediately readable than MIT.
