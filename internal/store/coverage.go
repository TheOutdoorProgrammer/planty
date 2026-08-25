package store

import (
	"context"
	"errors"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// EvidenceCoverage projects existing ledgers without inventing a second copy
// of their state. The garden is deliberately small, so clarity beats one
// brittle query spanning every optional source.
func (s *Store) EvidenceCoverage(ctx context.Context, now time.Time) ([]plant.EvidenceCoverage, error) {
	plants, err := s.ListPlants(ctx, PlantFilter{})
	if err != nil {
		return nil, err
	}
	out := make([]plant.EvidenceCoverage, 0, len(plants))
	for _, subject := range plants {
		coverage := plant.EvidenceCoverage{
			Plant: subject, BotanicalKnown: subject.BotanicalName != "",
			ToxicityChecked: subject.Toxicity.Checked(),
		}
		shots, err := s.Photos(ctx, subject.ID, 100)
		if err != nil {
			return nil, err
		}
		coverage.PhotoCount = len(shots)
		if len(shots) > 0 {
			latest := shots[0].TakenAt
			coverage.LatestPhoto = &latest
		}
		links, err := s.SensorLinks(ctx, &subject.ID)
		if err != nil {
			return nil, err
		}
		coverage.SensorCount = len(links)
		for _, link := range links {
			if link.Role == plant.RoleSoilMoisture {
				coverage.HasSoilSensor = true
				coverage.SoilCalibrated = coverage.SoilCalibrated || link.Calibrated()
			}
		}
		if _, err := s.LatestHealth(ctx, subject.ID); err == nil {
			coverage.HealthEstablished = true
		} else if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		coverage.Prioritize(now)
		out = append(out, coverage)
	}
	return out, nil
}
