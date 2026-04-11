package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sharedkafka "github.com/rohandave/tessa-rag/services/shared/kafka"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/config"
)

func TestRegisterRepoEventAccepted(t *testing.T) {
	cfg := config.Load()
	router := NewRouter(cfg, sharedkafka.NewDummyProducer())

	body := map[string]string{
		"repo_url":     "https://github.com/example/repo.git",
		"provider":     "github",
		"branch":       "main",
		"event_type":   "repo.created",
		"requested_by": "test-user",
	}

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/repo-event", bytes.NewReader(payload))
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, res.Code)
	}
}

func TestRegisterRepoEventValidation(t *testing.T) {
	cfg := config.Load()
	router := NewRouter(cfg, sharedkafka.NewDummyProducer())

	body := map[string]string{
		"repo_url":     "",
		"provider":     "github",
		"event_type":   "repo.created",
		"requested_by": "test-user",
	}

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/repo-event", bytes.NewReader(payload))
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}
}
