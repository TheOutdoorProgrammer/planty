package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

const stamp = "2006-01-02 15:04"

func (d Deps) plants(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("plants")
	domain := set.String("domain", "", "houseplant, edible_indoor or edible_outdoor")
	steward := set.String("steward", "", "whose plants")
	status := set.String("status", "", "alive, struggling, dormant, dead or gone")
	watering := set.String("watering", "", "letpot or hand")
	archived := set.Bool("archived", false, "include archived plants")
	if err := set.Parse(args); err != nil {
		return err
	}

	list, err := d.Store.ListPlants(ctx, store.PlantFilter{
		Domain:          plant.Domain(*domain),
		Steward:         *steward,
		Status:          plant.Status(*status),
		WateringMethod:  plant.WateringMethod(*watering),
		IncludeArchived: *archived,
	})
	if err != nil {
		return err
	}
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "no plants matched")
		return nil
	}

	for _, p := range list {
		extra := ""
		if p.ShelteredAt != nil {
			extra += ", sheltered indoors"
		}
		if p.ArchivedAt != nil {
			extra += ", archived"
		}
		_, _ = fmt.Fprintf(out, "%s: %s (%s, %s) at %q, watering %s%s\n",
			p.Slug, p.CommonName, p.Domain, p.Status, p.Location, p.WateringMethod, extra)
	}
	_, _ = fmt.Fprintf(out, "%d plants\n", len(list))
	return nil
}

func (d Deps) show(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("show")
	slug := set.String("plant", "", "the plant's slug")
	if err := set.Parse(args); err != nil {
		return err
	}
	p, err := d.lookUp(ctx, *slug)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(out, "%s (%s): %s, %s, steward %s\n",
		p.CommonName, p.Slug, p.Domain, p.Status, p.Steward)
	if p.BotanicalName != "" || p.Variety != "" {
		_, _ = fmt.Fprintf(out, "botanical: %s %s\n",
			p.BotanicalName, strings.TrimSpace(p.Variety))
	}

	place := fmt.Sprintf("location: %q, %s to reach", p.Location, p.Accessibility)
	if p.HAArea != "" {
		place += fmt.Sprintf(", HA area %q", p.HAArea)
	}
	if p.ShelteredAt != nil {
		place += ", sheltered indoors since " + p.ShelteredAt.Format(stamp)
	}
	_, _ = fmt.Fprintln(out, place)

	care := "watering: " + string(p.WateringMethod)
	if p.LetPotDripper != nil {
		care += fmt.Sprintf(" (dripper %d)", *p.LetPotDripper)
	}
	if p.LightExposure != "" {
		care += ", light " + string(p.LightExposure)
	}
	if p.MinTempF != nil {
		care += fmt.Sprintf(", protect below %gF", *p.MinTempF)
	}
	_, _ = fmt.Fprintln(out, care)

	if pot := describePot(p); pot != "" {
		_, _ = fmt.Fprintln(out, pot)
	}
	if p.AcquiredAt != nil {
		_, _ = fmt.Fprintf(out, "acquired: %s\n", p.AcquiredAt.Format("2006-01-02"))
	}
	for _, note := range []struct{ label, text string }{
		{"care notes", p.CareProfile.Notes},
		{"owner says", p.CareProfile.OwnerSays},
		{"quirks", p.CareProfile.Quirks},
	} {
		if note.text != "" {
			_, _ = fmt.Fprintf(out, "%s: %s\n", note.label, note.text)
		}
	}

	if at, err := d.Store.LastWatered(ctx, p.ID); err == nil {
		_, _ = fmt.Fprintf(out, "last watered: %s\n", at.Format(stamp))
	} else if errors.Is(err, store.ErrNotFound) {
		_, _ = fmt.Fprintln(out, "last watered: never recorded")
	} else {
		return err
	}

	if err := d.printReadings(ctx, out, p); err != nil {
		return err
	}

	if v, err := d.Store.LatestVerdict(ctx, p.ID); err == nil {
		acked := ""
		if v.AcknowledgedAt != nil {
			acked = ", acknowledged"
		}
		_, _ = fmt.Fprintf(out, "verdict for %s: %s — %s (confidence %.2f%s)\n",
			v.ForDate.Format("2006-01-02"), v.Action, v.Reasoning, v.Confidence, acked)
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}

	_, err = d.printReminders(ctx, out, p)
	return err
}

func describePot(p plant.Plant) string {
	var parts []string
	if p.PotSizeIn != nil {
		parts = append(parts, fmt.Sprintf("%g in", *p.PotSizeIn))
	}
	if p.PotMaterial != "" {
		parts = append(parts, p.PotMaterial)
	}
	if p.HasDrainage != nil {
		if *p.HasDrainage {
			parts = append(parts, "with drainage")
		} else {
			parts = append(parts, "NO drainage")
		}
	}
	if p.SoilMix != "" {
		parts = append(parts, "soil "+p.SoilMix)
	}
	if len(parts) == 0 {
		return ""
	}
	return "pot: " + strings.Join(parts, ", ")
}

func (d Deps) printReadings(ctx context.Context, out io.Writer, p plant.Plant) error {
	links, err := d.Store.SensorLinks(ctx, &p.ID)
	if err != nil {
		return err
	}
	for _, link := range links {
		_, _ = fmt.Fprintf(out, "probe %s (%s): %s\n",
			link.HAEntityID, link.Role, d.readingLine(ctx, link))
	}
	return nil
}

// readingLine says what one probe knows, in that probe's own terms: a raw
// number without its baselines is deliberately reported as meaning nothing.
func (d Deps) readingLine(ctx context.Context, link plant.SensorLink) string {
	latest, err := d.Store.LatestReading(ctx, link.ID)
	if errors.Is(err, store.ErrNotFound) {
		return "no readings yet"
	}
	if err != nil {
		return "readings unavailable"
	}

	line := fmt.Sprintf("%g raw at %s", latest.Value, latest.TakenAt.Format(stamp))
	if fraction, err := link.Fraction(latest.Value); err == nil {
		line += fmt.Sprintf(", %.0f%% of its range", fraction*100)
	} else {
		line += ", not calibrated so the number means nothing yet"
	}
	return line
}

// printReminders lists a plant's reminders and reports how many there were.
func (d Deps) printReminders(ctx context.Context, out io.Writer, p plant.Plant) (int, error) {
	reminders, err := d.Store.Reminders(ctx, p.ID)
	if err != nil {
		return 0, err
	}
	now := time.Now()
	for _, r := range reminders {
		var lastDone *time.Time
		if at, err := d.Store.LastObserved(ctx, p.ID, r.Kind); err == nil {
			lastDone = &at
		} else if !errors.Is(err, store.ErrNotFound) {
			return 0, err
		}

		line := fmt.Sprintf("reminder: %s %s", r.Kind, describe(r))
		if r.Note != "" {
			line += fmt.Sprintf(" (%s)", r.Note)
		}
		if !r.Active {
			line += " — inactive"
		} else if r.Due(lastDone, now) {
			line += " — DUE NOW"
		}
		_, _ = fmt.Fprintln(out, line)
	}
	return len(reminders), nil
}

func (d Deps) observations(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("observations")
	slug := set.String("plant", "", "the plant's slug")
	limit := set.Int("limit", 20, "how many to show")
	if err := set.Parse(args); err != nil {
		return err
	}
	p, err := d.lookUp(ctx, *slug)
	if err != nil {
		return err
	}

	history, err := d.Store.Observations(ctx, p.ID, *limit)
	if err != nil {
		return err
	}
	if len(history) == 0 {
		_, _ = fmt.Fprintf(out, "nothing has been recorded for %s yet\n", p.CommonName)
		return nil
	}
	for _, o := range history {
		who := string(o.Source)
		if o.Actor != "" {
			who += ", " + o.Actor
		}
		line := fmt.Sprintf("%s  %s (%s)", o.OccurredAt.Format(stamp), o.Kind, who)
		if o.Body != "" {
			line += ": " + o.Body
		}
		_, _ = fmt.Fprintln(out, line)
	}
	return nil
}

func (d Deps) reminders(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("reminders")
	slug := set.String("plant", "", "the plant's slug")
	if err := set.Parse(args); err != nil {
		return err
	}
	p, err := d.lookUp(ctx, *slug)
	if err != nil {
		return err
	}

	listed, err := d.printReminders(ctx, out, p)
	if err != nil {
		return err
	}
	if listed == 0 {
		_, _ = fmt.Fprintf(out, "no reminders are set for %s\n", p.CommonName)
	}
	return nil
}

func (d Deps) sensors(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("sensors")
	slug := set.String("plant", "", "narrow to one plant's probes")
	if err := set.Parse(args); err != nil {
		return err
	}

	var plantID *uuid.UUID
	if *slug != "" {
		p, err := d.lookUp(ctx, *slug)
		if err != nil {
			return err
		}
		plantID = &p.ID
	}

	links, err := d.Store.SensorLinks(ctx, plantID)
	if err != nil {
		return err
	}
	if len(links) == 0 {
		_, _ = fmt.Fprintln(out, "no sensors are linked")
		return nil
	}

	names, err := d.plantNames(ctx)
	if err != nil {
		return err
	}
	for _, link := range links {
		subject := fmt.Sprintf("zone %q", link.Zone)
		if link.PlantID != nil {
			subject = names[*link.PlantID]
		}
		calibration := "NOT calibrated, so it cannot drive watering"
		if link.Calibrated() {
			calibration = fmt.Sprintf("calibrated dry %g wet %g", *link.DryBaseline, *link.WetBaseline)
		}
		_, _ = fmt.Fprintf(out, "%s (%s) watching %s; %s; latest: %s\n",
			link.HAEntityID, link.Role, subject, calibration, d.readingLine(ctx, link))
	}
	return nil
}

// plantNames maps ids to printable names for rows that carry only a plant id.
func (d Deps) plantNames(ctx context.Context) (map[uuid.UUID]string, error) {
	all, err := d.Store.ListPlants(ctx, store.PlantFilter{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	names := make(map[uuid.UUID]string, len(all))
	for _, p := range all {
		names[p.ID] = fmt.Sprintf("%s (%s)", p.CommonName, p.Slug)
	}
	return names, nil
}

func (d Deps) today(ctx context.Context, out io.Writer, args []string) error {
	if err := newFlags("today").Parse(args); err != nil {
		return err
	}

	digest, err := d.Store.Digest(ctx, plant.StaleAfter)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "%d plants on the books\n", digest.Checked)

	switch {
	case digest.NeverRun:
		_, _ = fmt.Fprintln(out,
			"the daily judgment has never run, so silence here means unknown, not calm")
	case digest.StaleSince != nil:
		_, _ = fmt.Fprintf(out,
			"verdicts are stale: the newest is from %s\n", digest.StaleSince.Format(stamp))
	}

	if digest.AllClear() {
		_, _ = fmt.Fprintln(out, "all clear: nothing needs doing")
		return nil
	}
	for _, entry := range digest.Entries {
		_, _ = fmt.Fprintf(out, "%s: %s (%s) — %s\n",
			entry.Verdict.Action, entry.Plant.CommonName, entry.Plant.Slug, entry.Verdict.Reasoning)
	}
	return nil
}

func (d Deps) questions(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("questions")
	of := set.String("of", "", "only questions for this person")
	all := set.Bool("all", false, "include answered and dropped questions")
	if err := set.Parse(args); err != nil {
		return err
	}

	status := plant.QuestionOpen
	if *all {
		status = ""
	}
	list, err := d.Store.Questions(ctx, *of, status)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "no questions are waiting")
		return nil
	}

	names, err := d.plantNames(ctx)
	if err != nil {
		return err
	}
	for _, q := range list {
		about := ""
		if q.PlantID != nil {
			about = " about " + names[*q.PlantID]
		}
		line := fmt.Sprintf("%s: for %s%s — %q", q.ID, q.AskedOf, about, q.Question)
		if q.Why != "" {
			line += " (why: " + q.Why + ")"
		}
		if q.Status != plant.QuestionOpen {
			line += fmt.Sprintf(" [%s: %s]", q.Status, q.Answer)
		}
		_, _ = fmt.Fprintln(out, line)
	}
	return nil
}

func (d Deps) coldwatch(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("coldwatch")
	low := set.Float64("low", 0, "tonight's forecast low, Fahrenheit")
	if err := set.Parse(args); err != nil {
		return err
	}
	if !given(set)["low"] {
		return errors.New("what is tonight's low? pass --low <fahrenheit>")
	}

	exposed, err := d.Store.ColdWatch(ctx, *low)
	if err != nil {
		return err
	}
	if len(exposed) == 0 {
		_, _ = fmt.Fprintf(out, "nothing needs protecting at %gF\n", *low)
		return nil
	}
	for _, p := range exposed {
		state := ""
		if p.ShelteredAt != nil {
			state = " — already indoors"
		}
		_, _ = fmt.Fprintf(out, "%s (%s): protect below %gF%s\n",
			p.CommonName, p.Slug, *p.MinTempF, state)
	}
	_, _ = fmt.Fprintf(out, "%d plants are at risk at %gF\n", len(exposed), *low)
	return nil
}
