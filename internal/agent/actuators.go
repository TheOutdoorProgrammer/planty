package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
	"github.com/google/uuid"
)

func (d Deps) actuators(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("actuators")
	slug := set.String("plant", "", "show only actuators assigned to this plant slug")
	if err := parse(set, args); err != nil {
		return err
	}
	var selected *plant.Plant
	if *slug != "" {
		p, err := d.lookUp(ctx, *slug)
		if err != nil {
			return err
		}
		selected = &p
	}
	actuators, err := d.Store.Actuators(ctx)
	if err != nil {
		return err
	}
	plants, err := d.Store.ListPlants(ctx, store.PlantFilter{IncludeArchived: true})
	if err != nil {
		return err
	}
	plantSlugs := make(map[uuid.UUID]string, len(plants))
	for _, p := range plants {
		plantSlugs[p.ID] = p.Slug
	}
	matched := 0
	for _, actuator := range actuators {
		if selected != nil && !slices.Contains(actuator.PlantIDs, selected.ID) {
			continue
		}
		matched++
		state := "off"
		if lease, err := d.Store.ActiveActuatorLease(ctx, actuator.ID); err == nil {
			state = "on until " + lease.Deadline.Format(stamp)
		}
		assignedPlants := make([]string, 0, len(actuator.PlantIDs))
		for _, plantID := range actuator.PlantIDs {
			slug := plantSlugs[plantID]
			if slug == "" {
				slug = plantID.String()
			}
			assignedPlants = append(assignedPlants, slug)
		}
		_, _ = fmt.Fprintf(out, "%s: %s (%s, %s) plants=%s %s\n",
			actuator.ID, actuator.Name, actuator.Kind, actuator.EntityID,
			strings.Join(assignedPlants, ","), state)
	}
	if matched == 0 {
		if selected != nil {
			_, _ = fmt.Fprintf(out, "no actuator is assigned to %s\n", selected.CommonName)
		} else {
			_, _ = fmt.Fprintln(out, "no actuators are registered")
		}
	}
	return nil
}

func (d Deps) actuatorStart(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("actuatorstart")
	slug := set.String("plant", "", "plant slug this actuator is assigned to")
	idRaw := set.String("id", "", "allowlisted Planty actuator UUID")
	seconds := set.Int("seconds", 0, "bounded run duration in seconds")
	keyRaw := set.String("key", "", "idempotency UUID")
	if err := parse(set, args); err != nil {
		return err
	}
	if d.Actuators == nil {
		return errors.New("actuator control is not wired up in this process")
	}
	p, err := d.lookUp(ctx, *slug)
	if err != nil {
		return err
	}
	id, key, err := actuatorIDs(*idRaw, *keyRaw)
	if err != nil {
		return err
	}
	lease, created, err := d.Actuators.StartForPlant(ctx, id, p.ID, *seconds, "planty agent", plant.SourceAgent, key)
	if err != nil {
		return err
	}
	verb := "started"
	if !created {
		verb = "already accepted"
	}
	_, _ = fmt.Fprintf(out, "%s actuator %s until %s\n", verb, id, lease.Deadline.Format(stamp))
	return nil
}

func (d Deps) actuatorStop(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("actuatorstop")
	slug := set.String("plant", "", "plant slug this actuator is assigned to")
	idRaw := set.String("id", "", "allowlisted Planty actuator UUID")
	keyRaw := set.String("key", "", "idempotency UUID")
	if err := parse(set, args); err != nil {
		return err
	}
	if d.Actuators == nil {
		return errors.New("actuator control is not wired up in this process")
	}
	p, err := d.lookUp(ctx, *slug)
	if err != nil {
		return err
	}
	id, key, err := actuatorIDs(*idRaw, *keyRaw)
	if err != nil {
		return err
	}
	actuator, err := d.Store.Actuator(ctx, id)
	if err != nil {
		return err
	}
	if !slices.Contains(actuator.PlantIDs, p.ID) {
		return fmt.Errorf("actuator %s is not assigned to %s", id, p.CommonName)
	}
	stopped, err := d.Actuators.Stop(ctx, id, "planty agent", plant.SourceAgent, key)
	if err != nil {
		return err
	}
	if stopped {
		_, _ = fmt.Fprintf(out, "stopped actuator %s\n", id)
	} else {
		_, _ = fmt.Fprintf(out, "actuator %s was already stopped or this request was already handled\n", id)
	}
	return nil
}

func actuatorIDs(idRaw, keyRaw string) (uuid.UUID, uuid.UUID, error) {
	id, err := uuid.Parse(idRaw)
	if err != nil {
		return uuid.Nil, uuid.Nil, errors.New("--id must be an allowlisted actuator UUID")
	}
	key, err := uuid.Parse(keyRaw)
	if err != nil {
		return uuid.Nil, uuid.Nil, errors.New("--key must be an idempotency UUID")
	}
	return id, key, nil
}
