// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package github

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/go-github/v60/github"
)

const (
	maxRetries = 3

	maxRetryWait = 5 * time.Minute
)

// baseBackoff is the starting delay for exponential backoff. Exposed as a var

// so tests can set it to a small value without real sleeps.

var baseBackoff = 2 * time.Second

// retryGitHub calls fn, retrying up to maxRetries times on GitHub rate-limit

// errors and transient server errors (5xx / 429).

//

// Wait strategy:

//   - Primary rate limit (*RateLimitError):   wait until Rate.Reset, capped at maxRetryWait.

//   - Secondary rate limit (*AbuseRateLimitError): wait RetryAfter if set, else baseBackoff.

//   - Transient 5xx / 429:                    exponential backoff (2s, 4s, 8s …), capped at maxRetryWait.

//

// Non-retryable errors (4xx other than 429, network errors, context cancellation)

// are returned immediately.

func retryGitHub(ctx context.Context, fn func() error) error {

	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {

		lastErr = fn()

		if lastErr == nil {

			return nil

		}

		wait, ok := retryDelay(lastErr, attempt)

		if !ok {

			return lastErr

		}

		if wait > maxRetryWait {

			wait = maxRetryWait

		}

		slog.Warn("[scanner] GitHub API retry", "attempt", attempt+1, "max", maxRetries, "wait", wait.Round(time.Second), "error", lastErr)

		select {

		case <-ctx.Done():

			return ctx.Err()

		case <-time.After(wait):

		}

	}

	return lastErr

}

// retryDelay returns how long to wait before the next attempt and whether to retry.

func retryDelay(err error, attempt int) (time.Duration, bool) {

	// Primary rate limit: wait until the rate limit window resets.

	if rle, ok := errors.AsType[*github.RateLimitError](err); ok {

		wait := time.Until(rle.Rate.Reset.Time)

		if wait <= 0 {

			wait = baseBackoff

		}

		return wait, true

	}

	// Secondary (abuse) rate limit: respect the Retry-After header if present.

	if arle, ok := errors.AsType[*github.AbuseRateLimitError](err); ok {

		if arle.RetryAfter != nil {

			return *arle.RetryAfter, true

		}

		return baseBackoff << attempt, true

	}

	// Transient server errors from the GitHub API (5xx or 429).

	if ghResp, ok := errors.AsType[*github.ErrorResponse](err); ok && ghResp.Response != nil {

		code := ghResp.Response.StatusCode

		if code == 429 || code >= 500 {

			return baseBackoff << attempt, true

		}

	}

	return 0, false

}
