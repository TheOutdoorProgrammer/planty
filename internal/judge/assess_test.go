package judge

import (
	"context"
	"errors"
	"testing"
	"time"

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
	backend := &sequenceBackend{outcomes: []Outcome{{
		Answer: `{"action":"none","reasoning":"Wait today.","confidence":0.8,"sensor_summary":"Leaves look steady.","health_mode":"baseline","health_value":85,"health_reasoning":"The attached current photo shows upright leaves."}`,
		Model:  "model-a",
	}}}
	seat := &Judge{fallback: backend}

	result, err := seat.Assess(context.Background(), Evidence{
		LatestPhoto: &PhotoEvidence{
			ID: photoID, TakenAt: time.Now().Add(-time.Hour), Caption: "whole plant",
			Frame: Frame{Media: "image/jpeg", Bytes: []byte("photograph")},
		},
	})
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if result.HealthValue != 85 || len(backend.requests) != 1 {
		t.Fatalf("unexpected result or requests: %#v %d", result, len(backend.requests))
	}
	parts := backend.requests[0].Turns[0].Parts
	if len(parts) != 2 || parts[1].Image == nil || string(parts[1].Image.Bytes) != "photograph" {
		t.Fatalf("assessment did not receive the exact photo bytes: %#v", parts)
	}
	if parts[0].Text == "" || backend.requests[0].Job != JobAssess {
		t.Fatalf("assessment context was lost: %#v", backend.requests[0])
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
