package api_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

func TestPlantConversationHistoryCanBeListedAndResumed(t *testing.T) {
	h, db, ctx := newServer(t)
	slug := createPlant(t, h, map[string]any{"common_name": unique("Chat history")})
	p, err := db.GetPlant(ctx, slug)
	if err != nil {
		t.Fatal(err)
	}
	conversationID := uuid.New()
	first, err := db.SaveConsultTurn(ctx, store.ConsultTurn{
		PlantID: p.ID, ConversationID: conversationID,
		Asked: "Use this hidden context.\n\nUser's question:\nCan this wait?",
		Reply: judge.Answer{Reply: "Yes, check it tomorrow.", Confidence: 0.8},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveConsultTurn(ctx, store.ConsultTurn{
		PlantID: p.ID, ConversationID: conversationID, Asked: "What should I watch?",
		Reply: judge.Answer{Reply: "Watch for drooping.", Confidence: 0.9},
	}); err != nil {
		t.Fatal(err)
	}

	listed, body := do(t, h, http.MethodGet, "/v1/plants/"+slug+"/conversations", nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("list status = %d, body %s", listed.Code, listed.Body.String())
	}
	conversations, ok := body["conversations"].([]any)
	if !ok || len(conversations) != 1 {
		t.Fatalf("conversations = %#v", body["conversations"])
	}
	summary := conversations[0].(map[string]any)
	if summary["first_asked"] != "Can this wait?" || summary["turn_count"] != float64(2) {
		t.Fatalf("summary = %#v", summary)
	}

	resumed, transcript := do(t, h, http.MethodGet,
		"/v1/plants/"+slug+"/conversations/"+conversationID.String(), nil)
	if resumed.Code != http.StatusOK {
		t.Fatalf("resume status = %d, body %s", resumed.Code, resumed.Body.String())
	}
	turns, ok := transcript["turns"].([]any)
	if !ok || len(turns) != 2 {
		t.Fatalf("turns = %#v", transcript["turns"])
	}
	visible := turns[0].(map[string]any)
	if visible["id"] != first.ID.String() || visible["asked"] != "Can this wait?" {
		t.Fatalf("first turn = %#v", visible)
	}
}

func TestOnDemandAssessmentNeedsAJudge(t *testing.T) {
	h, _, _ := newServer(t)
	slug := createPlant(t, h, map[string]any{"common_name": unique("Assessment")})

	rec, _ := do(t, h, http.MethodPost, "/v1/plants/"+slug+"/assess", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}
