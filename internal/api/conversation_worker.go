package api

import (
	"context"
	"errors"
	"time"

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
		s.log.WarnContext(ctx, "conversation worker disabled", "reason", "judge unavailable")
		return
	}
	go s.runConversationWorker(ctx)
}

func (s *Server) runConversationWorker(ctx context.Context) {
	for {
		processed, err := s.processNextConversation(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			s.log.ErrorContext(ctx, "conversation worker failed", "error", err)
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

func (s *Server) processNextConversation(ctx context.Context) (bool, error) {
	turn, ok, err := s.store.ClaimConsultTurn(ctx, conversationLease)
	if err != nil || !ok {
		return false, err
	}

	ctx, span := otel.Tracer("planty/conversations").Start(ctx, "conversation.process")
	span.SetAttributes(attribute.Int("conversation.attempt", turn.Attempts))
	defer span.End()
	leaseCtx, stopLease := context.WithCancel(ctx)
	leaseDone := make(chan struct{})
	go func() {
		defer close(leaseDone)
		ticker := time.NewTicker(conversationLease / 3)
		defer ticker.Stop()
		for {
			select {
			case <-leaseCtx.Done():
				return
			case <-ticker.C:
				if err := s.store.RenewConsultTurn(
					leaseCtx, turn.ID, turn.LeaseID, conversationLease,
				); err != nil {
					s.log.WarnContext(leaseCtx, "conversation lease renewal failed", "error", err)
				}
			}
		}
	}()

	answer, err := s.answerQueuedConversation(ctx, turn)
	stopLease()
	<-leaseDone
	if err == nil {
		if _, err := s.store.CompleteConsultTurn(ctx, turn.ID, turn.LeaseID, answer); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "persist reply")
			return true, err
		}
		span.SetAttributes(attribute.String("conversation.status", "complete"))
		s.log.InfoContext(ctx, "conversation completed", "attempt", turn.Attempts)
		return true, nil
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	span.RecordError(err)
	if turn.Attempts < conversationMaxAttempts {
		if _, retryErr := s.store.RetryConsultTurn(ctx, turn.ID, turn.LeaseID,
			time.Duration(turn.Attempts)*2*time.Second); retryErr != nil {
			span.SetStatus(codes.Error, "schedule retry")
			return true, retryErr
		}
		span.SetAttributes(attribute.String("conversation.status", "retrying"))
		s.log.WarnContext(ctx, "conversation attempt failed; retrying",
			"attempt", turn.Attempts, "error", err)
		return true, nil
	}

	if _, failErr := s.store.FailConsultTurn(ctx, turn.ID, turn.LeaseID,
		"Planty could not answer this message. Ask again."); failErr != nil {
		span.SetStatus(codes.Error, "persist failure")
		return true, failErr
	}
	span.SetAttributes(attribute.String("conversation.status", "failed"))
	span.SetStatus(codes.Error, "model failed")
	s.log.ErrorContext(ctx, "conversation failed", "attempts", turn.Attempts, "error", err)
	return true, nil
}

func (s *Server) answerQueuedConversation(ctx context.Context,
	turn store.ConsultTurn) (judge.Answer, error) {
	p, err := s.store.GetPlantByID(ctx, turn.PlantID)
	if err != nil {
		return judge.Answer{}, err
	}

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

	history, err := job.Gather(ctx, s.store, p, time.Now().Add(-judge.ConsultWindow))
	if err != nil {
		return judge.Answer{}, err
	}
	return s.judge.Consult(ctx, history, s.offerPhotos(ctx, p),
		turn.Asked, prior, turn.ConversationID)
}
