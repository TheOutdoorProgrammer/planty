package plant

import "time"

// EvidenceCoverage says what Planty can actually support for one plant and
// names the single input that would reduce the most consequential uncertainty.
type EvidenceCoverage struct {
	Plant       Plant      `json:"plant"`
	PhotoCount  int        `json:"photo_count"`
	LatestPhoto *time.Time `json:"latest_photo_at,omitempty"`

	SensorCount       int  `json:"sensor_count"`
	HasSoilSensor     bool `json:"has_soil_sensor"`
	SoilCalibrated    bool `json:"soil_calibrated"`
	BotanicalKnown    bool `json:"botanical_known"`
	ToxicityChecked   bool `json:"toxicity_checked"`
	HealthEstablished bool `json:"health_established"`

	NextBestInput string `json:"next_best_input,omitempty"`
	Why           string `json:"why,omitempty"`
}

// Complete reports that Planty's core identity, safety, visual, and health
// claims all have supporting records. Sensors remain optional by design.
func (c EvidenceCoverage) Complete() bool {
	return c.BotanicalKnown && c.ToxicityChecked && c.PhotoCount > 0 && c.HealthEstablished
}

// Prioritize chooses one useful next input instead of presenting a guilt list.
func (c *EvidenceCoverage) Prioritize(now time.Time) {
	switch {
	case !c.BotanicalKnown:
		c.NextBestInput = "Confirm the botanical identity"
		c.Why = "Toxicity and species-specific care are unsafe to infer from a common name."
	case !c.ToxicityChecked:
		c.NextBestInput = "Verify toxicity from a named source"
		c.Why = "Unknown is not the same thing as safe for people or pets."
	case c.PhotoCount == 0:
		c.NextBestInput = "Take a whole-plant baseline photo"
		c.Why = "A later visual change needs something honest to compare against."
	case c.LatestPhoto != nil && now.Sub(*c.LatestPhoto) > 30*24*time.Hour:
		c.NextBestInput = "Take a current whole-plant photo"
		c.Why = "The latest visual evidence is more than 30 days old."
	case c.HasSoilSensor && !c.SoilCalibrated:
		c.NextBestInput = "Calibrate the soil sensor"
		c.Why = "An uncalibrated probe cannot support a watering decision."
	case !c.HealthEstablished:
		c.NextBestInput = "Establish health from current evidence"
		c.Why = "Planty keeps health unknown until an evidence-backed baseline exists."
	}
}
