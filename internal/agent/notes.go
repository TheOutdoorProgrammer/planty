package agent

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// notes lists what has been written down, about one plant or about the house.
func (d Deps) notes(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("notes")
	slug := set.String("plant", "", "the plant's slug")
	household := set.Bool("household", false, "notes about the house rather than a plant")
	if err := parse(set, args); err != nil {
		return err
	}
	if (*slug == "") == !*household {
		return errors.New("say which: --plant <slug> or --household, not both and not neither")
	}

	var owner uuid.UUID
	subject := "the household"
	if !*household {
		p, err := d.lookUp(ctx, *slug)
		if err != nil {
			return err
		}
		owner, subject = p.ID, p.Slug
	}

	notes, err := d.Store.Notes(ctx, owner)
	if err != nil {
		return err
	}
	if len(notes) == 0 {
		_, _ = fmt.Fprintf(out, "no notes on %s\n", subject)
		return nil
	}

	for _, n := range notes {
		_, _ = fmt.Fprintf(out, "%s  %s\n", n.ID, n.CreatedAt.Format(stamp))
		if n.Title != "" {
			_, _ = fmt.Fprintf(out, "  %s\n", n.Title)
		}
		_, _ = fmt.Fprintf(out, "  %s\n", n.Body)
	}
	return nil
}

// note writes, changes or removes one note. Which of the three it does is
// decided by the flags, since --plant can only mean a new one and --id can
// only mean an existing one.
func (d Deps) note(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("note")
	slug := set.String("plant", "", "the plant's slug, to write a new note about it")
	household := set.Bool("household", false, "write about the house rather than a plant")
	id := set.String("id", "", "an existing note's id, to change or remove it")
	title := set.String("title", "", "an optional heading")
	body := set.String("text", "", "the note itself")
	remove := set.Bool("delete", false, "remove the note named by --id")
	if err := parse(set, args); err != nil {
		return err
	}

	seen := given(set)
	subjects := 0
	for _, given := range []bool{*slug != "", *household, *id != ""} {
		if given {
			subjects++
		}
	}
	if subjects != 1 {
		return errors.New("say exactly one of: --plant <slug>, --household, or --id <id>")
	}

	if *id != "" {
		noteID, err := uuid.Parse(*id)
		if err != nil {
			return fmt.Errorf("%q is not a note id; ids come from the notes verb", *id)
		}
		if *remove {
			if err := d.Store.DeleteNote(ctx, noteID); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "removed note %s\n", noteID)
			return nil
		}
		return d.reword(ctx, out, noteID, seen, title, body)
	}

	if *remove {
		return errors.New("--delete needs the note's --id, not a plant")
	}
	if *body == "" {
		return errors.New("--text is what the note says, and it cannot be empty")
	}

	var owner uuid.UUID
	subject := "the household"
	if !*household {
		p, err := d.lookUp(ctx, *slug)
		if err != nil {
			return err
		}
		owner, subject = p.ID, p.Slug
	}

	written, err := d.Store.AddNote(ctx, plant.Note{
		PlantID: owner, Title: *title, Body: *body,
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "noted against %s: %s\n", subject, written.ID)
	return nil
}

// reword changes only the fields actually passed, so editing a body does not
// blank a title nobody mentioned.
func (d Deps) reword(ctx context.Context, out io.Writer, id uuid.UUID,
	seen map[string]bool, title, body *string) error {
	var newTitle, newBody *string
	if seen["title"] {
		newTitle = title
	}
	if seen["text"] {
		newBody = body
	}
	if newTitle == nil && newBody == nil {
		return errors.New("nothing to change: pass --title, --text, or --delete")
	}

	changed, err := d.Store.UpdateNote(ctx, id, newTitle, newBody)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "rewrote note %s\n", changed.ID)
	return nil
}

// attach files a photograph already sent in this conversation against one of
// the person's own plants, which is the only way a picture taken to ask a
// question can become part of that plant's history.
func (d Deps) attach(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("attach")
	slug := set.String("plant", "", "the plant's slug")
	photo := set.String("photo", "", "the photograph's id, given to you when it was sent")
	caption := set.String("caption", "", "what the picture shows")
	if err := parse(set, args); err != nil {
		return err
	}

	photoID, err := uuid.Parse(*photo)
	if err != nil {
		return fmt.Errorf("%q is not a photograph id", *photo)
	}
	p, err := d.lookUp(ctx, *slug)
	if err != nil {
		return err
	}

	if err := d.Store.AttachPhoto(ctx, photoID, p.ID, *caption); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "filed that photograph against %s\n", p.Slug)
	return nil
}
