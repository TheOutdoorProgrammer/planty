package judge

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
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

// Reading is always available; acting is not. A daily verdict that could run
// commands would be judging evidence it wrote itself.
func TestOnlyReadingIsAvailableWithoutActing(t *testing.T) {
	backend := newCLIBackend("claude", "claude-opus-5")

	textOnly, err := backend.arguments(Request{Turns: []Turn{ask(text("hi"))}}, false)
	if err != nil {
		t.Fatalf("arguments: %v", err)
	}
	withPhoto, err := backend.arguments(Request{
		Turns: []Turn{ask(picture("image/jpeg", []byte("x")))},
	}, false)
	if err != nil {
		t.Fatalf("arguments: %v", err)
	}

	for _, args := range [][]string{textOnly, withPhoto} {
		tools := valueOf(args, "--tools")
		if !strings.Contains(tools, "Read") {
			t.Errorf("reading was not granted: --tools %q", tools)
		}
		if strings.Contains(tools, "Bash") || strings.Contains(tools, "Web") {
			t.Errorf("a judgment with no Acting was handed %q", tools)
		}
	}
	// Isolation, and why it is not --safe-mode, is covered below.
	if !slices.Contains(textOnly, "--strict-mcp-config") {
		t.Error("ambient MCP servers can attach to a verdict")
	}
}

func TestTheSchemaIsHandedToTheCLI(t *testing.T) {
	backend := newCLIBackend("claude", "claude-opus-5")

	args, err := backend.arguments(Request{
		Turns:  []Turn{ask(text("hi"))},
		Schema: map[string]any{"type": "object"},
		Effort: EffortHigh,
	}, false)
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

// One-shot work has nothing to continue, and a session per daily verdict is a
// disk filling up for no reason.
func TestOneShotWorkPersistsNoSession(t *testing.T) {
	backend := newCLIBackend("claude", "claude-opus-5")

	args, err := backend.arguments(Request{Turns: []Turn{ask(text("hi"))}}, false)
	if err != nil {
		t.Fatalf("arguments: %v", err)
	}

	if !slices.Contains(args, "--no-session-persistence") {
		t.Error("a one-shot judgment left a session behind")
	}
	if slices.Contains(args, "--session-id") || slices.Contains(args, "--resume") {
		t.Errorf("a one-shot judgment asked for a session: %v", args)
	}
}

func TestTheFirstTurnEstablishesTheSession(t *testing.T) {
	backend := newCLIBackend("claude", "claude-opus-5")
	conversation := uuid.New()

	args, err := backend.arguments(Request{
		System:  "you judge plants",
		Turns:   []Turn{ask(text("hi"))},
		Session: &Session{ID: conversation, Resuming: false},
	}, false)
	if err != nil {
		t.Fatalf("arguments: %v", err)
	}

	if got := valueOf(args, "--session-id"); got != conversation.String() {
		t.Errorf("session id went out as %q", got)
	}
	if slices.Contains(args, "--no-session-persistence") {
		t.Error("the first turn threw away the session it was establishing")
	}
	if got := valueOf(args, "--system-prompt"); got != "you judge plants" {
		t.Errorf("the opening turn did not carry the system prompt: %q", got)
	}
}

// The saving is entirely in not re-sending: a resumed turn that still carried
// the system prompt would pay for a second copy of it every time.
func TestAResumedTurnRepeatsNothing(t *testing.T) {
	backend := newCLIBackend("claude", "claude-opus-5")
	conversation := uuid.New()

	args, err := backend.arguments(Request{
		System:  "you judge plants",
		Turns:   []Turn{ask(text("first")), answered("{}"), ask(text("second"))},
		Session: &Session{ID: conversation, Resuming: true},
	}, true)
	if err != nil {
		t.Fatalf("arguments: %v", err)
	}

	if got := valueOf(args, "--resume"); got != conversation.String() {
		t.Errorf("resumed %q", got)
	}
	if slices.Contains(args, "--system-prompt") {
		t.Error("a resumed turn re-sent the system prompt it is already holding")
	}
	if slices.Contains(args, "--session-id") {
		t.Error("a resumed turn tried to establish a new session")
	}
}

// Resuming sends only the new question. Sending the transcript as well would
// keep every earlier turn in the bill, which is the thing this exists to stop.
func TestResumingSendsOnlyTheNewQuestion(t *testing.T) {
	dir := t.TempDir()
	turns := []Turn{
		ask(text("what is wrong with it")),
		answered(`{"reply":"nothing"}`),
		ask(text("should i repot")),
	}

	whole, err := render(dir, turns)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	newest, err := render(dir, turns[len(turns)-1:])
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if !strings.Contains(whole, "what is wrong with it") {
		t.Error("a fresh session has to carry the whole conversation")
	}
	if strings.Contains(newest, "what is wrong with it") {
		t.Errorf("a resumed turn re-sent the conversation:\n%s", newest)
	}
	if !strings.Contains(newest, "should i repot") {
		t.Errorf("a resumed turn lost the question being asked:\n%s", newest)
	}
	if len(newest) >= len(whole) {
		t.Errorf("resuming sent %d bytes against %d for the whole thing", len(newest), len(whole))
	}
}

// An emptyDir dies with its pod, so a conversation can outlive the session it
// was using. Falling back to the transcript is slower and still correct.
func TestAMissingSessionIsRecognisedAsOrdinary(t *testing.T) {
	for _, complaint := range []string{
		"No conversation found with session ID: abc",
		"Error: session not found",
		"no session to resume",
	} {
		if !mentionsAMissingSession(complaint) {
			t.Errorf("%q was not recognised as a lost session", complaint)
		}
	}
	if mentionsAMissingSession("connection refused") {
		t.Error("a real failure was mistaken for a lost session and silently retried")
	}
}

// --tools takes a list, so a prompt following it is read as one more tool name
// and the call dies asking for the input it was just handed. Only the
// text-only path ends in --tools, which is why every photo test passed.
func TestThePromptIsNotSwallowedByTheToolList(t *testing.T) {
	backend := newCLIBackend("claude", "claude-opus-5")

	args, err := backend.arguments(Request{Turns: []Turn{ask(text("hi"))}}, false)
	if err != nil {
		t.Fatalf("arguments: %v", err)
	}
	full := append(args, "--", "the prompt")

	last := full[len(full)-1]
	if last != "the prompt" {
		t.Fatalf("the prompt is not last: %q", last)
	}
	if full[len(full)-2] != "--" {
		t.Errorf("nothing separates the prompt from the flags: %v", full[len(full)-4:])
	}
	if args[len(args)-2] != "--tools" {
		t.Skip("the text-only path no longer ends in a list flag")
	}
}

// Bypassing permissions to read a staged photograph hands away every check the
// process has, in a pod holding a token that can operate a water pump.
func TestReadingAPhotographDoesNotBypassPermissions(t *testing.T) {
	backend := newCLIBackend("claude", "claude-opus-5")

	args, err := backend.arguments(Request{
		Turns: []Turn{ask(picture("image/jpeg", []byte("x")))},
	}, false)
	if err != nil {
		t.Fatalf("arguments: %v", err)
	}

	if slices.Contains(args, "bypassPermissions") {
		t.Errorf("permissions were bypassed to read a file: %v", args)
	}
	if got := valueOf(args, "--tools"); got != "Read" {
		t.Errorf("tools are %q, want Read alone", got)
	}
}

// A judgment nobody asked to act must not be handed a shell. The daily verdict
// runs unattended over every plant; the blast radius of it writing is the
// whole record.
func TestOnlyAnActingRequestGetsAShell(t *testing.T) {
	backend := newCLIBackend("claude", "claude-opus-5")

	quiet, err := backend.arguments(Request{Turns: []Turn{ask(text("hi"))}}, false)
	if err != nil {
		t.Fatalf("arguments: %v", err)
	}
	if strings.Contains(valueOf(quiet, "--tools"), "Bash") {
		t.Error("a plain judgment was given Bash")
	}
	if slices.Contains(quiet, "--allowedTools") || slices.Contains(quiet, "--settings") {
		t.Errorf("a plain judgment carried permission flags: %v", quiet)
	}
}

// Two independent layers, because one of them is a string match on a command
// line and the other is a process that has to agree with it.
func TestActingIsGatedTwice(t *testing.T) {
	backend := newCLIBackend("claude", "claude-opus-5")

	args, err := backend.arguments(Request{
		Turns:  []Turn{ask(text("i watered it"))},
		Acting: &Acting{Binary: "/planty", Usage: "planty agent ..."},
	}, false)
	if err != nil {
		t.Fatalf("arguments: %v", err)
	}

	// Without dontAsk an allow rule only skips a prompt that print mode could
	// never have shown, so everything else would still run.
	if got := valueOf(args, "--permission-mode"); got != "dontAsk" {
		t.Errorf("permission mode is %q, want dontAsk", got)
	}
	if got := valueOf(args, "--allowedTools"); got != "Bash(planty agent *)" {
		t.Errorf("allowed %q, want only the agent verbs", got)
	}
	if !strings.Contains(valueOf(args, "--tools"), "Bash") {
		t.Error("acting was requested but Bash was never granted")
	}

	hooks := valueOf(args, "--settings")
	if !strings.Contains(hooks, "PreToolUse") || !strings.Contains(hooks, "/planty gate") {
		t.Errorf("the second layer is missing: %s", hooks)
	}
	if slices.Contains(args, "bypassPermissions") {
		t.Error("acting bypassed the permissions it just configured")
	}
}

func TestAJobVerbBoundaryReachesTheHook(t *testing.T) {
	backend := newCLIBackend("claude", "claude-opus-5")
	args, err := backend.arguments(Request{
		Turns: []Turn{ask(text("inspect the plant"))},
		Acting: &Acting{
			Binary:     "/planty",
			Usage:      "planty agent ...",
			AgentVerbs: []string{"show", "actuators", "actuatorstart", "actuatorstop"},
		},
	}, false)
	if err != nil {
		t.Fatalf("arguments: %v", err)
	}

	hooks := valueOf(args, "--settings")
	if !strings.Contains(hooks, "/planty gate --agent-verbs=show,actuators,actuatorstart,actuatorstop") {
		t.Errorf("the scoped verb boundary is missing from the hook: %s", hooks)
	}
}

// safe-mode leaves ambient permission rules in force and silently disables
// hooks, which would quietly remove one of the two layers above.
func TestIsolationDoesNotRelyOnSafeMode(t *testing.T) {
	backend := newCLIBackend("claude", "claude-opus-5")

	args, err := backend.arguments(Request{Turns: []Turn{ask(text("hi"))}}, false)
	if err != nil {
		t.Fatalf("arguments: %v", err)
	}
	if slices.Contains(args, "--safe-mode") {
		t.Error("--safe-mode is back; it does not isolate permissions and kills hooks")
	}
	if got := valueOf(args, "--setting-sources"); got != "" {
		t.Errorf("setting sources are %q, want none", got)
	}
	if !slices.Contains(args, "--strict-mcp-config") {
		t.Error("ambient MCP servers can still attach")
	}
}

// The command the model is allowed to run is planty's own, so it needs
// planty's own configuration. Stripping it made every agent verb report a
// missing database, which the model relayed as a broken machine.
func TestPlantyConfigurationReachesTheCommand(t *testing.T) {
	t.Setenv("PLANTY_DATABASE_URL", "postgres://example")
	t.Setenv("PLANTY_HA_TOKEN", "secret")
	t.Setenv("SOMETHING_ELSE", "should not travel")

	env := environment(false)

	var sawDatabase, sawToken, sawStranger bool
	for _, entry := range env {
		switch {
		case strings.HasPrefix(entry, "PLANTY_DATABASE_URL="):
			sawDatabase = true
		case strings.HasPrefix(entry, "PLANTY_HA_TOKEN="):
			sawToken = true
		case strings.HasPrefix(entry, "SOMETHING_ELSE="):
			sawStranger = true
		}
	}

	if !sawDatabase {
		t.Error("planty agent has no database to read")
	}
	if !sawToken {
		t.Error("the watering verb cannot reach Home Assistant")
	}
	if sawStranger {
		t.Errorf("the ambient environment leaked into the judgment: %v", env)
	}
}

// An allowlist of nothing is not the same as no allowlist, and the difference
// is the whole internet.
func TestNoTrustedHostsMeansNoWebAtAll(t *testing.T) {
	backend := newCLIBackend("claude", "claude-opus-5")

	args, err := backend.arguments(Request{
		Turns:  []Turn{ask(text("hi"))},
		Acting: &Acting{Binary: "/planty", Usage: "planty agent ..."},
	}, false)
	if err != nil {
		t.Fatalf("arguments: %v", err)
	}

	tools := valueOf(args, "--tools")
	if strings.Contains(tools, "Web") {
		t.Errorf("web tools were granted with nowhere trusted to read: %q", tools)
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "WebFetch(") || arg == "WebSearch" {
			t.Errorf("a web rule appeared with no trusted hosts: %q", arg)
		}
	}
}

// Search finds the page and fetch reads it, so a host list without search is
// nearly useless and search without the list is the open web.
func TestTrustedHostsBecomeFetchRules(t *testing.T) {
	backend := newCLIBackend("claude", "claude-opus-5")

	args, err := backend.arguments(Request{
		Turns: []Turn{ask(text("hi"))},
		Acting: &Acting{
			Binary:  "/planty",
			Usage:   "planty agent ...",
			Trusted: []string{"example.edu", "example.org"},
		},
	}, false)
	if err != nil {
		t.Fatalf("arguments: %v", err)
	}

	if tools := valueOf(args, "--tools"); !strings.Contains(tools, "WebFetch") {
		t.Errorf("tools are %q, want WebFetch among them", tools)
	}
	for _, want := range []string{
		"WebFetch(domain:example.edu)", "WebFetch(domain:example.org)", "WebSearch",
	} {
		if !slices.Contains(args, want) {
			t.Errorf("missing rule %q in %v", want, args)
		}
	}
	// The command it may run is unchanged by any of this.
	if !slices.Contains(args, "Bash(planty agent *)") {
		t.Error("granting the web dropped the one command it is allowed to run")
	}
}

// A person reading the reply is the difference between "queue this question"
// and "just ask them". The command is told which it is, per call, so a
// scheduled job cannot inherit a chat's flag however the pod is configured.
func TestOnlyAConversationSaysSomebodyIsThere(t *testing.T) {
	if slices.Contains(environment(false), "PLANTY_CHAT=1") {
		t.Error("a scheduled job was told somebody is reading")
	}
	if !slices.Contains(environment(true), "PLANTY_CHAT=1") {
		t.Error("a conversation did not say somebody is reading")
	}
}

// Reading a file it can already cat was an arbitrary difference, and the
// photographs are read with this.
func TestReadingIsAlwaysAvailable(t *testing.T) {
	b := newCLIBackend("planty", "claude-opus-5")
	args, err := b.arguments(Request{Schema: map[string]any{}}, false)
	if err != nil {
		t.Fatal(err)
	}

	var tools string
	for i, arg := range args {
		if arg == "--tools" && i+1 < len(args) {
			tools = args[i+1]
		}
	}
	if !strings.Contains(tools, "Read") {
		t.Errorf("Read was not granted: --tools %q", tools)
	}
}

// Every chat surface has the same powers. The no-plant chat used to be handed
// a list of trusted sites and no way to reach any of them.
func TestBothChatsGetTheSameTools(t *testing.T) {
	acting := &Acting{
		Binary:  "planty",
		Usage:   "usage text",
		Trusted: []string{"www.aspca.org"},
		Sources: "sources text",
	}
	b := newCLIBackend("planty", "claude-opus-5")

	withPlant, err := b.arguments(Request{Schema: map[string]any{}, Acting: acting}, false)
	if err != nil {
		t.Fatal(err)
	}
	withoutPlant, err := b.arguments(Request{Schema: map[string]any{}, Acting: acting}, false)
	if err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{withPlant, withoutPlant} {
		tools := valueOf(args, "--tools")
		for _, wanted := range []string{"Read", "Bash", "WebFetch", "WebSearch"} {
			if !strings.Contains(tools, wanted) {
				t.Errorf("%s was not granted: --tools %q", wanted, tools)
			}
		}
	}
}
