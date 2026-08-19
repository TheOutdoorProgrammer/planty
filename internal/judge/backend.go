package judge

import (
	"context"

	"github.com/google/uuid"
)

// Backend performs one judgment call and returns the model's JSON answer. Two
// exist because the same judgment is bought either metered through the API or
// against a subscription through the Claude Code CLI.
type Backend interface {
	Judge(ctx context.Context, req Request) (string, error)
	Name() string
}

// Role is who is speaking in one turn.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Effort is how hard to think about an answer. Empty leaves it to the backend.
type Effort string

const (
	EffortMedium Effort = "medium"
	EffortHigh   Effort = "high"
)

// Request is one call. Turns carries the whole conversation even when Session
// could resume it, because only one backend can resume and both have to be
// able to answer the same Request.
type Request struct {
	System    string
	Turns     []Turn
	Offered   []Offer
	Schema    map[string]any
	Effort    Effort
	MaxTokens int64
	Session   *Session
	Acting    *Acting
}

// Acting lets the model record what it is told, rather than only answering.
// Off everywhere except a consultation: a daily verdict that quietly wrote
// observations would be judging its own evidence.
type Acting struct {
	// Binary is what the model may run, and the only thing it may run.
	Binary string

	// Trusted are the hostnames the model may read from. Empty means no web
	// access at all: an allowlist of nothing is not the same as no allowlist,
	// and the difference is the whole internet.
	Trusted []string

	// Sources tells the model what those hostnames are for. Passed in beside
	// them so a rule and its explanation cannot drift apart.
	Sources string

	// Usage is that command's own help, handed to the model so the two cannot
	// drift. Passed in rather than imported: the package that defines these
	// verbs reaches the store, and the store reaches back here.
	Usage string
}

// Session lets a backend continue a conversation instead of re-reading it.
// Nil for one-shot work: a daily verdict has nothing to continue, and keeping
// a session per verdict would fill a disk to no purpose.
type Session struct {
	// ID doubles as the conversation's own id. One identifier for one
	// conversation is what keeps the two from ever disagreeing.
	ID uuid.UUID

	// Resuming is false for the first turn, which is the one that has to
	// establish the session rather than continue it.
	Resuming bool
}

// Offer is a photograph the model may look at and is not obliged to. Attaching
// a month of a plant's photos to every question is expensive and mostly
// wasted, and the model is the thing best placed to say whether it needs one.
type Offer struct {
	Label string
	Media string
	Bytes []byte
}

// Turn is one message in the conversation.
type Turn struct {
	Role  Role
	Parts []Part
}

// Part is either text or an image, never both.
type Part struct {
	Text  string
	Image *Image
}

// Image is one photograph carried inline in a turn.
type Image struct {
	Media string
	Bytes []byte
}

func text(s string) Part { return Part{Text: s} }

func picture(media string, raw []byte) Part {
	if media == "" {
		media = "image/jpeg"
	}
	return Part{Image: &Image{Media: media, Bytes: raw}}
}

func ask(parts ...Part) Turn { return Turn{Role: RoleUser, Parts: parts} }

func answered(body string) Turn {
	return Turn{Role: RoleAssistant, Parts: []Part{text(body)}}
}

// images counts the photographs in a conversation.
func (r Request) images() int {
	n := 0
	for _, turn := range r.Turns {
		for _, part := range turn.Parts {
			if part.Image != nil {
				n++
			}
		}
	}
	return n
}
