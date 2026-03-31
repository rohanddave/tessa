package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/config"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/kafka"
)

func TestRegisterRepoEventAccepted(t *testing.T) {
	cfg := config.Load()
	router := NewRouter(cfg, kafka.NewDummyProducer(cfg.Kafka.EventsTopic), kafka.NewDummyProducer(cfg.Kafka.LifeCycleTopic))

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

	req := httptest.NewRequest(http.MethodPost, "/register-repo-event", bytes.NewReader(payload))
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, res.Code)
	}
}

func TestRegisterRepoEventValidation(t *testing.T) {
	cfg := config.Load()
	router := NewRouter(cfg, kafka.NewDummyProducer(cfg.Kafka.EventsTopic), kafka.NewDummyProducer(cfg.Kafka.LifeCycleTopic))

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

	req := httptest.NewRequest(http.MethodPost, "/register-repo-event", bytes.NewReader(payload))
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}
}
