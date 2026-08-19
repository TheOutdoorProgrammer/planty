package agent

import (
	"bytes"
	"io"
	"slices"
	"strings"
	"testing"
)

func gated(t *testing.T, command string) int {
	t.Helper()

	payload := `{"tool_name":"Bash","tool_input":{"command":` + quote(command) + `}}`
	return Gate(strings.NewReader(payload), io.Discard)
}

func quote(s string) string {
	var b bytes.Buffer
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func TestTheAgentVerbsAreAllowed(t *testing.T) {
	for _, command := range []string{
		"planty agent log --plant golden-pothos --kind watered",
		"planty agent remind --plant blue-oyster --kind misted --every-days 1 --at 8,20",
		"planty agent forget --plant golden-pothos --kind watered",
		`planty agent log --plant golden-pothos --kind note --note "looked droopy"`,
	} {
		if code := gated(t, command); code != 0 {
			t.Errorf("blocked a permitted command: %s", command)
		}
	}
}

// The binary the model is being handed can also run a pump and migrate a
// schema. Prefix-matching on "planty" alone would give it both.
func TestThePlantyCLIIsNotItselfTheAllowlist(t *testing.T) {
	for _, command := range []string{
		"planty water",
		"planty migrate",
		"planty seed",
		"planty autopsy",
		"planty autopsy golden-pothos",
		"planty daily",
		"planty cold",
		"planty away",
		"planty chase",
		"planty thirst",
		"planty ingest",
		"planty remind",
		"planty serve",
		"planty gate",
	} {
		if code := gated(t, command); code != Blocked {
			t.Errorf("a dangerous planty verb was allowed through: %s", command)
		}
	}
}

// The gate only checks the `planty agent ` prefix; which verbs exist is Run's
// decision. TestExcludedCommandsAreNotAgentVerbs is the tripwire that fires if
// somebody ever adds an autopsy verb behind this passing prefix.
func TestTheGatePassesAgentAutopsyToRunToRefuse(t *testing.T) {
	if code := gated(t, "planty agent autopsy golden-pothos"); code != 0 {
		t.Error("the gate started deciding verbs; that job belongs to Run")
	}
}

// One command becoming two is the whole game.
func TestNothingTurnsOneCommandIntoTwo(t *testing.T) {
	for _, command := range []string{
		"planty agent log --plant x --kind watered && touch /tmp/pwned",
		"planty agent log --plant x --kind watered; whoami",
		"planty agent log --plant x --kind watered | tee /tmp/pwned",
		"planty agent log --plant $(whoami) --kind watered",
		"planty agent log --plant `whoami` --kind watered",
		"planty agent log --plant x --kind watered > /tmp/pwned",
		"planty agent log --plant x --kind watered\nwhoami",
		"planty agent log --plant x --kind watered & whoami",
	} {
		if code := gated(t, command); code != Blocked {
			t.Errorf("an escape was allowed through: %q", command)
		}
	}
}

// Reading is allowed and running is not, so the list here is anything that
// executes, reaches the network, or is planty wearing a different hat.
func TestAnythingThatIsNotReadingOrPlantyAgentIsBlocked(t *testing.T) {
	for _, command := range []string{
		"whoami",
		"env",
		"printenv",
		"curl https://example.com",
		"wget https://example.com",
		"/planty agent log --plant x --kind watered",
		"sh -c 'planty agent log'",
		"bash",
		"python3 -c 'print(1)'",
		"plantyagent log",
		"planty  agent log",
		"planty serve",
		"rm -rf /",
		"mv a b",
		"chmod 777 x",
	} {
		if code := gated(t, command); code != Blocked {
			t.Errorf("allowed through: %q", command)
		}
	}
}

// The hook is only wired to Bash. Blocking a tool it was never meant to see
// would break reading a photograph for no gain.
func TestOtherToolsPassThrough(t *testing.T) {
	payload := `{"tool_name":"Read","tool_input":{"command":"whatever"}}`
	if code := Gate(strings.NewReader(payload), io.Discard); code != 0 {
		t.Error("the gate blocked a tool it does not police")
	}
}

// A payload that cannot be read is not one that can be trusted.
func TestUnreadableInputIsBlocked(t *testing.T) {
	if code := Gate(strings.NewReader("not json"), io.Discard); code != Blocked {
		t.Error("unreadable hook input was allowed through")
	}
	if code := Gate(strings.NewReader(""), io.Discard); code != Blocked {
		t.Error("empty hook input was allowed through")
	}
}

func TestTheRefusalSaysWhy(t *testing.T) {
	var explained bytes.Buffer
	payload := `{"tool_name":"Bash","tool_input":{"command":"whoami"}}`

	if code := Gate(strings.NewReader(payload), &explained); code != Blocked {
		t.Fatal("whoami was allowed")
	}
	if !strings.Contains(explained.String(), "planty gate") {
		t.Errorf("the refusal does not say who refused: %q", explained.String())
	}
}

// The prompt tells the model where to read; the rules are what stop it. A
// list that only lives in prose is a suggestion, so both have to name the same
// hosts and the rules have to be the thing that actually holds.
func TestTheTrustedSourcesAreNamedInBothPlaces(t *testing.T) {
	if len(Trusted) == 0 {
		t.Fatal("no trusted hosts, which turns the web off entirely")
	}
	for _, host := range Trusted {
		if !strings.Contains(Sources, host) {
			t.Errorf("%s is allowlisted but the model is never told what it is for", host)
		}
		if strings.HasPrefix(host, "http") || strings.Contains(host, "/") {
			t.Errorf("%q is a URL, not a hostname; a WebFetch rule takes a bare host", host)
		}
	}
	// aspca.org redirects across hosts to www, and the fetcher will not follow
	// that, so the bare apex would silently never work.
	if strings.Contains(Sources, "aspca") && !slices.Contains(Trusted, "www.aspca.org") {
		t.Error("the ASPCA host must be www., or every fetch of it stops at a redirect")
	}
}
