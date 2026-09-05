package judge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// serve stands up an endpoint that replies with the given bodies in order, and
// records what it was sent.
func serve(t *testing.T, replies ...string) (*openaiBackend, *[]chatRequest) {
	t.Helper()
	var seen []chatRequest
	turn := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body chatRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("the request was not readable JSON: %v", err)
		}
		seen = append(seen, body)

		w.Header().Set("Content-Type", "application/json")
		if turn >= len(replies) {
			t.Errorf("the backend called %d times, %d replies were staged", turn+1, len(replies))
			http.Error(w, `{"error":{"message":"unexpected call"}}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(replies[turn]))
		turn++
	}))
	t.Cleanup(server.Close)

	backend := newOpenAIBackend(Provider{
		ID: "test", Kind: KindOpenAI, BaseURL: server.URL, APIKeyEnv: "PLANTY_TEST_KEY",
	}, "test-model")
	return backend, &seen
}

func replied(t *testing.T, content string) string {
	t.Helper()
	return `{"choices":[{"message":{"role":"assistant","content":` + quote(t, content) + `}}]}`
}

func quote(t *testing.T, s string) string {
	t.Helper()
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	return string(raw)
}

func TestTheSchemaAndEffortReachTheWire(t *testing.T) {
	backend, seen := serve(t, replied(t, `{"ok":true}`))

	out, err := backend.Judge(context.Background(), Request{
		System:    "be brief",
		Turns:     []Turn{ask(text("hello"))},
		Schema:    map[string]any{"type": "object"},
		Effort:    EffortMedium,
		MaxTokens: 512,
	})
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if out.Answer != `{"ok":true}` {
		t.Errorf("the answer was mangled: %q", out.Answer)
	}
	if out.Model != "test-model" {
		t.Errorf("the outcome does not name the model: %q", out.Model)
	}

	body := (*seen)[0]
	if body.ResponseFormat == nil || body.ResponseFormat.Type != "json_schema" {
		t.Error("the schema was not sent as json_schema")
	}
	if !body.ResponseFormat.JSONSchema.Strict {
		t.Error("the schema was not sent strict")
	}
	if body.ReasoningEffort != "medium" {
		t.Errorf("the effort was not carried: %q", body.ReasoningEffort)
	}
	if body.MaxTokens != 512 {
		t.Errorf("max_completion_tokens was not carried: %d", body.MaxTokens)
	}
	if body.Messages[0].Role != "system" {
		t.Errorf("the system prompt is not first: %q", body.Messages[0].Role)
	}
}

func TestOpenCodeReceivesStableConversationSessionHeader(t *testing.T) {
	var headers []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = append(headers, r.Header.Get("X-Opencode-Session"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(replied(t, `{}`)))
	}))
	defer server.Close()

	backend := newOpenAIBackend(Provider{
		ID: "opencode-go", Kind: KindOpenAI, BaseURL: server.URL,
	}, "test-model")
	conversation := uuid.New()
	if _, err := backend.Judge(context.Background(), Request{
		Session: &Session{ID: conversation},
		Turns:   []Turn{ask(text("hello"))},
	}); err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if len(headers) != 1 || headers[0] != conversation.String() {
		t.Fatalf("X-Opencode-Session = %v, want %q", headers, conversation)
	}
}

func TestKimiUsesASupportedReasoningEffort(t *testing.T) {
	backend, seen := serve(t, replied(t, `{}`))
	backend.model = "kimi-k3"

	if _, err := backend.Judge(context.Background(), Request{
		Turns:  []Turn{ask(text("hello"))},
		Effort: EffortMedium,
	}); err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if got := (*seen)[0].ReasoningEffort; got != "max" {
		t.Errorf("Kimi received unsupported reasoning effort %q", got)
	}
}

func TestOnlyTransientProviderStatusesAreRetried(t *testing.T) {
	for _, test := range []struct {
		status    int
		retryable bool
	}{
		{status: http.StatusBadRequest, retryable: false},
		{status: http.StatusUnauthorized, retryable: false},
		{status: http.StatusTooManyRequests, retryable: true},
		{status: http.StatusBadGateway, retryable: true},
	} {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(`{"error":{"message":"provider failure"}}`))
			}))
			defer server.Close()

			backend := newOpenAIBackend(Provider{
				ID: "test", Kind: KindOpenAI, BaseURL: server.URL,
			}, "test-model")
			_, err := backend.call(context.Background(), chatRequest{Model: "test-model"})
			if err == nil {
				t.Fatal("provider failure was accepted")
			}
			if got := Retryable(err); got != test.retryable {
				t.Fatalf("Retryable = %t, want %t for %d: %v", got, test.retryable, test.status, err)
			}
		})
	}
}

func TestAPhotographRidesAsADataURI(t *testing.T) {
	backend, seen := serve(t, replied(t, `{}`))

	_, err := backend.Judge(context.Background(), Request{
		Turns: []Turn{ask(picture("image/png", []byte("pretend-png")), text("what is it?"))},
	})
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}

	parts, ok := (*seen)[0].Messages[0].Content.([]any)
	if !ok {
		t.Fatalf("a turn with an image was not sent as parts: %T", (*seen)[0].Messages[0].Content)
	}
	var found bool
	for _, part := range parts {
		entry, _ := part.(map[string]any)
		if entry["type"] != "image_url" {
			continue
		}
		url, _ := entry["image_url"].(map[string]any)
		if text, _ := url["url"].(string); strings.HasPrefix(text, "data:image/png;base64,") {
			found = true
		}
	}
	if !found {
		t.Error("the photograph was not sent as a base64 data URI")
	}
}

// The whole reason the harness exists for Consult and Ask: it has to run the
// command itself and feed the result back.
func TestTheLoopRunsAToolAndFeedsTheResultBack(t *testing.T) {
	call := `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[
		{"id":"c1","type":"function","function":{"name":"planty_agent",
		 "arguments":"{\"command\":\"planty agent today\"}"}}]}}]}`
	backend, seen := serve(t, call, replied(t, `{"reply":"done"}`))

	out, err := backend.Judge(context.Background(), Request{
		Turns: []Turn{ask(text("what needs doing?"))},
		Acting: &Acting{
			Binary: "/bin/echo",
			Refuse: func(string, []string) string { return "" },
		},
	})
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if out.Answer != `{"reply":"done"}` {
		t.Errorf("the loop lost the final answer: %q", out.Answer)
	}

	if len((*seen)[0].Tools) == 0 {
		t.Fatal("no tools were offered to a request that granted Acting")
	}
	last := (*seen)[1].Messages
	if last[len(last)-1].Role != "tool" {
		t.Errorf("the tool result was not fed back: %q", last[len(last)-1].Role)
	}

	var ran bool
	for _, step := range out.Steps {
		if step.Kind == StepAction && step.Tool == "planty_agent" {
			ran = true
			if !strings.Contains(step.Detail, "planty agent today") {
				t.Errorf("the step does not say what ran: %q", step.Detail)
			}
		}
	}
	if !ran {
		t.Error("the phone would show no record of the command")
	}
}

func TestNoToolsAreOfferedWithoutActing(t *testing.T) {
	backend, seen := serve(t, replied(t, `{}`))
	if _, err := backend.Judge(context.Background(), Request{
		Turns: []Turn{ask(text("hi"))},
	}); err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if len((*seen)[0].Tools) != 0 {
		t.Error("a request with no Acting was handed tools")
	}
}

func TestOfferedHistoricalPhotoIsOpenedOnDemandWithoutActingPrivileges(t *testing.T) {
	call := `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[
		{"id":"photo-1","type":"function","function":{"name":"historical_photo",
		 "arguments":"{\"index\":0}"}}]}}]}`
	backend, seen := serve(t, call, replied(t, `{"reply":"I compared it."}`))

	out, err := backend.Judge(context.Background(), Request{
		Turns:   []Turn{ask(text("Has it changed?"))},
		Offered: []Offer{{Label: "1 August", Media: "image/jpeg", Bytes: testImage(t, "jpeg")}},
	})
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if len((*seen)[0].Tools) != 1 || (*seen)[0].Tools[0].Function.Name != "historical_photo" {
		t.Fatalf("photo-only request got the wrong toolbox: %+v", (*seen)[0].Tools)
	}
	last := (*seen)[1].Messages[len((*seen)[1].Messages)-1]
	if last.Role != "user" {
		t.Fatalf("selected image has role %q, want user", last.Role)
	}
	raw, err := json.Marshal(last.Content)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "data:image/jpeg;base64,") {
		t.Fatalf("user message did not carry the selected image: %s", raw)
	}
	if len(out.Steps) == 0 || out.Steps[0].Tool != "historical_photo" {
		t.Fatalf("opened photo was not disclosed: %+v", out.Steps)
	}
}

func TestAnUngatedToolboxRunsNothing(t *testing.T) {
	box := newToolbox(&Acting{Binary: "/bin/echo"})
	got := box.runAgent(context.Background(), "planty agent today")
	if !strings.HasPrefix(got, "Refused") {
		t.Errorf("a toolbox with no gate ran the command: %q", got)
	}
}

func TestTheOpenAIToolboxPassesTheJobVerbBoundaryToTheGate(t *testing.T) {
	var got []string
	box := newToolbox(&Acting{
		Binary:     "/bin/echo",
		AgentVerbs: []string{"show", "actuatorstart", "actuatorstop"},
		Refuse: func(_ string, allowed []string) string {
			got = append([]string(nil), allowed...)
			return "job boundary test"
		},
	})
	result := box.runAgent(context.Background(), "planty agent water")
	if !strings.HasPrefix(result, "Refused") {
		t.Fatalf("out-of-scope command was not refused: %q", result)
	}
	if !slices.Equal(got, []string{"show", "actuatorstart", "actuatorstop"}) {
		t.Errorf("gate received %v", got)
	}
}

func TestOnlyTrustedHostsAreFetched(t *testing.T) {
	box := newToolbox(&Acting{Trusted: []string{"www.aspca.org"}})

	if got := box.fetch(context.Background(), "https://example.com/x"); !strings.HasPrefix(got, "Refused") {
		t.Errorf("an untrusted host was fetched: %q", got)
	}
	if got := box.fetch(context.Background(), "http://www.aspca.org/x"); !strings.HasPrefix(got, "Refused") {
		t.Errorf("plain http was fetched: %q", got)
	}
}

func TestWebFetchIsNotOfferedWithNoTrustedSites(t *testing.T) {
	box := newToolbox(&Acting{Binary: "/bin/echo", Refuse: func(string, []string) string { return "" }})
	for _, def := range box.definitions() {
		if def.Function.Name == "web_fetch" {
			t.Error("web_fetch was offered with an empty allowlist")
		}
	}
}

func TestSplitArgsHonoursQuotedValues(t *testing.T) {
	got, err := splitArgs(`planty agent note --plant fern --text "two words"`)
	if err != nil {
		t.Fatalf("splitArgs: %v", err)
	}
	want := []string{"planty", "agent", "note", "--plant", "fern", "--text", "two words"}
	if len(got) != len(want) {
		t.Fatalf("got %q", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
	if _, err := splitArgs(`planty agent note --text "unclosed`); err == nil {
		t.Error("an unclosed quote was accepted")
	}
}

func TestReadableStripsScriptsAndTags(t *testing.T) {
	got := readable(`<html><head><style>p{color:red}</style>` +
		`<script>alert("x")</script></head><body><p>Water&nbsp;it</p></body></html>`)
	if strings.Contains(got, "alert") || strings.Contains(got, "color:red") {
		t.Errorf("script or style survived: %q", got)
	}
	if !strings.Contains(got, "Water it") {
		t.Errorf("the text did not survive: %q", got)
	}
}
