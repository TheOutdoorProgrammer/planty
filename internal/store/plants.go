package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// ErrNotFound is returned when a slug or id matches nothing live.
var ErrNotFound = errors.New("not found")

const plantColumns = `
	id, slug, common_name, coalesce(botanical_name, ''), coalesce(variety, ''),
	domain, steward, status,
	location, coalesce(ha_area, ''),
	accessibility, watering_method, letpot_dripper,
	pot_size_in, coalesce(pot_material, ''), has_drainage, coalesce(soil_mix, ''),
	light_exposure, min_temp_f, care_profile,
	acquired_at, archived_at, sheltered_at, created_at, updated_at`

func scanPlant(row pgx.Row) (plant.Plant, error) {
	var p plant.Plant
	var light *string
	var acquired *time.Time
	var profile []byte

	err := row.Scan(
		&p.ID, &p.Slug, &p.CommonName, &p.BotanicalName, &p.Variety,
		&p.Domain, &p.Steward, &p.Status,
		&p.Location, &p.HAArea,
		&p.Accessibility, &p.WateringMethod, &p.LetPotDripper,
		&p.PotSizeIn, &p.PotMaterial, &p.HasDrainage, &p.SoilMix,
		&light, &p.MinTempF, &profile,
		&acquired, &p.ArchivedAt, &p.ShelteredAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return plant.Plant{}, err
	}
	if light != nil {
		p.LightExposure = plant.LightExposure(*light)
	}
	p.AcquiredAt = acquired
	if len(profile) > 0 {
		if err := json.Unmarshal(profile, &p.CareProfile); err != nil {
			return plant.Plant{}, fmt.Errorf("care_profile for %s: %w", p.Slug, err)
		}
	}
	return p, nil
}

// CreatePlant inserts a plant and returns it as stored.
func (s *Store) CreatePlant(ctx context.Context, p plant.Plant) (plant.Plant, error) {
	if err := p.Valid(); err != nil {
		return plant.Plant{}, err
	}
	if p.Slug == "" {
		p.Slug = Slugify(p.CommonName)
	}

	profile, err := json.Marshal(p.CareProfile)
	if err != nil {
		return plant.Plant{}, fmt.Errorf("marshal care_profile: %w", err)
	}

	row := s.pool.QueryRow(ctx, `
		INSERT INTO plants (
			slug, common_name, botanical_name, variety,
			domain, steward, status, location, ha_area,
			accessibility, watering_method, letpot_dripper,
			pot_size_in, pot_material, has_drainage, soil_mix,
			light_exposure, min_temp_f, care_profile, acquired_at
		) VALUES (
			$1, $2, nullif($3,''), nullif($4,''),
			$5, $6, $7, $8, nullif($9,''),
			$10, $11, $12,
			$13, nullif($14,''), $15, nullif($16,''),
			nullif($17,'')::light_exposure, $18, $19, $20
		)
		RETURNING `+plantColumns,
		p.Slug, p.CommonName, p.BotanicalName, p.Variety,
		p.Domain, p.Steward, p.Status, p.Location, p.HAArea,
		p.Accessibility, p.WateringMethod, p.LetPotDripper,
		p.PotSizeIn, p.PotMaterial, p.HasDrainage, p.SoilMix,
		string(p.LightExposure), p.MinTempF, profile, p.AcquiredAt,
	)
	return scanPlant(row)
}

// GetPlant returns one plant by slug, archived or not.
func (s *Store) GetPlant(ctx context.Context, slug string) (plant.Plant, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+plantColumns+` FROM plants WHERE slug = $1`, slug)
	p, err := scanPlant(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.Plant{}, ErrNotFound
	}
	return p, err
}

// PlantFilter narrows a list. Zero values mean no restriction.
type PlantFilter struct {
	Domain          plant.Domain
	Steward         string
	Status          plant.Status
	WateringMethod  plant.WateringMethod
	IncludeArchived bool
}

// ListPlants returns plants matching the filter, most at risk of neglect first.
func (s *Store) ListPlants(ctx context.Context, f PlantFilter) ([]plant.Plant, error) {
	var where []string
	var args []any

	if !f.IncludeArchived {
		where = append(where, "archived_at IS NULL")
	}
	if f.Domain != "" {
		args = append(args, f.Domain)
		where = append(where, fmt.Sprintf("domain = $%d", len(args)))
	}
	if f.Steward != "" {
		args = append(args, f.Steward)
		where = append(where, fmt.Sprintf("steward = $%d", len(args)))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	if f.WateringMethod != "" {
		args = append(args, f.WateringMethod)
		where = append(where, fmt.Sprintf("watering_method = $%d", len(args)))
	}

	query := `SELECT ` + plantColumns + ` FROM plants`
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, " AND ")
	}
	query += ` ORDER BY common_name`

	rows, err := s.pool.Query(ctx, query, args...)
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

// ColdWatch returns live plants needing protection at or above the given
// forecast low, which is the question the nightly cold-snap job asks.
func (s *Store) ColdWatch(ctx context.Context, forecastLowF float64) ([]plant.Plant, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+plantColumns+`
		FROM plants
		WHERE archived_at IS NULL
		  AND status <> 'dead'
		  AND min_temp_f IS NOT NULL
		  AND min_temp_f >= $1
		ORDER BY min_temp_f DESC`, forecastLowF)
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

// ArchivePlant retires a plant without destroying its history.
func (s *Store) ArchivePlant(ctx context.Context, slug string, status plant.Status) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE plants
		SET archived_at = now(), status = $2, updated_at = now()
		WHERE slug = $1 AND archived_at IS NULL`, slug, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Slugify makes a stable, readable ref fragment out of a name.
func Slugify(name string) string {
	var b strings.Builder
	lastDash := true
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
