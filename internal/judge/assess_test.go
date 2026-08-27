package judge

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
)

type sequenceBackend struct {
	outcomes []Outcome
	errors   []error
	calls    int
	requests []Request
}

func (b *sequenceBackend) Judge(_ context.Context, request Request) (Outcome, error) {
	b.requests = append(b.requests, request)
	i := b.calls
	b.calls++
	if i < len(b.errors) && b.errors[i] != nil {
		return Outcome{}, b.errors[i]
	}
	return b.outcomes[i], nil
}

func TestAssessAttachesAndNamesTheExactLatestPhotograph(t *testing.T) {
	photoID := uuid.New()
	photo := testImage(t, "jpeg")
	backend := &sequenceBackend{outcomes: []Outcome{{
		Answer: `{"action":"none","reasoning":"Wait today.","confidence":0.8,"sensor_summary":"Leaves look steady.","health_mode":"baseline","health_value":85,"health_reasoning":"The attached current photo shows upright leaves."}`,
		Model:  "model-a",
	}}}
	seat := &Judge{fallback: backend}

	result, err := seat.Assess(context.Background(), Evidence{
		LatestPhoto: &PhotoEvidence{
			ID: photoID, TakenAt: time.Now().Add(-time.Hour), Caption: "whole plant",
			Frame: Frame{Media: "image/jpeg", Bytes: photo},
		},
	})
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if result.HealthValue != 85 || len(backend.requests) != 1 {
		t.Fatalf("unexpected result or requests: %#v %d", result, len(backend.requests))
	}
	parts := backend.requests[0].Turns[0].Parts
	if len(parts) != 2 || parts[1].Image == nil || !bytes.Equal(parts[1].Image.Bytes, photo) {
		t.Fatalf("assessment did not receive the exact photo bytes: %#v", parts)
	}
	if parts[0].Text == "" || backend.requests[0].Job != JobAssess {
		t.Fatalf("assessment context was lost: %#v", backend.requests[0])
	}
}

func TestScheduledAssessmentReceivesTheCompleteAgentReferenceAndFanRules(t *testing.T) {
	backend := &sequenceBackend{outcomes: []Outcome{{
		Answer: `{"action":"none","reasoning":"Airflow is already handled.","confidence":0.8,"sensor_summary":"Fan control was available.","health_mode":"unchanged","health_value":0,"health_reasoning":"No new health evidence."}`,
	}}}
	acting := &Acting{Binary: "/planty", Usage: "COMPLETE AGENT REFERENCE"}
	seat := (&Judge{fallback: backend}).Able(acting)

	if _, err := seat.Assess(context.Background(), Evidence{Plant: plant.Plant{Slug: "shared-fern"}}); err != nil {
		t.Fatalf("assess: %v", err)
	}
	request := backend.requests[0]
	if request.Acting == nil {
		t.Fatal("scheduled assessment was not granted the acting toolbox")
	}
	if request.Acting == acting {
		t.Fatal("scheduled assessment reused the unrestricted conversation toolbox")
	}
	for _, want := range []string{"show", "actuators", "actuatorstart", "actuatorstop"} {
		if !slices.Contains(request.Acting.AgentVerbs, want) {
			t.Errorf("scheduled assessment is missing allowed verb %q: %v", want, request.Acting.AgentVerbs)
		}
	}
	for _, refused := range []string{"water", "log", "update", "archive", "healthchange"} {
		if slices.Contains(request.Acting.AgentVerbs, refused) {
			t.Errorf("scheduled assessment was granted mutating verb %q", refused)
		}
	}
	for _, want := range []string{
		"COMPLETE AGENT REFERENCE",
		`slug is "shared-fern"`,
		"list its actuators",
		"enforced command set",
		"shortest useful bounded",
		"automatically records airflow on every plant",
		"Never run the water command",
	} {
		if !strings.Contains(request.System, want) {
			t.Errorf("scheduled prompt is missing %q:\n%s", want, request.System)
		}
	}
}

func TestEveryBuiltInPromptStatesItsAuthority(t *testing.T) {
	checks := map[string]struct {
		prompt string
		want   string
	}{
		"identify":     {identifySystem, "no authority to create one"},
		"postmortem":   {postmortemSystem, "control devices"},
		"owner update": {ownerUpdateSystem, "drafts a message only"},
		"scratch":      {scratchSystem, "Do not create a plant record"},
		"consult":      {consultSystem, "Answer the question asked"},
		"assess":       {system, "single most useful action"},
	}
	for name, check := range checks {
		if !strings.Contains(check.prompt, check.want) {
			t.Errorf("%s prompt is missing its authority boundary %q", name, check.want)
		}
	}
}

func (*sequenceBackend) Name() string { return "sequence" }

func TestAssessRepairsOneMalformedAnswerAndKeepsTheOriginal(t *testing.T) {
	backend := &sequenceBackend{outcomes: []Outcome{
		{Answer: `{"action":"water"`, Model: "model-a"},
		{Answer: `{"action":"none","reasoning":"Wait today.","confidence":0.8,"sensor_summary":"Soil is still damp.","health_mode":"unchanged","health_value":0,"health_reasoning":"No new evidence supports a change."}`, Model: "model-a"},
	}}
	seat := &Judge{fallback: backend}

	result, err := seat.Assess(context.Background(), Evidence{})
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if backend.calls != 2 || result.Attempts != 2 {
		t.Fatalf("repair calls=%d attempts=%d, want 2 and 2", backend.calls, result.Attempts)
	}
	if backend.requests[1].Acting != nil {
		t.Fatal("the schema repair pass retained physical acting authority")
	}
	if result.OriginalOutput != `{"action":"water"` || result.OriginalError == "" {
		t.Fatalf("original malformed answer was lost: %#v", result)
	}
	if result.Model != "model-a" {
		t.Fatalf("model provenance = %q", result.Model)
	}
}

func TestAssessReturnsBothFailuresWhenRepairFails(t *testing.T) {
	backend := &sequenceBackend{
		outcomes: []Outcome{{Answer: `{}`, Model: "model-b"}},
		errors:   []error{nil, errors.New("provider unavailable")},
	}
	seat := &Judge{fallback: backend}

	_, err := seat.Assess(context.Background(), Evidence{})
	var failure *AssessmentError
	if !errors.As(err, &failure) {
		t.Fatalf("error %T %v, want AssessmentError", err, err)
	}
	if failure.Model != "model-b" || failure.Attempts != 2 || failure.OriginalOutput != `{}` {
		t.Fatalf("failure lost diagnostic detail: %#v", failure)
	}
}

func TestDecodeResultRejectsSchemaValuesOutsideTheContract(t *testing.T) {
	_, err := decodeResult(Outcome{
		Answer: `{"action":"water","reasoning":"Water it.","confidence":2,"sensor_summary":"Dry.","health_mode":"unchanged","health_value":0,"health_reasoning":"No change."}`,
	}, Evidence{})
	if err == nil {
		t.Fatal("confidence outside the schema was accepted")
	}
}
