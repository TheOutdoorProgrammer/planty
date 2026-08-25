package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PushDevice is one APNs token registered by the iOS app. Tokens are scoped to
// the APNs environment because development and production tokens are not
// interchangeable even when they come from the same physical phone.
type PushDevice struct {
	Token          string    `json:"-"`
	Environment    string    `json:"environment"`
	InstallationID uuid.UUID `json:"installation_id"`
	AcceptedAt     time.Time `json:"accepted_at"`
}

func (s *Store) UpsertPushDevice(ctx context.Context, device PushDevice) (PushDevice, error) {
	if device.Token == "" {
		return PushDevice{}, fmt.Errorf("push token is required")
	}
	if device.Environment != "sandbox" && device.Environment != "production" {
		return PushDevice{}, fmt.Errorf("push environment must be sandbox or production")
	}
	if device.InstallationID == uuid.Nil {
		return PushDevice{}, fmt.Errorf("installation id is required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PushDevice{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// A token refresh replaces the row for this install, while a reinstall that
	// happens to reuse a token transfers it to the new installation identity.
	if _, err := tx.Exec(ctx, `
		DELETE FROM push_devices
		WHERE environment = $1 AND token = $2 AND installation_id <> $3`,
		device.Environment, device.Token, device.InstallationID); err != nil {
		return PushDevice{}, err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO push_devices (token, environment, installation_id, accepted_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (environment, installation_id)
		DO UPDATE SET token = excluded.token, accepted_at = now(), updated_at = now()
		RETURNING accepted_at`, device.Token, device.Environment, device.InstallationID).
		Scan(&device.AcceptedAt)
	if err != nil {
		return PushDevice{}, err
	}
	return device, tx.Commit(ctx)
}

func (s *Store) PushDeviceTokens(ctx context.Context, environment string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT token FROM push_devices
		WHERE environment = $1
		ORDER BY updated_at DESC`, environment)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			return nil, err
		}
		out = append(out, token)
	}
	return out, rows.Err()
}

func (s *Store) DeletePushDevice(ctx context.Context, environment, token string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM push_devices WHERE environment = $1 AND token = $2`, environment, token)
	return err
}

func (s *Store) PushDeviceForInstallation(ctx context.Context, environment string, installationID uuid.UUID) (PushDevice, error) {
	var device PushDevice
	err := s.pool.QueryRow(ctx, `
		SELECT token, environment, installation_id, accepted_at
		FROM push_devices
		WHERE environment = $1 AND installation_id = $2`, environment, installationID).
		Scan(&device.Token, &device.Environment, &device.InstallationID, &device.AcceptedAt)
	if err == pgx.ErrNoRows {
		return PushDevice{}, ErrNotFound
	}
	return device, err
}
