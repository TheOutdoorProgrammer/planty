# OPA decision policies

Planty embeds Open Policy Agent so an owner can turn current plant evidence into deterministic care decisions. Policies are Rego v1 modules edited in the iOS app or through the API. Planty owns the input, validates the output, records every production evaluation, and keeps physical safety outside Rego.

## Contract

Every module uses this versioned package:

```rego
package planty.v1

needs_water if { ... }
agent_guidance := "Check the soil by hand." if needs_water
```

Planty evaluates the independent rules materialized under `data.planty.v1`. There is no aggregate decision object and no required default. A missing rule is false. A boolean rule uses its boolean value. Every present non-boolean value, including an empty collection or null, is active. Planty records the original JSON value with the derived active flag.

The v1 care-rule family is `needs_<thing>`, including `needs_water`, `needs_misted`, `needs_fertilized`, `needs_pruned`, `needs_repotted`, `needs_light`, `needs_shade`, and `needs_airflow`. New suffixes work without a Planty release. The named care rules are `move_inside`, `move_outside`, `incident`, and `health`. Unknown top-level names outside the `needs_<thing>` family fail closed. Rules that request side effects have additional typed contracts and fail closed when malformed.

Policy source is limited to 64 KiB and evaluation to 250 ms. Combined output is limited to 256 KiB, each array to 100 items, and each text value to 4 KiB.

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
| `unit` | string, optional | Home Assistant unit for the raw value. |
| `fraction` | 0 through 1, optional | Probe-relative value between its own dry and wet calibration points. |
| `calibrated` | boolean | Whether a soil fraction is safe to use. Raw temperature, humidity, and illuminance do not require calibration. |
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

## Rules and outputs

Every supported top-level value in the package is a rule result. Planty returns results in sorted order as `{"name", "active", "value"}` records. Rules that do not materialize are omitted and therefore false.

```rego
package planty.v1

needs_water if input.sensors.soil_moisture.fraction < 0.25
needs_misted := false
health := {"state": "recovering", "confidence": 0.8}
```

Here `needs_water` and `health` are active, `needs_misted` is present but inactive, and every unmentioned rule is false. `health` remains an ordinary owner-defined rule. A health score mutation uses the separate typed `health_adjustment` rule.

Care rules accept any JSON value and are available to the app and agents. These additional names have built-in behavior when active:

| Rule | Value | Effect in enforce mode |
| --- | --- | --- |
| `health_adjustment` | `{"delta": -5, "reason": "New symptom with fresh evidence."}` | Append a bounded health change. Delta is non-zero and from -20 through 20. An existing score and newer durable evidence are required. |
| `notification` | `{"title", "body", "priority"}` | Send one notification. Priority is `info`, `warning`, or `critical`. |
| `notifications` | array of notification objects | Send every notification. |
| `fan_run` | `{"actuator_id", "duration_seconds", "reason"}` | Run one assigned, opted-in fan for 1 through 3600 seconds through its durable lease. |
| `fan_runs` | array of fan-run objects | Run every validated fan directive. |
| `agent_fact` or `agent_facts` | string or array of strings | Add facts to agent context. |
| `agent_guidance` | string or array of strings | Add owner-authored guidance to agent context. |
| `deny_action` or `deny_actions` | string or array of strings | Present owner constraints to agents for enforce-mode evaluations. This cannot revoke or grant tools. |

Singular and plural side-effect rules may both be present; Planty combines them. A false boolean typed rule has no effect. A present non-boolean typed rule must match its schema even when the policy is advisory, so broken automation cannot quietly earn trust.

## Example rules

```rego
package planty.v1

needs_water if {
  input.sensors.soil_moisture.calibrated
  input.sensors.soil_moisture.fraction < 0.25
  not input.care.last_watered.recent_24h
}

notification := {
  "title": "Plant needs water",
  "body": sprintf("Check %s before watering by hand.", [input.plant.common_name]),
  "priority": "warning",
} if needs_water

agent_fact := "The calibrated soil probe reads below 25%." if needs_water
agent_guidance := "Ask the owner to verify the soil before watering." if needs_water
deny_action := "water" if needs_water
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

These rules compose independently. Adding `move_inside` cannot conflict with `needs_water`, and no empty output envelope is required.

## Modes and safety

- Preview uses current production facts but never persists or acts.
- Advisory records rule results for the app and agents, but performs no directives.
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

Production records include the policy ID, version, mode, source fingerprint, input fingerprint, idempotency key, complete input, original rule values, derived active flags, normalized typed directives, duration, outcome, enforcement results, and timestamp. Daily evaluations use the UTC date as their idempotency key, so a retry cannot repeat same-day effects. Manual and agent evaluations use the complete input fingerprint. OpenTelemetry spans record IDs, mode, trigger, duration, outcome, and errors without recording Rego source or plant notes.
