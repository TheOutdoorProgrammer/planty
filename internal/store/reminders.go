package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

var reminderColumns = reminderColumnsFor("")

func reminderColumnsFor(alias string) string {
	q := ""
	if alias != "" {
		q = alias + "."
	}
	return fmt.Sprintf(`%[1]sid, %[1]splant_id, %[1]skind, %[1]severy_days,
		%[1]sat_hours, %[1]sactive, %[1]snote, %[1]slast_sent_at, %[1]screated_at`, q)
}

// DueReminder is a reminder that is owed, carried with the plant it belongs to
// and when the thing was last actually done.
type DueReminder struct {
	Reminder plant.Reminder `json:"reminder"`
	Plant    plant.Plant    `json:"plant"`
	LastDone *time.Time     `json:"last_done,omitempty"`
}

// SaveReminder creates or replaces the reminder of one kind for one plant.
// One per kind per plant: two watering schedules for the same pot is a bug
// somebody typed, not an intent worth honouring.
func (s *Store) SaveReminder(ctx context.Context, r plant.Reminder) (plant.Reminder, error) {
	r.Normalise()
	if err := r.Valid(); err != nil {
		return plant.Reminder{}, err
	}

	row := s.pool.QueryRow(ctx, `
		INSERT INTO reminders (plant_id, kind, every_days, at_hours, active, note)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (plant_id, kind) DO UPDATE SET
			every_days = excluded.every_days,
			at_hours   = excluded.at_hours,
			active     = excluded.active,
			note       = excluded.note
		RETURNING `+reminderColumns,
		r.PlantID, r.Kind, r.EveryDays, r.AtHours, r.Active, r.Note)
	return scanReminder(row)
}

// Reminders lists what is set for one plant.
func (s *Store) Reminders(ctx context.Context, plantID uuid.UUID) ([]plant.Reminder, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+reminderColumns+` FROM reminders WHERE plant_id = $1 ORDER BY kind`, plantID)
	if err != nil {
		return nil, fmt.Errorf("list reminders: %w", err)
	}
	defer rows.Close()

	out := make([]plant.Reminder, 0)
	for rows.Next() {
		r, err := scanReminder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteReminder removes one kind of reminder from one plant.
func (s *Store) DeleteReminder(ctx context.Context, plantID uuid.UUID, kind plant.ObservationKind) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM reminders WHERE plant_id = $1 AND kind = $2`, plantID, kind)
	if err != nil {
		return fmt.Errorf("delete reminder: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ActiveReminders returns every live reminder with the last time its kind was
// actually observed, which is what decides whether it is owed. The lateral is
// there because "most recent observation of this kind" is per reminder.
func (s *Store) ActiveReminders(ctx context.Context) ([]DueReminder, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+reminderColumnsFor("r")+`, `+plantColumnsFor("p")+`, done.occurred_at
		FROM reminders r
		JOIN plants p ON p.id = r.plant_id
		LEFT JOIN LATERAL (
			SELECT o.occurred_at
			FROM observations o
			WHERE o.plant_id = r.plant_id AND o.kind = r.kind
			ORDER BY o.occurred_at DESC
			LIMIT 1
		) done ON true
		WHERE r.active AND p.archived_at IS NULL AND p.status = 'alive'
		ORDER BY p.common_name, r.kind`)
	if err != nil {
		return nil, fmt.Errorf("active reminders: %w", err)
	}
	defer rows.Close()

	out := make([]DueReminder, 0)
	for rows.Next() {
		var due DueReminder
		var ps plantScan

		targets := append(reminderFields(&due.Reminder), ps.targets(&due.Plant)...)
		if err := rows.Scan(append(targets, &due.LastDone)...); err != nil {
			return nil, fmt.Errorf("scan reminder: %w", err)
		}
		if err := ps.finish(&due.Plant); err != nil {
			return nil, err
		}
		out = append(out, due)
	}
	return out, rows.Err()
}

// MarkReminderSent records that a notification went out. It only ever stops an
// hourly job sending the same slot twice, and is deliberately not what the
// next due time is computed from.
func (s *Store) MarkReminderSent(ctx context.Context, id uuid.UUID, at time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE reminders SET last_sent_at = $2 WHERE id = $1`, id, at)
	if err != nil {
		return fmt.Errorf("mark reminder sent: %w", err)
	}
	return nil
}

func reminderFields(r *plant.Reminder) []any {
	return []any{
		&r.ID, &r.PlantID, &r.Kind, &r.EveryDays, &r.AtHours,
		&r.Active, &r.Note, &r.LastSentAt, &r.CreatedAt,
	}
}

func scanReminder(row interface{ Scan(...any) error }) (plant.Reminder, error) {
	var r plant.Reminder
	if err := row.Scan(reminderFields(&r)...); err != nil {
		return plant.Reminder{}, fmt.Errorf("scan reminder: %w", err)
	}
	return r, nil
}
