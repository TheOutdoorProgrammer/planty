package judge

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Limits on what a trace carries. A tool result can be an entire web page, and
// the whole point is a thing a person skims, not a transcript nobody reads.
const (
	maxStepOutput = 2000
	maxSteps      = 40
)

// streamLine is one event from --output-format stream-json. Everything
// interesting is a content block on a message; the final result arrives as its
// own line carrying the same envelope the plain json format would have given.
type streamLine struct {
	Type    string `json:"type"`
	Message struct {
		Role    string `json:"role"`
		Content []struct {
			Type      string          `json:"type"`
			Text      string          `json:"text"`
			Thinking  string          `json:"thinking"`
			Name      string          `json:"name"`
			ID        string          `json:"id"`
			ToolUseID string          `json:"tool_use_id"`
			Input     json.RawMessage `json:"input"`
			Content   json.RawMessage `json:"content"`
			IsError   bool            `json:"is_error"`
		} `json:"content"`
	} `json:"message"`

	envelope
}

// outcomeFrom reads the streamed events for the answer and the record of how
// it was reached. A malformed line is skipped rather than failed on: losing a
// step is a far smaller loss than losing the answer it belongs to.
func outcomeFrom(raw []byte) (Outcome, error) {
	var out Outcome
	var final *envelope
	// Results arrive on later lines and are only theirs by id: two calls can
	// be in flight at once, and pairing them by order swaps their outputs.
	at := map[string]int{}

	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var event streamLine
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
		if event.Type == "result" {
			shot := event.envelope
			final = &shot
			continue
		}

		for _, block := range event.Message.Content {
			switch block.Type {
			case "thinking":
				out.addStep(Step{Kind: StepThought, Output: clip(block.Thinking)})
			case "tool_use":
				if out.addStep(Step{
					Kind:   StepAction,
					Tool:   block.Name,
					Detail: describeToolInput(block.Name, block.Input),
				}) {
					at[block.ID] = len(out.Steps) - 1
				}
			case "tool_result":
				out.attachResult(at, block.ToolUseID, block.IsError, block.Content)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return Outcome{}, fmt.Errorf("read claude output: %w", err)
	}

	if final == nil {
		return Outcome{}, fmt.Errorf("claude produced no result: %s", truncate(raw))
	}
	answer, err := answerFromEnvelope(*final)
	if err != nil {
		return Outcome{}, err
	}
	out.Answer = answer
	return out, nil
}

// addStep reports whether the step was kept, so a caller can record where it
// landed.
func (o *Outcome) addStep(s Step) bool {
	if len(o.Steps) >= maxSteps {
		return false
	}
	if s.Kind == StepThought && strings.TrimSpace(s.Output) == "" {
		return false
	}
	// StructuredOutput is how the answer is handed back, not something done to
	// reach it, and showing it prints the whole reply a second time.
	if s.Tool == "StructuredOutput" {
		return false
	}
	o.Steps = append(o.Steps, s)
	return true
}

// attachResult hangs a tool's output on the call it belongs to.
func (o *Outcome) attachResult(at map[string]int, id string, failed bool, content json.RawMessage) {
	i, ok := at[id]
	if !ok || i >= len(o.Steps) {
		return
	}
	text := clip(resultText(content))
	if failed {
		text = "failed: " + text
	}
	o.Steps[i].Output = text
}

// describeToolInput reduces a tool call to the one thing worth reading: the
// command, the URL, the file. Falls back to the whole input when the tool is
// not one of the handful this service grants.
func describeToolInput(_ string, input json.RawMessage) string {
	var fields map[string]any
	if err := json.Unmarshal(input, &fields); err != nil {
		return ""
	}

	for _, key := range []string{"command", "url", "file_path", "query", "pattern"} {
		if value, ok := fields[key].(string); ok && value != "" {
			return clip(value)
		}
	}
	return clip(string(input))
}

// resultText pulls readable text out of a tool result, which is sometimes a
// string and sometimes a list of content blocks.
func resultText(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	var plain string
	if err := json.Unmarshal(content, &plain); err == nil {
		return plain
	}

	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(content, &blocks); err == nil {
		var b strings.Builder
		for _, block := range blocks {
			if block.Text != "" {
				b.WriteString(block.Text)
				b.WriteString("\n")
			}
		}
		return strings.TrimSpace(b.String())
	}
	return string(content)
}

func clip(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxStepOutput {
		return s
	}
	return s[:maxStepOutput] + "\n… (truncated)"
}
