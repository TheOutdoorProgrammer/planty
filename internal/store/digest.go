package store

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// plantScan carries the three columns that cannot be scanned straight into a
// Plant. It exists so every query joining plants shares one field list: three
// hand-written copies of that list is three chances to shift by one column.
type plantScan struct {
	light    *string
	acquired *time.Time
	profile  []byte
	toxicity []byte
}

// targets are the destinations for plantColumnsFor, in its order.
func (ps *plantScan) targets(p *plant.Plant) []any {
	return []any{
		&p.ID, &p.Slug, &p.CommonName, &p.BotanicalName, &p.Variety,
		&p.Domain, &p.Steward, &p.Status,
		&p.Location, &p.HAArea,
		&p.Accessibility, &p.WateringMethod, &p.LetPotDripper,
		&p.PotSizeIn, &p.PotMaterial, &p.HasDrainage, &p.SoilMix,
		&ps.light, &p.MinTempF, &ps.profile, &ps.toxicity,
		&ps.acquired, &p.ArchivedAt, &p.ShelteredAt, &p.CreatedAt, &p.UpdatedAt,
	}
}

// finish applies what targets could only collect indirectly.
func (ps *plantScan) finish(p *plant.Plant) error {
	if ps.light != nil {
		p.LightExposure = plant.LightExposure(*ps.light)
	}
	p.AcquiredAt = ps.acquired
	if len(ps.toxicity) > 0 {
		if err := json.Unmarshal(ps.toxicity, &p.Toxicity); err != nil {
			return err
		}
	}
	// An absent rating reads as unknown rather than as the empty string, so
	// nothing downstream has to decide what "" means about a cat.
	for _, r := range []*plant.Harm{&p.Toxicity.Cats, &p.Toxicity.Dogs, &p.Toxicity.People} {
		if *r == "" {
			*r = plant.HarmUnknown
		}
	}
	if len(ps.profile) > 0 {
		return json.Unmarshal(ps.profile, &p.CareProfile)
	}
	return nil
}

func scanDigestEntry(row pgx.Row) (plant.DigestEntry, error) {
	var p plant.Plant
	var v plant.Verdict
	var ps plantScan
	var evidence []byte

	targets := append(ps.targets(&p),
		&v.ID, &v.PlantID, &v.ForDate, &v.Action, &v.Reasoning,
		&v.Confidence, &evidence, &v.CreatedAt, &v.AcknowledgedAt,
		&v.Escalations,
	)
	if err := row.Scan(targets...); err != nil {
		return plant.DigestEntry{}, err
	}
	if err := ps.finish(&p); err != nil {
		return plant.DigestEntry{}, err
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
