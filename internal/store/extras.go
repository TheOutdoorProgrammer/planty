package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

const questionColumns = `
	id, plant_id, asked_of, question, coalesce(why,''), status,
	coalesce(answer,''), answered_at, created_at`

func scanQuestion(row pgx.Row) (plant.Question, error) {
	var q plant.Question
	err := row.Scan(&q.ID, &q.PlantID, &q.AskedOf, &q.Question, &q.Why,
		&q.Status, &q.Answer, &q.AnsweredAt, &q.CreatedAt)
	return q, err
}

// AskOwner queues a question only the plant's owner can settle.
func (s *Store) AskOwner(ctx context.Context, q plant.Question) (plant.Question, error) {
	if q.Question == "" {
		return plant.Question{}, fmt.Errorf("%w: question is required", plant.ErrInvalid)
	}
	// Default to the plant's steward: an agent asking about a plant should not
	// have to know who owns it.
	if q.AskedOf == "" && q.PlantID != nil {
		if err := s.pool.QueryRow(ctx,
			`SELECT steward FROM plants WHERE id = $1`, *q.PlantID).Scan(&q.AskedOf); err != nil {
			return plant.Question{}, err
		}
	}
	if q.AskedOf == "" {
		return plant.Question{}, fmt.Errorf(
			"%w: asked_of is required when no plant is named", plant.ErrInvalid)
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO questions (plant_id, asked_of, question, why)
		VALUES ($1, $2, $3, nullif($4,''))
		RETURNING `+questionColumns,
		q.PlantID, q.AskedOf, q.Question, q.Why)
	return scanQuestion(row)
}

// Questions returns queued questions, optionally for one person.
func (s *Store) Questions(ctx context.Context, askedOf string, status plant.QuestionStatus) ([]plant.Question, error) {
	query := `SELECT ` + questionColumns + ` FROM questions WHERE 1=1`
	var args []any
	if askedOf != "" {
		args = append(args, askedOf)
		query += ` AND asked_of = $1`
	}
	if status != "" {
		args = append(args, status)
		query += ` AND status = $` + strconv.Itoa(len(args))
	}
	query += ` ORDER BY created_at`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []plant.Question{}
	for rows.Next() {
		q, err := scanQuestion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// AnswerQuestion records what the owner said.
func (s *Store) AnswerQuestion(ctx context.Context, id uuid.UUID, answer string) (plant.Question, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE questions
		SET answer = $2, status = 'answered', answered_at = now()
		WHERE id = $1
		RETURNING `+questionColumns, id, answer)
	q, err := scanQuestion(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.Question{}, ErrNotFound
	}
	return q, err
}

// GoAway records a window where nobody is home to act on a notification.
func (s *Store) GoAway(ctx context.Context, a plant.AwayPeriod) (plant.AwayPeriod, error) {
	if !a.EndsAt.After(a.StartsAt) {
		return plant.AwayPeriod{}, fmt.Errorf(
			"%w: ends_at must be after starts_at", plant.ErrInvalid)
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO away_periods (starts_at, ends_at, backup_contact, backup_notify, note)
		VALUES ($1, $2, nullif($3,''), nullif($4,''), nullif($5,''))
		RETURNING id, starts_at, ends_at, coalesce(backup_contact,''),
		          coalesce(backup_notify,''), coalesce(note,''), created_at`,
		a.StartsAt, a.EndsAt, a.BackupContact, a.BackupNotify, a.Note)

	var out plant.AwayPeriod
	err := row.Scan(&out.ID, &out.StartsAt, &out.EndsAt, &out.BackupContact,
		&out.BackupNotify, &out.Note, &out.CreatedAt)
	return out, err
}

// AwayAt returns the away period covering a moment, if there is one.
func (s *Store) AwayAt(ctx context.Context, at time.Time) (plant.AwayPeriod, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, starts_at, ends_at, coalesce(backup_contact,''),
		       coalesce(backup_notify,''), coalesce(note,''), created_at
		FROM away_periods
		WHERE starts_at <= $1 AND ends_at > $1
		ORDER BY starts_at DESC LIMIT 1`, at)

	var a plant.AwayPeriod
	err := row.Scan(&a.ID, &a.StartsAt, &a.EndsAt, &a.BackupContact,
		&a.BackupNotify, &a.Note, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.AwayPeriod{}, ErrNotFound
	}
	return a, err
}

// UpcomingAway returns the next away period starting within the window.
func (s *Store) UpcomingAway(ctx context.Context, within time.Duration) (plant.AwayPeriod, error) {
	now := time.Now().UTC()
	row := s.pool.QueryRow(ctx, `
		SELECT id, starts_at, ends_at, coalesce(backup_contact,''),
		       coalesce(backup_notify,''), coalesce(note,''), created_at
		FROM away_periods
		WHERE starts_at > $1 AND starts_at <= $2
		ORDER BY starts_at LIMIT 1`, now, now.Add(within))

	var a plant.AwayPeriod
	err := row.Scan(&a.ID, &a.StartsAt, &a.EndsAt, &a.BackupContact,
		&a.BackupNotify, &a.Note, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.AwayPeriod{}, ErrNotFound
	}
	return a, err
}
