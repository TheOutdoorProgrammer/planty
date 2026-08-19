package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// PlantPatch is a sparse update. Nil means leave alone, which is what lets an
// agent change one field from a sentence without resending the whole record.
type PlantPatch struct {
	CommonName     *string               `json:"common_name,omitempty"`
	BotanicalName  *string               `json:"botanical_name,omitempty"`
	Variety        *string               `json:"variety,omitempty"`
	Domain         *plant.Domain         `json:"domain,omitempty"`
	Steward        *string               `json:"steward,omitempty"`
	Status         *plant.Status         `json:"status,omitempty"`
	Location       *string               `json:"location,omitempty"`
	HAArea         *string               `json:"ha_area,omitempty"`
	Accessibility  *plant.Accessibility  `json:"accessibility,omitempty"`
	WateringMethod *plant.WateringMethod `json:"watering_method,omitempty"`
	LetPotDripper  *int                  `json:"letpot_dripper,omitempty"`
	PotSizeIn      *float64              `json:"pot_size_in,omitempty"`
	PotMaterial    *string               `json:"pot_material,omitempty"`
	HasDrainage    *bool                 `json:"has_drainage,omitempty"`
	SoilMix        *string               `json:"soil_mix,omitempty"`
	LightExposure  *plant.LightExposure  `json:"light_exposure,omitempty"`
	MinTempF       *float64              `json:"min_temp_f,omitempty"`
	CareProfile    *plant.CareProfile    `json:"care_profile,omitempty"`
	Toxicity       *plant.Toxicity       `json:"toxicity,omitempty"`
}

// UpdatePlant applies a sparse patch and returns the stored record.
func (s *Store) UpdatePlant(ctx context.Context, slug string, patch PlantPatch) (plant.Plant, error) {
	var sets []string
	var args []any

	set := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	if patch.CommonName != nil {
		set("common_name", *patch.CommonName)
	}
	if patch.BotanicalName != nil {
		set("botanical_name", nullify(*patch.BotanicalName))
	}
	if patch.Variety != nil {
		set("variety", nullify(*patch.Variety))
	}
	if patch.Domain != nil {
		set("domain", *patch.Domain)
	}
	if patch.Steward != nil {
		set("steward", *patch.Steward)
	}
	if patch.Status != nil {
		set("status", *patch.Status)
	}
	if patch.Location != nil {
		set("location", *patch.Location)
	}
	if patch.HAArea != nil {
		set("ha_area", nullify(*patch.HAArea))
	}
	if patch.Accessibility != nil {
		set("accessibility", *patch.Accessibility)
	}
	if patch.WateringMethod != nil {
		set("watering_method", *patch.WateringMethod)
		// The check constraint would reject the pair, so clear the stale dripper.
		if *patch.WateringMethod == plant.WateringHand && patch.LetPotDripper == nil {
			set("letpot_dripper", nil)
		}
	}
	if patch.LetPotDripper != nil {
		set("letpot_dripper", *patch.LetPotDripper)
	}
	if patch.PotSizeIn != nil {
		set("pot_size_in", *patch.PotSizeIn)
	}
	if patch.PotMaterial != nil {
		set("pot_material", nullify(*patch.PotMaterial))
	}
	if patch.HasDrainage != nil {
		set("has_drainage", *patch.HasDrainage)
	}
	if patch.SoilMix != nil {
		set("soil_mix", nullify(*patch.SoilMix))
	}
	if patch.LightExposure != nil {
		set("light_exposure", nullify(string(*patch.LightExposure)))
	}
	if patch.MinTempF != nil {
		set("min_temp_f", *patch.MinTempF)
	}
	if patch.CareProfile != nil {
		raw, err := json.Marshal(*patch.CareProfile)
		if err != nil {
			return plant.Plant{}, err
		}
		set("care_profile", raw)
	}
	if patch.Toxicity != nil {
		raw, err := marshalToxicity(*patch.Toxicity)
		if err != nil {
			return plant.Plant{}, err
		}
		set("toxicity", raw)
	}

	if len(sets) == 0 {
		return s.GetPlant(ctx, slug)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return plant.Plant{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := scanPlant(tx.QueryRow(ctx, `
		SELECT `+plantColumns+`
		FROM plants
		WHERE slug = $1 AND archived_at IS NULL
		FOR UPDATE`, slug))
	if err != nil {
		return plant.Plant{}, err
	}
	patch.apply(&current)
	if err := current.Valid(); err != nil {
		return plant.Plant{}, err
	}

	sets = append(sets, "updated_at = now()")

	args = append(args, slug)
	query := `UPDATE plants SET ` + strings.Join(sets, ", ") +
		fmt.Sprintf(` WHERE slug = $%d AND archived_at IS NULL RETURNING `, len(args)) + plantColumns

	p, err := scanPlant(tx.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.Plant{}, ErrNotFound
	}
	if err != nil {
		return plant.Plant{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return plant.Plant{}, err
	}
	return p, nil
}

func (patch PlantPatch) apply(p *plant.Plant) {
	applyValue(&p.CommonName, patch.CommonName)
	applyValue(&p.BotanicalName, patch.BotanicalName)
	applyValue(&p.Variety, patch.Variety)
	applyValue(&p.Domain, patch.Domain)
	applyValue(&p.Steward, patch.Steward)
	applyValue(&p.Status, patch.Status)
	applyValue(&p.Location, patch.Location)
	applyValue(&p.HAArea, patch.HAArea)
	applyValue(&p.Accessibility, patch.Accessibility)
	if patch.WateringMethod != nil {
		p.WateringMethod = *patch.WateringMethod
		if *patch.WateringMethod == plant.WateringHand && patch.LetPotDripper == nil {
			p.LetPotDripper = nil
		}
	}
	if patch.LetPotDripper != nil {
		p.LetPotDripper = patch.LetPotDripper
	}
	if patch.PotSizeIn != nil {
		p.PotSizeIn = patch.PotSizeIn
	}
	applyValue(&p.PotMaterial, patch.PotMaterial)
	if patch.HasDrainage != nil {
		p.HasDrainage = patch.HasDrainage
	}
	applyValue(&p.SoilMix, patch.SoilMix)
	applyValue(&p.LightExposure, patch.LightExposure)
	if patch.MinTempF != nil {
		p.MinTempF = patch.MinTempF
	}
	applyValue(&p.CareProfile, patch.CareProfile)
	applyValue(&p.Toxicity, patch.Toxicity)
}

func applyValue[T any](target *T, value *T) {
	if value != nil {
		*target = *value
	}
}

// nullify maps the empty string to SQL NULL, so clearing a field and never
// setting it look the same in the database.
func nullify(s string) any {
	if s == "" {
		return nil
	}
	return s
}
