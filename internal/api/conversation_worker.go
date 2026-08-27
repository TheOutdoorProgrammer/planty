package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/TheOutdoorProgrammer/planty/internal/job"
	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

const (
	conversationLease       = 30 * time.Second
	conversationPoll        = time.Second
	conversationMaxAttempts = 3
)

func (s *Server) StartConversationWorker(ctx context.Context) {
	if s.judge == nil {
		s.log.WarnContext(ctx, "model worker disabled", "reason", "judge unavailable")
		return
	}
	go s.runConversationWorker(ctx)
}

func (s *Server) runConversationWorker(ctx context.Context) {
	for {
		processed, err := s.processNextModelWork(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			s.log.ErrorContext(ctx, "model worker failed", "error", err)
		}
		if processed {
			continue
		}

		timer := time.NewTimer(conversationPoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *Server) processNextModelWork(ctx context.Context) (bool, error) {
	work, ok, err := s.store.ClaimModelWork(ctx, conversationLease)
	if err != nil || !ok {
		return false, err
	}

	spanName := "conversation.process"
	if work.Identification != nil {
		spanName = "identification.process"
	}
	ctx, span := otel.Tracer("planty/model-work").Start(ctx, spanName)
	span.SetAttributes(
		attribute.String("model.operation", work.Kind),
		attribute.Int("model.attempt", work.Attempts),
	)
	defer span.End()
	leaseCtx, stopLease := context.WithCancel(ctx)
	leaseDone := make(chan error, 1)
	go func() {
		defer close(leaseDone)
		ticker := time.NewTicker(conversationLease / 3)
		defer ticker.Stop()
		for {
			select {
			case <-leaseCtx.Done():
				return
			case <-ticker.C:
				if err := s.store.RenewModelWork(leaseCtx, work, conversationLease); err != nil {
					s.log.WarnContext(leaseCtx, "model lease renewal failed", "error", err)
					leaseDone <- err
					stopLease()
					return
				}
			}
		}
	}()

	var complete func(context.Context) error
	if work.Consult != nil {
		answer, answerErr := s.answerQueuedConversation(leaseCtx, *work.Consult)
		err = answerErr
		complete = func(ctx context.Context) error {
			_, err := s.store.CompleteConsultTurn(ctx, work.ID, work.LeaseID, answer)
			return err
		}
	} else {
		candidates, identifyErr := s.answerQueuedIdentification(leaseCtx, *work.Identification)
		err = identifyErr
		complete = func(ctx context.Context) error {
			_, err := s.store.CompleteIdentification(ctx, work.ID, work.LeaseID, candidates)
			return err
		}
	}
	stopLease()
	if leaseErr := <-leaseDone; leaseErr != nil {
		span.RecordError(leaseErr)
		span.SetStatus(codes.Error, "lease renewal")
		return true, fmt.Errorf("renew conversation lease: %w", leaseErr)
	}
	if err == nil {
		if err := complete(ctx); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "persist result")
			return true, err
		}
		span.SetAttributes(attribute.String("model.status", "complete"))
		if work.Consult != nil {
			s.log.InfoContext(ctx, "conversation completed", "attempt", work.Attempts)
		} else {
			s.log.InfoContext(ctx, "identification completed", "attempt", work.Attempts)
		}
		return true, nil
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	span.RecordError(err)
	if work.Attempts < conversationMaxAttempts && judge.Retryable(err) {
		if retryErr := s.store.RetryModelWork(ctx, work,
			time.Duration(work.Attempts)*2*time.Second); retryErr != nil {
			span.SetStatus(codes.Error, "schedule retry")
			return true, retryErr
		}
		span.SetAttributes(attribute.String("model.status", "retrying"))
		s.log.WarnContext(ctx, "model attempt failed; retrying",
			"operation", work.Kind, "attempt", work.Attempts, "error", err)
		return true, nil
	}

	if failErr := s.store.FailModelWork(ctx, work,
		"Planty could not finish this request. Try again."); failErr != nil {
		span.SetStatus(codes.Error, "persist failure")
		return true, failErr
	}
	span.SetAttributes(attribute.String("model.status", "failed"))
	span.SetStatus(codes.Error, "model failed")
	s.log.ErrorContext(ctx, "model work failed", "operation", work.Kind,
		"attempts", work.Attempts, "error", err)
	return true, nil
}

func (s *Server) answerQueuedConversation(ctx context.Context,
	turn store.ConsultTurn) (judge.Answer, error) {
	turns, err := s.store.Consultation(ctx, turn.ConversationID, turn.PlantID)
	if err != nil {
		return judge.Answer{}, err
	}
	prior := make([]judge.PriorAnswer, 0, len(turns))
	for _, earlier := range turns {
		if earlier.ID == turn.ID || earlier.Status != store.ConsultComplete {
			continue
		}
		prior = append(prior, judge.PriorAnswer{
			Asked: earlier.Asked, Reply: earlier.Reply, PhotoID: earlier.PhotoID,
		})
	}
	if turn.PlantID == uuid.Nil {
		shown := make([]judge.Offer, 0)
		for index, saved := range turns {
			if saved.PhotoID == nil {
				continue
			}
			raw, media, err := s.photoBytes(ctx, *saved.PhotoID)
			if err != nil {
				continue
			}
			shown = append(shown, judge.Offer{
				Label: fmt.Sprintf("sent with message %d", index+1), Media: media, Bytes: raw,
			})
		}
		return s.judge.Ask(ctx, turn.Asked, shown, prior, turn.ConversationID)
	}

	p, err := s.store.GetPlantByID(ctx, turn.PlantID)
	if err != nil {
		return judge.Answer{}, err
	}

	history, err := job.Gather(ctx, s.store, p, time.Now().Add(-judge.ConsultWindow))
	if err != nil {
		return judge.Answer{}, err
	}
	return s.judge.Consult(ctx, history, s.offerPhotos(ctx, p),
		turn.Asked, prior, turn.ConversationID)
}

func (s *Server) answerQueuedIdentification(ctx context.Context,
	job store.IdentificationJob) ([]judge.Candidate, error) {
	raw, media, err := s.photoBytes(ctx, job.PhotoID)
	if err != nil {
		return nil, err
	}
	takenAt := job.CreatedAt
	if job.Sighting.TakenAt != nil {
		takenAt = *job.Sighting.TakenAt
	}
	return s.judge.Identify(ctx, judge.Frame{Bytes: raw, Media: media, TakenAt: takenAt}, job.Sighting)
}
