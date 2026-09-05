package judge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// rounds caps the conversation. A model that has not answered after this many
// tool calls is looping, and an uncapped loop against a metered subscription
// is the expensive kind of bug.
const rounds = 12

type toolDef struct {
	Type     string      `json:"type"`
	Function functionDef `json:"function"`
}

type functionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// toolbox is what the model may do, and the only thing it may do. It exists so
// the loop cannot reach anything the Acting it was built from did not grant.
type toolbox struct {
	acting *Acting
	offers []Offer
	client *http.Client
}

func newToolbox(a *Acting, offers ...Offer) *toolbox {
	return &toolbox{acting: a, offers: offers, client: &http.Client{Timeout: 30 * time.Second}}
}

func (t *toolbox) definitions() []toolDef {
	defs := []toolDef{}
	if t.acting != nil {
		defs = append(defs, toolDef{
			Type: "function",
			Function: functionDef{
				Name: "planty_agent",
				Description: "Run one Planty command against the garden's own records. " +
					"The complete verb reference is in your instructions. " +
					"Pass the whole command, for example: planty agent show --plant golden-pothos",
				Parameters: map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"command"},
					"properties": map[string]any{
						"command": map[string]any{
							"type":        "string",
							"description": "The full command, beginning with `planty agent`",
						},
					},
				},
			},
		})
	}

	if len(t.offers) > 0 {
		defs = append(defs, toolDef{
			Type: "function",
			Function: functionDef{
				Name:        "historical_photo",
				Description: offeredPhotoInstructions(t.offers),
				Parameters: map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"index"},
					"properties": map[string]any{
						"index": map[string]any{
							"type": "integer", "minimum": 0, "maximum": len(t.offers) - 1,
							"description": "The zero-based index printed in the available photograph list, not a photo ID or message number.",
						},
					},
				},
			},
		})
	}

	if t.acting == nil || len(t.acting.Trusted) == 0 {
		return defs
	}
	return append(defs, toolDef{
		Type: "function",
		Function: functionDef{
			Name: "web_fetch",
			Description: "Read a page from one of the trusted sites named in your instructions. " +
				"Nothing else is reachable. Each of those sites carries its own search page, " +
				"so searching is fetching that site's search URL.",
			Parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"url"},
				"properties": map[string]any{
					"url": map[string]any{"type": "string", "description": "Absolute https URL"},
				},
			},
		},
	})
}

// run performs one tool call and returns what the model should be told. A
// refusal is an answer, not an error: the model is expected to read the reason
// and say so rather than retry blindly.
type toolResult struct {
	Content string
	Images  []contentPart
	Summary string
}

func textResult(body string) toolResult { return toolResult{Content: body, Summary: body} }

func (t *toolbox) run(ctx context.Context, call toolCall) toolResult {
	var args struct {
		Command string `json:"command"`
		URL     string `json:"url"`
		Index   *int   `json:"index"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return textResult("That call could not be read: " + err.Error())
	}

	switch call.Function.Name {
	case "planty_agent":
		return textResult(t.runAgent(ctx, args.Command))
	case "web_fetch":
		return textResult(t.fetch(ctx, args.URL))
	case "historical_photo":
		return t.openPhoto(ctx, args.Index)
	default:
		return textResult(fmt.Sprintf("There is no tool called %q.", call.Function.Name))
	}
}

func offeredPhotoInstructions(offers []Offer) string {
	if len(offers) == 0 {
		return "No photographs are available to open in this request."
	}
	var b strings.Builder
	b.WriteString("Photographs are available through historical_photo. " +
		"When asked to look at a photograph, open it before answering. " +
		"For questions the text record answers, opening photographs is optional. " +
		"Call with a JSON object containing the zero-based index, for example {\"index\":0}. " +
		"Use only the indices below, not photo IDs, dates, or message numbers. Available:\n")
	for i, offer := range offers {
		fmt.Fprintf(&b, "%d: %s\n", i, offer.Label)
	}
	return b.String()
}

func (t *toolbox) openPhoto(ctx context.Context, index *int) toolResult {
	ctx, span := otel.Tracer("planty/judge").Start(ctx, "model.photo.open")
	defer span.End()
	span.SetAttributes(attribute.Int("photo.offered_count", len(t.offers)))
	if len(t.offers) == 0 {
		span.SetAttributes(attribute.String("photo.status", "unavailable"))
		return textResult(offeredPhotoInstructions(t.offers))
	}
	if index == nil {
		span.SetAttributes(attribute.String("photo.status", "missing_index"))
		return textResult("Missing required integer argument index. " + offeredPhotoInstructions(t.offers))
	}
	span.SetAttributes(attribute.Int("photo.index", *index))
	if *index < 0 || *index >= len(t.offers) {
		span.SetAttributes(attribute.String("photo.status", "invalid_index"))
		return textResult(fmt.Sprintf("Photograph index %d is outside the available range 0 through %d. ",
			*index, len(t.offers)-1) + offeredPhotoInstructions(t.offers))
	}
	offer := t.offers[*index]
	prepared, err := prepareModelImage(ctx, &Image{Media: offer.Media, Bytes: offer.Bytes})
	if err != nil {
		span.SetAttributes(attribute.String("photo.status", "unreadable"))
		return textResult("That historical photograph could not be opened: " + err.Error())
	}
	span.SetAttributes(attribute.String("photo.status", "opened"))
	return toolResult{
		Content: "Opened " + offer.Label + ". The image follows the tool results in a user message.",
		Images: []contentPart{
			{Type: "text", Text: fmt.Sprintf("historical_photo index %d: %s", *index, offer.Label)},
			{Type: "image_url", ImageURL: &imageURL{URL: "data:" + prepared.Media + ";base64," +
				base64.StdEncoding.EncodeToString(prepared.Bytes)}},
		},
		Summary: "Opened " + offer.Label,
	}
}

func (t *toolbox) runAgent(ctx context.Context, command string) string {
	command = strings.TrimSpace(command)
	if t.acting == nil || t.acting.Refuse == nil {
		return "Refused: nothing may be run here."
	}
	if reason := t.acting.Refuse(command, t.acting.AgentVerbs); reason != "" {
		return "Refused: " + reason
	}

	fields := strings.Fields(command)
	if len(fields) < 2 || fields[0] != "planty" || fields[1] != "agent" {
		return "Refused: only `planty agent <verb>` may be run."
	}

	// Argv rather than a shell, so the gate's reading of the command is the
	// only reading there is.
	parsed, err := splitArgs(command)
	if err != nil {
		return "Refused: " + err.Error()
	}

	out, err := exec.CommandContext(ctx, t.acting.Binary, parsed[1:]...).CombinedOutput()
	body := strings.TrimSpace(string(out))
	if err != nil && body == "" {
		return "That command failed: " + err.Error()
	}
	return clip(body)
}

func (t *toolbox) fetch(ctx context.Context, raw string) string {
	if t.acting == nil {
		return "Refused: no web access was granted."
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "That URL could not be read: " + err.Error()
	}
	if parsed.Scheme != "https" {
		return "Refused: only https may be fetched."
	}
	if !slices.Contains(t.acting.Trusted, parsed.Hostname()) {
		return fmt.Sprintf("Refused: %s is not one of the trusted sites.", parsed.Hostname())
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "That page could not be requested: " + err.Error()
	}
	request.Header.Set("User-Agent", "planty/1.0 (+houseplant care)")

	response, err := t.client.Do(request)
	if err != nil {
		return "That page could not be read: " + err.Error()
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode >= 300 {
		return fmt.Sprintf("%s returned %d.", parsed.Host, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return "That page could not be read: " + err.Error()
	}
	return clip(readable(string(body)))
}

// No backreference for the closing tag: RE2 has none, so each element is
// spelled out rather than captured and matched back.
var (
	dropped = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>` +
		`|<style[^>]*>.*?</style>` +
		`|<noscript[^>]*>.*?</noscript>`)
	tags    = regexp.MustCompile(`(?s)<[^>]+>`)
	runs    = regexp.MustCompile(`[ \t]+`)
	spacing = regexp.MustCompile(`[ \t]*\n\s*\n\s*`)
)

// readable reduces a page to its text. Crude on purpose: the trusted sites are
// extension articles, and a real parser would be a dependency earning nothing.
func readable(page string) string {
	page = dropped.ReplaceAllString(page, " ")
	page = tags.ReplaceAllString(page, " ")
	page = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<",
		"&gt;", ">", "&quot;", `"`, "&#39;", "'").Replace(page)
	page = runs.ReplaceAllString(page, " ")
	return strings.TrimSpace(spacing.ReplaceAllString(page, "\n\n"))
}

// splitArgs reads a command into argv, honouring the double quotes the agent
// reference tells the model to use for values containing spaces.
func splitArgs(command string) ([]string, error) {
	var args []string
	var current strings.Builder
	quoted, any := false, false

	for _, r := range command {
		switch {
		case r == '"':
			quoted, any = !quoted, true
		case (r == ' ' || r == '\t') && !quoted:
			if any || current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
				any = false
			}
		default:
			current.WriteRune(r)
		}
	}
	if quoted {
		return nil, fmt.Errorf("that command has an unclosed quote")
	}
	if any || current.Len() > 0 {
		args = append(args, current.String())
	}
	return args, nil
}

// converse runs the model to an answer, performing whatever tool calls it asks
// for along the way and recording each as a step the phone can show.
func (b *openaiBackend) converse(ctx context.Context, req Request, messages []chatMessage, box *toolbox, sessionID uuid.UUID) (Outcome, error) {
	var out Outcome
	out.Model = modelFor(req, b.model)

	for range rounds {
		body := b.request(req, messages)
		if box != nil {
			body.Tools = box.definitions()

			// Withheld on purpose: given a schema and a tool together, a model
			// answers in schema without calling anything and invents what the
			// call would have said. qwen3.8-max fabricated a day of care.
			body.ResponseFormat = nil
		}

		reply, err := b.callWithSession(ctx, body, sessionID)
		if err != nil {
			return Outcome{}, err
		}
		answer := reply.Choices[0].Message

		if answer.Reasoning != "" {
			out.addStep(Step{Kind: StepThought, Output: clip(answer.Reasoning)})
		}
		if len(answer.ToolCalls) == 0 {
			if strings.TrimSpace(answer.Content) == "" {
				return Outcome{}, fmt.Errorf("%s returned no answer", b.provider.ID)
			}
			if box != nil && len(req.Schema) > 0 {
				return b.render(ctx, req, messages, answer.Content, out, sessionID)
			}
			out.Answer = answer.Content
			return out, nil
		}
		if box == nil {
			return Outcome{}, fmt.Errorf("%s asked to run a tool it was not given", b.provider.ID)
		}

		messages = append(messages, chatMessage{
			Role:      "assistant",
			Content:   answer.Content,
			ToolCalls: answer.ToolCalls,
		})
		var images []contentPart
		for _, call := range answer.ToolCalls {
			result := box.run(ctx, call)
			out.addStep(Step{
				Kind:   StepAction,
				Tool:   call.Function.Name,
				Detail: describeCall(call),
				Output: clip(result.Summary),
			})
			messages = append(messages, chatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    result.Content,
			})
			images = append(images, result.Images...)
		}
		// Chat Completions tool messages are text-only. Resolve every tool call
		// before adding images as user content, including for parallel calls.
		if len(images) > 0 {
			messages = append(messages, chatMessage{Role: "user", Content: images})
		}
	}
	return Outcome{}, fmt.Errorf("%s kept calling tools without answering", b.provider.ID)
}

// render asks for the answer a second time in the schema, with no tools to
// reach for. It is the other half of withholding the schema during the loop.
func (b *openaiBackend) render(ctx context.Context, req Request, messages []chatMessage, draft string, out Outcome, sessionID uuid.UUID) (Outcome, error) {
	messages = append(messages,
		chatMessage{Role: "assistant", Content: draft},
		chatMessage{Role: "user", Content: "Give that same answer in the required JSON format. " +
			"Do not change what it says, and do not look anything else up."},
	)

	reply, err := b.callWithSession(ctx, b.request(req, messages), sessionID)
	if err != nil {
		return Outcome{}, err
	}
	answer := reply.Choices[0].Message.Content
	if strings.TrimSpace(answer) == "" {
		return Outcome{}, fmt.Errorf("%s could not render its answer", b.provider.ID)
	}
	out.Answer = answer
	return out, nil
}

func describeCall(call toolCall) string {
	var args struct {
		Command string `json:"command"`
		URL     string `json:"url"`
		Index   *int   `json:"index"`
	}
	_ = json.Unmarshal([]byte(call.Function.Arguments), &args)
	if args.Command != "" {
		return args.Command
	}
	if args.Index != nil {
		return fmt.Sprintf("historical photograph %d", *args.Index)
	}
	return args.URL
}
