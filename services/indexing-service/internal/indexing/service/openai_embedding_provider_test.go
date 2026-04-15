package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/rohandave/tessa-rag/services/indexing-service/internal/config"
)

func TestOpenAIEmbeddingProviderEmbed(t *testing.T) {
	provider := NewOpenAIEmbeddingProvider(nil, &config.OpenAIConfig{
		APIKey:              "test-key",
		BaseURL:             "https://api.openai.test/v1",
		EmbeddingModel:      "text-embedding-3-small",
		EmbeddingDimensions: 3,
	})
	provider.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s, want %s", r.Method, http.MethodPost)
			}
			if r.URL.String() != "https://api.openai.test/v1/embeddings" {
				t.Fatalf("url = %s, want https://api.openai.test/v1/embeddings", r.URL.String())
			}
			if r.Header.Get("Authorization") != "Bearer test-key" {
				t.Fatalf("authorization = %s, want Bearer test-key", r.Header.Get("Authorization"))
			}

			var got map[string]any
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if got["model"] != "text-embedding-3-small" {
				t.Fatalf("model = %v, want text-embedding-3-small", got["model"])
			}
			if got["input"] != "hello world" {
				t.Fatalf("input = %v, want hello world", got["input"])
			}
			if got["dimensions"] != float64(3) {
				t.Fatalf("dimensions = %v, want 3", got["dimensions"])
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))),
				Header:     make(http.Header),
			}, nil
		}),
	}

	embedding, err := provider.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}
	if len(embedding) != 3 {
		t.Fatalf("embedding len = %d, want 3", len(embedding))
	}
	if embedding[0] != 0.1 || embedding[1] != 0.2 || embedding[2] != 0.3 {
		t.Fatalf("embedding = %v, want [0.1 0.2 0.3]", embedding)
	}
}
