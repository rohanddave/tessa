package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/rohandave/tessa-rag/services/indexing-service/internal/config"
	"github.com/rohandave/tessa-rag/services/shared/domain"
)

func TestPineconeIndexerIndexChunkUpsertsVector(t *testing.T) {
	chunk := &domain.Chunk{
		ChunkID:     "chunk-123",
		RepoURL:     "https://github.com/example/repo",
		FilePath:    "services/example.go",
		Language:    "go",
		Content:     "func Example() {}",
		ContentHash: "hash-123",
		SymbolName:  "Example",
		SymbolType:  "function",
		StartLine:   10,
		EndLine:     12,
	}

	indexer := NewPineconeIndexer(nil, &config.PineconeConfig{
		Host:       "http://pinecone-index:5081",
		APIKey:     "pclocal",
		APIVersion: "2025-01",
		Index:      "tessa-chunks",
		Namespace:  "code",
	}, staticEmbeddingProvider{embedding: []float32{0.1, 0.2, 0.3}})
	indexer.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			assertPineconeVectorUpsertRequest(t, r, chunk)

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	if err := indexer.IndexChunk(context.Background(), chunk); err != nil {
		t.Fatalf("IndexChunk returned error: %v", err)
	}
}

func assertPineconeVectorUpsertRequest(t *testing.T, r *http.Request, chunk *domain.Chunk) {
	t.Helper()

	if r.Method != http.MethodPost {
		t.Fatalf("method = %s, want %s", r.Method, http.MethodPost)
	}
	if r.URL.String() != "http://pinecone-index:5081/vectors/upsert" {
		t.Fatalf("url = %s, want http://pinecone-index:5081/vectors/upsert", r.URL.String())
	}
	if r.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("content type = %s, want application/json", r.Header.Get("Content-Type"))
	}
	if r.Header.Get("Api-Key") != "pclocal" {
		t.Fatalf("api key = %s, want pclocal", r.Header.Get("Api-Key"))
	}
	if r.Header.Get("X-Pinecone-Api-Version") != "2025-01" {
		t.Fatalf("api version = %s, want 2025-01", r.Header.Get("X-Pinecone-Api-Version"))
	}

	var got struct {
		Namespace string `json:"namespace"`
		Vectors   []struct {
			ID       string         `json:"id"`
			Values   []float32      `json:"values"`
			Metadata map[string]any `json:"metadata"`
		} `json:"vectors"`
	}
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if got.Namespace != "code" {
		t.Fatalf("namespace = %s, want code", got.Namespace)
	}
	if len(got.Vectors) != 1 {
		t.Fatalf("vectors len = %d, want 1", len(got.Vectors))
	}
	if got.Vectors[0].ID != chunk.ChunkID {
		t.Fatalf("id = %s, want %s", got.Vectors[0].ID, chunk.ChunkID)
	}
	if len(got.Vectors[0].Values) != 3 {
		t.Fatalf("values len = %d, want 3", len(got.Vectors[0].Values))
	}
	if got.Vectors[0].Metadata["chunk_id"] != chunk.ChunkID {
		t.Fatalf("metadata chunk_id = %v, want %s", got.Vectors[0].Metadata["chunk_id"], chunk.ChunkID)
	}
	if got.Vectors[0].Metadata["content"] != chunk.Content {
		t.Fatalf("metadata content = %v, want %s", got.Vectors[0].Metadata["content"], chunk.Content)
	}
}

type staticEmbeddingProvider struct {
	embedding []float32
}

func (p staticEmbeddingProvider) Embed(ctx context.Context, input string) ([]float32, error) {
	return p.embedding, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
