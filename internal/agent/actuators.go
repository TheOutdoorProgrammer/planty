package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
)

func (d Deps) actuators(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("actuators")
	if err := parse(set, args); err != nil {
		return err
	}
	actuators, err := d.Store.Actuators(ctx)
	if err != nil {
		return err
	}
	for _, actuator := range actuators {
		state := "off"
		if lease, err := d.Store.ActiveActuatorLease(ctx, actuator.ID); err == nil {
			state = "on until " + lease.Deadline.Format(stamp)
		}
		plantIDs := make([]string, 0, len(actuator.PlantIDs))
		for _, plantID := range actuator.PlantIDs {
			plantIDs = append(plantIDs, plantID.String())
		}
		_, _ = fmt.Fprintf(out, "%s: %s (%s, %s) plants=%s %s\n",
			actuator.ID, actuator.Name, actuator.Kind, actuator.EntityID,
			strings.Join(plantIDs, ","), state)
	}
	return nil
}

func (d Deps) actuatorStart(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("actuatorstart")
	idRaw := set.String("id", "", "allowlisted Planty actuator UUID")
	seconds := set.Int("seconds", 0, "bounded run duration in seconds")
	keyRaw := set.String("key", "", "idempotency UUID")
	if err := parse(set, args); err != nil {
		return err
	}
	if d.Actuators == nil {
		return errors.New("actuator control is not wired up in this process")
	}
	id, key, err := actuatorIDs(*idRaw, *keyRaw)
	if err != nil {
		return err
	}
	lease, created, err := d.Actuators.Start(ctx, id, *seconds, "planty agent", plant.SourceAgent, key)
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
	idRaw := set.String("id", "", "allowlisted Planty actuator UUID")
	keyRaw := set.String("key", "", "idempotency UUID")
	if err := parse(set, args); err != nil {
		return err
	}
	if d.Actuators == nil {
		return errors.New("actuator control is not wired up in this process")
	}
	id, key, err := actuatorIDs(*idRaw, *keyRaw)
	if err != nil {
		return err
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
