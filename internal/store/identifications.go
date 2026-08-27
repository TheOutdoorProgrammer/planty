package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
)

type IdentificationJob struct {
	ID         uuid.UUID
	PhotoID    uuid.UUID
	Sighting   judge.Sighting
	Candidates []judge.Candidate
	Status     ConsultStatus
	Failure    string
	Attempts   int
	LeaseID    uuid.UUID
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type identificationPayload struct {
	TakenAt   *time.Time `json:"taken_at,omitempty"`
	Latitude  *float64   `json:"latitude,omitempty"`
	Longitude *float64   `json:"longitude,omitempty"`
}

func (s *Store) QueueIdentification(ctx context.Context, job IdentificationJob) (IdentificationJob, error) {
	payload, err := json.Marshal(identificationPayload{
		TakenAt: job.Sighting.TakenAt, Latitude: job.Sighting.Latitude,
		Longitude: job.Sighting.Longitude,
	})
	if err != nil {
		return IdentificationJob{}, err
	}
	photoID := job.PhotoID
	saved, err := s.queueTurn(ctx, kindIdentify, turn{
		ID: job.ID, ConversationID: job.ID, Asked: string(payload), PhotoID: &photoID,
	})
	if err != nil {
		return IdentificationJob{}, err
	}
	return asIdentification(saved)
}

func (s *Store) Identification(ctx context.Context, id uuid.UUID) (IdentificationJob, error) {
	job, err := s.turnOfKind(ctx, id, kindIdentify)
	if err != nil {
		return IdentificationJob{}, err
	}
	return asIdentification(job)
}

func (s *Store) CompleteIdentification(ctx context.Context, id, leaseID uuid.UUID,
	candidates []judge.Candidate) (IdentificationJob, error) {
	raw, err := json.Marshal(candidates)
	if err != nil {
		return IdentificationJob{}, err
	}
	completed, err := s.completeTurn(ctx, id, leaseID, raw)
	if err != nil {
		return IdentificationJob{}, err
	}
	return asIdentification(completed)
}

func asIdentification(t turn) (IdentificationJob, error) {
	if t.Kind != kindIdentify || t.PhotoID == nil {
		return IdentificationJob{}, ErrNotFound
	}
	var payload identificationPayload
	if err := json.Unmarshal([]byte(t.Asked), &payload); err != nil {
		return IdentificationJob{}, err
	}
	job := IdentificationJob{
		ID: t.ID, PhotoID: *t.PhotoID, Status: t.Status, Failure: t.Failure,
		Attempts: t.Attempts, LeaseID: t.LeaseID, CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
		Sighting: judge.Sighting{
			TakenAt: payload.TakenAt, Latitude: payload.Latitude, Longitude: payload.Longitude,
		},
	}
	if len(t.Reply) > 0 {
		if err := json.Unmarshal(t.Reply, &job.Candidates); err != nil {
			return IdentificationJob{}, err
		}
	}
	if job.Candidates == nil {
		job.Candidates = []judge.Candidate{}
	}
	return job, nil
}

type ModelWork struct {
	Kind           string
	ID             uuid.UUID
	LeaseID        uuid.UUID
	Attempts       int
	Consult        *ConsultTurn
	Identification *IdentificationJob
}

func (s *Store) ClaimModelWork(ctx context.Context, lease time.Duration) (ModelWork, bool, error) {
	claimed, ok, err := s.claimTurn(ctx, []string{kindConsult, kindIdentify}, lease)
	if err != nil || !ok {
		return ModelWork{}, ok, err
	}
	work := ModelWork{
		Kind: claimed.Kind, ID: claimed.ID, LeaseID: claimed.LeaseID, Attempts: claimed.Attempts,
	}
	switch claimed.Kind {
	case kindConsult:
		consult, err := asConsult(claimed)
		if err != nil {
			return ModelWork{}, false, err
		}
		work.Consult = &consult
	case kindIdentify:
		identification, err := asIdentification(claimed)
		if err != nil {
			return ModelWork{}, false, err
		}
		work.Identification = &identification
	default:
		return ModelWork{}, false, errors.New("claimed unsupported model work")
	}
	return work, true, nil
}

func (s *Store) RenewModelWork(ctx context.Context, work ModelWork, lease time.Duration) error {
	return s.renewTurn(ctx, work.ID, work.LeaseID, lease)
}

func (s *Store) RetryModelWork(ctx context.Context, work ModelWork, after time.Duration) error {
	_, err := s.retryTurn(ctx, work.ID, work.LeaseID, after)
	return err
}

func (s *Store) FailModelWork(ctx context.Context, work ModelWork, failure string) error {
	_, err := s.failTurn(ctx, work.ID, work.LeaseID, failure)
	return err
}
