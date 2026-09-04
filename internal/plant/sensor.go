package plant

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SensorRole is what a linked Home Assistant entity measures.
type SensorRole string

const (
	RoleSoilMoisture    SensorRole = "soil_moisture"
	RoleAmbientTemp     SensorRole = "ambient_temp"
	RoleAmbientHumidity SensorRole = "ambient_humidity"
	RoleIlluminance     SensorRole = "illuminance"
)

// RequiresCalibration reports whether a role needs probe-relative baselines.
func (r SensorRole) RequiresCalibration() bool {
	return r == RoleSoilMoisture
}

// SensorLink ties a Home Assistant entity to a plant, or to a zone when PlantID
// is nil. Soil calibration follows the probe when its plant changes.
type SensorLink struct {
	ID      uuid.UUID  `json:"id"`
	PlantID *uuid.UUID `json:"plant_id,omitempty"`
	Zone    string     `json:"zone,omitempty"`

	HAEntityID string     `json:"ha_entity_id"`
	Role       SensorRole `json:"role"`

	DryBaseline  *float64   `json:"dry_baseline,omitempty"`
	WetBaseline  *float64   `json:"wet_baseline,omitempty"`
	CalibratedAt *time.Time `json:"calibrated_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

type SensorAssignment struct {
	PlantID *uuid.UUID `json:"plant_id,omitempty"`
	Zone    string     `json:"zone,omitempty"`
}

func (a SensorAssignment) Valid(role SensorRole) error {
	hasPlant := a.PlantID != nil
	hasZone := strings.TrimSpace(a.Zone) != ""
	if hasPlant == hasZone {
		return invalid("a sensor assignment needs exactly one plant or place")
	}
	if role == RoleSoilMoisture && !hasPlant {
		return invalid("a soil moisture sensor must belong to a plant")
	}
	return nil
}

// Calibrated reports whether a soil link has usable dry and wet baselines.
func (s SensorLink) Calibrated() bool {
	return s.Role.RequiresCalibration() && s.DryBaseline != nil && s.WetBaseline != nil &&
		*s.WetBaseline > *s.DryBaseline
}

// Fraction maps a raw reading onto 0..1 between this probe's own baselines.
// Absolute values from two probes are never comparable, so all thresholds are
// expressed against the probe that produced them.
func (s SensorLink) Fraction(raw float64) (float64, error) {
	if !s.Calibrated() {
		return 0, fmt.Errorf("sensor %s is not calibrated", s.HAEntityID)
	}
	f := (raw - *s.DryBaseline) / (*s.WetBaseline - *s.DryBaseline)
	return min(max(f, 0), 1), nil
}

// Valid reports whether this link can be stored.
func (s SensorLink) Valid() error {
	switch s.Role {
	case RoleSoilMoisture, RoleAmbientTemp, RoleAmbientHumidity, RoleIlluminance:
	default:
		return invalid("unknown sensor role %q", s.Role)
	}
	if s.HAEntityID == "" {
		return invalid("ha_entity_id is required")
	}
	return (SensorAssignment{PlantID: s.PlantID, Zone: s.Zone}).Valid(s.Role)
}

// Reading is one sample. It keys on the link rather than the plant, because the
// plant a probe serves changes whenever the probe is moved.
type Reading struct {
	ID           uuid.UUID `json:"id"`
	SensorLinkID uuid.UUID `json:"sensor_link_id"`
	Value        float64   `json:"value"`
	Unit         string    `json:"unit,omitempty"`
	TakenAt      time.Time `json:"taken_at"`
}

type CalibrationProposalStatus string

const (
	CalibrationPending  CalibrationProposalStatus = "pending"
	CalibrationApproved CalibrationProposalStatus = "approved"
	CalibrationDenied   CalibrationProposalStatus = "denied"
)

type CalibrationProposal struct {
	ID               uuid.UUID                 `json:"id"`
	SensorLinkID     uuid.UUID                 `json:"sensor_link_id"`
	PlantID          uuid.UUID                 `json:"plant_id"`
	ReadingID        uuid.UUID                 `json:"reading_id"`
	ActualValue      float64                   `json:"actual_value"`
	Unit             string                    `json:"unit,omitempty"`
	CurrentDry       float64                   `json:"current_dry"`
	CurrentWet       float64                   `json:"current_wet"`
	ProposedDry      float64                   `json:"proposed_dry"`
	ProposedWet      float64                   `json:"proposed_wet"`
	CurrentRelative  float64                   `json:"current_relative"`
	ProposedRelative float64                   `json:"proposed_relative"`
	Reason           string                    `json:"reason"`
	ModelVersion     string                    `json:"model_version,omitempty"`
	Status           CalibrationProposalStatus `json:"status"`
	CreatedAt        time.Time                 `json:"created_at"`
	ResolvedAt       *time.Time                `json:"resolved_at,omitempty"`
	ResolvedBy       string                    `json:"resolved_by,omitempty"`
}
