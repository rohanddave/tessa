package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	sharedkafka "github.com/rohandave/tessa-rag/services/shared/kafka"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/config"
	reposync "github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync"
)

type handler struct {
	config   config.Config
	producer sharedkafka.Producer
}

func NewRouter(cfg config.Config, producer sharedkafka.Producer) http.Handler {
	h := handler{
		config:   cfg,
		producer: producer,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", h.handleHealth)
	mux.HandleFunc("/repo-event", h.handleRepoEvent)
	mux.HandleFunc("/", h.handleRoot)

	return mux
}

func (h handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": h.config.ServiceName,
	})
}

func (h handler) handleRoot(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": h.config.ServiceName,
		"kafka": map[string]string{
			"brokers":     h.config.Kafka.Brokers,
			"eventstopic": h.config.Kafka.EventsTopic,
		},
		"s3": map[string]string{
			"endpoint": h.config.Storage.Endpoint,
			"region":   h.config.Storage.Region,
			"bucket":   h.config.Storage.Bucket,
			"useSSL":   h.config.Storage.UseSSL,
		},
	})
}

func (h handler) handleRepoEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
			"error": "method not allowed",
		})
		return
	}

	var req reposync.RepoEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON body",
		})
		return
	}

	if err := req.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	event := reposync.RepoEvent{
		EventID: fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		// Status:      "accepted",
		RepoURL:     strings.TrimSpace(req.RepoURL),
		Provider:    strings.TrimSpace(req.Provider),
		Branch:      defaultBranch(strings.TrimSpace(req.Branch)),
		CommitSHA:   strings.TrimSpace(req.CommitSHA),
		EventType:   strings.TrimSpace(req.EventType),
		RequestedBy: strings.TrimSpace(req.RequestedBy),
		ReceivedAt:  time.Now().UTC(),
	}

	value, err := json.Marshal(event)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to encode repo event",
		})
		return
	}

	if err := h.producer.Produce(h.config.Kafka.EventsTopic, []byte(event.RepoURL), value); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": "failed to publish repo event",
		})
		return
	}

	writeJSON(w, http.StatusAccepted, event)
}

func defaultBranch(branch string) string {
	if branch == "" {
		return "main"
	}

	return branch
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
