// Package policy evaluates owner-authored Rego against Planty's typed facts.
package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	InputVersion         = "planty.policy.input/v1"
	Entrypoint           = "data.planty.decision"
	MaxSourceBytes       = 64 << 10
	MaxDecisionBytes     = 256 << 10
	MaxDecisionItems     = 100
	MaxDecisionTextBytes = 4 << 10
)

type Mode string

const (
	ModeAdvisory Mode = "advisory"
	ModeEnforce  Mode = "enforce"
)

type Trigger string

const (
	TriggerPreview Trigger = "preview"
	TriggerManual  Trigger = "manual"
	TriggerDaily   Trigger = "daily"
	TriggerAgent   Trigger = "agent"
)

type Policy struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Source      string     `json:"source"`
	Mode        Mode       `json:"mode"`
	Enabled     bool       `json:"enabled"`
	Version     int        `json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
}

func (p Policy) Valid() error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("policy name is required")
	}
	if len(p.Source) == 0 {
		return fmt.Errorf("policy source is required")
	}
	if len(p.Source) > MaxSourceBytes {
		return fmt.Errorf("policy source exceeds %d bytes", MaxSourceBytes)
	}
	switch p.Mode {
	case ModeAdvisory, ModeEnforce:
		return nil
	default:
		return fmt.Errorf("unknown policy mode %q", p.Mode)
	}
}

func (p Policy) Fingerprint() string {
	digest := sha256.Sum256([]byte(p.Source))
	return hex.EncodeToString(digest[:])
}

func FingerprintInput(input Input) (string, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func IdempotencyKey(input Input, fingerprint string) string {
	if input.Context.Trigger == TriggerDaily {
		return input.Context.Now.UTC().Format(time.DateOnly)
	}
	return fingerprint
}

type Input struct {
	Version   string          `json:"version"`
	Context   Context         `json:"context"`
	Plant     PlantFacts      `json:"plant"`
	Care      CareFacts       `json:"care"`
	Health    HealthFacts     `json:"health"`
	Sensors   SensorFacts     `json:"sensors"`
	Weather   *WeatherFacts   `json:"weather,omitempty"`
	Verdict   *VerdictFacts   `json:"verdict,omitempty"`
	Actuators []ActuatorFacts `json:"actuators"`
}

type Context struct {
	Trigger Trigger   `json:"trigger"`
	Now     time.Time `json:"now"`
}

type PlantFacts struct {
	ID             uuid.UUID  `json:"id"`
	Slug           string     `json:"slug"`
	CommonName     string     `json:"common_name"`
	BotanicalName  string     `json:"botanical_name,omitempty"`
	Domain         string     `json:"domain"`
	Status         string     `json:"status"`
	Location       string     `json:"location"`
	AgeDays        *int       `json:"age_days,omitempty"`
	AcquiredAt     *time.Time `json:"acquired_at,omitempty"`
	IsSick         bool       `json:"is_sick"`
	MinTempF       *float64   `json:"min_temp_f,omitempty"`
	WateringMethod string     `json:"watering_method"`
	WantsLight     string     `json:"wants_light,omitempty"`
	HumidityPref   string     `json:"humidity_pref,omitempty"`
	FrostSensitive *bool      `json:"frost_sensitive,omitempty"`
	Risk           int        `json:"risk"`
}

type EventFact struct {
	At        time.Time `json:"at"`
	HoursAgo  float64   `json:"hours_ago"`
	Recent24H bool      `json:"recent_24h"`
	Body      string    `json:"body,omitempty"`
	RecordID  uuid.UUID `json:"record_id"`
}

type CareFacts struct {
	LastWatered    *EventFact `json:"last_watered,omitempty"`
	LastMisted     *EventFact `json:"last_misted,omitempty"`
	LastAirflow    *EventFact `json:"last_airflow,omitempty"`
	LastFertilized *EventFact `json:"last_fertilized,omitempty"`
	LastMoved      *EventFact `json:"last_moved,omitempty"`
	LatestSymptom  *EventFact `json:"latest_symptom,omitempty"`
}

type HealthFacts struct {
	Known       bool       `json:"known"`
	Score       *float64   `json:"score,omitempty"`
	AssessedAt  *time.Time `json:"assessed_at,omitempty"`
	EvidenceNew bool       `json:"evidence_new"`
}

type SensorFacts struct {
	SoilMoisture    *SensorFact `json:"soil_moisture,omitempty"`
	AmbientTemp     *SensorFact `json:"ambient_temp,omitempty"`
	AmbientHumidity *SensorFact `json:"ambient_humidity,omitempty"`
	Illuminance     *SensorFact `json:"illuminance,omitempty"`
}

type SensorFact struct {
	ReadingID  uuid.UUID `json:"reading_id"`
	Raw        float64   `json:"raw"`
	Fraction   *float64  `json:"fraction,omitempty"`
	Calibrated bool      `json:"calibrated"`
	TakenAt    time.Time `json:"taken_at"`
	AgeMinutes float64   `json:"age_minutes"`
	Stale      bool      `json:"stale"`
}

type WeatherFacts struct {
	CurrentTempF  *float64 `json:"current_temp_f,omitempty"`
	ForecastLowF  *float64 `json:"forecast_low_f,omitempty"`
	ForecastHighF *float64 `json:"forecast_high_f,omitempty"`
	FrostRisk     bool     `json:"frost_risk"`
}

type VerdictFacts struct {
	Action     string    `json:"action"`
	Reasoning  string    `json:"reasoning"`
	Confidence float64   `json:"confidence"`
	CreatedAt  time.Time `json:"created_at"`
}

type ActuatorFacts struct {
	ID                   uuid.UUID  `json:"id"`
	Name                 string     `json:"name"`
	Kind                 string     `json:"kind"`
	EntityID             string     `json:"entity_id"`
	PolicyControlEnabled bool       `json:"policy_control_enabled"`
	ActiveUntil          *time.Time `json:"active_until,omitempty"`
}

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

type SignalKind string

const (
	SignalNeedsWatered SignalKind = "needs_watered"
	SignalNeedsMisted  SignalKind = "needs_misted"
	SignalMoveInside   SignalKind = "move_inside"
	SignalMoveOutside  SignalKind = "move_outside"
	SignalIncident     SignalKind = "incident"
	SignalHealth       SignalKind = "health"
	SignalAirflow      SignalKind = "airflow"
)

type Decision struct {
	Summary       string            `json:"summary"`
	Signals       []Signal          `json:"signals"`
	Health        *HealthAdjustment `json:"health,omitempty"`
	Notifications []Notification    `json:"notifications"`
	FanRuns       []FanRun          `json:"fan_runs"`
	Agent         AgentGuidance     `json:"agent"`
}

type Signal struct {
	Kind       SignalKind `json:"kind"`
	Active     bool       `json:"active"`
	Severity   Severity   `json:"severity"`
	Reason     string     `json:"reason"`
	Confidence float64    `json:"confidence,omitempty"`
}

type HealthAdjustment struct {
	Delta  float64 `json:"delta"`
	Reason string  `json:"reason"`
}

type Notification struct {
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Priority Severity `json:"priority"`
}

type FanRun struct {
	ActuatorID      uuid.UUID `json:"actuator_id"`
	DurationSeconds int       `json:"duration_seconds"`
	Reason          string    `json:"reason"`
}

type AgentGuidance struct {
	Facts       []string `json:"facts"`
	Guidance    []string `json:"guidance"`
	DenyActions []string `json:"deny_actions"`
}

type Evaluation struct {
	ID                uuid.UUID `json:"id"`
	PolicyID          uuid.UUID `json:"policy_id"`
	PolicyVersion     int       `json:"policy_version"`
	PolicyMode        Mode      `json:"policy_mode"`
	PlantID           uuid.UUID `json:"plant_id"`
	Trigger           Trigger   `json:"trigger"`
	InputFingerprint  string    `json:"input_fingerprint"`
	IdempotencyKey    string    `json:"idempotency_key"`
	PolicyFingerprint string    `json:"policy_fingerprint"`
	Input             Input     `json:"input"`
	Decision          Decision  `json:"decision"`
	DurationMS        float64   `json:"duration_ms"`
	Outcome           string    `json:"outcome"`
	Error             string    `json:"error,omitempty"`
	Enforced          []string  `json:"enforced"`
	CreatedAt         time.Time `json:"created_at"`
}
