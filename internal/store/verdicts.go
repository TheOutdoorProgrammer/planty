package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// SaveVerdict upserts today's judgment for a plant.
func (s *Store) SaveVerdict(ctx context.Context, v plant.Verdict) (plant.Verdict, error) {
	evidence, err := json.Marshal(v.Evidence)
	if err != nil {
		return plant.Verdict{}, err
	}
	if v.ForDate.IsZero() {
		v.ForDate = time.Now().UTC()
	}

	row := s.pool.QueryRow(ctx, `
		INSERT INTO verdicts (plant_id, for_date, action, reasoning, confidence, evidence)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (plant_id, for_date) DO UPDATE
			SET action = excluded.action, reasoning = excluded.reasoning,
			    confidence = excluded.confidence, evidence = excluded.evidence,
			    created_at = now(), acknowledged_at = NULL,
			    escalations = 0, escalated_at = NULL
		RETURNING id, plant_id, for_date, action, reasoning, confidence,
		          evidence, created_at, acknowledged_at`,
		v.PlantID, v.ForDate, v.Action, v.Reasoning, v.Confidence, evidence)

	return scanVerdict(row)
}

func scanVerdict(row pgx.Row) (plant.Verdict, error) {
	var v plant.Verdict
	var evidence []byte
	err := row.Scan(&v.ID, &v.PlantID, &v.ForDate, &v.Action, &v.Reasoning,
		&v.Confidence, &evidence, &v.CreatedAt, &v.AcknowledgedAt)
	if err != nil {
		return plant.Verdict{}, err
	}
	if len(evidence) > 0 {
		if err := json.Unmarshal(evidence, &v.Evidence); err != nil {
			return plant.Verdict{}, err
		}
	}
	return v, nil
}

// LatestVerdict returns a plant's most recent judgment.
func (s *Store) LatestVerdict(ctx context.Context, plantID uuid.UUID) (plant.Verdict, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, plant_id, for_date, action, reasoning, confidence,
		       evidence, created_at, acknowledged_at
		FROM verdicts WHERE plant_id = $1
		ORDER BY for_date DESC LIMIT 1`, plantID)

	v, err := scanVerdict(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.Verdict{}, ErrNotFound
	}
	return v, err
}

// AckVerdict marks a verdict as handled, which stops its escalation.
func (s *Store) AckVerdict(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE verdicts SET acknowledged_at = now() WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Digest is what needs doing; StaleSince separates calm from no-fresh-data.
func (s *Store) Digest(ctx context.Context, staleAfter time.Duration) (plant.Digest, error) {
	digest := plant.Digest{Date: time.Now().UTC()}

	if err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM plants WHERE archived_at IS NULL AND status <> 'dead'`,
	).Scan(&digest.Checked); err != nil {
		return digest, err
	}

	var newest *time.Time
	if err := s.pool.QueryRow(ctx,
		`SELECT max(created_at) FROM verdicts`).Scan(&newest); err != nil {
		return digest, err
	}
	switch {
	case newest == nil:
		digest.NeverRun = true
	case time.Since(*newest) > staleAfter:
		digest.StaleSince = newest
	}

	rows, err := s.pool.Query(ctx, `
		SELECT `+plantColumnsFor("p")+`,
		       v.id, v.plant_id, v.for_date, v.action, v.reasoning,
		       v.confidence, v.evidence, v.created_at, v.acknowledged_at,
		       v.escalations
		FROM verdicts v
		JOIN plants p ON p.id = v.plant_id
		WHERE v.acknowledged_at IS NULL
		  AND v.action <> 'none'
		  AND p.archived_at IS NULL
		ORDER BY v.for_date DESC`)
	if err != nil {
		return digest, err
	}
	defer rows.Close()

	for rows.Next() {
		entry, err := scanDigestEntry(rows)
		if err != nil {
			return digest, err
		}
		digest.Entries = append(digest.Entries, entry)
	}
	if err := rows.Err(); err != nil {
		return digest, err
	}

	sortByRisk(digest.Entries)
	return digest, nil
}

// Postmortem stores what a dead plant taught.
func (s *Store) SavePostmortem(ctx context.Context, p plant.Postmortem) error {
	evidence, err := json.Marshal(p.Evidence)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO postmortems (plant_id, likely_cause, narrative, evidence, lesson)
		VALUES ($1, $2, $3, $4, nullif($5,''))
		ON CONFLICT (plant_id) DO UPDATE
			SET likely_cause = excluded.likely_cause, narrative = excluded.narrative,
			    evidence = excluded.evidence, lesson = excluded.lesson`,
		p.PlantID, p.LikelyCause, p.Narrative, evidence, p.Lesson)
	return err
}

// DeadWithoutPostmortem returns plants that died and were never analysed.
// Archived ones are included: archiving is exactly how a death is recorded.
func (s *Store) DeadWithoutPostmortem(ctx context.Context) ([]plant.Plant, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+plantColumnsFor("p")+`
		FROM plants p
		LEFT JOIN postmortems m ON m.plant_id = p.id
		WHERE p.status = 'dead' AND m.id IS NULL
		ORDER BY p.updated_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []plant.Plant
	for rows.Next() {
		p, err := scanPlant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Postmortem returns a dead plant's analysis.
func (s *Store) Postmortem(ctx context.Context, plantID uuid.UUID) (plant.Postmortem, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, plant_id, likely_cause, narrative, evidence,
		       coalesce(lesson,''), created_at
		FROM postmortems WHERE plant_id = $1`, plantID)

	var p plant.Postmortem
	var evidence []byte
	err := row.Scan(&p.ID, &p.PlantID, &p.LikelyCause, &p.Narrative,
		&evidence, &p.Lesson, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.Postmortem{}, ErrNotFound
	}
	if err != nil {
		return plant.Postmortem{}, err
	}
	if len(evidence) > 0 {
		if err := json.Unmarshal(evidence, &p.Evidence); err != nil {
			return plant.Postmortem{}, err
		}
	}
	return p, nil
}
