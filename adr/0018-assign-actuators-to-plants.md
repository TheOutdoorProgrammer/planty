# Assign every plant actuator to the plants it serves

## Context and problem statement

The durable actuator allowlist proves that a Home Assistant entity was deliberately selected, but an actuator UUID alone does not say which plants it serves.
An app or agent could therefore run the wrong fan while still satisfying every lease safety rule.
One fan may also serve several plants in the same cabinet or room, so a single `plant_id` column would encode a false one-to-one relationship.

## Considered options

- Keep actuators garden-global and rely on their display names.
- Attach one plant to each actuator.
- Require an explicit many-to-many assignment between actuators and living plants.

## Decision outcome

Every registered actuator must be assigned to at least one living plant through a join table.
The API and clients carry stable Planty plant UUIDs, and an agent acting from a plant ref may only use an actuator assigned to that plant.
Shared fans remain one actuator with several plant assignments rather than several registrations for one Home Assistant entity.

This makes plant-scoped control enforceable and lets incident correlation inspect a real relationship.
The cost is one required selection during registration and migration work for any older unassigned actuator; none exists in production yet.
