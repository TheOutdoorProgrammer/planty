package judge

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type assignmentSource func(context.Context, Job) (Model, bool, error)

func (s assignmentSource) For(ctx context.Context, job Job) (Model, bool, error) {
	return s(ctx, job)
}

func TestDispatchStopsOnAssignmentReadError(t *testing.T) {
	want := errors.New("assignment database unavailable")
	fallback := &requestCaptureBackend{}
	assigned := &requestCaptureBackend{}
	seat := (&Judge{
		fallback: fallback, backends: map[string]Backend{"opencode-go": assigned},
	}).Assigned(assignmentSource(func(_ context.Context, job Job) (Model, bool, error) {
		if job != JobAssess {
			t.Fatalf("assignment lookup for %q", job)
		}
		return Model{}, false, want
	}))

	_, err := seat.dispatch(context.Background(), Request{Job: JobAssess})
	if !errors.Is(err, want) || !strings.Contains(err.Error(), "assess") {
		t.Fatalf("dispatch error = %v, want contextual assignment error", err)
	}
	if fallback.request.Job != "" || assigned.request.Job != "" {
		t.Fatal("an assignment lookup failure called a model backend")
	}
}

func TestDispatchUsesFallbackOnlyWithoutAssignment(t *testing.T) {
	chosen, ok := Lookup("opencode-go", "qwen3.8-max")
	if !ok {
		t.Fatal("missing catalog fixture")
	}
	for _, assigned := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent", true: "assigned"}[assigned], func(t *testing.T) {
			fallback := &requestCaptureBackend{}
			selected := &requestCaptureBackend{}
			seat := (&Judge{
				fallback: fallback, backends: map[string]Backend{chosen.Provider: selected},
			}).Assigned(assignmentSource(func(context.Context, Job) (Model, bool, error) {
				return chosen, assigned, nil
			}))
			if _, err := seat.dispatch(context.Background(), Request{Job: JobAssess}); err != nil {
				t.Fatal(err)
			}
			if assigned {
				if selected.request.Model != chosen.ID || selected.request.Job != JobAssess || fallback.request.Job != "" {
					t.Fatal("explicit assignment did not exclusively select its model backend")
				}
			} else if fallback.request.Job != JobAssess || fallback.request.Model != "" || selected.request.Job != "" {
				t.Fatal("absent assignment did not exclusively select the fallback")
			}
		})
	}
}

func TestDispatchDoesNotFallbackFromInvalidAssignment(t *testing.T) {
	for _, tc := range []struct {
		name, model string
		configured  bool
		want        string
	}{
		{"incompatible", "deepseek-v4-flash", true, "images"},
		{"unconfigured", "qwen3.8-max", false, "not configured"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chosen, ok := Lookup("opencode-go", tc.model)
			if !ok {
				t.Fatal("missing catalog fixture")
			}
			fallback := &requestCaptureBackend{}
			selected := &requestCaptureBackend{}
			seat := (&Judge{fallback: fallback, backends: map[string]Backend{}}).Assigned(
				assignmentSource(func(context.Context, Job) (Model, bool, error) { return chosen, true, nil }),
			)
			if tc.configured {
				seat.backends[chosen.Provider] = selected
			}
			_, err := seat.dispatch(context.Background(), Request{Job: JobAssess})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("dispatch error = %v, want %q", err, tc.want)
			}
			if !tc.configured && (!errors.Is(err, ErrProviderUnavailable) || Retryable(err)) {
				t.Fatalf("missing provider was not classified as permanent: %v", err)
			}
			if fallback.request.Job != "" || selected.request.Job != "" {
				t.Fatal("an invalid assignment called a model backend")
			}
		})
	}
}

func TestDispatchWithoutFallbackRequiresProviderConfiguration(t *testing.T) {
	_, err := (&Judge{}).dispatch(context.Background(), Request{Job: JobAssess})
	if !errors.Is(err, ErrProviderUnavailable) || Retryable(err) {
		t.Fatalf("missing fallback was not classified as permanent: %v", err)
	}
}
