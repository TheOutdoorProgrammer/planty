package store

import (
	"context"
	"time"
)

type ChoiceKind string

const (
	ChoicePlace       ChoiceKind = "place"
	ChoiceOwner       ChoiceKind = "owner"
	ChoicePotMaterial ChoiceKind = "pot_material"
)

// ChoiceCandidate is one spelling observed in Planty's own data. The API
// folds equivalent spellings together and adds Home Assistant areas before
// presenting the managed choice catalog to clients.
type ChoiceCandidate struct {
	Kind   ChoiceKind
	Value  string
	Source string
	UsedAt time.Time
}

// ChoiceCandidates returns every open-vocabulary value Planty already knows,
// along with when it was most recently attached to a record. No catalog table
// is needed: the user's own records remain the source of truth.
func (s *Store) ChoiceCandidates(ctx context.Context) ([]ChoiceCandidate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT kind, value, source, used_at
		FROM (
			SELECT 'place'::text AS kind, location AS value,
			       'plant_location'::text AS source, updated_at AS used_at
			FROM plants WHERE nullif(btrim(location), '') IS NOT NULL
			UNION ALL
			SELECT 'place', ha_area, 'home_assistant_area', updated_at
			FROM plants WHERE nullif(btrim(ha_area), '') IS NOT NULL
			UNION ALL
			SELECT 'place', zone, 'sensor_zone', created_at
			FROM sensor_links WHERE nullif(btrim(zone), '') IS NOT NULL
			UNION ALL
			SELECT 'owner', steward, 'plant_owner', updated_at
			FROM plants WHERE nullif(btrim(steward), '') IS NOT NULL
			UNION ALL
			SELECT 'pot_material', pot_material, 'plant_pot_material', updated_at
			FROM plants WHERE nullif(btrim(pot_material), '') IS NOT NULL
		) choices
		ORDER BY used_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ChoiceCandidate{}
	for rows.Next() {
		var kind string
		var candidate ChoiceCandidate
		if err := rows.Scan(&kind, &candidate.Value, &candidate.Source, &candidate.UsedAt); err != nil {
			return nil, err
		}
		candidate.Kind = ChoiceKind(kind)
		out = append(out, candidate)
	}
	return out, rows.Err()
}
