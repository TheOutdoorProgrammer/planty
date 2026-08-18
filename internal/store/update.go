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

	if len(sets) == 0 {
		return s.GetPlant(ctx, slug)
	}
	sets = append(sets, "updated_at = now()")

	args = append(args, slug)
	query := `UPDATE plants SET ` + strings.Join(sets, ", ") +
		fmt.Sprintf(` WHERE slug = $%d AND archived_at IS NULL RETURNING `, len(args)) + plantColumns

	p, err := scanPlant(s.pool.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.Plant{}, ErrNotFound
	}
	return p, err
}

// nullify maps the empty string to SQL NULL, so clearing a field and never
// setting it look the same in the database.
func nullify(s string) any {
	if s == "" {
		return nil
	}
	return s
}
