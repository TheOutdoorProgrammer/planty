package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
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

// chaining turns one command into two, and can only do so outside quotes.
// Inside either kind it is ordinary text.
const chaining = ";&|<>()\n\r"

// expanding runs something. Single quotes make these literal and double quotes
// do not, which is why this gate has to know the difference: "$(id)" is a
// subshell and '$(id)' is four characters.
const expanding = "$`\\"

// looking are read-only commands the model may use to orient itself. Nothing
// here writes, and redirection is refused separately, so the worst outcome is
// reading something inside this pod.
var looking = map[string]bool{
	"ls": true, "cat": true, "head": true, "tail": true, "grep": true,
	"find": true, "wc": true, "pwd": true, "date": true, "echo": true,
	"file": true, "stat": true, "du": true, "df": true, "which": true,
	"sort": true, "uniq": true, "cut": true, "tr": true, "basename": true,
	"dirname": true, "realpath": true, "seq": true, "true": true, "false": true,
}

// Gate reads a hook payload and returns the exit code to leave with.
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

// refuse names why a command is not allowed, or is empty when it is. It reads
// the command as bash would rather than scanning the raw string, which used to
// refuse a note containing "toxic/non-toxic" or a semicolon inside a sentence.
func refuse(command string) string {
	if reason := checkQuoting(command); reason != "" {
		return reason
	}

	verb, rest, _ := strings.Cut(command, " ")
	switch {
	case verb == "planty":
		if !strings.HasPrefix(rest, "agent ") {
			// Says where the answer already is: the refusal that only said no
			// was followed by the model trying two more ways to read help.
			return "planty may only be run as `planty agent <verb>`, and its " +
				"complete reference is already in your instructions, so there " +
				"is nothing to learn by running help"
		}
	case looking[verb]:
		if reason := checkSecrets(command); reason != "" {
			return reason
		}
	default:
		return fmt.Sprintf("%q is not a command this may run: `planty agent`, "+
			"or one of the read-only shell commands", verb)
	}
	return ""
}

// checkSecrets keeps this pod's environment out of a conversation that is
// stored and rendered back: it holds a Home Assistant token that operates a
// physical pump, a database URL and an OAuth token.
func checkSecrets(command string) string {
	lowered := strings.ToLower(command)
	for _, giveaway := range []string{"environ", "/proc/self/", "/proc/1/"} {
		if strings.Contains(lowered, giveaway) {
			return "reading this process's environment would put its credentials " +
				"into the conversation, which is stored and shown in the app"
		}
	}
	return ""
}

// checkQuoting reports a command whose shape could run something, reading it
// as bash would.
func checkQuoting(command string) string {
	var inSingle, inDouble bool

	for _, r := range command {
		switch {
		case inSingle:
			// Single quotes make everything literal, including a subshell.
			if r == '\'' {
				inSingle = false
			}
		case inDouble:
			if r == '"' {
				inDouble = false
				continue
			}
			if strings.ContainsRune(expanding, r) {
				return "a value in double quotes may not contain " + name(r) +
					", which bash would still expand; use single quotes for literal text"
			}
		default:
			switch {
			case r == '\'':
				inSingle = true
			case r == '"':
				inDouble = true
			case strings.ContainsRune(chaining, r), strings.ContainsRune(expanding, r):
				return "a command may not contain " + name(r) + " outside quotes; " +
					"run one command at a time"
			}
		}
	}

	// An unclosed quote means the rest of the line was read as text when it
	// might not be, so the reading above cannot be trusted.
	if inSingle || inDouble {
		return "a command may not leave a quote open"
	}
	return ""
}

// name says which character was the problem, since "shell metacharacters" sent
// the model rewriting the wrong part of a long command.
func name(r rune) string {
	switch r {
	case '\n', '\r':
		return "a line break"
	case '`':
		return "a backtick"
	case '\\':
		return "a backslash"
	}
	return string(r)
}

// LiveChat is set on the command only while a person is on the other end of a
// conversation. It is set per call rather than on the process, so a scheduled
// job cannot inherit it.
const LiveChat = "PLANTY_CHAT"

// notWhileTalking refuses to queue a question for somebody who is already
// reading the reply.
//
// A queued question is for a steward who is not here: the friend whose plants
// these are. Asked of the person in the conversation it goes into a table
// nobody opens, which is exactly what happened — ten of them.
func notWhileTalking(of string) error {
	if os.Getenv(LiveChat) == "" {
		return nil
	}
	if of != "" && of != plant.StewardSelf {
		return nil
	}
	return errors.New("there is a person reading this reply right now, so ask them in it. " +
		"The ask verb is for a question aimed at somebody who is not here, " +
		"named with --of")
}
