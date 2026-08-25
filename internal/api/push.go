package api

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

type pushDeviceRequest struct {
	Token          string    `json:"token"`
	Environment    string    `json:"environment"`
	InstallationID uuid.UUID `json:"installation_id"`
}

func (s *Server) registerPushDevice(w http.ResponseWriter, r *http.Request) {
	var request pushDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("decode push device: %w", err))
		return
	}
	request.Token = strings.ToLower(strings.TrimSpace(request.Token))
	request.Environment = strings.ToLower(strings.TrimSpace(request.Environment))
	if request.Token == "" {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("push token is required"))
		return
	}
	if _, err := hex.DecodeString(request.Token); err != nil {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("push token is not hexadecimal"))
		return
	}
	accepted, err := s.store.UpsertPushDevice(r.Context(), store.PushDevice{
		Token: request.Token, Environment: request.Environment, InstallationID: request.InstallationID,
	})
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	s.ok(w, http.StatusOK, accepted)
}

type pushInstallationRequest struct {
	InstallationID uuid.UUID `json:"installation_id"`
	Environment    string    `json:"environment"`
}

func (s *Server) pushHealth(w http.ResponseWriter, r *http.Request) {
	status := s.pushSender.Status()
	body := map[string]any{"server": status}
	id, err := uuid.Parse(strings.TrimSpace(r.URL.Query().Get("installation_id")))
	environment := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("environment")))
	if err == nil && environment != "" {
		if registered, findErr := s.store.PushDeviceForInstallation(r.Context(), environment, id); findErr == nil {
			body["registration"] = registered
		} else if findErr != store.ErrNotFound {
			s.fail(w, http.StatusInternalServerError, findErr)
			return
		}
	}
	s.ok(w, http.StatusOK, body)
}

func (s *Server) testPush(w http.ResponseWriter, r *http.Request) {
	if s.pushSender == nil {
		s.fail(w, http.StatusServiceUnavailable, fmt.Errorf("APNs is not configured"))
		return
	}
	var request pushInstallationRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("decode push test: %w", err))
		return
	}
	request.Environment = strings.ToLower(strings.TrimSpace(request.Environment))
	if request.InstallationID == uuid.Nil {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("installation id is required"))
		return
	}
	if err := s.pushSender.SendTest(r.Context(), request.InstallationID, request.Environment); err != nil {
		s.fail(w, http.StatusBadGateway, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]string{"status": "accepted_by_apns"})
}
