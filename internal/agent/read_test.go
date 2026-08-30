package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

func TestDescribeReadingTrustsTemperatureWithoutSoilCalibration(t *testing.T) {
	got := describeReading(
		plant.SensorLink{Role: plant.RoleAmbientTemp},
		plant.Reading{Value: 68.5, Unit: "°F", TakenAt: time.Now()},
	)

	if !strings.Contains(got, "68.5°F") {
		t.Errorf("temperature reading = %q", got)
	}
	if strings.Contains(got, "not calibrated") {
		t.Errorf("temperature was subjected to soil calibration: %q", got)
	}
}

func TestDescribeReadingKeepsUncalibratedSoilUntrusted(t *testing.T) {
	got := describeReading(
		plant.SensorLink{Role: plant.RoleSoilMoisture},
		plant.Reading{Value: 41, Unit: "%", TakenAt: time.Now()},
	)

	if !strings.Contains(got, "not calibrated") {
		t.Errorf("uncalibrated soil reading = %q", got)
	}
}
