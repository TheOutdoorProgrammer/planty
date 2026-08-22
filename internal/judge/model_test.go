package judge

import "testing"

// The model rides on the request so six call sites can share one Judge without
// agreeing on a model. The backend's own is only the fallback.
func TestTheRequestsModelWinsOverTheBackends(t *testing.T) {
	backend := newCLIBackend("claude", "claude-opus-5")

	chosen, err := backend.arguments(Request{
		Turns: []Turn{ask(text("hi"))},
		Model: "claude-haiku-4-5-20251001",
	}, false)
	if err != nil {
		t.Fatalf("arguments: %v", err)
	}
	if got := valueOf(chosen, "--model"); got != "claude-haiku-4-5-20251001" {
		t.Errorf("the request's model was ignored: --model %q", got)
	}

	fallback, err := backend.arguments(Request{Turns: []Turn{ask(text("hi"))}}, false)
	if err != nil {
		t.Fatalf("arguments: %v", err)
	}
	if got := valueOf(fallback, "--model"); got != "claude-opus-5" {
		t.Errorf("a request naming no model lost the backend's: --model %q", got)
	}
}

func TestModelForPrefersTheRequest(t *testing.T) {
	if got := modelFor(Request{Model: "a"}, "b"); got != "a" {
		t.Errorf("modelFor picked %q over the request's own", got)
	}
	if got := modelFor(Request{}, "b"); got != "b" {
		t.Errorf("modelFor dropped the fallback, giving %q", got)
	}
}
