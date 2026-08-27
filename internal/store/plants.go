package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// ErrNotFound is returned when a slug or id matches nothing live.
var ErrNotFound = errors.New("not found")

// plantColumns is the unaliased list; joins use plantColumnsFor instead.
var plantColumns = plantColumnsFor("")

// plantColumnsFor builds the select list, optionally qualified by an alias.
// Generated, not string-substituted: splitting on commas shreds coalesce calls.
func plantColumnsFor(alias string) string {
	q := ""
	if alias != "" {
		q = alias + "."
	}
	return fmt.Sprintf(`
	%[1]sid, %[1]sslug, %[1]scommon_name,
	coalesce(%[1]sbotanical_name, ''), coalesce(%[1]svariety, ''),
	%[1]sdomain, %[1]ssteward, %[1]sstatus,
	%[1]slocation, coalesce(%[1]sha_area, ''),
	%[1]saccessibility, %[1]swatering_method, %[1]sletpot_dripper,
	%[1]spot_size_in, coalesce(%[1]spot_material, ''), %[1]shas_drainage,
	coalesce(%[1]ssoil_mix, ''),
	%[1]slight_exposure, %[1]smin_temp_f, %[1]scare_profile, %[1]stoxicity,
	%[1]sacquired_at, %[1]sarchived_at, %[1]ssheltered_at,
	%[1]screated_at, %[1]supdated_at`, q)
}

// marshalToxicity validates before writing, since the CHECK constraint only
// guards the three ratings and everything that makes them trustworthy — the
// principle, the parts, the basis — is prose Postgres cannot police.
func marshalToxicity(t plant.Toxicity) ([]byte, error) {
	if err := t.Valid(); err != nil {
		return nil, err
	}
	if !t.Checked() {
		// An untouched record stays the empty document the column defaults to,
		// so "nobody has looked" keeps one representation rather than two.
		return []byte(`{}`), nil
	}
	raw, err := json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("marshal toxicity: %w", err)
	}
	return raw, nil
}

func scanPlant(row pgx.Row) (plant.Plant, error) {
	var p plant.Plant
	var ps plantScan

	if err := row.Scan(ps.targets(&p)...); err != nil {
		return plant.Plant{}, classify(err)
	}
	if err := ps.finish(&p); err != nil {
		return plant.Plant{}, fmt.Errorf("care_profile for %s: %w", p.Slug, err)
	}
	return p, nil
}

// CreatePlant inserts a plant and returns it as stored.
func (s *Store) CreatePlant(ctx context.Context, p plant.Plant) (plant.Plant, error) {
	if err := p.Valid(); err != nil {
		return plant.Plant{}, err
	}

	profile, err := json.Marshal(p.CareProfile)
	if err != nil {
		return plant.Plant{}, fmt.Errorf("marshal care_profile: %w", err)
	}
	toxicity, err := marshalToxicity(p.Toxicity)
	if err != nil {
		return plant.Plant{}, err
	}

	if p.Slug == "" {
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return plant.Plant{}, err
		}
		defer func() { _ = tx.Rollback(ctx) }()

		base := Slugify(p.CommonName)
		if base == "" {
			return plant.Plant{}, fmt.Errorf("no slug can be made from %q", p.CommonName)
		}
		// One allocator per base slug. The lock is transaction-scoped, so a
		// crashed creator cannot strand it and unrelated plant names proceed.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, base); err != nil {
			return plant.Plant{}, fmt.Errorf("lock slug %s: %w", base, err)
		}
		p.Slug, err = freeSlug(ctx, tx, base)
		if err != nil {
			return plant.Plant{}, err
		}
		created, err := insertPlant(ctx, tx, p, profile, toxicity)
		if err != nil {
			return plant.Plant{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return plant.Plant{}, err
		}
		return created, nil
	}

	return insertPlant(ctx, s.pool, p, profile, toxicity)
}

type plantInserter interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func insertPlant(ctx context.Context, db plantInserter, p plant.Plant, profile, toxicity []byte) (plant.Plant, error) {
	row := db.QueryRow(ctx, `
		INSERT INTO plants (
			slug, common_name, botanical_name, variety,
			domain, steward, status, location, ha_area,
			accessibility, watering_method, letpot_dripper,
			pot_size_in, pot_material, has_drainage, soil_mix,
			light_exposure, min_temp_f, care_profile, acquired_at,
			toxicity
		) VALUES (
			$1, $2, nullif($3,''), nullif($4,''),
			$5, $6, $7, $8, nullif($9,''),
			$10, $11, $12,
			$13, nullif($14,''), $15, nullif($16,''),
			nullif($17,'')::light_exposure, $18, $19, $20,
			$21
		)
		RETURNING `+plantColumns,
		p.Slug, p.CommonName, p.BotanicalName, p.Variety,
		p.Domain, p.Steward, p.Status, p.Location, p.HAArea,
		p.Accessibility, p.WateringMethod, p.LetPotDripper,
		p.PotSizeIn, p.PotMaterial, p.HasDrainage, p.SoilMix,
		string(p.LightExposure), p.MinTempF, profile, p.AcquiredAt,
		toxicity,
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

func (s *Store) GetPlantByID(ctx context.Context, id uuid.UUID) (plant.Plant, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+plantColumns+` FROM plants WHERE id = $1`, id)
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

	out := []plant.Plant{}
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

	out := []plant.Plant{}
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
	if err := status.ValidateArchive(); err != nil {
		return err
	}
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

func (s *Store) RestorePlant(ctx context.Context, slug string) (plant.Plant, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE plants
		SET archived_at = NULL, status = 'alive', updated_at = now()
		WHERE slug = $1 AND archived_at IS NOT NULL
		RETURNING `+plantColumns, slug)
	p, err := scanPlant(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.Plant{}, ErrNotFound
	}
	return p, classify(err)
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

// FreeSlug returns a slug nobody holds yet, numbering from the second one on.
// Owning three pothos is ordinary, and slugs are unique, so naming a plant
// after its species has to survive doing it twice.
func (s *Store) FreeSlug(ctx context.Context, name string) (string, error) {
	base := Slugify(name)
	if base == "" {
		return "", fmt.Errorf("no slug can be made from %q", name)
	}
	return freeSlug(ctx, s.pool, base)
}

type slugQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func freeSlug(ctx context.Context, db slugQuerier, base string) (string, error) {
	rows, err := db.Query(ctx,
		`SELECT slug FROM plants WHERE slug = $1 OR slug LIKE $1 || '-%'`, base)
	if err != nil {
		return "", fmt.Errorf("look up slugs like %s: %w", base, err)
	}
	defer rows.Close()

	taken := make(map[string]bool)
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return "", err
		}
		taken[slug] = true
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	if !taken[base] {
		return base, nil
	}
	for n := 2; n <= len(taken)+2; n++ {
		if candidate := fmt.Sprintf("%s-%d", base, n); !taken[candidate] {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no free slug for %q", base)
}
