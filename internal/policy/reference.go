package policy

type ReferenceSection struct {
	Title  string           `json:"title"`
	Fields []ReferenceField `json:"fields"`
}

type ReferenceField struct {
	Path        string `json:"path"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

type ReferenceDocument struct {
	InputVersion string             `json:"input_version"`
	Entrypoint   string             `json:"entrypoint"`
	Sections     []ReferenceSection `json:"sections"`
	Output       []ReferenceField   `json:"output"`
	Example      string             `json:"example"`
	Safety       []string           `json:"safety"`
}

func Reference() ReferenceDocument {
	return ReferenceDocument{
		InputVersion: InputVersion,
		Entrypoint:   Entrypoint,
		Sections: []ReferenceSection{
			{Title: "Context", Fields: []ReferenceField{
				{Path: "input.version", Type: "planty.policy.input/v1", Description: "Versioned contract identifier."},
				{Path: "input.context.trigger", Type: "preview | manual | daily | agent", Description: "Why this evaluation is running."},
				{Path: "input.context.now", Type: "RFC 3339 timestamp", Description: "The server clock captured once for this evaluation."},
			}},
			{Title: "Plant", Fields: []ReferenceField{
				{Path: "input.plant.id", Type: "UUID", Description: "Stable plant ID."},
				{Path: "input.plant.slug", Type: "string", Description: "Human-readable API identity."},
				{Path: "input.plant.common_name", Type: "string", Description: "Display name."},
				{Path: "input.plant.botanical_name", Type: "string or undefined", Description: "Recorded species."},
				{Path: "input.plant.domain", Type: "houseplant | edible_indoor | edible_outdoor", Description: "Care domain."},
				{Path: "input.plant.status", Type: "alive | struggling | dormant | dead | gone", Description: "Current lifecycle state."},
				{Path: "input.plant.location", Type: "string", Description: "Recorded location."},
				{Path: "input.plant.age_days", Type: "number or undefined", Description: "Whole days since acquired_at. Missing when acquisition is unknown."},
				{Path: "input.plant.acquired_at", Type: "timestamp or undefined", Description: "Recorded acquisition time."},
				{Path: "input.plant.is_sick", Type: "boolean", Description: "True for a struggling plant or when the latest symptom is still recent."},
				{Path: "input.plant.min_temp_f", Type: "number or undefined", Description: "Lowest configured safe temperature."},
				{Path: "input.plant.watering_method", Type: "hand | letpot", Description: "Recorded method. This never grants policy watering authority."},
				{Path: "input.plant.wants_light", Type: "string or undefined", Description: "Desired light from the care profile."},
				{Path: "input.plant.humidity_pref", Type: "string or undefined", Description: "Recorded humidity preference."},
				{Path: "input.plant.frost_sensitive", Type: "boolean or undefined", Description: "Recorded frost sensitivity."},
				{Path: "input.plant.risk", Type: "number", Description: "Planty's neglect risk score."},
			}},
			{Title: "Care history", Fields: []ReferenceField{
				{Path: "input.care.last_watered", Type: "event or undefined", Description: "Last watering with at, hours_ago, recent_24h, body, and record_id."},
				{Path: "input.care.last_misted", Type: "event or undefined", Description: "Last misting event."},
				{Path: "input.care.last_airflow", Type: "event or undefined", Description: "Last recorded fan or airflow event."},
				{Path: "input.care.last_fertilized", Type: "event or undefined", Description: "Last fertilizing event."},
				{Path: "input.care.last_moved", Type: "event or undefined", Description: "Last recorded move."},
				{Path: "input.care.latest_symptom", Type: "event or undefined", Description: "Most recent reported symptom."},
				{Path: "event.at", Type: "RFC 3339 timestamp", Description: "When the recorded care event happened."},
				{Path: "event.hours_ago", Type: "number", Description: "Elapsed hours, clamped at zero."},
				{Path: "event.recent_24h", Type: "boolean", Description: "Convenience flag for common safety checks."},
				{Path: "event.body", Type: "string or undefined", Description: "Recorded note."},
				{Path: "event.record_id", Type: "UUID", Description: "Durable evidence record."},
			}},
			{Title: "Health", Fields: []ReferenceField{
				{Path: "input.health.known", Type: "boolean", Description: "Whether an absolute health baseline exists."},
				{Path: "input.health.score", Type: "0..100 or undefined", Description: "Latest append-only health score."},
				{Path: "input.health.assessed_at", Type: "timestamp or undefined", Description: "When the current score was recorded."},
				{Path: "input.health.evidence_new", Type: "boolean", Description: "Whether a reading or observation is newer than the health score."},
			}},
			{Title: "Sensors", Fields: []ReferenceField{
				{Path: "input.sensors.<role>", Type: "sensor or undefined", Description: "Role is soil_moisture, ambient_temp, ambient_humidity, or illuminance."},
				{Path: "input.sensors.<role>.reading_id", Type: "UUID", Description: "Durable sensor reading record."},
				{Path: "input.sensors.<role>.raw", Type: "number", Description: "Home Assistant value in the sensor's native unit."},
				{Path: "input.sensors.<role>.unit", Type: "string or undefined", Description: "Home Assistant unit for the raw value."},
				{Path: "input.sensors.soil_moisture.fraction", Type: "0..1 or undefined", Description: "Probe-relative moisture. Only present after dry and wet calibration."},
				{Path: "input.sensors.<role>.calibrated", Type: "boolean", Description: "Whether a soil fraction is safe to use. Other roles use raw and unit directly."},
				{Path: "input.sensors.<role>.taken_at", Type: "RFC 3339 timestamp", Description: "Reading time."},
				{Path: "input.sensors.<role>.age_minutes", Type: "number", Description: "Reading age when the input was built."},
				{Path: "input.sensors.<role>.stale", Type: "boolean", Description: "True when the latest reading is older than Planty's freshness window."},
			}},
			{Title: "Weather", Fields: []ReferenceField{
				{Path: "input.weather", Type: "object or undefined", Description: "Absent when Home Assistant weather is unavailable."},
				{Path: "input.weather.current_temp_f", Type: "number or undefined", Description: "Current weather entity temperature."},
				{Path: "input.weather.forecast_low_f", Type: "number or undefined", Description: "Lowest forecast temperature in the next 36 hours when weather was available."},
				{Path: "input.weather.forecast_high_f", Type: "number or undefined", Description: "Highest forecast temperature in the next 36 hours."},
				{Path: "input.weather.frost_risk", Type: "boolean", Description: "True when the forecast indicates freezing conditions."},
			}},
			{Title: "Latest model verdict", Fields: []ReferenceField{
				{Path: "input.verdict", Type: "object or undefined", Description: "Latest model judgment when one exists."},
				{Path: "input.verdict.action", Type: "string", Description: "Latest model action."},
				{Path: "input.verdict.reasoning", Type: "string", Description: "Latest model explanation."},
				{Path: "input.verdict.confidence", Type: "0..1", Description: "Latest model confidence."},
				{Path: "input.verdict.created_at", Type: "RFC 3339 timestamp", Description: "When the verdict was recorded."},
			}},
			{Title: "Assigned actuators", Fields: []ReferenceField{
				{Path: "input.actuators", Type: "array", Description: "Only devices assigned to the current plant."},
				{Path: "input.actuators[_].id", Type: "UUID", Description: "Planty actuator ID used in fan_runs output."},
				{Path: "input.actuators[_].name", Type: "string", Description: "Display name."},
				{Path: "input.actuators[_].kind", Type: "fan | switch", Description: "Home Assistant domain. Only fans may be policy-controlled."},
				{Path: "input.actuators[_].entity_id", Type: "string", Description: "Exact Home Assistant entity."},
				{Path: "input.actuators[_].policy_control_enabled", Type: "boolean", Description: "Owner opt-in required before an enforcing policy may run this fan."},
				{Path: "input.actuators[_].active_until", Type: "timestamp or undefined", Description: "Deadline of the active durable lease."},
			}},
		},
		Output: []ReferenceField{
			{Path: "needs_<thing>", Type: "any JSON value", Description: "Extensible care rules such as needs_water, needs_misted, or needs_staked. Missing means false; a boolean uses its value; every present non-boolean value is active."},
			{Path: "move_inside | move_outside | incident | health", Type: "any JSON value", Description: "Named care rules with the same activity semantics."},
			{Path: "health_adjustment", Type: "object", Description: "A delta from -20 through 20 plus a reason. Enforce mode also requires newer record-backed evidence."},
			{Path: "notification | notifications", Type: "object | array", Description: "Each notification requires title, body, and an info, warning, or critical priority."},
			{Path: "fan_run | fan_runs", Type: "object | array", Description: "Each run requires actuator_id, duration_seconds from 1 through 3600, and reason."},
			{Path: "agent_fact | agent_facts", Type: "string | array of strings", Description: "Owner-authored facts appended to agent context."},
			{Path: "agent_guidance", Type: "string | array of strings", Description: "Owner-authored guidance appended to agent context."},
			{Path: "deny_action | deny_actions", Type: "string | array of strings", Description: "Presented as owner constraints in enforce mode. Cannot revoke or grant tools."},
		},
		Example: ExampleSource,
		Safety: []string{
			"Preview never writes records, sends notifications, or controls devices.",
			"Advisory policies only record rule results.",
			"Watering, misting, and moving plants always require a person.",
			"Fan runs require enforce mode, plant assignment, owner opt-in, and a durable bounded lease.",
			"Unknown top-level rule names outside the needs_<thing> family fail closed.",
			"Source is limited to 64 KiB, evaluation to 250 ms, output to 256 KiB, and each output array to 100 items.",
			"Policy failures are fail-closed and do not block Planty's built-in safety jobs.",
		},
	}
}

const ExampleSource = `package planty.v1

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

deny_action := "water" if needs_water`
