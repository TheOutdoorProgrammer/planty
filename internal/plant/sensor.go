package plant

import (
	"fmt"
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

// SensorLink ties a Home Assistant entity to a plant, or to a zone when PlantID
// is nil. Calibration lives here because it belongs to a probe in a pot.
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

// Calibrated reports whether this link may drive an automated decision.
func (s SensorLink) Calibrated() bool {
	return s.DryBaseline != nil && s.WetBaseline != nil && *s.WetBaseline > *s.DryBaseline
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
		return fmt.Errorf("unknown sensor role %q", s.Role)
	}
	if s.HAEntityID == "" {
		return fmt.Errorf("ha_entity_id is required")
	}
	if s.PlantID == nil && s.Zone == "" {
		return fmt.Errorf("a sensor link needs either a plant or a zone")
	}
	if s.Role == RoleSoilMoisture && s.PlantID == nil {
		return fmt.Errorf("a soil moisture sensor must belong to a plant")
	}
	return nil
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
