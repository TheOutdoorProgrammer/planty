package store

import (
	"context"
	"fmt"
)

// PushDevice is one APNs token registered by the iOS app. Tokens are scoped to
// the APNs environment because development and production tokens are not
// interchangeable even when they come from the same physical phone.
type PushDevice struct {
	Token       string
	Environment string
}

func (s *Store) UpsertPushDevice(ctx context.Context, device PushDevice) error {
	if device.Token == "" {
		return fmt.Errorf("push token is required")
	}
	if device.Environment != "sandbox" && device.Environment != "production" {
		return fmt.Errorf("push environment must be sandbox or production")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO push_devices (token, environment)
		VALUES ($1, $2)
		ON CONFLICT (environment, token)
		DO UPDATE SET updated_at = now()`, device.Token, device.Environment)
	return err
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
