package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// describeToxicity renders a rating for show. An unchecked plant says so in
// as many words, because a blank line here reads as reassurance.
func describeToxicity(t plant.Toxicity) string {
	if !t.Checked() {
		return "toxicity: nobody has looked this up. Do not tell anyone it is safe."
	}

	line := fmt.Sprintf("toxicity: cats %s, dogs %s, people %s",
		t.Cats, t.Dogs, t.People)
	if t.IdentifiedAs != "" {
		line += fmt.Sprintf(" (as %s)", t.IdentifiedAs)
	}
	if t.Basis == plant.BasisDerived {
		line += "; levels graded here rather than by the source"
	}
	for _, extra := range []struct{ label, value string }{
		{"principle", t.Principle},
		{"signs", t.Signs},
		{"parts", strings.Join(t.Parts, ", ")},
		{"routes", strings.Join(t.Routes, ", ")},
		{"notes", t.Notes},
		{"first aid", t.FirstAid},
		{"source", t.Source},
	} {
		if extra.value != "" {
			line += fmt.Sprintf("\n  %s: %s", extra.label, extra.value)
		}
	}
	return line
}

// toxicity records what a plant does to whatever eats it. Write only: reading
// it back is part of show, since a rating is meaningless without the plant it
// is about.
func (d Deps) toxicity(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("toxicity")
	slug := set.String("plant", "", "the plant's slug")
	cats := set.String("cats", "", "safe, mild, moderate, severe or unknown")
	dogs := set.String("dogs", "", "safe, mild, moderate, severe or unknown")
	people := set.String("people", "", "safe, mild, moderate, severe or unknown")
	basis := set.String("basis", "", "source if the reference stated this level, derived if you graded it")
	identified := set.String("identified-as", "", "the botanical name you looked up")
	principle := set.String("principle", "", "the toxic principle, e.g. insoluble calcium oxalates")
	signs := set.String("signs", "", "what you would see in an animal that ate it")
	parts := set.String("parts", "", "comma-separated: "+strings.Join(plant.PartNames, ", "))
	routes := set.String("routes", "", "comma-separated: "+strings.Join(plant.RouteNames, ", "))
	notes := set.String("notes", "", "why the audiences differ, dose caveats")
	firstAid := set.String("first-aid", "", "only when the obvious advice would be wrong")
	source := set.String("source", "", "the host you read it from")
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
		return errors.New("nothing to record: pass at least one field flag")
	}

	// Built on top of what is stored, so recording a first-aid note later does
	// not silently blank the ratings somebody already looked up.
	tox := p.Toxicity
	for flagName, apply := range map[string]func(){
		"cats":          func() { tox.Cats = plant.Harm(*cats) },
		"dogs":          func() { tox.Dogs = plant.Harm(*dogs) },
		"people":        func() { tox.People = plant.Harm(*people) },
		"basis":         func() { tox.Basis = plant.Basis(*basis) },
		"identified-as": func() { tox.IdentifiedAs = *identified },
		"principle":     func() { tox.Principle = *principle },
		"signs":         func() { tox.Signs = *signs },
		"parts":         func() { tox.Parts = splitSlugs(*parts) },
		"routes":        func() { tox.Routes = splitSlugs(*routes) },
		"notes":         func() { tox.Notes = *notes },
		"first-aid":     func() { tox.FirstAid = *firstAid },
		"source":        func() { tox.Source = *source },
	} {
		if seen[flagName] {
			apply()
		}
	}

	// An unrated plant that gains any of this has now been checked, and the
	// date is what separates "looked and found nothing" from "never looked".
	now := time.Now().UTC()
	tox.CheckedAt = &now

	if err := tox.Valid(); err != nil {
		return err
	}

	updated, err := d.Store.UpdatePlant(ctx, p.Slug, store.PlantPatch{Toxicity: &tox})
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(out, "%s: cats %s, dogs %s, people %s\n",
		updated.Slug, updated.Toxicity.Cats, updated.Toxicity.Dogs, updated.Toxicity.People)
	if updated.Toxicity.Diverges() {
		_, _ = fmt.Fprintln(out, "the audiences differ, which is worth saying out loud")
	}
	return nil
}
