package plant

import (
	"time"
)

const WateringDetectionWindow = 3 * time.Hour

type WateringRise struct {
	Baseline     Reading
	Rise         Reading
	Confirmation Reading
}

// DetectWateringRise requires a stable baseline and independently reported,
// sustained improvement. Polling the same cached HA state is not confirmation.
func DetectWateringRise(link SensorLink, readings []Reading, now time.Time) *WateringRise {
	if !link.Calibrated() || !finite(*link.DryBaseline) || !finite(*link.WetBaseline) {
		return nil
	}
	span := *link.WetBaseline - *link.DryBaseline
	if !finite(span) {
		return nil
	}
	samples := make([]Reading, 0, len(readings))
	for _, r := range readings {
		if r.SensorLinkID != link.ID || !finite(r.Value) || r.ReportedAt == nil || r.ReportedAt.IsZero() ||
			r.TakenAt.After(now) || r.ReportedAt.After(r.TakenAt) ||
			r.TakenAt.Sub(*r.ReportedAt) > 45*time.Minute || r.ReportedAt.Before(now.Add(-WateringDetectionWindow)) {
			samples = nil
			continue
		}
		if len(samples) > 0 {
			previous := samples[len(samples)-1]
			if r.ReportedAt.Equal(*previous.ReportedAt) {
				if r.Value != previous.Value || r.Unit != previous.Unit {
					return nil
				}
				continue
			}
			gap := r.ReportedAt.Sub(*previous.ReportedAt)
			if gap < 0 || gap > 45*time.Minute || r.Unit != previous.Unit {
				samples = nil
			}
		}
		samples = append(samples, r)
	}
	if len(samples) < 5 || now.Sub(*samples[len(samples)-1].ReportedAt) > 45*time.Minute {
		return nil
	}
	for n := 3; n+1 < len(samples); n++ {
		start := n - 3
		for start > 0 && samples[n-1].ReportedAt.Sub(*samples[start].ReportedAt) < 30*time.Minute {
			start--
		}
		baseline := samples[start:n]
		if baseline[len(baseline)-1].ReportedAt.Sub(*baseline[0].ReportedAt) < 30*time.Minute {
			continue
		}
		low, high := baseline[0].Value, baseline[0].Value
		for _, r := range baseline[1:] {
			low, high = min(low, r.Value), max(high, r.Value)
		}
		if high-low > span*0.1 {
			continue
		}
		wet := samples[n:]
		if wet[len(wet)-1].ReportedAt.Sub(*wet[0].ReportedAt) < 10*time.Minute {
			continue
		}
		minimum := wet[0].Value
		for _, r := range wet[1:] {
			minimum = min(minimum, r.Value)
		}
		if minimum-high >= span*0.2 {
			return &WateringRise{Baseline: baseline[len(baseline)-1], Rise: wet[0], Confirmation: wet[len(wet)-1]}
		}
	}
	return nil
}
