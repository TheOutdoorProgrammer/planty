package judge

import (
	"context"
	"encoding/base64"
	"fmt"
	"slices"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// apiBackend calls the Anthropic API directly, billed per token.
type apiBackend struct {
	client anthropic.Client
	model  string
}

func newAPIBackend(apiKey, model string) *apiBackend {
	return &apiBackend{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
	}
}

func (b *apiBackend) Name() string { return "anthropic api" }

func (b *apiBackend) Judge(ctx context.Context, req Request) (Outcome, error) {
	output := anthropic.OutputConfigParam{
		Format: anthropic.JSONOutputFormatParam{Schema: req.Schema},
	}
	switch req.Effort {
	case EffortMedium:
		output.Effort = anthropic.OutputConfigEffortMedium
	case EffortHigh:
		output.Effort = anthropic.OutputConfigEffortHigh
	}

	model := modelFor(req, b.model)
	message, err := b.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:        anthropic.Model(model),
		MaxTokens:    req.MaxTokens,
		System:       []anthropic.TextBlockParam{{Text: req.System}},
		Messages:     messagesFor(withOffers(req)),
		OutputConfig: output,
	})
	if err != nil {
		return Outcome{}, err
	}
	if message.StopReason == anthropic.StopReasonRefusal {
		return Outcome{}, ErrRefused
	}

	for _, block := range message.Content {
		if answer, ok := block.AsAny().(anthropic.TextBlock); ok {
			return Outcome{Answer: answer.Text, Model: model}, nil
		}
	}
	return Outcome{}, fmt.Errorf("no answer in the reply")
}

// withOffers names the photographs without attaching them, because an image
// block sent is an image block read and paid for. Saying what exists beats
// quietly turning "you may look" into "you must look at all thirty".
func withOffers(req Request) []Turn {
	if len(req.Offered) == 0 {
		return req.Turns
	}

	var b strings.Builder
	b.WriteString("Photographs exist for these days but are not attached, " +
		"and you have not seen them. Say so if one would have changed your answer.\n")
	for _, offer := range req.Offered {
		fmt.Fprintf(&b, "  %s\n", offer.Label)
	}

	turns := slices.Clone(req.Turns)
	turns = append(turns, ask(text(b.String())))
	return turns
}

func messagesFor(turns []Turn) []anthropic.MessageParam {
	out := make([]anthropic.MessageParam, 0, len(turns))
	for _, turn := range turns {
		role := anthropic.MessageParamRoleUser
		if turn.Role == RoleAssistant {
			role = anthropic.MessageParamRoleAssistant
		}

		blocks := make([]anthropic.ContentBlockParamUnion, 0, len(turn.Parts))
		for _, part := range turn.Parts {
			if part.Image != nil {
				blocks = append(blocks, anthropic.NewImageBlockBase64(part.Image.Media,
					base64.StdEncoding.EncodeToString(part.Image.Bytes)))
				continue
			}
			blocks = append(blocks, anthropic.NewTextBlock(part.Text))
		}
		out = append(out, anthropic.MessageParam{Role: role, Content: blocks})
	}
	return out
}
