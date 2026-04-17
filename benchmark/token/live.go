// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package token

import (
	"context"
	"errors"
	"fmt"
	"os"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// ErrNoAPIKey is returned by NewLiveRunner when ANTHROPIC_API_KEY is not set.
var ErrNoAPIKey = errors.New("ANTHROPIC_API_KEY environment variable not set")

// LiveRunner measures real token usage by calling the Anthropic API.
type LiveRunner struct {
	client *anthropic.Client
}

// LiveResult holds token measurements for a single question comparison.
type LiveResult struct {
	Index        int
	Question     string
	DocScoutToks int
	NaiveToks    int
	SavingsPct   float64
}

// NewLiveRunner creates a LiveRunner. It reads the API key from ANTHROPIC_API_KEY
// and returns ErrNoAPIKey if the variable is empty. No network call is made.
func NewLiveRunner(_ context.Context) (*LiveRunner, error) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return nil, ErrNoAPIKey
	}
	client := anthropic.NewClient(option.WithAPIKey(key))
	return &LiveRunner{client: &client}, nil
}

// MeasureQuestion calls Claude twice — once with docscoutContext and once with
// naiveContext — and returns the input token counts for each. MaxTokens is
// intentionally limited to 100 to keep API costs low.
func (r *LiveRunner) MeasureQuestion(ctx context.Context, idx int, question, docscoutContext, naiveContext string) (LiveResult, error) {
	dsToks, err := r.countInputTokens(ctx, question, docscoutContext)
	if err != nil {
		return LiveResult{}, fmt.Errorf("docscout call for question %d: %w", idx, err)
	}

	naiveToks, err := r.countInputTokens(ctx, question, naiveContext)
	if err != nil {
		return LiveResult{}, fmt.Errorf("naive call for question %d: %w", idx, err)
	}

	fmt.Fprintf(os.Stderr, "question %d: docscout=%d naive=%d tokens\n", idx, dsToks, naiveToks)

	return LiveResult{
		Index:        idx,
		Question:     question,
		DocScoutToks: dsToks,
		NaiveToks:    naiveToks,
		SavingsPct:   SavingsPct(dsToks, naiveToks),
	}, nil
}

// countInputTokens sends a single-turn message and returns the input token count
// from the API response.
func (r *LiveRunner) countInputTokens(ctx context.Context, question, contextText string) (int, error) {
	userContent := contextText + "\n\n" + question
	msg, err := r.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeHaiku4_5,
		MaxTokens: 100,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userContent)),
		},
	})
	if err != nil {
		return 0, err
	}
	return int(msg.Usage.InputTokens), nil
}
