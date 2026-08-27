package store

import (
	"testing"

	"github.com/TheOutdoorProgrammer/planty/internal/policy"
)

func TestPoliciesAreVersionedAndEvaluationsAreReplayableAndIdempotent(t *testing.T) {
	s, ctx := testStore(t)
	subject := newPlant(t, s, ctx, "Policy subject")

	created, err := s.CreatePolicy(ctx, policy.Policy{
		Name: "Dry soil", Description: "Warn from calibrated soil evidence.",
		Source: policy.ExampleSource, Mode: policy.ModeAdvisory, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.ArchivePolicy(ctx, created.ID) })
	if created.Version != 1 || !created.Enabled {
		t.Fatalf("created policy = %#v", created)
	}

	created.Description = "Updated description"
	updated, err := s.UpdatePolicy(ctx, created.ID, created)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.Description != "Updated description" {
		t.Fatalf("updated policy = %#v", updated)
	}

	input := policy.Input{
		Version:   policy.InputVersion,
		Context:   policy.Context{Trigger: policy.TriggerDaily},
		Plant:     policy.PlantFacts{ID: subject.ID, Slug: subject.Slug, CommonName: subject.CommonName},
		Actuators: []policy.ActuatorFacts{},
	}
	fingerprint, err := policy.FingerprintInput(input)
	if err != nil {
		t.Fatal(err)
	}
	evaluation := policy.Evaluation{
		PolicyID: updated.ID, PolicyVersion: updated.Version, PolicyMode: updated.Mode,
		PlantID: subject.ID, Trigger: policy.TriggerDaily,
		InputFingerprint: fingerprint, IdempotencyKey: policy.IdempotencyKey(input, fingerprint),
		PolicyFingerprint: updated.Fingerprint(), Input: input,
		Result: policy.Result{
			Rules: []policy.Rule{}, Notifications: []policy.Notification{}, FanRuns: []policy.FanRun{},
			Agent: policy.AgentGuidance{Facts: []string{}, Guidance: []string{}, DenyActions: []string{}},
		},
		Outcome: "advisory", Enforced: []string{},
	}
	first, inserted, err := s.SavePolicyEvaluation(ctx, evaluation)
	if err != nil || !inserted {
		t.Fatalf("first evaluation = %#v, %v, %v", first, inserted, err)
	}
	replayed, inserted, err := s.SavePolicyEvaluation(ctx, evaluation)
	if err != nil || inserted || replayed.ID != first.ID {
		t.Fatalf("replayed evaluation = %#v, %v, %v", replayed, inserted, err)
	}

	history, err := s.PolicyEvaluations(ctx, &subject.ID, 10)
	if err != nil || len(history) != 1 || history[0].InputFingerprint != fingerprint {
		t.Fatalf("history = %#v, %v", history, err)
	}
	if err := s.ArchivePolicy(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	listed, err := s.Policies(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range listed {
		if item.ID == created.ID {
			t.Fatal("archived policy remained active")
		}
	}
}
