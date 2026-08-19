package judge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// cliBackend runs the judgment through the Claude Code CLI, which bills a
// Claude subscription instead of metering tokens.
type cliBackend struct {
	binary string
	model  string
}

func newCLIBackend(binary, model string) *cliBackend {
	return &cliBackend{binary: binary, model: model}
}

func (b *cliBackend) Name() string { return "claude code cli" }

// envelope is the --output-format json wrapper around the answer.
type envelope struct {
	IsError          bool            `json:"is_error"`
	Subtype          string          `json:"subtype"`
	StopReason       string          `json:"stop_reason"`
	Result           string          `json:"result"`
	StructuredOutput json.RawMessage `json:"structured_output"`
}

func (b *cliBackend) Judge(ctx context.Context, req Request) (string, error) {
	answer, err := b.run(ctx, req, req.Session != nil && req.Session.Resuming)
	if !errors.Is(err, errSessionGone) {
		return answer, err
	}

	// The session outlived nothing: an emptyDir goes with its pod. Starting it
	// again from the transcript is slower and still right, which is the only
	// acceptable way for this optimisation to fail.
	return b.run(ctx, req, false)
}

// run performs one call, either continuing the session or establishing it.
func (b *cliBackend) run(ctx context.Context, req Request, resuming bool) (string, error) {
	// Dies with the call: the photographs, and whatever the CLI writes beside
	// them. Sessions are keyed by id rather than directory, so a fresh one
	// every time still resumes.
	dir, err := os.MkdirTemp("", "planty-judge-")
	if err != nil {
		return "", fmt.Errorf("scratch directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	// Resuming means the model already read everything above, so only the new
	// question is sent. That is the whole saving.
	turns := req.Turns
	if resuming {
		turns = turns[len(turns)-1:]
	}

	prompt, err := render(dir, turns)
	if err != nil {
		return "", err
	}
	catalogue, err := stage(dir, req.Offered)
	if err != nil {
		return "", err
	}
	prompt += catalogue
	args, err := b.arguments(req, resuming)
	if err != nil {
		return "", err
	}

	// "--" or the prompt is swallowed: --tools takes a list, so a prompt
	// following it is read as one more tool name and the call dies asking for
	// input it was handed.
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, b.binary, append(args, "--", prompt)...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.Env = environment()

	if err := cmd.Run(); err != nil {
		complaint := strings.TrimSpace(stderr.String())
		if resuming && mentionsAMissingSession(complaint) {
			return "", errSessionGone
		}
		return "", fmt.Errorf("claude: %w: %s", err, complaint)
	}
	return answerFrom(stdout.Bytes())
}

// errSessionGone reports that a session we expected to continue is not there,
// which is ordinary rather than broken and is retried from the transcript.
var errSessionGone = errors.New("the session is gone")

func mentionsAMissingSession(complaint string) bool {
	lowered := strings.ToLower(complaint)
	for _, phrase := range []string{"no conversation found", "session not found", "no session"} {
		if strings.Contains(lowered, phrase) {
			return true
		}
	}
	return false
}

func (b *cliBackend) arguments(req Request, resuming bool) ([]string, error) {
	schema, err := json.Marshal(req.Schema)
	if err != nil {
		return nil, fmt.Errorf("encode schema: %w", err)
	}

	args := []string{
		"--print",
		"--output-format", "json",
		"--model", b.model,
		"--json-schema", string(schema),
		// Isolation, and not --safe-mode: that leaves ambient permission rules
		// in force and silently disables hooks. These two exclude the settings
		// files and the MCP servers that would otherwise attach.
		"--setting-sources", "",
		"--strict-mcp-config",
	}

	switch {
	case req.Session == nil:
		// One-shot work has nothing to continue, and a session per daily
		// verdict is a disk filling up for no reason.
		args = append(args, "--no-session-persistence", "--system-prompt", req.System)
	case resuming:
		// The system prompt is already in the session, and passing it again
		// would put a second copy of it in the context being paid for.
		args = append(args, "--resume", req.Session.ID.String())
	default:
		args = append(args, "--session-id", req.Session.ID.String(),
			"--system-prompt", req.System)
	}

	if req.Effort != "" {
		args = append(args, "--effort", string(req.Effort))
	}

	tools := make([]string, 0, 2)
	// Images cannot ride inline on a CLI prompt, so they are files the model
	// opens, and Read is the only tool it needs to do it.
	if req.images() > 0 || len(req.Offered) > 0 {
		tools = append(tools, "Read")
	}
	if req.Acting != nil {
		tools = append(tools, "Bash")
		tools = append(tools, webTools(req.Acting)...)
		args = append(args, acting(req.Acting)...)
	}
	return append(args, "--tools", strings.Join(tools, ",")), nil
}

// acting grants one command, twice over. dontAsk is what turns an allowlist
// into a boundary, since print mode starts in manual where a rule only spares
// a prompt nobody could answer. The hook is an independent second layer.
func acting(a *Acting) []string {
	hooks := fmt.Sprintf(
		`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":%q}]}]}}`,
		a.Binary+" gate")

	allowed := []string{"Bash(planty agent *)"}
	for _, host := range a.Trusted {
		allowed = append(allowed, fmt.Sprintf("WebFetch(domain:%s)", host))
	}
	if len(a.Trusted) > 0 {
		// Search finds the page; fetch is what reads it, and only from a host
		// on the list. Searching without fetching is nearly useless, and
		// fetching without the list is the whole internet.
		allowed = append(allowed, "WebSearch")
	}

	args := []string{"--permission-mode", "dontAsk"}
	for _, rule := range allowed {
		args = append(args, "--allowedTools", rule)
	}
	return append(args, "--settings", hooks)
}

// webTools are added when the model has somewhere trusted to read.
func webTools(a *Acting) []string {
	if a == nil || len(a.Trusted) == 0 {
		return nil
	}
	return []string{"WebSearch", "WebFetch"}
}

// environment strips the ambient session so a judgment run from a developer's
// shell behaves exactly like one run from the pod.
func environment() []string {
	keep := []string{"PATH", "HOME", "CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_API_KEY"}

	out := make([]string, 0, len(keep)+8)
	for _, name := range keep {
		if value, ok := os.LookupEnv(name); ok {
			out = append(out, name+"="+value)
		}
	}

	// Planty's own configuration reaches the command Planty lets it run.
	// Stripping this made `planty agent` report a missing database, which the
	// model then relayed as an environment problem on the machine.
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "PLANTY_") {
			out = append(out, entry)
		}
	}
	return out
}

// render flattens a conversation into the single prompt string the CLI takes,
// writing photographs into dir for the model to open by name.
func render(dir string, turns []Turn) (string, error) {
	var body strings.Builder
	var frames int
	transcript := len(turns) > 1

	for _, turn := range turns {
		if transcript {
			fmt.Fprintf(&body, "<%s>\n", turn.Role)
		}
		for _, part := range turn.Parts {
			if part.Image == nil {
				body.WriteString(part.Text)
				body.WriteString("\n")
				continue
			}
			frames++
			name := fmt.Sprintf("photo-%02d%s", frames, extensionFor(part.Image.Media))
			if err := os.WriteFile(filepath.Join(dir, name), part.Image.Bytes, 0o600); err != nil {
				return "", fmt.Errorf("stage photograph: %w", err)
			}
			fmt.Fprintf(&body, "[photograph %s]\n", name)
		}
		if transcript {
			fmt.Fprintf(&body, "</%s>\n\n", turn.Role)
		}
	}

	if frames == 0 {
		return body.String(), nil
	}
	return fmt.Sprintf("Read all %d photographs named below from the current "+
		"directory before answering. They are the evidence.\n\n%s",
		frames, body.String()), nil
}

// stage writes the offered photographs into dir and returns the catalogue that
// tells the model they are there. This is where "you may look" is actually
// expressed: the files exist, nothing has been read, and it decides.
func stage(dir string, offered []Offer) (string, error) {
	if len(offered) == 0 {
		return "", nil
	}

	var b strings.Builder
	b.WriteString("\n\nPhotographs of this plant are on disk in the current " +
		"directory. Nothing has been looked at. Open one only if seeing it " +
		"would change your answer; most questions do not need any.\n")

	for i, offer := range offered {
		name := fmt.Sprintf("day-%02d%s", i+1, extensionFor(offer.Media))
		if err := os.WriteFile(filepath.Join(dir, name), offer.Bytes, 0o600); err != nil {
			return "", fmt.Errorf("stage offered photograph: %w", err)
		}
		fmt.Fprintf(&b, "  %s  %s\n", name, offer.Label)
	}
	return b.String(), nil
}

func extensionFor(media string) string {
	switch media {
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg"
	}
}

// answerFrom pulls the judgment out of the wrapper, preferring the schema
// validated object over the raw text that carries the same thing unchecked.
func answerFrom(raw []byte) (string, error) {
	var wrapper envelope
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return "", fmt.Errorf("decode claude output: %w: %s", err, truncate(raw))
	}
	if wrapper.StopReason == "refusal" {
		return "", ErrRefused
	}
	if wrapper.IsError || wrapper.Subtype != "success" {
		return "", fmt.Errorf("claude reported %s: %s", wrapper.Subtype, wrapper.Result)
	}
	if len(wrapper.StructuredOutput) > 0 {
		return string(wrapper.StructuredOutput), nil
	}
	if wrapper.Result == "" {
		return "", fmt.Errorf("claude returned no answer")
	}
	return wrapper.Result, nil
}

func truncate(raw []byte) string {
	const limit = 500
	if len(raw) <= limit {
		return string(raw)
	}
	return string(raw[:limit]) + "..."
}
