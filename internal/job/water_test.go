package job

import "testing"

// The gap between the two is the whole safety margin: inside it, doing nothing
// is the right answer, and a system with no dead band oscillates.
func TestThresholdsLeaveADeadBand(t *testing.T) {
	if Thirsty >= Soaked {
		t.Fatalf("dry (%v) must sit below wet (%v)", Thirsty, Soaked)
	}
	if Soaked-Thirsty < 0.2 {
		t.Errorf("dead band of %.2f is too narrow to stop oscillation", Soaked-Thirsty)
	}
}

// Overwatering kills more houseplants than drought, so the dry threshold has to
// sit well below halfway; watering at 45% would be watering damp soil.
func TestDryThresholdBiasesAgainstOverwatering(t *testing.T) {
	if Thirsty > 0.35 {
		t.Errorf("dry threshold %.2f waters soil that is not actually dry", Thirsty)
	}
}

func TestSettleWindowOutlastsASensorReportingInterval(t *testing.T) {
	// Zigbee soil sensors commonly report every 20 minutes; checking sooner
	// would call a perfectly good watering a failure.
	if SettleWindow.Minutes() < 30 {
		t.Errorf("settle window %v is shorter than a sensor reporting cycle", SettleWindow)
	}
}

func TestWaterRefusesWithoutAPumpConfigured(t *testing.T) {
	if err := (Water{}).Run(t.Context()); err == nil {
		t.Fatal("running with no pump switch configured must be an error")
	}
}
