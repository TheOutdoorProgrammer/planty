package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// PNT-028 expands the existing away verb without adding another capability
// name. Keeping one verb preserves the allowlist/gate contract while making
// creation, inspection, correction, and cancellation available to the agent.
func init() {
	verbs["away"] = Deps.manageAway
}

func (d Deps) manageAway(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("away")
	idRaw := set.String("id", "", "the away period id, from planty agent away")
	from := set.String("from", "", "when the period starts")
	until := set.String("until", "", "when it ends")
	contact := set.String("contact", "", "who can act meanwhile")
	notify := set.String("notify", "", "how to reach them, a notify service")
	note := set.String("note", "", "anything worth remembering")
	cancel := set.Bool("cancel", false, "cancel the period named by --id")
	if err := parse(set, args); err != nil {
		return err
	}
	if d.Store == nil {
		return errors.New("planty has no database to read")
	}

	seen := given(set)
	if len(seen) == 0 {
		return d.listAway(ctx, out)
	}

	if *idRaw == "" {
		if *cancel {
			return errors.New("cancelling coverage needs --id; planty agent away lists ids")
		}
		if *from == "" || *until == "" {
			return errors.New("new coverage needs --from and --until; omit all flags to list existing coverage")
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
		_, _ = fmt.Fprintf(out, "recorded away from %s until %s (id %s)\n",
			saved.StartsAt.Format("2006-01-02"), saved.EndsAt.Format("2006-01-02"), saved.ID)
		return nil
	}

	id, err := uuid.Parse(*idRaw)
	if err != nil {
		return fmt.Errorf("%q is not an away period id; planty agent away lists ids", *idRaw)
	}
	if *cancel {
		if len(seen) != 2 || !seen["id"] || !seen["cancel"] {
			return errors.New("--cancel cannot be combined with edits; cancel it or change it, not both")
		}
		if err := d.Store.DeleteAway(ctx, id); errors.Is(err, store.ErrNotFound) {
			return errors.New("no away period has that id; planty agent away lists current coverage")
		} else if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "cancelled away period %s\n", id)
		return nil
	}

	current, err := d.Store.AwayPeriod(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return errors.New("no away period has that id; planty agent away lists current coverage")
	}
	if err != nil {
		return err
	}

	changed := false
	if seen["from"] {
		current.StartsAt, err = parseWhen(*from)
		if err != nil {
			return err
		}
		changed = true
	}
	if seen["until"] {
		current.EndsAt, err = parseWhen(*until)
		if err != nil {
			return err
		}
		changed = true
	}
	if seen["contact"] {
		current.BackupContact = *contact
		changed = true
	}
	if seen["notify"] {
		current.BackupNotify = *notify
		changed = true
	}
	if seen["note"] {
		current.Note = *note
		changed = true
	}
	if !changed {
		return errors.New("nothing to change; pass a field with --id, or --cancel")
	}

	saved, err := d.Store.UpdateAway(ctx, id, current)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "updated away period %s: %s until %s\n",
		saved.ID, saved.StartsAt.Format("2006-01-02"), saved.EndsAt.Format("2006-01-02"))
	return nil
}

func (d Deps) listAway(ctx context.Context, out io.Writer) error {
	periods, err := d.Store.AwayPeriods(ctx, false)
	if err != nil {
		return err
	}
	if len(periods) == 0 {
		_, _ = fmt.Fprintln(out, "no active or upcoming away periods")
		return nil
	}

	now := time.Now().UTC()
	for _, period := range periods {
		state := "upcoming"
		if period.Covers(now) {
			state = "active now"
		}
		line := fmt.Sprintf("%s %s: %s until %s",
			period.ID, state, period.StartsAt.Format("2006-01-02"), period.EndsAt.Format("2006-01-02"))
		if period.BackupContact != "" {
			line += ", backup " + period.BackupContact
		}
		if period.Note != "" {
			line += ", note " + fmt.Sprintf("%q", period.Note)
		}
		_, _ = fmt.Fprintln(out, line)
	}
	return nil
}
