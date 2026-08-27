package api_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/api"
	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/photos"
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

func TestPlantMessageIsAcceptedBeforeTheModelRuns(t *testing.T) {
	_, db, ctx := newServer(t)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := api.New(db, quiet).WithJudge(&judge.Judge{}).Handler()
	slug := createPlant(t, h, map[string]any{"common_name": unique("Durable chat")})
	conversationID := uuid.New()
	turnID := uuid.New()

	accepted, body := do(t, h, http.MethodPost,
		"/v1/plants/"+slug+"/conversations/"+conversationID.String()+"/messages",
		map[string]any{"id": turnID, "message": "Keep answering after I leave."})
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("enqueue status = %d, body %s", accepted.Code, accepted.Body.String())
	}
	if body["id"] != turnID.String() || body["status"] != string(store.ConsultPending) {
		t.Fatalf("accepted turn = %#v", body)
	}

	resumed, transcript := do(t, h, http.MethodGet,
		"/v1/plants/"+slug+"/conversations/"+conversationID.String(), nil)
	if resumed.Code != http.StatusOK {
		t.Fatalf("resume status = %d, body %s", resumed.Code, resumed.Body.String())
	}
	turns, ok := transcript["turns"].([]any)
	if !ok || len(turns) != 1 {
		t.Fatalf("turns = %#v", transcript["turns"])
	}
	stored := turns[0].(map[string]any)
	if stored["status"] != string(store.ConsultPending) || stored["reply"] != nil {
		t.Fatalf("stored turn = %#v", stored)
	}
	if _, err := db.Consultation(ctx, conversationID, uuid.Nil); !errors.Is(err, store.ErrConversationOwner) {
		t.Fatalf("plant conversation lost its owner: %v", err)
	}
}

func TestScratchMessageIsAcceptedAndResumable(t *testing.T) {
	_, db, ctx := newServer(t)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := api.New(db, quiet).WithJudge(&judge.Judge{}).Handler()
	conversationID := uuid.New()
	turnID := uuid.New()

	accepted, body := do(t, h, http.MethodPost,
		"/v1/conversations/"+conversationID.String()+"/messages",
		map[string]any{"id": turnID, "message": "Keep answering after I leave."})
	if accepted.Code != http.StatusAccepted || body["id"] != turnID.String() {
		t.Fatalf("enqueue = %d %#v", accepted.Code, body)
	}

	resumed, transcript := do(t, h, http.MethodGet,
		"/v1/conversations/"+conversationID.String(), nil)
	if resumed.Code != http.StatusOK {
		t.Fatalf("resume status = %d, body %s", resumed.Code, resumed.Body.String())
	}
	turns, ok := transcript["turns"].([]any)
	if !ok || len(turns) != 1 {
		t.Fatalf("turns = %#v", transcript["turns"])
	}
	stored := turns[0].(map[string]any)
	if stored["status"] != string(store.ConsultPending) || stored["asked"] != "Keep answering after I leave." {
		t.Fatalf("stored turn = %#v", stored)
	}
	if _, err := db.Consultation(ctx, conversationID, uuid.Nil); err != nil {
		t.Fatalf("scratch conversation = %v", err)
	}
}

func TestIdentificationIsAcceptedBeforeTheModelRuns(t *testing.T) {
	_, db, _ := newServer(t)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	storage := &readinessPhotos{state: photos.StateReady}
	h := api.New(db, quiet).WithPhotos(storage, &judge.Judge{}).Handler()
	id := uuid.New()
	path := "/v1/identifications/" + id.String()
	jpeg := []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43}

	for attempt := 1; attempt <= 2; attempt++ {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(jpeg))
		req.Header.Set("Content-Type", "image/jpeg")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("attempt %d = %d, body %s", attempt, rec.Code, rec.Body.String())
		}
	}

	resumed, body := do(t, h, http.MethodGet, path, nil)
	if resumed.Code != http.StatusOK || body["id"] != id.String() ||
		body["status"] != string(store.ConsultPending) {
		t.Fatalf("identification = %d %#v", resumed.Code, body)
	}
}

func TestPlantMessageRetryKeepsOneTurnAndOnePhoto(t *testing.T) {
	_, db, ctx := newServer(t)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	storage := &readinessPhotos{state: photos.StateReady}
	h := api.New(db, quiet).WithPhotos(storage, &judge.Judge{}).Handler()
	slug := createPlant(t, h, map[string]any{"common_name": unique("Durable photo chat")})
	p, err := db.GetPlant(ctx, slug)
	if err != nil {
		t.Fatal(err)
	}
	conversationID := uuid.New()
	turnID := uuid.New()
	body := map[string]any{
		"id": turnID, "message": "What does this show?",
		"photo": base64.StdEncoding.EncodeToString([]byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43}),
	}
	requestPath := "/v1/plants/" + slug + "/conversations/" + conversationID.String() + "/messages"

	for attempt := 1; attempt <= 2; attempt++ {
		accepted, response := do(t, h, http.MethodPost, requestPath, body)
		if accepted.Code != http.StatusAccepted || response["id"] != turnID.String() {
			t.Fatalf("attempt %d = %d %#v", attempt, accepted.Code, response)
		}
	}

	turns, err := db.Consultation(ctx, conversationID, p.ID)
	if err != nil || len(turns) != 1 {
		t.Fatalf("turns = %#v, %v", turns, err)
	}
	storedPhotos, err := db.Photos(ctx, p.ID, 10)
	if err != nil || len(storedPhotos) != 1 {
		t.Fatalf("photos = %#v, %v", storedPhotos, err)
	}
	if turns[0].PhotoID == nil || *turns[0].PhotoID != storedPhotos[0].ID {
		t.Fatalf("turn photo = %v, stored photo = %s", turns[0].PhotoID, storedPhotos[0].ID)
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
