package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Blocked is the exit code a PreToolUse hook uses to stop a tool call. It is
// evaluated before the permission rules, which is what makes this a second
// layer rather than a duplicate of the allowlist.
const Blocked = 2

// hookInput is the part of the PreToolUse payload the gate reads.
type hookInput struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
}

// forbidden are the characters that turn one command into two. The allowlist
// already splits compounds and refuses each part, so this agrees with it
// rather than replacing it: two independent gates, one shell to get past.
const forbidden = ";&|`$()<>\n\r"

// Gate reads a hook payload and returns the exit code to leave with. It blocks
// everything but `planty agent`, including this binary's own other verbs: the
// model may record what happened to a plant and may not run the pump.
func Gate(in io.Reader, explain io.Writer) int {
	var payload hookInput
	if err := json.NewDecoder(in).Decode(&payload); err != nil {
		// A payload that cannot be read is not a payload that can be trusted.
		_, _ = fmt.Fprintf(explain, "planty gate: unreadable hook input: %v\n", err)
		return Blocked
	}

	// Only Bash is gated; the hook should not be matching anything else, and
	// silently blocking a tool it was never meant to see would be worse.
	if payload.ToolName != "Bash" {
		return 0
	}

	command := strings.TrimSpace(payload.ToolInput.Command)
	if reason := refuse(command); reason != "" {
		_, _ = fmt.Fprintf(explain, "planty gate: %s\n", reason)
		return Blocked
	}
	return 0
}

// refuse names why a command is not allowed, or is empty when it is.
func refuse(command string) string {
	if strings.ContainsAny(command, forbidden) {
		return "a command may not contain shell metacharacters"
	}
	if !strings.HasPrefix(command, "planty agent ") {
		return "only `planty agent` commands are permitted, and this is not one"
	}
	if strings.Contains(command, "..") || strings.Contains(command, "/") {
		return "a command may not contain a path"
	}
	return ""
}
