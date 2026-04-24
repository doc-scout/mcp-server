// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package scanner

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-github/v60/github"
)

func init() {

	// Speed up retry tests by using a near-zero base backoff.

	baseBackoff = time.Millisecond

}

func TestRetryGitHub_SuccessFirstCall(t *testing.T) {

	calls := 0

	err := retryGitHub(t.Context(), func() error {

		calls++

		return nil

	})

	if err != nil {

		t.Fatalf("unexpected error: %v", err)

	}

	if calls != 1 {

		t.Errorf("expected 1 call, got %d", calls)

	}

}

func TestRetryGitHub_NonRetryableError(t *testing.T) {

	calls := 0

	sentinel := errors.New("not retryable")

	err := retryGitHub(t.Context(), func() error {

		calls++

		return sentinel

	})

	if !errors.Is(err, sentinel) {

		t.Fatalf("expected sentinel error, got %v", err)

	}

	// Non-retryable errors must not be retried.

	if calls != 1 {

		t.Errorf("expected 1 call for non-retryable error, got %d", calls)

	}

}

func TestRetryGitHub_TransientServerError_EventuallySucceeds(t *testing.T) {

	calls := 0

	err := retryGitHub(t.Context(), func() error {

		calls++

		if calls < 3 {

			return &github.ErrorResponse{

				Response: &http.Response{StatusCode: http.StatusServiceUnavailable},
			}

		}

		return nil

	})

	if err != nil {

		t.Fatalf("expected success after retries, got: %v", err)

	}

	if calls != 3 {

		t.Errorf("expected 3 calls, got %d", calls)

	}

}

func TestRetryGitHub_ExhaustsRetries(t *testing.T) {

	calls := 0

	transient := &github.ErrorResponse{

		Response: &http.Response{StatusCode: http.StatusBadGateway},
	}

	err := retryGitHub(t.Context(), func() error {

		calls++

		return transient

	})

	if err == nil {

		t.Fatal("expected error after exhausting retries")

	}

	if calls != maxRetries+1 {

		t.Errorf("expected %d calls, got %d", maxRetries+1, calls)

	}

}

func TestRetryGitHub_ContextCancellation(t *testing.T) {

	ctx, cancel := context.WithCancel(t.Context())

	calls := 0

	err := retryGitHub(ctx, func() error {

		calls++

		cancel() // cancel after first attempt

		return &github.ErrorResponse{

			Response: &http.Response{StatusCode: http.StatusInternalServerError},
		}

	})

	if !errors.Is(err, context.Canceled) {

		t.Errorf("expected context.Canceled, got: %v", err)

	}

}

func TestRetryDelay_PrimaryRateLimit(t *testing.T) {

	reset := time.Now().Add(30 * time.Second)

	rle := &github.RateLimitError{

		Rate: github.Rate{Reset: github.Timestamp{Time: reset}},
	}

	wait, ok := retryDelay(rle, 0)

	if !ok {

		t.Fatal("expected retryable")

	}

	if wait < 25*time.Second || wait > 35*time.Second {

		t.Errorf("expected ~30s wait, got %s", wait)

	}

}

func TestRetryDelay_AbuseRateLimitWithRetryAfter(t *testing.T) {

	retryAfter := 60 * time.Second

	arle := &github.AbuseRateLimitError{RetryAfter: &retryAfter}

	wait, ok := retryDelay(arle, 0)

	if !ok {

		t.Fatal("expected retryable")

	}

	if wait != retryAfter {

		t.Errorf("expected %s, got %s", retryAfter, wait)

	}

}

func TestRetryDelay_404NotRetryable(t *testing.T) {

	err := &github.ErrorResponse{

		Response: &http.Response{StatusCode: http.StatusNotFound},
	}

	_, ok := retryDelay(err, 0)

	if ok {

		t.Error("expected 404 to be non-retryable")

	}

}

func TestRetryDelay_ExponentialBackoff(t *testing.T) {

	err := &github.ErrorResponse{

		Response: &http.Response{StatusCode: http.StatusInternalServerError},
	}

	wait0, _ := retryDelay(err, 0)

	wait1, _ := retryDelay(err, 1)

	wait2, _ := retryDelay(err, 2)

	if wait1 != 2*wait0 {

		t.Errorf("expected wait1 = 2*wait0, got %s and %s", wait1, wait0)

	}

	if wait2 != 2*wait1 {

		t.Errorf("expected wait2 = 2*wait1, got %s and %s", wait2, wait1)

	}

}
