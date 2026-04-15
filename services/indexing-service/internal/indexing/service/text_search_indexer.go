package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/rohandave/tessa-rag/services/indexing-service/internal/config"
	"github.com/rohandave/tessa-rag/services/shared/domain"
)

type ElasticsearchIndexer struct {
	logger     *log.Logger
	config     *config.ElasticsearchConfig
	httpClient *http.Client
}

func NewElasticsearchIndexer(logger *log.Logger, cfg *config.ElasticsearchConfig) *ElasticsearchIndexer {
	return &ElasticsearchIndexer{
		logger:     logger,
		config:     cfg,
		httpClient: http.DefaultClient,
	}
}

func (i *ElasticsearchIndexer) IndexChunk(ctx context.Context, chunk *domain.Chunk) error {
	if chunk == nil {
		return fmt.Errorf("chunk is nil")
	}

	body, err := json.Marshal(chunk)
	if err != nil {
		return fmt.Errorf("marshal chunk %s: %w", chunk.ChunkID, err)
	}

	endpoint, err := i.documentURL(chunk.ChunkID)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create elasticsearch index request for chunk %s: %w", chunk.ChunkID, err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := i.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute elasticsearch index request for chunk %s: %w", chunk.ChunkID, err)
	}
	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("elasticsearch index chunk %s failed: %s", chunk.ChunkID, responseSummary(res))
	}

	i.logf("elasticsearch indexed chunk index=%s chunk_id=%s content_hash=%s", i.config.Index, chunk.ChunkID, chunk.ContentHash)
	return nil
}

func (i *ElasticsearchIndexer) DeleteChunk(ctx context.Context, chunkID string) error {
	endpoint, err := i.documentURL(chunkID)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create elasticsearch delete request for chunk %s: %w", chunkID, err)
	}

	res, err := i.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute elasticsearch delete request for chunk %s: %w", chunkID, err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotFound {
		i.logf("elasticsearch chunk already absent index=%s chunk_id=%s", i.config.Index, chunkID)
		return nil
	}
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("elasticsearch delete chunk %s failed: %s", chunkID, responseSummary(res))
	}

	i.logf("elasticsearch deleted chunk index=%s chunk_id=%s", i.config.Index, chunkID)
	return nil
}

func (i *ElasticsearchIndexer) documentURL(chunkID string) (string, error) {
	if i == nil || i.config == nil {
		return "", fmt.Errorf("elasticsearch config is nil")
	}
	if strings.TrimSpace(i.config.URL) == "" {
		return "", fmt.Errorf("elasticsearch url is empty")
	}
	if strings.TrimSpace(i.config.Index) == "" {
		return "", fmt.Errorf("elasticsearch index is empty")
	}
	if strings.TrimSpace(chunkID) == "" {
		return "", fmt.Errorf("chunk id is empty")
	}

	baseURL, err := url.Parse(strings.TrimRight(i.config.URL, "/"))
	if err != nil {
		return "", fmt.Errorf("parse elasticsearch url %q: %w", i.config.URL, err)
	}
	if baseURL.Scheme == "" || baseURL.Host == "" {
		return "", fmt.Errorf("elasticsearch url must include scheme and host: %q", i.config.URL)
	}

	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/" + url.PathEscape(i.config.Index) + "/_doc/" + url.PathEscape(chunkID)
	return baseURL.String(), nil
}

func (i *ElasticsearchIndexer) logf(format string, args ...any) {
	if i.logger != nil {
		i.logger.Printf(format, args...)
	}
}

func responseSummary(res *http.Response) string {
	body, err := io.ReadAll(io.LimitReader(res.Body, 4096))
	if err != nil || len(body) == 0 {
		return res.Status
	}

	return fmt.Sprintf("%s: %s", res.Status, strings.TrimSpace(string(body)))
}
