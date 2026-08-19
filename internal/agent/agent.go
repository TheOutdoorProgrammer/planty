// Package agent is the only thing the model is allowed to do to the garden.
//
// It exists because "let it run planty" is too much: the CLI can migrate the
// schema, seed fixtures, and start another judgment that spends more tokens.
// This is the full set of things worth saying to a plant app that are also
// safe to say to a model — every capability the service has, except the ones
// that spawn another model run (autopsy, diagnosis, identify) and the ones
// that operate the deployment itself. Watering is here on purpose: the verb
// calls the same surveyed, verified LetPot pass the manual command runs, so
// the model can never pump into a pot a calibrated probe calls wet.
//
// Two layers hold it shut, per adr/0002: the Bash allowlist and the gate in
// this package. Neither knows about individual verbs, which is why Run refuses
// anything unlisted by name instead of falling through to the rest of the CLI.
package agent

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// Deps is what the verbs act through. Store is required. Water is the
// configured LetPot watering pass — the same one `planty water` runs — and may
// be nil in a process with no pump wired, where the verb refuses plainly.
type Deps struct {
	Store *store.Store
	Water func(context.Context) error
}

// Usage is the reference handed to the model and the text help prints, so the
// two cannot drift. Deliberately exhaustive — every verb, flag, valid value
// and an example each — so a model never has to explore.
const Usage = `planty agent <verb> --flag value

One verb per command. Flags are long-form; --flag value and --flag=value both
work, and a value with spaces goes in double quotes. Times are a date like
2026-08-17 or full RFC3339 like 2026-08-17T09:30:00Z. Slugs come from the
plants verb or from the conversation.

Reading the garden:

  plants      list plants; every flag narrows the list
              planty agent plants [--status <status>] [--domain <domain>]
                [--steward <name>] [--watering letpot|hand] [--archived]
              domains: houseplant, edible_indoor, edible_outdoor
              statuses: alive, struggling, dormant, dead, gone
              example: planty agent plants --status struggling

  show        one plant in full: record, care notes, last watered, probe
              readings, current verdict, reminders
              planty agent show --plant <slug>
              example: planty agent show --plant golden-pothos

  observations  a plant's history, newest first
              planty agent observations --plant <slug> [--limit N]
              example: planty agent observations --plant golden-pothos --limit 10

  reminders   what is set for one plant, and whether each is owed right now
              planty agent reminders --plant <slug>
              example: planty agent reminders --plant golden-pothos

  sensors     sensor links with calibration state and the latest reading
              planty agent sensors [--plant <slug>]
              example: planty agent sensors --plant golden-pothos

  today       the digest across every plant: what needs doing, and whether
              the data behind it is fresh; takes no flags
              example: planty agent today

  questions   questions waiting on an owner, with the ids answer needs
              planty agent questions [--of <person>] [--all]
              example: planty agent questions --of self

  coldwatch   which plants need bringing in if tonight reaches a given low
              planty agent coldwatch --low <fahrenheit>
              example: planty agent coldwatch --low 40

Recording what happened:

  log         record something done to or seen on a plant; --when says when it
              happened and omitting it means now
              planty agent log --plant <slug> --kind <kind> [--note <text>] [--when <time>]
              kinds: watered, misted, fertilized, pruned, repotted, moved,
              harvested, symptom, note, died
              example: planty agent log --plant golden-pothos --kind watered --note "about a cup"

  harvest     record a yield with a quantity, so seasons can be added up
              planty agent harvest --plant <slug> --quantity <n> --unit <text> [--note <text>] [--when <time>]
              example: planty agent harvest --plant cherry-tomato --quantity 6 --unit fruit

Reminders:

  remind      set or replace the reminder for one kind of chore; --at is hours
              of the day, 0 to 23; defaults are every day at 08:00
              planty agent remind --plant <slug> --kind <kind> [--every-days N] [--at 8,20] [--note <text>]
              kinds: watered, misted, fertilized, pruned, repotted
              example: planty agent remind --plant blue-oyster --kind misted --every-days 1 --at 8,20

  forget      stop reminding about one kind
              planty agent forget --plant <slug> --kind <kind>
              example: planty agent forget --plant golden-pothos --kind misted

The plants themselves:

  create      add a plant; only --name is required, and what is not given
              defaults to a hand-watered houseplant, alive, steward self,
              easy to reach; the slug is assigned and printed
              planty agent create --name <text> [--location <text>]
                [--domain <domain>] [--steward <name>] [--botanical <text>]
                [--variety <text>] [--watering letpot|hand] [--dripper N]
                [--accessibility easy|awkward|hard] [--light <light>]
                [--pot-size <inches>] [--pot-material <text>]
                [--drainage=true|false] [--soil <text>]
                [--min-temp <fahrenheit>] [--acquired <date>] [--notes <text>]
              light: direct, bright_indirect, medium, low
              example: planty agent create --name "Monstera" --location "west sill" --light bright_indirect

  update      change fields on one plant; only the flags you pass change
              planty agent update --plant <slug> [--name <text>]
                [--location <text>] [--steward <name>] [--status <status>]
                [--domain <domain>] [--botanical <text>] [--variety <text>]
                [--ha-area <area>] [--watering letpot|hand] [--dripper N]
                [--accessibility easy|awkward|hard] [--light <light>]
                [--pot-size <inches>] [--pot-material <text>]
                [--drainage=true|false] [--soil <text>]
                [--min-temp <fahrenheit>] [--notes <text>]
              example: planty agent update --plant golden-pothos --location "bedroom shelf" --light medium

  archive     retire a plant, keeping its whole history; never a delete
              planty agent archive --plant <slug> [--status dead|gone]
              example: planty agent archive --plant basil --status dead

Water and cold:

  water       run the LetPot watering pass, exactly as the manual command
              does: survey the calibrated probes, pump only if something reads
              dry and nothing on the line is soaked, then verify water
              arrived; no flags, and only when a person asked for it
              example: planty agent water

  shelter     record that plants came indoors ahead of cold; --plant takes a
              comma-separated list, or --all covers every plant with a cold
              threshold that is still outside
              planty agent shelter --plant <slug,slug> | --all
              example: planty agent shelter --all

  unshelter   record that plants went back outside
              planty agent unshelter --plant <slug,slug> | --all
              example: planty agent unshelter --plant meyer-lemon

Sensors:

  link        attach a Home Assistant entity to a plant, or to a zone
              planty agent link --entity <ha_entity_id> --role <role> --plant <slug>
              planty agent link --entity <ha_entity_id> --role <role> --zone <name>
              roles: soil_moisture (must name a plant), ambient_temp,
              ambient_humidity, illuminance
              example: planty agent link --plant golden-pothos --entity sensor.pothos_soil --role soil_moisture

  calibrate   record a probe's own dry and saturated raw readings; until it is
              calibrated it never drives a watering decision
              planty agent calibrate --entity <ha_entity_id> --dry <raw> --wet <raw> [--role <role>]
              example: planty agent calibrate --entity sensor.pothos_soil --dry 18 --wet 55

Verdicts and owners:

  ack         acknowledge a plant's current verdict, which stops it escalating
              planty agent ack --plant <slug>
              example: planty agent ack --plant golden-pothos

  ask         queue a question for a plant's owner; who it is asked of
              defaults to the plant's steward
              planty agent ask --plant <slug> --question <text> [--why <text>] [--of <person>]
              example: planty agent ask --plant fiddle-leaf --question "Has it ever been repotted?"

  answer      record what the owner said; ids come from the questions verb
              planty agent answer --question <id> --answer <text>
              example: planty agent answer --question 6f0dd1c2-6b3a-4e0e-9d3f-2a4b8c9d0e1f --answer "Repotted in March"

  notes       read what has been written down about a plant
              planty agent notes --plant <slug>
              example: planty agent notes --plant golden-pothos

  note        write, change or remove one note. --plant writes a new one, --id
              changes or removes an existing one; ids come from the notes verb
              planty agent note --plant <slug> --text <text> [--title <title>]
              planty agent note --id <id> [--text <text>] [--title <title>]
              planty agent note --id <id> --delete
              Changing only --text leaves the title alone, and the other way round.
              example: planty agent note --plant golden-pothos --text "the cat keeps chewing this one"

  attach      file a photograph the person just sent against one of their plants,
              so it joins that plant's history instead of being only a question.
              The id is given to you in the catalogue when a photograph is sent.
              Only for their own plants, and only for a picture not already filed.
              planty agent attach --plant <slug> --photo <id> [--caption <text>]
              example: planty agent attach --plant golden-pothos --photo 6f0dd1c2-6b3a-4e0e-9d3f-2a4b8c9d0e1f --caption "undersides of the leaves"

  toxicity    record what a plant does to whatever eats it
              planty agent toxicity --plant <slug> [--cats <level>] [--dogs <level>]
                [--people <level>] [--basis source|derived] [--identified-as <botanical name>]
                [--principle <text>] [--signs <text>] [--parts <a,b>] [--routes <a,b>]
                [--notes <text>] [--first-aid <text>] [--source <host>]
              levels: safe, mild, moderate, severe, unknown
                safe     no toxic principle reported
                mild     self-limiting; rinse the mouth and watch
                moderate systemic effects plausible; ring a vet
                severe   can kill from a household mouthful; go now
              parts: all, bulb, leaf, stem, sap, flower, fruit, seed, root
              routes: eaten, skin, eyes, breathed
              --basis source means the reference stated that level itself, derived
                means you graded it. The ASPCA only publishes toxic or non-toxic,
                so anything finer than that is derived, and saying so is required.
              --identified-as is the botanical name you actually looked up, and
                matters more than any other field here: "lily" is six unrelated
                plants and two of them kill cats. Never rate from a common name.
              --principle is required for moderate or severe.
              --first-aid only when the obvious advice is wrong, e.g. a cat that
                groomed lily pollen needs a vet before any sign shows.
              example: planty agent toxicity --plant easter-lily --cats severe --dogs mild
                --people mild --basis derived --identified-as "Lilium longiflorum"
                --principle "unidentified nephrotoxin" --source www.aspca.org

  away        record a period nobody is home, with a backup contact
              planty agent away --from <date> --until <date> [--contact <name>] [--notify <service>] [--note <text>]
              example: planty agent away --from 2026-08-20 --until 2026-08-27 --contact "Sam"`

// verbs is the dispatch table. A name absent here is refused by name in Run
// rather than falling through to the rest of the CLI, and a test keeps this
// table and the Usage text agreeing with each other.
var verbs = map[string]func(Deps, context.Context, io.Writer, []string) error{
	"plants":       Deps.plants,
	"show":         Deps.show,
	"observations": Deps.observations,
	"reminders":    Deps.reminders,
	"sensors":      Deps.sensors,
	"today":        Deps.today,
	"questions":    Deps.questions,
	"coldwatch":    Deps.coldwatch,

	"log":     Deps.logObservation,
	"harvest": Deps.harvest,

	"notes":  Deps.notes,
	"note":   Deps.note,
	"attach": Deps.attach,

	"remind": Deps.setReminder,
	"forget": Deps.forgetReminder,

	"create":   Deps.create,
	"update":   Deps.update,
	"archive":  Deps.archive,
	"toxicity": Deps.toxicity,

	"water":     Deps.water,
	"shelter":   Deps.shelter,
	"unshelter": Deps.unshelter,

	"link":      Deps.link,
	"calibrate": Deps.calibrate,

	"ack":    Deps.ack,
	"ask":    Deps.ask,
	"answer": Deps.answer,
	"away":   Deps.away,
}

// Run performs one verb. Anything unlisted — autopsy above all — is refused
// by name rather than falling through to the rest of the CLI.
func Run(ctx context.Context, deps Deps, out io.Writer, args []string) error {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(out, Usage)
		return errors.New("no verb given")
	}

	switch args[0] {
	case "help", "-h", "--help":
		_, _ = fmt.Fprintln(out, Usage)
		return nil
	}

	verb, ok := verbs[args[0]]
	if !ok {
		_, _ = fmt.Fprintln(out, Usage)
		return fmt.Errorf("there is no agent verb %q", args[0])
	}
	return verb(deps, ctx, out, args[1:])
}

func (d Deps) logObservation(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("log")
	slug := set.String("plant", "", "the plant's slug")
	kind := set.String("kind", "", "what was done")
	note := set.String("note", "", "anything worth remembering")
	when := set.String("when", "", "when it happened; empty means now")
	if err := parse(set, args); err != nil {
		return err
	}

	// Everything the caller typed is checked before the database is touched,
	// so a mistyped time comes back as a sentence rather than a lookup.
	occurred, err := parseWhen(*when)
	if err != nil {
		return err
	}
	p, err := d.lookUp(ctx, *slug)
	if err != nil {
		return err
	}
	observation := plant.Observation{
		PlantID:    p.ID,
		Kind:       plant.ObservationKind(*kind),
		Body:       *note,
		OccurredAt: occurred,
		// Recorded as the agent, never as the app: who said a thing happened is
		// the first question asked when the record turns out to be wrong.
		Source: plant.SourceAgent,
		Actor:  "planty agent",
	}

	saved, err := d.Store.AddObservation(ctx, observation)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "recorded: %s %s for %s\n",
		saved.Kind, saved.OccurredAt.Format("2006-01-02 15:04"), p.CommonName)
	return nil
}

func (d Deps) setReminder(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("remind")
	slug := set.String("plant", "", "the plant's slug")
	kind := set.String("kind", "", "what to be reminded about")
	everyDays := set.Int("every-days", 1, "how often a day qualifies")
	at := set.String("at", "", "hours of the day, comma separated")
	note := set.String("note", "", "anything worth remembering")
	if err := parse(set, args); err != nil {
		return err
	}

	p, err := d.lookUp(ctx, *slug)
	if err != nil {
		return err
	}
	hours, err := parseHours(*at)
	if err != nil {
		return err
	}

	saved, err := d.Store.SaveReminder(ctx, plant.Reminder{
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

func (d Deps) forgetReminder(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("forget")
	slug := set.String("plant", "", "the plant's slug")
	kind := set.String("kind", "", "which reminder to drop")
	if err := parse(set, args); err != nil {
		return err
	}

	p, err := d.lookUp(ctx, *slug)
	if err != nil {
		return err
	}
	if err := d.Store.DeleteReminder(ctx, p.ID, plant.ObservationKind(*kind)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("no %s reminder is set for %s", *kind, p.CommonName)
		}
		return err
	}
	_, _ = fmt.Fprintf(out, "reminder removed: %s for %s\n", *kind, p.CommonName)
	return nil
}

// newFlags builds a flag set that reports errors instead of printing and
// exiting, so a mistyped flag comes back to the model as a sentence.
func newFlags(verb string) *flag.FlagSet {
	set := flag.NewFlagSet(verb, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	return set
}

// parse reads the flags and refuses leftovers. Without this, an unquoted
// `--name Big Pothos` sets the name to "Big" and drops "Pothos" on the floor,
// which renames a plant to something nobody asked for and says it worked.
func parse(set *flag.FlagSet, args []string) error {
	if err := set.Parse(args); err != nil {
		return err
	}
	if extra := set.Args(); len(extra) > 0 {
		return fmt.Errorf(
			"%q is not attached to a flag; put a value with spaces in double quotes",
			strings.Join(extra, " "))
	}
	return nil
}

// given says which flags were actually passed, which is how a sparse update
// tells "set to the zero value" apart from "left alone".
func given(set *flag.FlagSet) map[string]bool {
	seen := map[string]bool{}
	set.Visit(func(f *flag.Flag) { seen[f.Name] = true })
	return seen
}

func (d Deps) lookUp(ctx context.Context, slug string) (plant.Plant, error) {
	if slug == "" {
		return plant.Plant{}, errors.New("which plant? pass --plant <slug>")
	}
	// A missing dependency is a misconfiguration, and a model reading a crash
	// has nothing useful to tell anybody.
	if d.Store == nil {
		return plant.Plant{}, errors.New("planty has no database to read")
	}
	p, err := d.Store.GetPlant(ctx, slug)
	if errors.Is(err, store.ErrNotFound) {
		return plant.Plant{}, fmt.Errorf(
			"no plant is called %q; planty agent plants lists them", slug)
	}
	return p, err
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

// parseWhen reads a moment as a bare date or RFC3339. Empty means "let the
// store default to now", so a time is only ever recorded when somebody gave
// one, which the acting rules demand.
func parseWhen(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf(
		"%q is not a time; use a date like 2026-08-17, or RFC3339 like 2026-08-17T09:30:00Z", raw)
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
