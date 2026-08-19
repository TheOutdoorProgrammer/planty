// Package agent is the only thing the model is allowed to do to a plant.
//
// It exists because "let it run planty" is too much: the CLI can run the pump,
// migrate the schema, and start another judgment that spends more tokens. This
// is a deliberately short list of the things worth saying to a plant app that
// are also safe to say to a model.
package agent

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// Usage is handed to the model so it knows what it may do. It is the same text
// the command prints when asked for help, so the two cannot drift.
const Usage = `planty agent <verb>

  log     record something that was done to a plant
          planty agent log --plant <slug> --kind <kind> [--note <text>]
          kinds: watered, misted, fertilized, pruned, repotted, moved, symptom, note

  remind  set or replace a reminder for one plant
          planty agent remind --plant <slug> --kind <kind> [--every-days N] [--at 8,20]
          kinds: watered, misted, fertilized, pruned, repotted

  forget  stop reminding about one kind
          planty agent forget --plant <slug> --kind <kind>`

// Run performs one verb. Anything not listed is refused by name rather than
// falling through to the rest of the CLI.
func Run(ctx context.Context, db *store.Store, out io.Writer, args []string) error {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(out, Usage)
		return errors.New("no verb given")
	}

	switch args[0] {
	case "log":
		return logObservation(ctx, db, out, args[1:])
	case "remind":
		return setReminder(ctx, db, out, args[1:])
	case "forget":
		return forgetReminder(ctx, db, out, args[1:])
	case "help", "-h", "--help":
		_, _ = fmt.Fprintln(out, Usage)
		return nil
	default:
		_, _ = fmt.Fprintln(out, Usage)
		return fmt.Errorf("there is no agent verb %q", args[0])
	}
}

func logObservation(ctx context.Context, db *store.Store, out io.Writer, args []string) error {
	set := flag.NewFlagSet("log", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	slug := set.String("plant", "", "the plant's slug")
	kind := set.String("kind", "", "what was done")
	note := set.String("note", "", "anything worth remembering")
	if err := set.Parse(args); err != nil {
		return err
	}

	p, err := lookUp(ctx, db, *slug)
	if err != nil {
		return err
	}
	observation := plant.Observation{
		PlantID: p.ID,
		Kind:    plant.ObservationKind(*kind),
		Body:    *note,
		// Recorded as the agent, never as the app: who said a thing happened is
		// the first question asked when the record turns out to be wrong.
		Source: plant.SourceAgent,
		Actor:  "planty agent",
	}

	saved, err := db.AddObservation(ctx, observation)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "recorded: %s %s for %s\n", saved.Kind, saved.OccurredAt.Format("15:04"), p.CommonName)
	return nil
}

func setReminder(ctx context.Context, db *store.Store, out io.Writer, args []string) error {
	set := flag.NewFlagSet("remind", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	slug := set.String("plant", "", "the plant's slug")
	kind := set.String("kind", "", "what to be reminded about")
	everyDays := set.Int("every-days", 1, "how often a day qualifies")
	at := set.String("at", "", "hours of the day, comma separated")
	note := set.String("note", "", "anything worth remembering")
	if err := set.Parse(args); err != nil {
		return err
	}

	p, err := lookUp(ctx, db, *slug)
	if err != nil {
		return err
	}
	hours, err := parseHours(*at)
	if err != nil {
		return err
	}

	saved, err := db.SaveReminder(ctx, plant.Reminder{
		PlantID:   p.ID,
		Kind:      plant.ObservationKind(*kind),
		EveryDays: *everyDays,
		AtHours:   hours,
		Active:    true,
		Note:      *note,
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "reminder set: %s, %s\n", saved.Kind, describe(saved))
	return nil
}

func forgetReminder(ctx context.Context, db *store.Store, out io.Writer, args []string) error {
	set := flag.NewFlagSet("forget", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	slug := set.String("plant", "", "the plant's slug")
	kind := set.String("kind", "", "which reminder to drop")
	if err := set.Parse(args); err != nil {
		return err
	}

	p, err := lookUp(ctx, db, *slug)
	if err != nil {
		return err
	}
	if err := db.DeleteReminder(ctx, p.ID, plant.ObservationKind(*kind)); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "reminder removed: %s for %s\n", *kind, p.CommonName)
	return nil
}

func lookUp(ctx context.Context, db *store.Store, slug string) (plant.Plant, error) {
	if slug == "" {
		return plant.Plant{}, errors.New("which plant? pass --plant <slug>")
	}
	return db.GetPlant(ctx, slug)
}

// parseHours reads "8,20". Empty means the reminder's own default rather than
// an error, so "remind me to mist this" works without naming a time.
func parseHours(raw string) ([]int, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	var hours []int
	for _, field := range strings.Split(raw, ",") {
		hour, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil {
			return nil, fmt.Errorf("%q is not an hour", field)
		}
		hours = append(hours, hour)
	}
	return hours, nil
}

func describe(r plant.Reminder) string {
	hours := make([]string, 0, len(r.AtHours))
	for _, hour := range r.AtHours {
		hours = append(hours, fmt.Sprintf("%02d:00", hour))
	}
	if r.EveryDays == 1 {
		return "every day at " + strings.Join(hours, " and ")
	}
	return fmt.Sprintf("every %d days at %s", r.EveryDays, strings.Join(hours, " and "))
}
