package judge

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestOneTurnRendersAsPlainPrompt(t *testing.T) {
	prompt, err := render(t.TempDir(), []Turn{ask(text("Plant: pothos. What now?"))})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if strings.Contains(prompt, "<user>") {
		t.Errorf("a single question was wrapped in transcript markers:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Plant: pothos") {
		t.Errorf("the question went missing:\n%s", prompt)
	}
}

// The CLI takes one prompt, so a follow-up has to arrive as a transcript or the
// model answers today's question with none of the conversation behind it.
func TestPriorTurnsSurviveAsATranscript(t *testing.T) {
	prompt, err := render(t.TempDir(), []Turn{
		ask(text("what is wrong with it")),
		answered(`{"observed":"yellow lower leaves"}`),
		ask(text("should i repot")),
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	for _, want := range []string{"what is wrong with it", "yellow lower leaves", "should i repot"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the transcript dropped %q:\n%s", want, prompt)
		}
	}
	if strings.Index(prompt, "<assistant>") < strings.Index(prompt, "<user>") {
		t.Error("the transcript does not open with the user")
	}
}

func TestPhotographsAreStagedAsFilesTheModelIsToldToOpen(t *testing.T) {
	dir := t.TempDir()

	prompt, err := render(dir, []Turn{ask(
		text("Taken 3 days ago:"),
		picture("image/jpeg", []byte("first")),
		text("Taken 1 hour ago:"),
		picture("image/png", []byte("second")),
	)})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	for name, want := range map[string]string{"photo-01.jpg": "first", "photo-02.png": "second"} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s was never written: %v", name, err)
		}
		if string(got) != want {
			t.Errorf("%s holds %q, want %q", name, got, want)
		}
		if !strings.Contains(prompt, name) {
			t.Errorf("the prompt never names %s, so nothing opens it:\n%s", name, prompt)
		}
	}
	if !strings.Contains(prompt, "Read all 2 photographs") {
		t.Errorf("the prompt does not tell the model to look at them:\n%s", prompt)
	}
}

func TestToolsAreOffUnlessThereArePhotographs(t *testing.T) {
	backend := newCLIBackend("claude", "claude-opus-5")

	textOnly, err := backend.arguments(Request{Turns: []Turn{ask(text("hi"))}})
	if err != nil {
		t.Fatalf("arguments: %v", err)
	}
	withPhoto, err := backend.arguments(Request{
		Turns: []Turn{ask(picture("image/jpeg", []byte("x")))},
	})
	if err != nil {
		t.Fatalf("arguments: %v", err)
	}

	if got := valueOf(textOnly, "--tools"); got != "" {
		t.Errorf("a text-only judgment was handed tools: %q", got)
	}
	if got := valueOf(withPhoto, "--tools"); got != "Read" {
		t.Errorf("photographs went out with tools %q, want Read", got)
	}
	// A judgment that inherits the surrounding CLAUDE.md is not the judgment
	// the system prompt in this package describes.
	if !slices.Contains(textOnly, "--safe-mode") {
		t.Error("--safe-mode is missing, so ambient config leaks into verdicts")
	}
}

func TestTheSchemaIsHandedToTheCLI(t *testing.T) {
	backend := newCLIBackend("claude", "claude-opus-5")

	args, err := backend.arguments(Request{
		Turns:  []Turn{ask(text("hi"))},
		Schema: map[string]any{"type": "object"},
		Effort: EffortHigh,
	})
	if err != nil {
		t.Fatalf("arguments: %v", err)
	}

	if got := valueOf(args, "--json-schema"); !strings.Contains(got, `"type":"object"`) {
		t.Errorf("schema went out as %q", got)
	}
	if got := valueOf(args, "--effort"); got != "high" {
		t.Errorf("effort went out as %q, want high", got)
	}
}

func TestTheValidatedObjectBeatsTheRawText(t *testing.T) {
	answer, err := answerFrom([]byte(`{
		"is_error": false, "subtype": "success",
		"result": "here you go: {\"action\":\"none\"}",
		"structured_output": {"action":"none"}
	}`))
	if err != nil {
		t.Fatalf("answerFrom: %v", err)
	}

	if strings.Contains(answer, "here you go") {
		t.Errorf("took the unvalidated text over the checked object: %s", answer)
	}
	if !strings.Contains(answer, `"action":"none"`) {
		t.Errorf("lost the answer: %s", answer)
	}
}

func TestRefusalIsItsOwnError(t *testing.T) {
	_, err := answerFrom([]byte(`{"is_error":false,"subtype":"success","stop_reason":"refusal"}`))

	if !errors.Is(err, ErrRefused) {
		t.Errorf("a refusal came back as %v, want ErrRefused", err)
	}
}

func TestAFailedRunIsNotAnEmptyVerdict(t *testing.T) {
	for _, raw := range []string{
		`{"is_error":true,"subtype":"error_during_execution","result":"boom"}`,
		`{"is_error":false,"subtype":"success"}`,
		`not json at all`,
	} {
		if _, err := answerFrom([]byte(raw)); err == nil {
			t.Errorf("%s was accepted as an answer", raw)
		}
	}
}

func valueOf(args []string, flag string) string {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return "<absent>"
}

