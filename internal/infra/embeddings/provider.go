// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package embeddings

import (
	"context"
	"log/slog"
	"os"
)

// EmbeddingProvider generates vector embeddings for a batch of texts.

// Implementations must be safe for concurrent use.

type EmbeddingProvider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)

	// ModelKey returns "<provider>:<model>" (e.g. "openai:text-embedding-3-small").

	// A change in ModelKey triggers re-indexing of all stored vectors.

	ModelKey() string
}

// Config holds embedding provider configuration.

type Config struct {
	OpenAIKey string

	OpenAIModel string

	OllamaURL string

	OllamaModel string
}

// ConfigFromEnv reads provider configuration from environment variables.

func ConfigFromEnv() Config {

	c := Config{

		OpenAIKey: os.Getenv("DOCSCOUT_EMBED_OPENAI_KEY"),

		OpenAIModel: os.Getenv("DOCSCOUT_EMBED_OPENAI_MODEL"),

		OllamaURL: os.Getenv("DOCSCOUT_EMBED_OLLAMA_URL"),

		OllamaModel: os.Getenv("DOCSCOUT_EMBED_OLLAMA_MODEL"),
	}

	if c.OpenAIModel == "" {

		c.OpenAIModel = "text-embedding-3-small"

	}

	if c.OllamaModel == "" {

		c.OllamaModel = "nomic-embed-text"

	}

	return c

}

// NewProvider returns the appropriate EmbeddingProvider based on cfg.

// Returns nil when no provider is configured (Plus feature disabled).

// When both OpenAI key and Ollama URL are set, OpenAI takes precedence.

func NewProvider(cfg Config) EmbeddingProvider {

	if cfg.OpenAIKey != "" && cfg.OllamaURL != "" {

		slog.Warn("[embeddings] Both DOCSCOUT_EMBED_OPENAI_KEY and DOCSCOUT_EMBED_OLLAMA_URL are set; using OpenAI")

	}

	if cfg.OpenAIKey != "" {

		return NewOpenAIProvider(cfg.OpenAIKey, cfg.OpenAIModel)

	}

	if cfg.OllamaURL != "" {

		return NewOllamaProvider(cfg.OllamaURL, cfg.OllamaModel)

	}

	return nil

}

