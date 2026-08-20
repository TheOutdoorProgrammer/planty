package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFailSanitizesServerErrorsAndCorrelatesLogs(t *testing.T) {
	var logs bytes.Buffer
	s := &Server{log: slog.New(slog.NewTextHandler(&logs, nil))}
	rec := httptest.NewRecorder()

	s.fail(rec, http.StatusInternalServerError,
		errors.New("postgres dial failed: password=super-secret"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var body struct {
		Error     string `json:"error"`
		Code      string `json:"code"`
		RequestID string `json:"request_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != "internal_error" {
		t.Errorf("code = %q, want internal_error", body.Code)
	}
	if body.Error != "The service could not complete the request." {
		t.Errorf("public error = %q", body.Error)
	}
	if body.RequestID == "" {
		t.Fatal("response has no request_id")
	}
	if got := rec.Header().Get(requestIDHeader); got != body.RequestID {
		t.Errorf("header request id = %q, body = %q", got, body.RequestID)
	}
	if strings.Contains(rec.Body.String(), "super-secret") {
		t.Fatal("internal error detail leaked in response")
	}
	if !strings.Contains(logs.String(), "super-secret") {
		t.Fatal("internal error detail was not retained in logs")
	}
	if !strings.Contains(logs.String(), body.RequestID) {
		t.Fatal("request id was not retained in logs")
	}
}

func TestFailKeepsCallerErrorDetail(t *testing.T) {
	s := &Server{log: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))}
	rec := httptest.NewRecorder()

	s.fail(rec, http.StatusBadRequest, errors.New("common_name is required"))

	var body struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error != "common_name is required" {
		t.Errorf("error = %q", body.Error)
	}
	if body.Code != "bad_request" {
		t.Errorf("code = %q, want bad_request", body.Code)
	}
}
