package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

func (d Deps) create(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("create")
	name := set.String("name", "", "what to call it")
	location := set.String("location", "", "where it lives")
	domain := set.String("domain", "", "houseplant, edible_indoor or edible_outdoor")
	steward := set.String("steward", "", "who it belongs to")
	botanical := set.String("botanical", "", "botanical name")
	variety := set.String("variety", "", "cultivar")
	watering := set.String("watering", "", "letpot or hand")
	dripper := set.Int("dripper", 0, "which LetPot dripper")
	access := set.String("accessibility", "", "easy, awkward or hard")
	light := set.String("light", "", "direct, bright_indirect, medium or low")
	potSize := set.Float64("pot-size", 0, "pot diameter in inches")
	potMaterial := set.String("pot-material", "", "terracotta, plastic, ...")
	drainage := set.Bool("drainage", false, "whether the pot has a drainage hole")
	soil := set.String("soil", "", "the soil mix")
	minTemp := set.Float64("min-temp", 0, "protect below this, Fahrenheit")
	acquired := set.String("acquired", "", "when it arrived")
	notes := set.String("notes", "", "care notes worth keeping")
	if err := parse(set, args); err != nil {
		return err
	}
	if *name == "" {
		return errors.New("a plant needs a name: pass --name")
	}

	p := plant.Plant{
		CommonName:     *name,
		Location:       *location,
		Domain:         plant.Domain(*domain),
		Steward:        *steward,
		BotanicalName:  *botanical,
		Variety:        *variety,
		WateringMethod: plant.WateringMethod(*watering),
		Accessibility:  plant.Accessibility(*access),
		PotMaterial:    *potMaterial,
		SoilMix:        *soil,
	}
	// The same defaults the API applies to a sparse create, so a plant named in
	// one sentence still validates. (Mirrors api.applyPlantDefaults, which is
	// unexported there.)
	if p.Domain == "" {
		p.Domain = plant.DomainHouseplant
	}
	p.Status = plant.StatusAlive
	if p.Steward == "" {
		p.Steward = plant.StewardSelf
	}
	if p.Accessibility == "" {
		p.Accessibility = plant.AccessEasy
	}
	if p.WateringMethod == "" {
		p.WateringMethod = plant.WateringHand
	}

	seen := given(set)
	if seen["dripper"] {
		p.LetPotDripper = dripper
	}
	if seen["pot-size"] {
		p.PotSizeIn = potSize
	}
	if seen["drainage"] {
		p.HasDrainage = drainage
	}
	if seen["min-temp"] {
		p.MinTempF = minTemp
	}
	if seen["light"] {
		l, err := lightExposure(*light)
		if err != nil {
			return err
		}
		p.LightExposure = l
	}
	if seen["acquired"] {
		at, err := parseWhen(*acquired)
		if err != nil {
			return err
		}
		p.AcquiredAt = &at
	}
	if *notes != "" {
		p.CareProfile.Notes = *notes
	}

	slug, err := d.Store.FreeSlug(ctx, *name)
	if err != nil {
		return err
	}
	p.Slug = slug

	created, err := d.Store.CreatePlant(ctx, p)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "created %s (%s): %s, watering %s, at %q\n",
		created.CommonName, created.Slug, created.Domain, created.WateringMethod, created.Location)
	return nil
}

func (d Deps) update(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("update")
	slug := set.String("plant", "", "the plant's slug")
	name := set.String("name", "", "what to call it")
	location := set.String("location", "", "where it lives")
	steward := set.String("steward", "", "who it belongs to")
	status := set.String("status", "", "alive, struggling, dormant, dead or gone")
	domain := set.String("domain", "", "houseplant, edible_indoor or edible_outdoor")
	botanical := set.String("botanical", "", "botanical name")
	variety := set.String("variety", "", "cultivar")
	haArea := set.String("ha-area", "", "the Home Assistant area")
	watering := set.String("watering", "", "letpot or hand")
	dripper := set.Int("dripper", 0, "which LetPot dripper")
	access := set.String("accessibility", "", "easy, awkward or hard")
	light := set.String("light", "", "direct, bright_indirect, medium or low")
	potSize := set.Float64("pot-size", 0, "pot diameter in inches")
	potMaterial := set.String("pot-material", "", "terracotta, plastic, ...")
	drainage := set.Bool("drainage", false, "whether the pot has a drainage hole")
	soil := set.String("soil", "", "the soil mix")
	minTemp := set.Float64("min-temp", 0, "protect below this, Fahrenheit")
	notes := set.String("notes", "", "care notes; replaces the existing notes")
	if err := parse(set, args); err != nil {
		return err
	}

	p, err := d.lookUp(ctx, *slug)
	if err != nil {
		return err
	}

	seen := given(set)
	delete(seen, "plant")
	if len(seen) == 0 {
		return errors.New("nothing to change: pass at least one field flag")
	}

	// candidate carries the patch applied to the stored record, so the whole
	// result is validated by the one plant.Valid the app also goes through.
	candidate := p
	patch := store.PlantPatch{}
	var changed []string

	for flagName, apply := range map[string]func() error{
		"name": func() error {
			candidate.CommonName = *name
			patch.CommonName = name
			return nil
		},
		"location": func() error {
			candidate.Location = *location
			patch.Location = location
			return nil
		},
		"steward": func() error {
			candidate.Steward = *steward
			patch.Steward = steward
			return nil
		},
		"status": func() error {
			s := plant.Status(*status)
			candidate.Status = s
			patch.Status = &s
			return nil
		},
		"domain": func() error {
			dom := plant.Domain(*domain)
			candidate.Domain = dom
			patch.Domain = &dom
			return nil
		},
		"botanical": func() error {
			candidate.BotanicalName = *botanical
			patch.BotanicalName = botanical
			return nil
		},
		"variety": func() error {
			candidate.Variety = *variety
			patch.Variety = variety
			return nil
		},
		"ha-area": func() error {
			candidate.HAArea = *haArea
			patch.HAArea = haArea
			return nil
		},
		"watering": func() error {
			w := plant.WateringMethod(*watering)
			candidate.WateringMethod = w
			patch.WateringMethod = &w
			if w == plant.WateringHand {
				candidate.LetPotDripper = nil
			}
			return nil
		},
		"dripper": func() error {
			candidate.LetPotDripper = dripper
			patch.LetPotDripper = dripper
			return nil
		},
		"accessibility": func() error {
			a := plant.Accessibility(*access)
			candidate.Accessibility = a
			patch.Accessibility = &a
			return nil
		},
		"light": func() error {
			l, err := lightExposure(*light)
			if err != nil {
				return err
			}
			candidate.LightExposure = l
			patch.LightExposure = &l
			return nil
		},
		"pot-size": func() error {
			candidate.PotSizeIn = potSize
			patch.PotSizeIn = potSize
			return nil
		},
		"pot-material": func() error {
			candidate.PotMaterial = *potMaterial
			patch.PotMaterial = potMaterial
			return nil
		},
		"drainage": func() error {
			candidate.HasDrainage = drainage
			patch.HasDrainage = drainage
			return nil
		},
		"soil": func() error {
			candidate.SoilMix = *soil
			patch.SoilMix = soil
			return nil
		},
		"min-temp": func() error {
			candidate.MinTempF = minTemp
			patch.MinTempF = minTemp
			return nil
		},
		"notes": func() error {
			profile := p.CareProfile
			profile.Notes = *notes
			candidate.CareProfile = profile
			patch.CareProfile = &profile
			return nil
		},
	} {
		if !seen[flagName] {
			continue
		}
		if err := apply(); err != nil {
			return err
		}
		changed = append(changed, flagName)
	}

	if err := candidate.Valid(); err != nil {
		return err
	}

	updated, err := d.Store.UpdatePlant(ctx, p.Slug, patch)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("%s is archived, so it cannot be updated", p.CommonName)
	}
	if err != nil {
		return err
	}

	// Map iteration shuffles the field order; sorted, the confirmation is stable.
	slices.Sort(changed)
	_, _ = fmt.Fprintf(out, "updated %s (%s): %s\n",
		updated.CommonName, updated.Slug, strings.Join(changed, ", "))
	return nil
}

func (d Deps) archive(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("archive")
	slug := set.String("plant", "", "the plant's slug")
	status := set.String("status", string(plant.StatusGone), "dead or gone")
	if err := parse(set, args); err != nil {
		return err
	}

	p, err := d.lookUp(ctx, *slug)
	if err != nil {
		return err
	}
	final := plant.Status(*status)
	if err := final.ValidateArchive(); err != nil {
		return err
	}

	err = d.Store.ArchivePlant(ctx, p.Slug, final)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("%s is already archived", p.CommonName)
	}
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "archived %s (%s) as %s; its history is kept\n",
		p.CommonName, p.Slug, final)
	return nil
}

func (d Deps) harvest(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("harvest")
	slug := set.String("plant", "", "the plant's slug")
	quantity := set.Float64("quantity", 0, "how much came off")
	unit := set.String("unit", "", "fruit, g, oz, ...")
	note := set.String("note", "", "anything worth remembering")
	when := set.String("when", "", "when it happened; empty means now")
	if err := parse(set, args); err != nil {
		return err
	}

	p, err := d.lookUp(ctx, *slug)
	if err != nil {
		return err
	}
	seen := given(set)
	if !seen["quantity"] || *unit == "" {
		return errors.New("a harvest needs --quantity and --unit, so seasons can be added up")
	}
	occurred, err := parseWhen(*when)
	if err != nil {
		return err
	}

	saved, err := d.Store.AddHarvest(ctx, plant.Harvest{
		PlantID:    p.ID,
		Quantity:   *quantity,
		Unit:       *unit,
		Notes:      *note,
		OccurredAt: occurred,
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "recorded harvest from %s: %g %s\n",
		p.CommonName, saved.Quantity, saved.Unit)
	return nil
}

// water runs the same surveyed, verified LetPot pass as `planty water`. The
// decisions stay in the job: this verb adds no way to force the pump on.
func (d Deps) water(ctx context.Context, out io.Writer, args []string) error {
	if err := parse(newFlags("water"), args); err != nil {
		return err
	}
	if d.Water == nil {
		return errors.New("watering is not wired up in this process, so nothing was run")
	}
	// Counted first, because "the pass completed" reads as "water moved" and a
	// garden with nothing on the line completes without watering anything.
	onLine := -1
	if d.Store != nil {
		listed, err := d.Store.ListPlants(ctx, store.PlantFilter{
			Status:         plant.StatusAlive,
			WateringMethod: plant.WateringLetPot,
		})
		if err != nil {
			return err
		}
		onLine = len(listed)
	}

	if onLine == 0 {
		_, _ = fmt.Fprintln(out, "nothing is on the LetPot line, so there was nothing to water; "+
			"every plant here is watered by hand")
		return nil
	}

	if err := d.Water(ctx); err != nil {
		return err
	}
	subject := "the plants on the line"
	if onLine > 0 {
		subject = fmt.Sprintf("%d plant(s) on the line", onLine)
	}
	_, _ = fmt.Fprintf(out, "the LetPot pass ran over %s. It waters only where a calibrated "+
		"probe reads dry and nothing is already soaked, and it checks afterwards that water "+
		"reached the soil, so it may well have watered none of them\n", subject)
	return nil
}

func (d Deps) shelter(ctx context.Context, out io.Writer, args []string) error {
	return d.moveInOrOut(ctx, out, args, true)
}

func (d Deps) unshelter(ctx context.Context, out io.Writer, args []string) error {
	return d.moveInOrOut(ctx, out, args, false)
}

func (d Deps) moveInOrOut(ctx context.Context, out io.Writer, args []string, indoors bool) error {
	verb := "unshelter"
	if indoors {
		verb = "shelter"
	}
	set := newFlags(verb)
	list := set.String("plant", "", "comma-separated slugs")
	all := set.Bool("all", false, "every plant it applies to")
	if err := parse(set, args); err != nil {
		return err
	}

	slugs := splitSlugs(*list)
	if *all == (len(slugs) > 0) {
		return fmt.Errorf("say which plants: --plant <slug,slug> or --all, not both and not neither")
	}

	var moved int64
	var err error
	switch {
	case *all && indoors:
		moved, err = d.Store.ShelterAll(ctx)
	case *all:
		moved, err = d.Store.UnshelterAll(ctx)
	case indoors:
		moved, err = d.Store.Shelter(ctx, slugs)
	default:
		moved, err = d.Store.Unshelter(ctx, slugs)
	}
	if err != nil {
		return err
	}

	direction := "back outside"
	if indoors {
		direction = "indoors"
	}
	if *all {
		if moved == 0 {
			_, _ = fmt.Fprintf(out, "nothing needed recording: no plant was left to move %s\n", direction)
			return nil
		}
		_, _ = fmt.Fprintf(out, "recorded %d plants as %s\n", moved, direction)
		return nil
	}
	if int(moved) < len(slugs) {
		_, _ = fmt.Fprintf(out,
			"recorded %d of %d as %s; the rest are unknown, archived, or already there\n",
			moved, len(slugs), direction)
		return nil
	}
	_, _ = fmt.Fprintf(out, "recorded %s as %s\n", strings.Join(slugs, ", "), direction)
	return nil
}

func splitSlugs(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (d Deps) link(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("link")
	slug := set.String("plant", "", "the plant this probe watches")
	zone := set.String("zone", "", "or the zone it serves")
	entity := set.String("entity", "", "the Home Assistant entity id")
	role := set.String("role", "", "soil_moisture, ambient_temp, ambient_humidity or illuminance")
	if err := parse(set, args); err != nil {
		return err
	}
	if *slug != "" && *zone != "" {
		return errors.New("a sensor watches a plant or a zone, not both")
	}

	l := plant.SensorLink{
		Zone:       *zone,
		HAEntityID: *entity,
		Role:       plant.SensorRole(*role),
	}
	subject := fmt.Sprintf("zone %q", *zone)
	if *slug != "" {
		p, err := d.lookUp(ctx, *slug)
		if err != nil {
			return err
		}
		l.PlantID = &p.ID
		subject = fmt.Sprintf("%s (%s)", p.CommonName, p.Slug)
	}

	saved, err := d.Store.LinkSensor(ctx, l)
	if err != nil {
		return err
	}
	line := fmt.Sprintf("linked %s as %s for %s", saved.HAEntityID, saved.Role, subject)
	if !saved.Calibrated() {
		line += "; calibrate it before its readings can drive anything"
	}
	_, _ = fmt.Fprintln(out, line)
	return nil
}

func (d Deps) calibrate(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("calibrate")
	entity := set.String("entity", "", "the Home Assistant entity id")
	role := set.String("role", "", "needed only when one entity holds several roles")
	dry := set.Float64("dry", 0, "the raw reading in bone dry soil")
	wet := set.Float64("wet", 0, "the raw reading just after saturating")
	if err := parse(set, args); err != nil {
		return err
	}
	if *entity == "" {
		return errors.New("which probe? pass --entity <ha_entity_id>")
	}
	seen := given(set)
	if !seen["dry"] || !seen["wet"] {
		return errors.New("calibration needs both --dry and --wet raw readings")
	}

	links, err := d.Store.SensorLinks(ctx, nil)
	if err != nil {
		return err
	}
	var matched []plant.SensorLink
	for _, l := range links {
		if l.HAEntityID != *entity {
			continue
		}
		if *role != "" && l.Role != plant.SensorRole(*role) {
			continue
		}
		matched = append(matched, l)
	}
	switch len(matched) {
	case 0:
		return fmt.Errorf("no sensor link exists for %q; link it first with planty agent link", *entity)
	case 1:
	default:
		return fmt.Errorf("%q is linked in %d roles; pass --role to say which", *entity, len(matched))
	}

	saved, err := d.Store.Calibrate(ctx, matched[0].ID, *dry, *wet)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "calibrated %s (%s): dry %g, wet %g; its readings mean something now\n",
		saved.HAEntityID, saved.Role, *saved.DryBaseline, *saved.WetBaseline)
	return nil
}

func (d Deps) ack(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("ack")
	slug := set.String("plant", "", "the plant's slug")
	if err := parse(set, args); err != nil {
		return err
	}
	p, err := d.lookUp(ctx, *slug)
	if err != nil {
		return err
	}

	v, err := d.Store.LatestVerdict(ctx, p.ID)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("there is no verdict for %s to acknowledge", p.CommonName)
	}
	if err != nil {
		return err
	}
	if v.AcknowledgedAt != nil {
		_, _ = fmt.Fprintf(out, "the %s verdict for %s was already acknowledged\n",
			v.Action, p.CommonName)
		return nil
	}

	if err := d.Store.AckVerdict(ctx, v.ID); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "acknowledged the %s verdict for %s; it will stop escalating\n",
		v.Action, p.CommonName)
	return nil
}

func (d Deps) ask(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("ask")
	slug := set.String("plant", "", "the plant it is about")
	question := set.String("question", "", "what only the owner can settle")
	why := set.String("why", "", "why it matters")
	of := set.String("of", "", "who to ask; defaults to the plant's steward")
	if err := parse(set, args); err != nil {
		return err
	}
	if *question == "" {
		return errors.New("what is the question? pass --question")
	}
	if err := notWhileTalking(*of); err != nil {
		return err
	}

	q := plant.Question{AskedOf: *of, Question: *question, Why: *why}
	if *slug != "" {
		p, err := d.lookUp(ctx, *slug)
		if err != nil {
			return err
		}
		q.PlantID = &p.ID
	}

	saved, err := d.Store.AskOwner(ctx, q)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "queued for %s: %q (id %s)\n", saved.AskedOf, saved.Question, saved.ID)
	return nil
}

func (d Deps) answer(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("answer")
	id := set.String("question", "", "the question's id, from planty agent questions")
	text := set.String("answer", "", "what the owner said")
	if err := parse(set, args); err != nil {
		return err
	}
	if *text == "" {
		return errors.New("what did they say? pass --answer")
	}
	qid, err := uuid.Parse(*id)
	if err != nil {
		return fmt.Errorf("%q is not a question id; planty agent questions lists them", *id)
	}

	saved, err := d.Store.AnswerQuestion(ctx, qid, *text)
	if errors.Is(err, store.ErrNotFound) {
		return errors.New("no question has that id; planty agent questions lists them")
	}
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "recorded the answer; %q is settled\n", saved.Question)
	return nil
}

func (d Deps) away(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("away")
	from := set.String("from", "", "when the period starts")
	until := set.String("until", "", "when it ends")
	contact := set.String("contact", "", "who can act meanwhile")
	notify := set.String("notify", "", "how to reach them, a notify service")
	note := set.String("note", "", "anything worth remembering")
	if err := parse(set, args); err != nil {
		return err
	}
	if *from == "" || *until == "" {
		return errors.New("an away period needs --from and --until")
	}
	starts, err := parseWhen(*from)
	if err != nil {
		return err
	}
	ends, err := parseWhen(*until)
	if err != nil {
		return err
	}

	saved, err := d.Store.GoAway(ctx, plant.AwayPeriod{
		StartsAt:      starts,
		EndsAt:        ends,
		BackupContact: *contact,
		BackupNotify:  *notify,
		Note:          *note,
	})
	if err != nil {
		return err
	}
	line := fmt.Sprintf("recorded away from %s until %s",
		saved.StartsAt.Format("2006-01-02"), saved.EndsAt.Format("2006-01-02"))
	if saved.BackupContact != "" {
		line += ", with " + saved.BackupContact + " as backup"
	}
	_, _ = fmt.Fprintln(out, line)
	return nil
}

// lightExposure checks the one enum plant.Valid leaves to the database, whose
// cast failure is not a sentence anyone can relay.
func lightExposure(raw string) (plant.LightExposure, error) {
	l := plant.LightExposure(raw)
	switch l {
	case plant.LightDirect, plant.LightBrightIndirect, plant.LightMedium, plant.LightLow:
		return l, nil
	}
	return "", fmt.Errorf("%q is not a light exposure; use direct, bright_indirect, medium or low", raw)
}
