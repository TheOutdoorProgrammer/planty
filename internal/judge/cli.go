package judge

import (
	"bytes"
	"context"
	"encoding/json"
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
	// Everything for one call lives here and dies with it: the photographs the
	// model reads, and whatever state the CLI decides to write beside them.
	dir, err := os.MkdirTemp("", "planty-judge-")
	if err != nil {
		return "", fmt.Errorf("scratch directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	prompt, err := render(dir, req.Turns)
	if err != nil {
		return "", err
	}
	catalogue, err := stage(dir, req.Offered)
	if err != nil {
		return "", err
	}
	prompt += catalogue
	args, err := b.arguments(req)
	if err != nil {
		return "", err
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, b.binary, append(args, prompt)...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.Env = environment()

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return answerFrom(stdout.Bytes())
}

func (b *cliBackend) arguments(req Request) ([]string, error) {
	schema, err := json.Marshal(req.Schema)
	if err != nil {
		return nil, fmt.Errorf("encode schema: %w", err)
	}

	args := []string{
		"--print",
		"--output-format", "json",
		"--model", b.model,
		"--system-prompt", req.System,
		"--json-schema", string(schema),
		// Without this the judgment inherits whatever CLAUDE.md, hooks and MCP
		// servers happen to sit above the working directory, which for a plant
		// verdict is contamination rather than context.
		"--safe-mode",
		"--no-session-persistence",
	}
	if req.Effort != "" {
		args = append(args, "--effort", string(req.Effort))
	}

	if req.images() == 0 && len(req.Offered) == 0 {
		return append(args, "--tools", ""), nil
	}
	// Images cannot ride inline on a CLI prompt, so they are files the model
	// has to open, and Read is the only tool it gets to do it with.
	return append(args, "--tools", "Read", "--permission-mode", "bypassPermissions"), nil
}

// environment strips the ambient session so a judgment run from a developer's
// shell behaves exactly like one run from the pod.
func environment() []string {
	keep := []string{"PATH", "HOME", "CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_API_KEY"}

	out := make([]string, 0, len(keep))
	for _, name := range keep {
		if value, ok := os.LookupEnv(name); ok {
			out = append(out, name+"="+value)
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
