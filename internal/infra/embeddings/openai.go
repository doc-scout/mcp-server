// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrRateLimit is returned when the OpenAI API responds with HTTP 429.

var ErrRateLimit = errors.New("openai: rate limit exceeded (HTTP 429)")

type openAIProvider struct {
	apiKey string

	model string

	baseURL string

	client *http.Client
}

// NewOpenAIProvider creates a provider that calls the production OpenAI embeddings API.

func NewOpenAIProvider(apiKey, model string) EmbeddingProvider {

	return NewOpenAIProviderWithURL(apiKey, model, "https://api.openai.com")

}

// NewOpenAIProviderWithURL creates a provider with a configurable base URL (used in tests).

func NewOpenAIProviderWithURL(apiKey, model, baseURL string) EmbeddingProvider {

	return &openAIProvider{

		apiKey: apiKey,

		model: model,

		baseURL: baseURL,

		client: &http.Client{Timeout: 30 * time.Second},
	}

}

func (p *openAIProvider) ModelKey() string { return "openai:" + p.model }

type openAIEmbedRequest struct {
	Input []string `json:"input"`

	Model string `json:"model"`
}

type openAIEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func (p *openAIProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {

	var all [][]float32

	for i := 0; i < len(texts); i += 2048 {

		end := i + 2048

		if end > len(texts) {

			end = len(texts)

		}

		batch, err := p.embedBatch(ctx, texts[i:end])

		if err != nil {

			return nil, err

		}

		all = append(all, batch...)

	}

	return all, nil

}

func (p *openAIProvider) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {

	body, err := json.Marshal(openAIEmbedRequest{Input: texts, Model: p.model})

	if err != nil {

		return nil, fmt.Errorf("openai embed: marshal: %w", err)

	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/embeddings", bytes.NewReader(body))

	if err != nil {

		return nil, err

	}

	req.Header.Set("Content-Type", "application/json")

	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)

	if err != nil {

		return nil, fmt.Errorf("openai embed: %w", err)

	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {

		return nil, ErrRateLimit

	}

	if resp.StatusCode != http.StatusOK {

		b, _ := io.ReadAll(resp.Body)

		return nil, fmt.Errorf("openai embed: HTTP %d: %s", resp.StatusCode, string(b))

	}

	var result openAIEmbedResponse

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {

		return nil, fmt.Errorf("openai embed: decode: %w", err)

	}

	out := make([][]float32, len(result.Data))

	for i, d := range result.Data {

		out[i] = d.Embedding

	}

	return out, nil

}

