package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Store) DerivePlant(ctx context.Context, sourceSlug string, request plant.DerivePlantRequest) (plant.Plant, plant.PlantLineage, error) {
	if err := request.Valid(); err != nil {
		return plant.Plant{}, plant.PlantLineage{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return plant.Plant{}, plant.PlantLineage{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	source, err := scanPlant(tx.QueryRow(ctx, `SELECT `+plantColumns+` FROM plants WHERE slug = $1 FOR UPDATE`, sourceSlug))
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.Plant{}, plant.PlantLineage{}, ErrNotFound
	}
	if err != nil {
		return plant.Plant{}, plant.PlantLineage{}, err
	}

	child := source
	child.ID = uuid.Nil
	child.Slug = ""
	child.CommonName = strings.TrimSpace(request.CommonName)
	if value := strings.TrimSpace(request.BotanicalName); value != "" {
		child.BotanicalName = value
	}
	if value := strings.TrimSpace(request.Variety); value != "" {
		child.Variety = value
	}
	if value := strings.TrimSpace(request.Location); value != "" {
		child.Location = value
	}
	child.Status = plant.StatusAlive
	child.ArchivedAt = nil
	child.ShelteredAt = nil

	profile, err := json.Marshal(child.CareProfile)
	if err != nil {
		return plant.Plant{}, plant.PlantLineage{}, err
	}
	toxicity, err := marshalToxicity(child.Toxicity)
	if err != nil {
		return plant.Plant{}, plant.PlantLineage{}, err
	}
	base := Slugify(child.CommonName)
	if base == "" {
		return plant.Plant{}, plant.PlantLineage{}, plant.ErrInvalid
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, base); err != nil {
		return plant.Plant{}, plant.PlantLineage{}, err
	}
	child.Slug, err = freeSlug(ctx, tx, base)
	if err != nil {
		return plant.Plant{}, plant.PlantLineage{}, err
	}
	created, err := insertPlant(ctx, tx, child, profile, toxicity)
	if err != nil {
		return plant.Plant{}, plant.PlantLineage{}, err
	}
	derivedAt := time.Now().UTC()
	if _, err := tx.Exec(ctx, `INSERT INTO plant_lineage (child_plant_id, source_plant_id, derived_at) VALUES ($1,$2,$3)`, created.ID, source.ID, derivedAt); err != nil {
		return plant.Plant{}, plant.PlantLineage{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return plant.Plant{}, plant.PlantLineage{}, err
	}
	return created, plant.PlantLineage{
		SourcePlantID: source.ID, SourceSlug: source.Slug,
		SourceCommonName: source.CommonName, DerivedAt: derivedAt,
	}, nil
}

func (s *Store) PlantLineage(ctx context.Context, childID uuid.UUID) (plant.PlantLineage, error) {
	var lineage plant.PlantLineage
	err := s.pool.QueryRow(ctx, `
		SELECT source.id, source.slug, source.common_name, lineage.derived_at
		FROM plant_lineage lineage
		JOIN plants source ON source.id = lineage.source_plant_id
		WHERE lineage.child_plant_id = $1`, childID).Scan(
		&lineage.SourcePlantID, &lineage.SourceSlug, &lineage.SourceCommonName, &lineage.DerivedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.PlantLineage{}, ErrNotFound
	}
	return lineage, err
}
