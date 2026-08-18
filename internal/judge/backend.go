package judge

import "context"

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

// Request is one call: a system prompt, a conversation, and the shape the
// answer has to take. Every backend must return JSON matching Schema.
type Request struct {
	System    string
	Turns     []Turn
	Schema    map[string]any
	Effort    Effort
	MaxTokens int64
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
