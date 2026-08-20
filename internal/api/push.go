package api

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

type pushDeviceRequest struct {
	Token       string `json:"token"`
	Environment string `json:"environment"`
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
	if err := s.store.UpsertPushDevice(r.Context(), store.PushDevice{
		Token: request.Token, Environment: request.Environment,
	}); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]string{"status": "registered"})
}
