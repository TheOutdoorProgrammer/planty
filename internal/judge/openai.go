package judge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// openaiBackend answers through any endpoint speaking OpenAI chat completions,
// which is what makes the subscription behind it replaceable without another
// backend being written.
type openaiBackend struct {
	provider Provider
	model    string
	client   *http.Client
}

func newOpenAIBackend(p Provider, model string) *openaiBackend {
	return &openaiBackend{
		provider: p,
		model:    model,
		// Long, because a judgment with tool calls is several round trips and
		// the context deadline is the real bound.
		client: &http.Client{Timeout: 10 * time.Minute},
	}
}

func (b *openaiBackend) Name() string { return "openai " + b.provider.ID }

type chatRequest struct {
	Model           string          `json:"model"`
	Messages        []chatMessage   `json:"messages"`
	MaxTokens       int64           `json:"max_completion_tokens,omitempty"`
	ResponseFormat  *responseFormat `json:"response_format,omitempty"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
	Tools           []toolDef       `json:"tools,omitempty"`
}

type responseFormat struct {
	Type       string     `json:"type"`
	JSONSchema jsonSchema `json:"json_schema"`
}

type jsonSchema struct {
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

type chatMessage struct {
	Role       string     `json:"role"`
	Content    any        `json:"content,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content   string     `json:"content"`
			Reasoning string     `json:"reasoning_content"`
			ToolCalls []toolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (b *openaiBackend) Judge(ctx context.Context, req Request) (Outcome, error) {
	messages, err := messagesForChat(req)
	if err != nil {
		return Outcome{}, err
	}
	sessionID := uuid.New()
	if req.Session != nil && req.Session.ID != uuid.Nil {
		sessionID = req.Session.ID
	}

	// Built per request, not per backend: what the model may do belongs to the
	// call that granted it, and a conversation must not inherit another's.
	var box *toolbox
	if req.Acting != nil || len(req.Offered) > 0 {
		box = newToolbox(req.Acting, req.Offered...)
	}
	return b.converse(ctx, req, messages, box, sessionID)
}

// call performs one round trip. finish_reason is deliberately ignored: luna
// returns null on success, so branching on it would reject good answers.
func (b *openaiBackend) call(ctx context.Context, body chatRequest) (chatResponse, error) {
	return b.callWithSession(ctx, body, uuid.Nil)
}

func (b *openaiBackend) callWithSession(ctx context.Context, body chatRequest, sessionID uuid.UUID) (chatResponse, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return chatResponse{}, fmt.Errorf("encode request: %w", err)
	}

	url := strings.TrimSuffix(b.provider.BaseURL, "/") + "/chat/completions"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return chatResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+b.provider.Key())
	if b.provider.ID == "opencode-go" && sessionID != uuid.Nil {
		request.Header.Set("X-Opencode-Session", sessionID.String())
	}

	response, err := b.client.Do(request)
	if err != nil {
		return chatResponse{}, fmt.Errorf("%s: %w", b.provider.ID, err)
	}
	defer func() { _ = response.Body.Close() }()

	payload, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return chatResponse{}, fmt.Errorf("read reply: %w", err)
	}

	var decoded chatResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		err := fmt.Errorf("decode reply (%d): %s", response.StatusCode, truncate(payload))
		if permanentHTTPStatus(response.StatusCode) {
			return chatResponse{}, permanent(err)
		}
		return chatResponse{}, err
	}
	if response.StatusCode >= 300 {
		message := truncate(payload)
		if decoded.Error != nil {
			message = decoded.Error.Message
		}
		err := fmt.Errorf("%s returned %d: %s", b.provider.ID, response.StatusCode, message)
		if permanentHTTPStatus(response.StatusCode) {
			return chatResponse{}, permanent(err)
		}
		return chatResponse{}, err
	}
	if decoded.Error != nil {
		return chatResponse{}, fmt.Errorf("%s: %s", b.provider.ID, decoded.Error.Message)
	}
	if len(decoded.Choices) == 0 {
		return chatResponse{}, fmt.Errorf("%s returned no answer", b.provider.ID)
	}
	return decoded, nil
}

func permanentHTTPStatus(status int) bool {
	if status < 400 || status >= 500 {
		return false
	}
	switch status {
	case http.StatusRequestTimeout, http.StatusConflict, http.StatusTooEarly,
		http.StatusTooManyRequests:
		return false
	default:
		return true
	}
}

// request builds the wire body shared by every round trip in a conversation.
func (b *openaiBackend) request(req Request, messages []chatMessage) chatRequest {
	model := modelFor(req, b.model)
	body := chatRequest{
		Model:     model,
		Messages:  messages,
		MaxTokens: req.MaxTokens,
	}
	if len(req.Schema) > 0 {
		body.ResponseFormat = &responseFormat{
			Type:       "json_schema",
			JSONSchema: jsonSchema{Name: "answer", Strict: true, Schema: req.Schema},
		}
	}
	if req.Effort != "" {
		body.ReasoningEffort = openAIReasoningEffort(model, req.Effort)
	}
	return body
}

func openAIReasoningEffort(model string, effort Effort) string {
	// Kimi rejects Planty's portable medium tier. Max preserves the model's
	// documented reasoning behavior instead of silently disabling it.
	if model == "kimi-k3" && effort == EffortMedium {
		return string(EffortMax)
	}
	return string(effort)
}

// messagesForChat flattens a conversation into chat messages. Offered photos
// are named rather than attached, exactly as the Anthropic path does: sending
// every photo with every question is the cost this deliberately avoids.
func messagesForChat(req Request) ([]chatMessage, error) {
	out := make([]chatMessage, 0, len(req.Turns)+2)
	if req.System != "" {
		out = append(out, chatMessage{Role: "system", Content: req.System})
	}

	for _, turn := range withOffers(req) {
		role := "user"
		if turn.Role == RoleAssistant {
			role = "assistant"
		}

		if text, only := soleText(turn); only {
			out = append(out, chatMessage{Role: role, Content: text})
			continue
		}

		parts := make([]contentPart, 0, len(turn.Parts))
		for _, part := range turn.Parts {
			if part.Image == nil {
				parts = append(parts, contentPart{Type: "text", Text: part.Text})
				continue
			}
			media := part.Image.Media
			if media == "" {
				media = "image/jpeg"
			}
			parts = append(parts, contentPart{Type: "image_url", ImageURL: &imageURL{
				URL: "data:" + media + ";base64," +
					base64.StdEncoding.EncodeToString(part.Image.Bytes),
			}})
		}
		out = append(out, chatMessage{Role: role, Content: parts})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("nothing was asked")
	}
	return out, nil
}

// soleText reports a turn that is one text part, which rides as a plain string
// because some endpoints reject a single-element content array.
func soleText(turn Turn) (string, bool) {
	if len(turn.Parts) == 1 && turn.Parts[0].Image == nil {
		return turn.Parts[0].Text, true
	}
	return "", false
}
