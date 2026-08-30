package plant

import "testing"

func TestOnlyTerminalStatusesCanArchiveAPlant(t *testing.T) {
	for _, status := range []Status{StatusDead, StatusGone} {
		if err := status.ValidateArchive(); err != nil {
			t.Errorf("%s should archive: %v", status, err)
		}
	}
	for _, status := range []Status{StatusAlive, StatusStruggling, StatusDormant, "invented"} {
		if err := status.ValidateArchive(); err == nil {
			t.Errorf("%s archived a contradictory record", status)
		}
	}
}

func TestRiskRanksNeglectNotThirst(t *testing.T) {
	forgettable := Plant{
		WateringMethod: WateringHand,
		Accessibility:  AccessHard,
		Steward:        "Maya",
		Status:         StatusAlive,
	}
	safe := Plant{
		WateringMethod: WateringLetPot,
		Accessibility:  AccessEasy,
		Steward:        StewardSelf,
		Status:         StatusAlive,
	}

	if forgettable.Risk() <= safe.Risk() {
		t.Fatalf("hand-watered, hard to reach, someone else's should outrank an easy own plant: %d vs %d",
			forgettable.Risk(), safe.Risk())
	}
}

func TestFriendsPlantOutranksOwnWhenOtherwiseIdentical(t *testing.T) {
	base := Plant{WateringMethod: WateringHand, Accessibility: AccessEasy, Status: StatusAlive}

	mine := base
	mine.Steward = StewardSelf
	theirs := base
	theirs.Steward = "Maya"

	if theirs.Risk() <= mine.Risk() {
		t.Fatal("a friend's plant should carry more risk than an identical own plant")
	}
}

func TestIsFriends(t *testing.T) {
	for _, tc := range []struct {
		steward string
		want    bool
	}{
		{StewardSelf, false},
		{"", false},
		{"Maya", true},
	} {
		if got := (Plant{Steward: tc.steward}).IsFriends(); got != tc.want {
			t.Errorf("steward %q: got %v want %v", tc.steward, got, tc.want)
		}
	}
}

func TestValidRejectsDripperOnHandWateredPlant(t *testing.T) {
	dripper := 3
	p := Plant{
		CommonName:     "Peace lily",
		Domain:         DomainHouseplant,
		Status:         StatusAlive,
		Accessibility:  AccessEasy,
		WateringMethod: WateringHand,
		LetPotDripper:  &dripper,
	}
	if err := p.Valid(); err == nil {
		t.Fatal("a hand-watered plant must not carry a dripper number")
	}
}

func TestValidAcceptsAMinimalPlant(t *testing.T) {
	p := Plant{
		CommonName:     "Bonsai",
		Domain:         DomainHouseplant,
		Status:         StatusAlive,
		Accessibility:  AccessEasy,
		WateringMethod: WateringHand,
	}
	if err := p.Valid(); err != nil {
		t.Fatalf("minimal plant should validate: %v", err)
	}
}

func TestValidRejectsValuesThatCannotRepresentAPlant(t *testing.T) {
	valid := Plant{
		CommonName:     "Bonsai",
		Domain:         DomainHouseplant,
		Status:         StatusAlive,
		Accessibility:  AccessEasy,
		WateringMethod: WateringHand,
	}

	blankName := valid
	blankName.CommonName = "   "
	invalidLight := valid
	invalidLight.LightExposure = "sun-shaped"

	for name, subject := range map[string]Plant{
		"blank name":             blankName,
		"unknown light exposure": invalidLight,
	} {
		t.Run(name, func(t *testing.T) {
			if err := subject.Valid(); err == nil {
				t.Fatal("invalid plant was accepted")
			}
		})
	}
}

func TestFractionNeedsCalibration(t *testing.T) {
	uncalibrated := SensorLink{HAEntityID: "sensor.a", Role: RoleSoilMoisture}
	if _, err := uncalibrated.Fraction(40); err == nil {
		t.Fatal("an uncalibrated probe must refuse to produce a fraction")
	}
}

func TestFractionIsRelativeToItsOwnBaselines(t *testing.T) {
	dry, wet := 20.0, 60.0
	link := SensorLink{Role: RoleSoilMoisture, DryBaseline: &dry, WetBaseline: &wet}

	for _, tc := range []struct{ raw, want float64 }{
		{20, 0},
		{40, 0.5},
		{60, 1},
		{5, 0},  // clamped, not negative
		{99, 1}, // clamped, not above one
	} {
		got, err := link.Fraction(tc.raw)
		if err != nil {
			t.Fatalf("raw %v: %v", tc.raw, err)
		}
		if got != tc.want {
			t.Errorf("raw %v: got %v want %v", tc.raw, got, tc.want)
		}
	}
}

func TestCalibratedRejectsInvertedBaselines(t *testing.T) {
	dry, wet := 60.0, 20.0
	if (SensorLink{Role: RoleSoilMoisture, DryBaseline: &dry, WetBaseline: &wet}).Calibrated() {
		t.Fatal("wet must exceed dry for a probe to count as calibrated")
	}
}

func TestOnlySoilMoistureRequiresCalibration(t *testing.T) {
	if !RoleSoilMoisture.RequiresCalibration() {
		t.Fatal("soil moisture must require probe-relative calibration")
	}
	for _, role := range []SensorRole{RoleAmbientTemp, RoleAmbientHumidity, RoleIlluminance} {
		if role.RequiresCalibration() {
			t.Errorf("%s unexpectedly requires calibration", role)
		}
	}
}

func TestSoilSensorMustBelongToAPlant(t *testing.T) {
	orphan := SensorLink{HAEntityID: "sensor.soil", Role: RoleSoilMoisture, Zone: "greenhouse"}
	if err := orphan.Valid(); err == nil {
		t.Fatal("a soil probe measures one pot, so it must name a plant")
	}
}

func TestAllClearDistinguishesCalmFromStale(t *testing.T) {
	calm := Digest{Checked: 12, Expected: 12, RunComplete: true}
	if !calm.AllClear() {
		t.Fatal("no entries and a complete fresh run is the calm state")
	}

	stale := Digest{Checked: 12, Expected: 12, RunComplete: true, StaleSince: &calm.Date}
	if stale.AllClear() {
		t.Fatal("stale data must never render as all clear")
	}

	// A fresh install has no verdicts at all, which is not the same as calm.
	never := Digest{Checked: 12, NeverRun: true}
	if never.AllClear() {
		t.Fatal("a judgment that has never run must not render as all clear")
	}
}

func TestVerifiableOnlyCoversWatering(t *testing.T) {
	if !(Observation{Kind: ObservedWatered}).Verifiable() {
		t.Error("watering is the claim a sensor can check")
	}
	if (Observation{Kind: ObservedPruned}).Verifiable() {
		t.Error("pruning cannot be verified by a soil sensor")
	}
}
