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

func TestElasticsearchIndexerIndexChunk(t *testing.T) {
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

	indexer := NewElasticsearchIndexer(nil, &config.ElasticsearchConfig{
		URL:   "http://elasticsearch:9200",
		Index: "tessa-chunks",
	})
	indexer.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			assertIndexChunkRequest(t, r, chunk)

			return &http.Response{
				StatusCode: http.StatusCreated,
				Status:     "201 Created",
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	if err := indexer.IndexChunk(context.Background(), chunk); err != nil {
		t.Fatalf("IndexChunk returned error: %v", err)
	}
}

func TestElasticsearchIndexerDeleteChunkIgnoresNotFound(t *testing.T) {
	indexer := NewElasticsearchIndexer(nil, &config.ElasticsearchConfig{
		URL:   "http://elasticsearch:9200",
		Index: "tessa-chunks",
	})
	indexer.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodDelete {
				t.Fatalf("method = %s, want %s", r.Method, http.MethodDelete)
			}
			if r.URL.String() != "http://elasticsearch:9200/tessa-chunks/_doc/missing-chunk" {
				t.Fatalf("url = %s, want http://elasticsearch:9200/tessa-chunks/_doc/missing-chunk", r.URL.String())
			}

			return &http.Response{
				StatusCode: http.StatusNotFound,
				Status:     "404 Not Found",
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	if err := indexer.DeleteChunk(context.Background(), "missing-chunk"); err != nil {
		t.Fatalf("DeleteChunk returned error: %v", err)
	}
}

func assertIndexChunkRequest(t *testing.T, r *http.Request, chunk *domain.Chunk) {
	t.Helper()

	if r.Method != http.MethodPut {
		t.Fatalf("method = %s, want %s", r.Method, http.MethodPut)
	}
	if r.URL.String() != "http://elasticsearch:9200/tessa-chunks/_doc/chunk-123" {
		t.Fatalf("url = %s, want http://elasticsearch:9200/tessa-chunks/_doc/chunk-123", r.URL.String())
	}
	if r.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("content type = %s, want application/json", r.Header.Get("Content-Type"))
	}

	var got domain.Chunk
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if got != *chunk {
		t.Fatalf("body = %+v, want %+v", got, *chunk)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
