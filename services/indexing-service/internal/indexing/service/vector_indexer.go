package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/rohandave/tessa-rag/services/indexing-service/internal/config"
	"github.com/rohandave/tessa-rag/services/shared/domain"
)

type PineconeIndexer struct {
	logger     *log.Logger
	config     *config.PineconeConfig
	httpClient *http.Client
}

func NewPineconeIndexer(logger *log.Logger, cfg *config.PineconeConfig) *PineconeIndexer {
	return &PineconeIndexer{
		logger:     logger,
		config:     cfg,
		httpClient: http.DefaultClient,
	}
}

func (i *PineconeIndexer) IndexChunk(ctx context.Context, chunk *domain.Chunk) error {
	if chunk == nil {
		return fmt.Errorf("chunk is nil")
	}

	record, err := pineconeRecordFromChunk(chunk, i.textField())
	if err != nil {
		return err
	}

	body, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal pinecone upsert request for chunk %s: %w", chunk.ChunkID, err)
	}
	body = append(body, '\n')

	endpoint, err := i.recordsUpsertURL()
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create pinecone upsert request for chunk %s: %w", chunk.ChunkID, err)
	}
	i.setHeaders(req, "application/x-ndjson")

	res, err := i.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute pinecone upsert request for chunk %s: %w", chunk.ChunkID, err)
	}
	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("pinecone upsert chunk %s failed: %s", chunk.ChunkID, responseSummary(res))
	}

	i.logger.Printf("pinecone upserted chunk host=%s index=%s namespace=%s chunk_id=%s", i.config.Host, i.config.Index, i.namespace(), chunk.ChunkID)
	return nil
}

func (i *PineconeIndexer) DeleteChunk(ctx context.Context, chunkID string) error {
	if strings.TrimSpace(chunkID) == "" {
		return fmt.Errorf("chunk id is empty")
	}

	body, err := json.Marshal(map[string]any{
		"ids":       []string{chunkID},
		"namespace": i.namespace(),
	})
	if err != nil {
		return fmt.Errorf("marshal pinecone delete request for chunk %s: %w", chunkID, err)
	}

	endpoint, err := i.vectorsDeleteURL()
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create pinecone delete request for chunk %s: %w", chunkID, err)
	}
	i.setHeaders(req, "application/json")

	res, err := i.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute pinecone delete request for chunk %s: %w", chunkID, err)
	}
	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("pinecone delete chunk %s failed: %s", chunkID, responseSummary(res))
	}

	i.logger.Printf("pinecone deleted chunk host=%s index=%s namespace=%s chunk_id=%s", i.config.Host, i.config.Index, i.namespace(), chunkID)
	return nil
}

func pineconeRecordFromChunk(chunk *domain.Chunk, textField string) (map[string]any, error) {
	body, err := json.Marshal(chunk)
	if err != nil {
		return nil, fmt.Errorf("marshal chunk %s for pinecone record: %w", chunk.ChunkID, err)
	}

	var record map[string]any
	if err := json.Unmarshal(body, &record); err != nil {
		return nil, fmt.Errorf("unmarshal chunk %s for pinecone record: %w", chunk.ChunkID, err)
	}

	record["_id"] = chunk.ChunkID
	record[textField] = chunk.Content
	return record, nil
}

func (i *PineconeIndexer) recordsUpsertURL() (string, error) {
	return i.buildURL("/records/namespaces/" + url.PathEscape(i.namespace()) + "/upsert")
}

func (i *PineconeIndexer) vectorsDeleteURL() (string, error) {
	return i.buildURL("/vectors/delete")
}

func (i *PineconeIndexer) buildURL(path string) (string, error) {
	if i == nil || i.config == nil {
		return "", fmt.Errorf("pinecone config is nil")
	}
	if strings.TrimSpace(i.config.Host) == "" {
		return "", fmt.Errorf("pinecone host is empty")
	}

	baseURL, err := url.Parse(strings.TrimRight(i.config.Host, "/"))
	if err != nil {
		return "", fmt.Errorf("parse pinecone host %q: %w", i.config.Host, err)
	}
	if baseURL.Scheme == "" || baseURL.Host == "" {
		return "", fmt.Errorf("pinecone host must include scheme and host: %q", i.config.Host)
	}

	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + path
	return baseURL.String(), nil
}

func (i *PineconeIndexer) namespace() string {
	if i == nil || i.config == nil || strings.TrimSpace(i.config.Namespace) == "" {
		return "__default__"
	}

	return i.config.Namespace
}

func (i *PineconeIndexer) textField() string {
	if i == nil || i.config == nil || strings.TrimSpace(i.config.TextField) == "" {
		return "content"
	}

	return i.config.TextField
}

func (i *PineconeIndexer) setHeaders(req *http.Request, contentType string) {
	req.Header.Set("Content-Type", contentType)
	if i != nil && i.config != nil && strings.TrimSpace(i.config.APIKey) != "" {
		req.Header.Set("Api-Key", i.config.APIKey)
	}
	if i != nil && i.config != nil && strings.TrimSpace(i.config.APIVersion) != "" {
		req.Header.Set("X-Pinecone-Api-Version", i.config.APIVersion)
	}
}
