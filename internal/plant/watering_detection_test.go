package plant

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDetectWateringRise(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		values []float64
		mutate func(*SensorLink, []Reading)
		want   bool
	}{
		{name: "sustained watering", values: []float64{31, 31.1, 31, 38, 37}, want: true},
		{name: "threshold inclusive", values: []float64{31, 31, 31, 35, 35}, want: true},
		{name: "noise", values: []float64{31, 31.1, 31, 32, 32.1}},
		{name: "single spike", values: []float64{31, 31, 31, 40, 31}},
		{name: "unconfirmed", values: []float64{31, 31, 31, 40}},
		{name: "insertion ramp", values: []float64{1, 10, 20, 30, 40}},
		{name: "unstable baseline", values: []float64{30, 34, 31, 40, 40}},
		{name: "uncalibrated", values: []float64{31, 31, 31, 40, 40}, mutate: func(l *SensorLink, _ []Reading) { l.WetBaseline = nil }},
		{name: "ambient", values: []float64{31, 31, 31, 40, 40}, mutate: func(l *SensorLink, _ []Reading) { l.Role = RoleAmbientHumidity }},
		{name: "nan calibration", values: []float64{31, 31, 31, 40, 40}, mutate: func(l *SensorLink, _ []Reading) { v := math.NaN(); l.WetBaseline = &v }},
		{name: "invalid sample", values: []float64{31, 31, 31, math.Inf(1), 40}},
		{name: "cached confirmation", values: []float64{31, 31, 31, 40, 40}, mutate: func(_ *SensorLink, r []Reading) { r[4].ReportedAt = r[3].ReportedAt }},
		{name: "missing report time", values: []float64{31, 31, 31, 40, 40}, mutate: func(_ *SensorLink, r []Reading) { r[4].ReportedAt = nil }},
		{name: "future report", values: []float64{31, 31, 31, 40, 40}, mutate: func(_ *SensorLink, r []Reading) { at := now.Add(time.Minute); r[4].ReportedAt = &at }},
		{name: "stale reports", values: []float64{31, 31, 31, 40, 40}, mutate: func(_ *SensorLink, r []Reading) {
			for n := range r {
				at := r[n].TakenAt.Add(-time.Hour)
				r[n].ReportedAt = &at
			}
		}},
		{name: "units changed", values: []float64{31, 31, 31, 40, 40}, mutate: func(_ *SensorLink, r []Reading) { r[4].Unit = "different" }},
		{name: "wrong probe", values: []float64{31, 31, 31, 40, 40}, mutate: func(_ *SensorLink, r []Reading) { r[4].SensorLinkID = uuid.New() }},
		{name: "small calibrated range", values: []float64{60, 60.1, 60, 61, 61}, mutate: func(l *SensorLink, _ []Reading) { dry, wet := 60.0, 63.0; l.DryBaseline = &dry; l.WetBaseline = &wet }, want: true},
		{name: "above wet baseline still observable", values: []float64{51, 51, 51, 56, 56}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dry, wet := 30.0, 50.0
			link := SensorLink{ID: uuid.New(), Role: RoleSoilMoisture, DryBaseline: &dry, WetBaseline: &wet}
			var readings []Reading
			for n, value := range tc.values {
				at := now.Add(time.Duration(n-len(tc.values)+1) * 20 * time.Minute)
				readings = append(readings, Reading{ID: uuid.New(), SensorLinkID: link.ID, Value: value, Unit: "%", TakenAt: at, ReportedAt: &at})
			}
			if tc.mutate != nil {
				tc.mutate(&link, readings)
			}
			rise := DetectWateringRise(link, readings, now)
			if (rise != nil) != tc.want {
				t.Fatalf("rise=%+v, want detection=%v", rise, tc.want)
			}
		})
	}
}

func TestDetectWateringWithFrequentIngestion(t *testing.T) {
	now := time.Now().UTC()
	dry, wet := 30.0, 50.0
	link := SensorLink{ID: uuid.New(), Role: RoleSoilMoisture, DryBaseline: &dry, WetBaseline: &wet}
	var readings []Reading
	for minute := -60; minute <= 0; minute += 5 {
		at := now.Add(time.Duration(minute) * time.Minute)
		value := 31.0
		if minute >= -15 {
			value = 40
		}
		readings = append(readings, Reading{SensorLinkID: link.ID, Value: value, TakenAt: at, ReportedAt: &at})
	}
	if rise := DetectWateringRise(link, readings, now); rise == nil || !rise.Rise.ReportedAt.Equal(now.Add(-15*time.Minute)) {
		t.Fatalf("rise=%+v", rise)
	}
}
