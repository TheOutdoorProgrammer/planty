# OPA decision policies

Planty embeds Open Policy Agent so an owner can turn current plant evidence into deterministic care decisions. Policies are Rego v1 modules edited in the iOS app or through the API. Planty owns the input, validates the output, records every production evaluation, and keeps physical safety outside Rego.

## Contract

Every module uses this package and entrypoint:

```rego
package planty

decision := { ... }
```

Planty evaluates `data.planty.decision`. It must return exactly one object matching the output contract below. Unknown fields, invalid enum values, unbounded health changes, and unbounded fan runs fail closed.

Policy source is limited to 64 KiB and evaluation to 250 ms. A decision is limited to 256 KiB, each array to 100 items, and each text value to 4 KiB.

The input version is `planty.policy.input/v1`. A breaking input change gets a new version rather than silently changing a field under existing rules.

## Inputs

All timestamps are RFC 3339. Optional values are undefined in Rego when Planty does not know them. Rules must check optional paths before comparing them.

| Path | Type | Meaning |
| --- | --- | --- |
| `input.version` | string | Always `planty.policy.input/v1`. |
| `input.context.trigger` | `preview`, `manual`, `daily`, or `agent` | Why the rule is running. |
| `input.context.now` | timestamp | Server time captured once for the evaluation. |
| `input.plant.id` | UUID | Stable plant ID. |
| `input.plant.slug` | string | Human-readable API identity. |
| `input.plant.common_name` | string | Display name. |
| `input.plant.botanical_name` | string, optional | Recorded species. |
| `input.plant.domain` | `houseplant`, `edible_indoor`, or `edible_outdoor` | Care domain. |
| `input.plant.status` | `alive`, `struggling`, `dormant`, `dead`, or `gone` | Lifecycle state. |
| `input.plant.location` | string | Recorded location. |
| `input.plant.age_days` | integer, optional | Whole days since acquisition. |
| `input.plant.acquired_at` | timestamp, optional | Acquisition date. |
| `input.plant.is_sick` | boolean | True when status is struggling or a symptom was recorded in the last seven days. |
| `input.plant.min_temp_f` | number, optional | Configured minimum safe temperature. |
| `input.plant.watering_method` | `hand` or `letpot` | How watering is performed. This does not grant policy watering authority. |
| `input.plant.wants_light` | string, optional | Desired light from the care profile. |
| `input.plant.humidity_pref` | string, optional | Recorded humidity preference. |
| `input.plant.frost_sensitive` | boolean, optional | Recorded frost sensitivity. |
| `input.plant.risk` | integer | Planty's neglect risk score. |

Care event paths are `last_watered`, `last_misted`, `last_airflow`, `last_fertilized`, `last_moved`, and `latest_symptom` under `input.care`. Each is optional and, when present, contains:

| Field | Type | Meaning |
| --- | --- | --- |
| `at` | timestamp | When it happened. |
| `hours_ago` | number | Elapsed hours, never negative. |
| `recent_24h` | boolean | Convenience flag for a common safety check. |
| `body` | string, optional | Recorded note. |
| `record_id` | UUID | Durable evidence record. |

Health fields:

| Path | Type | Meaning |
| --- | --- | --- |
| `input.health.known` | boolean | Whether an absolute health baseline exists. |
| `input.health.score` | 0 through 100, optional | Current append-only health score. |
| `input.health.assessed_at` | timestamp, optional | When the current score was recorded. |
| `input.health.evidence_new` | boolean | A reading or observation is newer than the current score. |

Sensor roles are `soil_moisture`, `ambient_temp`, `ambient_humidity`, and `illuminance` under `input.sensors`. Each optional sensor contains:

| Field | Type | Meaning |
| --- | --- | --- |
| `reading_id` | UUID | Durable reading record. |
| `raw` | number | Home Assistant value. |
| `fraction` | 0 through 1, optional | Probe-relative value between its own dry and wet calibration points. |
| `calibrated` | boolean | Whether the fraction is safe to use for a decision. |
| `taken_at` | timestamp | Reading time. |
| `age_minutes` | number | Reading age. |
| `stale` | boolean | Older than Planty's 36-hour evidence freshness boundary. |

Weather is optional because Home Assistant can be unavailable without blocking plant care:

| Path | Type | Meaning |
| --- | --- | --- |
| `input.weather.current_temp_f` | number, optional | Current weather entity temperature. |
| `input.weather.forecast_low_f` | number, optional | Lowest forecast in the next 36 hours. |
| `input.weather.forecast_high_f` | number, optional | Highest forecast in the next 36 hours. |
| `input.weather.frost_risk` | boolean | Forecast low is 32F or lower. |

`input.verdict` is optional and contains the latest model `action`, `reasoning`, `confidence`, and `created_at`.

`input.actuators` contains only devices assigned to the current plant:

| Field | Type | Meaning |
| --- | --- | --- |
| `id` | UUID | Planty actuator ID, which policy output must use. |
| `name` | string | Display name. |
| `kind` | `fan` or `switch` | Home Assistant domain. Only fans may be policy-controlled. |
| `entity_id` | string | Exact Home Assistant entity. |
| `policy_control_enabled` | boolean | Separate owner opt-in for enforcing policies. |
| `active_until` | timestamp, optional | Deadline of the active durable lease. |

## Outputs

Every decision requires these fields, even when their arrays are empty:

```json
{
  "summary": "No policy action needed.",
  "signals": [],
  "notifications": [],
  "fan_runs": [],
  "agent": {
    "facts": [],
    "guidance": [],
    "deny_actions": []
  }
}
```

Signals have `kind`, `active`, `severity`, `reason`, and optional `confidence` from 0 through 1. Supported kinds are:

- `needs_watered`
- `needs_misted`
- `move_inside`
- `move_outside`
- `incident`
- `health`
- `airflow`

Severity is `info`, `warning`, or `critical`. A policy incident is a policy signal. It does not impersonate Planty's separately correlated garden incident records.

Optional output directives:

- `health`: `{"delta": -5, "reason": "New symptom with fresh evidence."}`. Delta must be non-zero and between -20 and 20. Enforcement requires an existing score and newer record-backed evidence.
- `notifications`: each item has `title`, `body`, and `priority`.
- `fan_runs`: each item has an assigned Planty `actuator_id`, `duration_seconds` from 1 through 3600, and `reason`.
- `agent`: `facts` and `guidance` are added to agent context. `deny_actions` is presented to agents as an owner constraint only for enforce-mode decisions. Policy context can narrow authority but never grant a tool.

## Example rules

```rego
package planty

default decision := {
  "summary": "No policy action needed.",
  "signals": [],
  "notifications": [],
  "fan_runs": [],
  "agent": {"facts": [], "guidance": [], "deny_actions": []},
}

decision := {
  "summary": sprintf("%s looks dry", [input.plant.common_name]),
  "signals": [{
    "kind": "needs_watered",
    "active": true,
    "severity": "warning",
    "reason": "Calibrated soil moisture is below 25%.",
    "confidence": 0.95,
  }],
  "notifications": [{
    "title": "Plant needs water",
    "body": sprintf("Check %s before watering by hand.", [input.plant.common_name]),
    "priority": "warning",
  }],
  "fan_runs": [],
  "agent": {
    "facts": ["The calibrated soil probe reads below 25%."],
    "guidance": ["Ask the owner to verify the soil before watering."],
    "deny_actions": ["water"],
  },
} if {
  input.sensors.soil_moisture.calibrated
  input.sensors.soil_moisture.fraction < 0.25
  not input.care.last_watered.recent_24h
}
```

Weather and fan rule fragments:

```rego
move_inside if {
  input.plant.frost_sensitive
  input.weather.forecast_low_f < input.plant.min_temp_f + 3
}

needs_airflow if {
  input.sensors.ambient_humidity.raw > 75
  some fan in input.actuators
  fan.kind == "fan"
  fan.policy_control_enabled
}
```

Use these conditions to construct the one `decision` object. Multiple complete `decision` rules must be mutually exclusive or OPA will report a conflict.

## Modes and safety

- Preview uses current production facts but never persists or acts.
- Advisory records decisions for the app and agents, but performs no directives.
- Enforce may send notifications, write a bounded health delta from fresh durable evidence, and run an explicitly opted-in assigned fan through the existing durable lease path.
- No policy can start the shared watering line, mist a plant, move it, create a recurring schedule, call arbitrary Home Assistant services, or invoke a shell command.
- `http.send`, DNS lookup, clock, random, UUID, and OPA runtime built-ins are blocked. Inputs therefore determine outputs and can be replayed.
- A compile, evaluation, output validation, or enforcement error fails closed and is recorded. Planty's built-in watering verification, actuator reconciliation, cold watch, and other safety jobs continue independently.

## API

- `GET /v1/policies/reference` returns the same reference rendered by the iOS app.
- `GET|POST /v1/policies` lists or creates policies.
- `GET|PUT|DELETE /v1/policies/{id}` reads, versions, or archives a policy.
- `POST /v1/policies/preview` compiles unsaved source and evaluates it against one plant without side effects.
- `POST /v1/policies/{id}/evaluate` records a manual evaluation and enforces it when configured to do so.
- `GET /v1/policy-evaluations` returns replayable evaluation history.

Production records include the policy ID, version, mode, source fingerprint, input fingerprint, idempotency key, complete input, typed decision, duration, outcome, enforcement results, and timestamp. Daily evaluations use the UTC date as their idempotency key, so a retry cannot repeat same-day effects. Manual and agent evaluations use the complete input fingerprint. OpenTelemetry spans record IDs, mode, trigger, duration, outcome, and errors without recording Rego source or plant notes.
