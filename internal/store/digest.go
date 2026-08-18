package store

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

func scanDigestEntry(row pgx.Row) (plant.DigestEntry, error) {
	var p plant.Plant
	var v plant.Verdict
	var light *string
	var acquired *time.Time
	var profile, evidence []byte

	err := row.Scan(
		&p.ID, &p.Slug, &p.CommonName, &p.BotanicalName, &p.Variety,
		&p.Domain, &p.Steward, &p.Status,
		&p.Location, &p.HAArea,
		&p.Accessibility, &p.WateringMethod, &p.LetPotDripper,
		&p.PotSizeIn, &p.PotMaterial, &p.HasDrainage, &p.SoilMix,
		&light, &p.MinTempF, &profile,
		&acquired, &p.ArchivedAt, &p.ShelteredAt, &p.CreatedAt, &p.UpdatedAt,
		&v.ID, &v.PlantID, &v.ForDate, &v.Action, &v.Reasoning,
		&v.Confidence, &evidence, &v.CreatedAt, &v.AcknowledgedAt,
		&v.Escalations,
	)
	if err != nil {
		return plant.DigestEntry{}, err
	}
	if light != nil {
		p.LightExposure = plant.LightExposure(*light)
	}
	p.AcquiredAt = acquired
	if len(profile) > 0 {
		if err := json.Unmarshal(profile, &p.CareProfile); err != nil {
			return plant.DigestEntry{}, err
		}
	}
	if len(evidence) > 0 {
		if err := json.Unmarshal(evidence, &v.Evidence); err != nil {
			return plant.DigestEntry{}, err
		}
	}
	return plant.DigestEntry{Plant: p, Verdict: v, Risk: p.Risk()}, nil
}

// actionWeight orders the digest so genuine danger outranks a routine chore.
func actionWeight(a plant.Action) int {
	switch a {
	case plant.ActionUrgent:
		return 4
	case plant.ActionWater:
		return 3
	case plant.ActionHarvest:
		return 2
	case plant.ActionCheck:
		return 1
	default:
		return 0
	}
}

// sortByRisk puts urgency first, then the plants most likely to be forgotten.
func sortByRisk(entries []plant.DigestEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		ai, aj := actionWeight(entries[i].Verdict.Action), actionWeight(entries[j].Verdict.Action)
		if ai != aj {
			return ai > aj
		}
		if entries[i].Risk != entries[j].Risk {
			return entries[i].Risk > entries[j].Risk
		}
		return entries[i].Plant.CommonName < entries[j].Plant.CommonName
	})
}
