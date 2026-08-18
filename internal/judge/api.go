package judge

import (
	"context"
	"encoding/base64"
	"fmt"

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

func (b *apiBackend) Judge(ctx context.Context, req Request) (string, error) {
	output := anthropic.OutputConfigParam{
		Format: anthropic.JSONOutputFormatParam{Schema: req.Schema},
	}
	switch req.Effort {
	case EffortMedium:
		output.Effort = anthropic.OutputConfigEffortMedium
	case EffortHigh:
		output.Effort = anthropic.OutputConfigEffortHigh
	}

	message, err := b.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:        anthropic.Model(b.model),
		MaxTokens:    req.MaxTokens,
		System:       []anthropic.TextBlockParam{{Text: req.System}},
		Messages:     messagesFor(req.Turns),
		OutputConfig: output,
	})
	if err != nil {
		return "", err
	}
	if message.StopReason == anthropic.StopReasonRefusal {
		return "", ErrRefused
	}

	for _, block := range message.Content {
		if answer, ok := block.AsAny().(anthropic.TextBlock); ok {
			return answer.Text, nil
		}
	}
	return "", fmt.Errorf("no answer in the reply")
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
