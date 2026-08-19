package agent

import "testing"

// The two commands from production that were refused for containing ordinary
// English. Both are inert text inside a quoted flag value.
func TestProseInAQuotedValueIsNotAShell(t *testing.T) {
	allowed := []string{
		`planty agent toxicity --plant golden-pothos --notes "ASPCA publishes only toxic/non-toxic"`,
		`planty agent toxicity --plant golden-pothos --notes "toxic to cats and dogs; severity grading (mild) is derived"`,
		`planty agent note --plant fern --text "it lives by the window & gets morning sun"`,
		`planty agent note --plant fern --text 'cost me $40'`,
		`planty agent toxicity --plant fern --source "https://www.aspca.org/toxic-plants"`,
		`planty agent note --plant fern --text 'the cat chews it; I moved it'`,
	}
	for _, command := range allowed {
		if reason := refuse(command); reason != "" {
			t.Errorf("refused a legitimate command: %s\n  because: %s", command, reason)
		}
	}
}

// Relaxing quotes must not relax what bash actually expands.
func TestQuotingDoesNotOpenASubshell(t *testing.T) {
	refused := []string{
		`planty agent note --plant fern --text "$(cat /etc/passwd)"`,
		"planty agent note --plant fern --text \"`id`\"",
		`planty agent note --plant fern --text "${SECRET}"`,
		// bash reads $4 as a positional parameter, so this really does become
		// "cost me 0". Refusing it is right; single quotes are the fix.
		`planty agent note --plant fern --text "cost me $40"`,
		`planty agent plants && cat /etc/passwd`,
		`planty agent plants; cat /etc/passwd`,
		`planty agent plants | cat`,
		`planty agent plants > /tmp/out`,
		`planty agent note --text "unclosed`,
		`planty agent note --text 'unclosed`,
		"planty agent plants\ncat /etc/passwd",
	}
	for _, command := range refused {
		if refuse(command) == "" {
			t.Errorf("ALLOWED a command that can run something: %s", command)
		}
	}
}

// Single quotes make a subshell literal, which is why they are the escape
// hatch offered when double quotes are refused.
func TestSingleQuotesAreLiteral(t *testing.T) {
	if reason := refuse(`planty agent note --plant fern --text '$(id) is not run here'`); reason != "" {
		t.Errorf("refused literal text in single quotes: %s", reason)
	}
}

func TestOnlyTheAllowedCommandsRun(t *testing.T) {
	for _, command := range []string{
		"curl https://example.com", "python3 -c 'print(1)'", "sh", "bash -c ls",
		"rm -rf /", "planty serve", "planty daily", "planty autopsy fern",
		"/usr/local/bin/planty agent plants", "env", "printenv",
	} {
		if refuse(command) == "" {
			t.Errorf("ALLOWED a command outside the allowlist: %s", command)
		}
	}
}

func TestLookingAroundIsAllowed(t *testing.T) {
	for _, command := range []string{
		"ls -la", "cat /etc/hostname", "grep -r pothos .", "pwd", "date",
		"find . -name '*.jpg'", "wc -l photos.txt", "head -20 day-01.jpg",
	} {
		if reason := refuse(command); reason != "" {
			t.Errorf("refused a harmless look: %s\n  because: %s", command, reason)
		}
	}
}

// The pod's environment holds a Home Assistant token that runs a pump.
func TestTheEnvironmentStaysOutOfTheConversation(t *testing.T) {
	for _, command := range []string{
		"cat /proc/self/environ",
		"cat /proc/1/environ",
		"grep PLANTY /proc/self/environ",
		"ls /proc/self/",
	} {
		if refuse(command) == "" {
			t.Errorf("ALLOWED reading the environment: %s", command)
		}
	}
}
