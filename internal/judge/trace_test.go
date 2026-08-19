package judge

import (
	"strings"
	"testing"
)

// Two calls in flight, results arriving in the other order. Pairing by
// position put each output under the wrong command, which in a feature for
// showing what happened is worse than showing nothing.
func TestAResultLandsOnItsOwnCall(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"assistant","message":{"role":"assistant","content":[ {"type":"tool_use","id":"a","name":"Bash","input":{"command":"planty agent show --plant fern"}}, {"type":"tool_use","id":"b","name":"Bash","input":{"command":"planty agent notes --plant fern"}}]}}`,
		`{"type":"user","message":{"role":"user","content":[ {"type":"tool_result","tool_use_id":"b","content":"no notes on fern"}]}}`,
		`{"type":"user","message":{"role":"user","content":[ {"type":"tool_result","tool_use_id":"a","content":"Fern (fern): houseplant"}]}}`,
		`{"type":"result","subtype":"success","result":"{\"reply\":\"ok\"}"}`,
	}, "\n")

	out, err := outcomeFrom([]byte(stream))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Steps) != 2 {
		t.Fatalf("expected two steps, got %d", len(out.Steps))
	}
	if !strings.Contains(out.Steps[0].Detail, "show") ||
		!strings.Contains(out.Steps[0].Output, "houseplant") {
		t.Errorf("the show command got the wrong output: %+v", out.Steps[0])
	}
	if !strings.Contains(out.Steps[1].Detail, "notes") ||
		!strings.Contains(out.Steps[1].Output, "no notes") {
		t.Errorf("the notes command got the wrong output: %+v", out.Steps[1])
	}
}

func TestARefusalIsShownAsOne(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"assistant","message":{"role":"assistant","content":[ {"type":"tool_use","id":"a","name":"Bash","input":{"command":"rm -rf /"}}]}}`,
		`{"type":"user","message":{"role":"user","content":[ {"type":"tool_result","tool_use_id":"a","is_error":true,"content":"gate: refused"}]}}`,
		`{"type":"result","subtype":"success","result":"{\"reply\":\"ok\"}"}`,
	}, "\n")

	out, err := outcomeFrom([]byte(stream))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.Steps[0].Output, "failed:") {
		t.Errorf("a refused command did not read as refused: %q", out.Steps[0].Output)
	}
}

// The answer must survive a trace that cannot be parsed, since the reply is
// what the person actually asked for.
func TestRubbishInTheStreamDoesNotLoseTheAnswer(t *testing.T) {
	stream := strings.Join([]string{
		`not json at all`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"hmm"}]}}`,
		`{"type":"result","subtype":"success","result":"{\"reply\":\"still here\"}"}`,
	}, "\n")

	out, err := outcomeFrom([]byte(stream))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Answer, "still here") {
		t.Errorf("the answer was lost: %q", out.Answer)
	}
	if len(out.Steps) != 1 || out.Steps[0].Kind != StepThought {
		t.Errorf("the thought was lost: %+v", out.Steps)
	}
}

// The answer-returning mechanism is not a step, and printing it would repeat
// the whole reply inside its own trace.
func TestTheAnswerIsNotAlsoAStep(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"assistant","message":{"role":"assistant","content":[ {"type":"tool_use","id":"a","name":"StructuredOutput","input":{"reply":"the whole answer"}}]}}`,
		`{"type":"result","subtype":"success","result":"{\"reply\":\"the whole answer\"}"}`,
	}, "\n")

	out, err := outcomeFrom([]byte(stream))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Steps) != 0 {
		t.Errorf("StructuredOutput appeared in the trace: %+v", out.Steps)
	}
}

func TestAStreamWithNoResultIsAnError(t *testing.T) {
	if _, err := outcomeFrom([]byte(`{"type":"assistant","message":{"content":[]}}`)); err == nil {
		t.Error("a stream that never produced an answer was accepted")
	}
}

// A page fetch is worth showing as its URL, not as its JSON input blob.
func TestAFetchIsShownAsItsURL(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"assistant","message":{"role":"assistant","content":[ {"type":"tool_use","id":"a","name":"WebFetch","input":{"url":"https://www.aspca.org/x","prompt":"is it toxic"}}]}}`,
		`{"type":"result","subtype":"success","result":"{\"reply\":\"ok\"}"}`,
	}, "\n")

	out, err := outcomeFrom([]byte(stream))
	if err != nil {
		t.Fatal(err)
	}
	if out.Steps[0].Detail != "https://www.aspca.org/x" {
		t.Errorf("the fetch did not show its URL: %q", out.Steps[0].Detail)
	}
}
