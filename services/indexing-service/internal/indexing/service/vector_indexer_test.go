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

func TestPineconeIndexerIndexChunk(t *testing.T) {
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
		Host:       "http://pinecone-local:5080",
		APIKey:     "pclocal",
		APIVersion: "2026-04",
		Index:      "tessa-chunks",
		Namespace:  "code",
		TextField:  "chunk_text",
	})
	indexer.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			assertPineconeUpsertRequest(t, r, chunk)

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

func TestPineconeIndexerDeleteChunk(t *testing.T) {
	indexer := NewPineconeIndexer(nil, &config.PineconeConfig{
		Host:       "http://pinecone-local:5080",
		APIKey:     "pclocal",
		APIVersion: "2026-04",
		Index:      "tessa-chunks",
		Namespace:  "code",
		TextField:  "chunk_text",
	})
	indexer.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s, want %s", r.Method, http.MethodPost)
			}
			if r.URL.String() != "http://pinecone-local:5080/vectors/delete" {
				t.Fatalf("url = %s, want http://pinecone-local:5080/vectors/delete", r.URL.String())
			}
			if r.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("content type = %s, want application/json", r.Header.Get("Content-Type"))
			}
			if r.Header.Get("Api-Key") != "pclocal" {
				t.Fatalf("api key = %s, want pclocal", r.Header.Get("Api-Key"))
			}

			var got struct {
				IDs       []string `json:"ids"`
				Namespace string   `json:"namespace"`
			}
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if len(got.IDs) != 1 || got.IDs[0] != "chunk-123" {
				t.Fatalf("ids = %v, want [chunk-123]", got.IDs)
			}
			if got.Namespace != "code" {
				t.Fatalf("namespace = %s, want code", got.Namespace)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	if err := indexer.DeleteChunk(context.Background(), "chunk-123"); err != nil {
		t.Fatalf("DeleteChunk returned error: %v", err)
	}
}

func assertPineconeUpsertRequest(t *testing.T, r *http.Request, chunk *domain.Chunk) {
	t.Helper()

	if r.Method != http.MethodPost {
		t.Fatalf("method = %s, want %s", r.Method, http.MethodPost)
	}
	if r.URL.String() != "http://pinecone-local:5080/records/namespaces/code/upsert" {
		t.Fatalf("url = %s, want http://pinecone-local:5080/records/namespaces/code/upsert", r.URL.String())
	}
	if r.Header.Get("Content-Type") != "application/x-ndjson" {
		t.Fatalf("content type = %s, want application/x-ndjson", r.Header.Get("Content-Type"))
	}
	if r.Header.Get("Api-Key") != "pclocal" {
		t.Fatalf("api key = %s, want pclocal", r.Header.Get("Api-Key"))
	}
	if r.Header.Get("X-Pinecone-Api-Version") != "2026-04" {
		t.Fatalf("api version = %s, want 2026-04", r.Header.Get("X-Pinecone-Api-Version"))
	}

	var record map[string]any
	if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if record["_id"] != chunk.ChunkID {
		t.Fatalf("_id = %v, want %s", record["_id"], chunk.ChunkID)
	}
	if record["chunk_text"] != chunk.Content {
		t.Fatalf("chunk_text = %v, want %s", record["chunk_text"], chunk.Content)
	}
	if record["chunk_id"] != chunk.ChunkID {
		t.Fatalf("chunk_id = %v, want %s", record["chunk_id"], chunk.ChunkID)
	}
	if record["file_path"] != chunk.FilePath {
		t.Fatalf("file_path = %v, want %s", record["file_path"], chunk.FilePath)
	}
	if record["content_hash"] != chunk.ContentHash {
		t.Fatalf("content_hash = %v, want %s", record["content_hash"], chunk.ContentHash)
	}
}
