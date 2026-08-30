# 29. Treat actuator type as plant behavior, not Home Assistant domain

Date: 2026-08-30

## Status

Accepted.

## Context and Problem Statement

Home Assistant often exposes physical lights, fans, and pumps as switch entities.
Planty currently copies the Home Assistant domain into actuator kind, so a switch-backed light is treated as airflow and cannot use light controls or scheduling.

## Considered Options

1. Keep deriving actuator kind from the Home Assistant entity domain
2. Guess semantic type from the entity name and domain
3. Store an explicit semantic actuator type independently from the entity domain

## Decision Outcome

Chosen: **option 3**.

Planty stores an owner-selected semantic kind for each actuator. The selected kind controls Planty behavior and presentation. The domain parsed from `entity_id` is used only to call Home Assistant's `turn_on` and `turn_off` services. Fan leases and policy control remain fan-only, light state and schedules remain light-only, and water or generic devices gain no control path until their role has explicit safety behavior.

## Consequences

### Good

- Switch-backed lights and fans receive the correct Planty controls
- The UI and OPA input describe what a device does instead of how Home Assistant exposes it
- New actuator roles can define their own safety rules without becoming a freeform Home Assistant proxy

### Bad

- Registration and editing require one additional explicit choice
- Existing switch registrations remain generic until the owner classifies them
- Changing a light to another type must remove its now-invalid light schedule

### Rejected because

- The Home Assistant domain is transport information and cannot distinguish devices behind smart plugs.
- Name guessing is ambiguous and silently recreates the same class of bug.
