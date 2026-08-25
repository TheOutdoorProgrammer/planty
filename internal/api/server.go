// Package api is the HTTP surface. The iOS app and the Dusk plugin are both
// clients of it and have identical powers; see docs/DATA-MODEL.md.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/ha"
	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/photos"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

const requestIDHeader = "X-Request-ID"

// Server routes HTTP onto the store.
//
// There is no authentication by deliberate choice; keep it on the LAN.
type Server struct {
	store         *store.Store
	log           *slog.Logger
	photos        photos.Storage
	judge         *judge.Judge
	homeAssistant homeAssistantDiscoverer
}

// New builds a server. Photo storage and the judge are optional: without them
// the photo routes report unavailable rather than the whole service failing.
func New(s *store.Store, log *slog.Logger) *Server {
	server := &Server{store: s, log: log}
	if baseURL, token := os.Getenv("PLANTY_HA_URL"), os.Getenv("PLANTY_HA_TOKEN"); baseURL != "" && token != "" {
		server.homeAssistant = ha.New(baseURL, token)
	}
	return server
}

// WithPhotos enables the photo timeline and vision diagnosis routes.
func (s *Server) WithPhotos(p photos.Storage, j *judge.Judge) *Server {
	s.photos, s.judge = p, j
	return s
}

// WithJudge enables model-backed routes even when photograph storage is not
// configured. The two dependencies fail independently.
func (s *Server) WithJudge(j *judge.Judge) *Server {
	s.judge = j
	return s
}

// Handler returns the routed mux. Route patterns are generated from the
// OpenAPI contract so the server and clients cannot silently spell one
// differently. Every response receives a request id for log correlation.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc(routeHealth, s.health)
	mux.HandleFunc(routeReady, s.ready)

	mux.HandleFunc(routeListPlants, s.listPlants)
	mux.HandleFunc(routeCreatePlant, s.createPlant)
	mux.HandleFunc(routeGetPlant, s.getPlant)
	mux.HandleFunc(routeUpdatePlant, s.updatePlant)
	mux.HandleFunc(routeArchivePlant, s.archivePlant)

	mux.HandleFunc(routeListObservations, s.listObservations)
	mux.HandleFunc(routeAddObservation, s.addObservation)

	mux.HandleFunc(routeListPlantNotes, s.listNotes)
	mux.HandleFunc(routeAddPlantNote, s.addNote)
	mux.HandleFunc(routeListHouseholdNotes, s.listHouseholdNotes)
	mux.HandleFunc(routeAddHouseholdNote, s.addHouseholdNote)
	mux.HandleFunc(routeUpdateNote, s.updateNote)
	mux.HandleFunc(routeDeleteNote, s.deleteNote)

	mux.HandleFunc(routeListHarvests, s.listHarvests)
	mux.HandleFunc(routeListPlantHarvests, s.listHarvests)
	mux.HandleFunc(routeAddHarvest, s.addHarvest)
	mux.HandleFunc(routeUploadPhoto, s.uploadPhoto)
	mux.HandleFunc(routeGetTimeline, s.timeline)
	mux.HandleFunc(routeAskPlant, s.consult)

	mux.HandleFunc(routeListReminders, s.listReminders)
	mux.HandleFunc(routeSetReminder, s.setReminder)
	mux.HandleFunc(routeDeleteReminder, s.deleteReminder)
	mux.HandleFunc(routeCompleteReminder, s.completeReminder)

	// Device registration is deliberately independent of APNs credentials. A
	// phone may register before the server is configured to send; delivery then
	// starts automatically as soon as credentials land.
	mux.HandleFunc(routeRegisterPushDevice, s.registerPushDevice)
	mux.HandleFunc(routeListModels, s.listModels)
	mux.HandleFunc(routeListModelAssignments, s.listModelAssignments)
	mux.HandleFunc(routeSetModelAssignment, s.setModelAssignment)
	mux.HandleFunc(routeClearModelAssignment, s.clearModelAssignment)

	// No slug: a question about something in a shop is not a plant you own.
	mux.HandleFunc(routeAsk, s.ask)

	mux.HandleFunc(routeIdentify, s.identify)
	mux.HandleFunc(routeCreatePlantFromPhoto, s.plantFromPhoto)

	mux.HandleFunc(routeListPostmortems, s.listPostmortems)
	mux.HandleFunc(routeCreatePostmortem, s.autopsy)

	mux.HandleFunc(routeToday, s.today)
	mux.HandleFunc(routeAckVerdict, s.ackVerdict)
	mux.HandleFunc(routeCompleteVerdict, s.completeVerdict)

	mux.HandleFunc(routeListChoices, s.listManagedChoices)
	mux.HandleFunc(routeListHomeAssistantEntities, s.discoverHomeAssistantEntities)

	mux.HandleFunc(routeListSensors, s.listSensors)
	mux.HandleFunc(routeLinkSensor, s.linkSensor)
	mux.HandleFunc(routeCalibrateSensor, s.calibrateSensor)

	mux.HandleFunc(routeListQuestions, s.listQuestions)
	mux.HandleFunc(routeCreateQuestion, s.askOwner)
	mux.HandleFunc(routeAnswerQuestion, s.answerQuestion)
	mux.HandleFunc(routeCreateOwnerUpdate, s.createOwnerUpdate)

	mux.HandleFunc(routeListAway, s.listAway)
	mux.HandleFunc(routeCreateAway, s.goAway)
	mux.HandleFunc(routeUpdateAway, s.updateAway)
	mux.HandleFunc(routeDeleteAway, s.cancelAway)

	mux.HandleFunc(routeColdWatch, s.coldWatch)
	mux.HandleFunc(routeShelter, s.shelter)
	mux.HandleFunc(routeUnshelter, s.unshelter)

	return withRequestID(browserWriteGuard(mux))
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(requestIDHeader, uuid.NewString())
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Healthy(r.Context()); err != nil {
		s.fail(w, http.StatusServiceUnavailable, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{
		"status": "ok",
		"components": map[string]string{
			"database": "ready",
			"photos":   s.photoState(),
		},
	})
}

// ready is stricter than liveness: a configured dependency that is still
// reconnecting keeps this pod out of service without asking Kubernetes to kill
// the process that is doing the healing.
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Healthy(r.Context()); err != nil {
		s.fail(w, http.StatusServiceUnavailable, err)
		return
	}
	if state := s.photoState(); state == string(photos.StateStarting) || state == string(photos.StateUnavailable) {
		s.fail(w, http.StatusServiceUnavailable, photos.ErrUnavailable)
		return
	}
	s.ok(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) photoState() string {
	if s.photos == nil {
		return "disabled"
	}
	if reporter, ok := s.photos.(interface{ Status() (photos.State, error) }); ok {
		state, _ := reporter.Status()
		return string(state)
	}
	return string(photos.StateReady)
}

func (s *Server) ok(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		s.log.Error("encode response", "error", err)
	}
}

// fail maps the error onto a status, then draws a hard boundary between public
// failures and internal detail. 4xx errors may explain the caller's mistake;
// 5xx errors expose only a stable code and request id while the wrapped error
// remains in structured logs.
func (s *Server) fail(w http.ResponseWriter, code int, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		code = http.StatusNotFound
	case errors.Is(err, store.ErrConversationOwner):
		code = http.StatusConflict
	case errors.Is(err, plant.ErrInvalid):
		code = http.StatusBadRequest
	}

	requestID := w.Header().Get(requestIDHeader)
	if requestID == "" {
		requestID = uuid.NewString()
		w.Header().Set(requestIDHeader, requestID)
	}
	publicMessage := err.Error()
	if code >= http.StatusInternalServerError {
		publicMessage = publicServerError(code)
		s.log.Error("request failed",
			"request_id", requestID,
			"status", code,
			"code", publicErrorCode(code),
			"error", err,
		)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":      publicMessage,
		"code":       publicErrorCode(code),
		"request_id": requestID,
	})
}

func publicServerError(code int) string {
	switch code {
	case http.StatusBadGateway:
		return "An upstream service could not complete the request."
	case http.StatusServiceUnavailable:
		return "A required service is unavailable."
	case http.StatusGatewayTimeout:
		return "An upstream service did not answer in time."
	default:
		return "The service could not complete the request."
	}
}

func publicErrorCode(code int) string {
	switch code {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusRequestEntityTooLarge:
		return "payload_too_large"
	case http.StatusUnprocessableEntity:
		return "unprocessable_entity"
	case http.StatusBadGateway:
		return "upstream_error"
	case http.StatusServiceUnavailable:
		return "service_unavailable"
	case http.StatusGatewayTimeout:
		return "upstream_timeout"
	case http.StatusInternalServerError:
		return "internal_error"
	default:
		if code >= http.StatusInternalServerError {
			return "server_error"
		}
		return "request_error"
	}
}
