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
)

type OpenAIEmbeddingProvider struct {
	logger     *log.Logger
	config     *config.OpenAIConfig
	httpClient *http.Client
}

func NewOpenAIEmbeddingProvider(logger *log.Logger, cfg *config.OpenAIConfig) *OpenAIEmbeddingProvider {
	return &OpenAIEmbeddingProvider{
		logger:     logger,
		config:     cfg,
		httpClient: http.DefaultClient,
	}
}

func (p *OpenAIEmbeddingProvider) Embed(ctx context.Context, input string) ([]float32, error) {
	if p == nil || p.config == nil {
		return nil, fmt.Errorf("openai embedding config is nil")
	}
	if strings.TrimSpace(p.config.APIKey) == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is empty")
	}
	if strings.TrimSpace(input) == "" {
		return nil, fmt.Errorf("embedding input is empty")
	}

	body, err := json.Marshal(p.embeddingRequest(input))
	if err != nil {
		return nil, fmt.Errorf("marshal openai embedding request: %w", err)
	}

	endpoint, err := p.embeddingsURL()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create openai embedding request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute openai embedding request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("openai embedding request failed: %s", responseSummary(res))
	}

	var embeddingRes openAIEmbeddingResponse
	if err := json.NewDecoder(res.Body).Decode(&embeddingRes); err != nil {
		return nil, fmt.Errorf("decode openai embedding response: %w", err)
	}
	if len(embeddingRes.Data) == 0 || len(embeddingRes.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("openai embedding response did not include an embedding")
	}

	p.logf("created openai embedding model=%s dimensions=%d", p.model(), len(embeddingRes.Data[0].Embedding))
	return embeddingRes.Data[0].Embedding, nil
}

func (p *OpenAIEmbeddingProvider) embeddingRequest(input string) map[string]any {
	request := map[string]any{
		"model": p.model(),
		"input": input,
	}
	if p.config.EmbeddingDimensions > 0 {
		request["dimensions"] = p.config.EmbeddingDimensions
	}

	return request
}

func (p *OpenAIEmbeddingProvider) embeddingsURL() (string, error) {
	base := strings.TrimRight(p.baseURL(), "/")
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse openai base url %q: %w", base, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("openai base url must include scheme and host: %q", base)
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/embeddings"
	return parsed.String(), nil
}

func (p *OpenAIEmbeddingProvider) model() string {
	if strings.TrimSpace(p.config.EmbeddingModel) == "" {
		return "text-embedding-3-small"
	}

	return p.config.EmbeddingModel
}

func (p *OpenAIEmbeddingProvider) baseURL() string {
	if strings.TrimSpace(p.config.BaseURL) == "" {
		return "https://api.openai.com/v1"
	}

	return p.config.BaseURL
}

func (p *OpenAIEmbeddingProvider) logf(format string, args ...any) {
	if p.logger != nil {
		p.logger.Printf(format, args...)
	}
}

type openAIEmbeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}
