package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
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
		schedule := actuator.LightSchedule
		if actuator.Kind == plant.ActuatorFan {
			schedule = actuator.FanSchedule
		}
		if schedule != nil {
			state = fmt.Sprintf("schedule=%s %s enabled=%t",
				formatAgentScheduleWindows(schedule.EffectiveWindows()), schedule.Timezone, schedule.Enabled)
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

func (d Deps) lightState(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("lightstate")
	slug := set.String("plant", "", "plant slug this light is assigned to")
	idRaw := set.String("id", "", "allowlisted Planty actuator UUID")
	state := set.String("state", "", "on or off")
	if err := parse(set, args); err != nil {
		return err
	}
	if d.Actuators == nil {
		return errors.New("actuator control is not wired up in this process")
	}
	p, actuator, err := d.assignedLight(ctx, *slug, *idRaw)
	if err != nil {
		return err
	}
	on := *state == "on"
	if !on && *state != "off" {
		return errors.New("--state must be on or off")
	}
	if err := d.Actuators.SetLight(ctx, actuator.ID, on, "planty agent", plant.SourceAgent); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "turned %s %s for %s\n", actuator.Name, *state, p.CommonName)
	return nil
}

func (d Deps) lightSchedule(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("lightschedule")
	slug := set.String("plant", "", "plant slug this light is assigned to")
	idRaw := set.String("id", "", "allowlisted Planty actuator UUID")
	start := set.String("start", "", "local start time HH:MM")
	end := set.String("end", "", "local end time HH:MM")
	var windowValues scheduleWindowValues
	set.Var(&windowValues, "window", "local window HH:MM-HH:MM; repeat for split schedules")
	timezone := set.String("timezone", "America/New_York", "IANA timezone")
	enabled := set.Bool("enabled", true, "whether the schedule is active")
	if err := parse(set, args); err != nil {
		return err
	}
	p, actuator, err := d.assignedLight(ctx, *slug, *idRaw)
	if err != nil {
		return err
	}
	windows := make([]plant.ActuatorScheduleWindow, 0, max(1, len(windowValues)))
	if len(windowValues) == 0 {
		window, err := parseScheduleWindow(*start + "-" + *end)
		if err != nil {
			return fmt.Errorf("--start/--end: %w", err)
		}
		windows = append(windows, window)
	} else {
		for _, value := range windowValues {
			window, err := parseScheduleWindow(value)
			if err != nil {
				return fmt.Errorf("--window %q: %w", value, err)
			}
			windows = append(windows, window)
		}
	}
	schedule, err := d.Store.SetLightSchedule(ctx, plant.LightSchedule{
		ActuatorID: actuator.ID, Windows: windows, Timezone: *timezone, Enabled: *enabled,
	}, "planty agent", plant.SourceAgent)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "scheduled %s for %s at %s %s enabled=%t\n",
		actuator.Name, p.CommonName, formatAgentScheduleWindows(schedule.Windows),
		schedule.Timezone, schedule.Enabled)
	return nil
}

type scheduleWindowValues []string

func (v *scheduleWindowValues) String() string { return strings.Join(*v, ",") }

func (v *scheduleWindowValues) Set(value string) error {
	*v = append(*v, value)
	return nil
}

func parseScheduleWindow(value string) (plant.ActuatorScheduleWindow, error) {
	start, end, ok := strings.Cut(value, "-")
	if !ok {
		return plant.ActuatorScheduleWindow{}, errors.New("window must be HH:MM-HH:MM")
	}
	startMinute, err := parseClockMinute(start)
	if err != nil {
		return plant.ActuatorScheduleWindow{}, err
	}
	endMinute, err := parseClockMinute(end)
	if err != nil {
		return plant.ActuatorScheduleWindow{}, err
	}
	return plant.ActuatorScheduleWindow{StartMinute: startMinute, EndMinute: endMinute}, nil
}

func formatAgentScheduleWindows(windows []plant.ActuatorScheduleWindow) string {
	formatted := make([]string, 0, len(windows))
	for _, window := range windows {
		formatted = append(formatted, fmt.Sprintf("%02d:%02d-%02d:%02d",
			window.StartMinute/60, window.StartMinute%60,
			window.EndMinute/60, window.EndMinute%60))
	}
	return strings.Join(formatted, ",")
}

func (d Deps) assignedLight(ctx context.Context, slug, idRaw string) (plant.Plant, plant.Actuator, error) {
	p, err := d.lookUp(ctx, slug)
	if err != nil {
		return plant.Plant{}, plant.Actuator{}, err
	}
	id, err := uuid.Parse(idRaw)
	if err != nil {
		return plant.Plant{}, plant.Actuator{}, errors.New("--id must be an allowlisted actuator UUID")
	}
	actuator, err := d.Store.Actuator(ctx, id)
	if err != nil {
		return plant.Plant{}, plant.Actuator{}, err
	}
	if actuator.Kind != plant.ActuatorLight {
		return plant.Plant{}, plant.Actuator{}, errors.New("the selected actuator is not a light")
	}
	if !slices.Contains(actuator.PlantIDs, p.ID) {
		return plant.Plant{}, plant.Actuator{}, fmt.Errorf("light %s is not assigned to %s", id, p.CommonName)
	}
	return p, actuator, nil
}

func parseClockMinute(value string) (int, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, errors.New("time must be HH:MM")
	}
	hour, hourErr := strconv.Atoi(parts[0])
	minute, minuteErr := strconv.Atoi(parts[1])
	if hourErr != nil || minuteErr != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, errors.New("time must be HH:MM")
	}
	return hour*60 + minute, nil
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
