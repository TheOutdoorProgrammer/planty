package store

import (
	"errors"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
)

func TestCalibrationProposalRequiresReviewAndEnforcesCooldown(t *testing.T) {
	s, ctx := testStore(t)
	if _, err := s.ResolveCalibrationProposal(ctx, uuid.New(), true, "owner"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolve missing proposal = %v, want ErrNotFound", err)
	}
	p := newPlant(t, s, ctx, "Calibration review")
	link, err := s.LinkSensor(ctx, plant.SensorLink{
		PlantID: &p.ID, HAEntityID: "sensor.calibration_" + p.ID.String(), Role: plant.RoleSoilMoisture,
	})
	if err != nil {
		t.Fatal(err)
	}
	link, err = s.Calibrate(ctx, link.ID, 100, 500)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordReading(ctx, plant.Reading{
		SensorLinkID: link.ID, Value: 300, Unit: "raw", TakenAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	reading, err := s.LatestReading(ctx, link.ID)
	if err != nil {
		t.Fatal(err)
	}
	input := plant.CalibrationProposal{
		SensorLinkID: link.ID, ReadingID: reading.ID, ProposedDry: 120, ProposedWet: 540,
		Reason: "Repeated recorded wet readings exceed the existing wet endpoint.", ModelVersion: "test-model",
	}
	proposal, created, err := s.ProposeCalibration(ctx, input)
	if err != nil || !created {
		t.Fatalf("proposal created=%t err=%v", created, err)
	}
	if proposal.CurrentRelative != 0.5 || proposal.ProposedRelative >= proposal.CurrentRelative {
		t.Fatalf("relative values = current %.3f proposed %.3f", proposal.CurrentRelative, proposal.ProposedRelative)
	}
	if _, created, err := s.ProposeCalibration(ctx, input); err != nil || created {
		t.Fatalf("cooldown created=%t err=%v", created, err)
	}
	pending, err := s.PendingCalibrationProposals(ctx, p.ID)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending=%#v err=%v", pending, err)
	}
	resolved, err := s.ResolveCalibrationProposal(ctx, proposal.ID, true, "owner")
	if err != nil || resolved.Status != plant.CalibrationApproved {
		t.Fatalf("resolved=%#v err=%v", resolved, err)
	}
	links, err := s.SensorLinks(ctx, &p.ID)
	if err != nil || len(links) != 1 || *links[0].DryBaseline != 120 || *links[0].WetBaseline != 540 {
		t.Fatalf("approved calibration was not applied: %#v err=%v", links, err)
	}
	if _, err := s.ResolveCalibrationProposal(ctx, proposal.ID, false, "owner"); err == nil {
		t.Fatal("resolved proposal was accepted twice")
	}
}

func TestAssignSensorDeniesPendingCalibrationProposal(t *testing.T) {
	s, ctx := testStore(t)
	oldPlant := newPlant(t, s, ctx, "Old calibration pot")
	newPlant := newPlant(t, s, ctx, "New calibration pot")
	link, err := s.LinkSensor(ctx, plant.SensorLink{
		PlantID: &oldPlant.ID, HAEntityID: "sensor.reassigned_" + oldPlant.ID.String(), Role: plant.RoleSoilMoisture,
	})
	if err != nil {
		t.Fatal(err)
	}
	link, err = s.Calibrate(ctx, link.ID, 100, 500)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordReading(ctx, plant.Reading{
		SensorLinkID: link.ID, Value: 300, Unit: "raw", TakenAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	reading, err := s.LatestReading(ctx, link.ID)
	if err != nil {
		t.Fatal(err)
	}
	proposal, created, err := s.ProposeCalibration(ctx, plant.CalibrationProposal{
		SensorLinkID: link.ID, ReadingID: reading.ID, ProposedDry: 120, ProposedWet: 540,
		Reason: "This proposal belongs to the old pot.", ModelVersion: "test-model",
	})
	if err != nil || !created {
		t.Fatalf("proposal created=%t err=%v", created, err)
	}

	moved, err := s.AssignSensor(ctx, link.ID, plant.SensorAssignment{PlantID: &newPlant.ID})
	if err != nil || moved.PlantID == nil || *moved.PlantID != newPlant.ID {
		t.Fatalf("moved=%#v err=%v", moved, err)
	}
	if pending, err := s.PendingCalibrationProposals(ctx, oldPlant.ID); err != nil || len(pending) != 0 {
		t.Fatalf("old plant pending=%#v err=%v", pending, err)
	}
	resolved, err := scanCalibrationProposal(s.pool.QueryRow(ctx, `SELECT `+calibrationProposalColumns+`
		FROM sensor_calibration_proposals WHERE id = $1`, proposal.ID))
	if err != nil || resolved.Status != plant.CalibrationDenied || resolved.ResolvedBy != "sensor reassigned" {
		t.Fatalf("resolved=%#v err=%v", resolved, err)
	}
}
