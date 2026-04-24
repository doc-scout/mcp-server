// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"strings"
	"testing"
)

func TestSanitizeObservations_Valid(t *testing.T) {

	valid, skipped := sanitizeObservations("my-svc", []string{

		"lang:go",

		"version:1.0.0",

		"_source:catalog-info",
	})

	if len(valid) != 3 {

		t.Errorf("want 3 valid, got %d: %v", len(valid), valid)

	}

	if len(skipped) != 0 {

		t.Errorf("want 0 skipped, got %d: %v", len(skipped), skipped)

	}

}

func TestSanitizeObservations_Empty(t *testing.T) {

	valid, skipped := sanitizeObservations("svc", []string{"", "   ", "\t"})

	if len(valid) != 0 {

		t.Errorf("want 0 valid, got %v", valid)

	}

	if len(skipped) != 3 {

		t.Errorf("want 3 skipped, got %d", len(skipped))

	}

	for _, s := range skipped {

		if s.Reason != "empty or whitespace-only" {

			t.Errorf("unexpected reason %q", s.Reason)

		}

	}

}

func TestSanitizeObservations_TooShort(t *testing.T) {

	valid, skipped := sanitizeObservations("svc", []string{"x", "ok"})

	// "x" is 1 char (< minObsLen=2), "ok" is exactly 2 chars — valid

	if len(valid) != 1 || valid[0] != "ok" {

		t.Errorf("want [ok], got %v", valid)

	}

	if len(skipped) != 1 || skipped[0].Observation != "x" {

		t.Errorf("want 1 skipped for 'x', got %v", skipped)

	}

}

func TestSanitizeObservations_TooLong(t *testing.T) {

	long := strings.Repeat("a", maxObsLen+1)

	valid, skipped := sanitizeObservations("svc", []string{long, "short"})

	if len(valid) != 1 || valid[0] != "short" {

		t.Errorf("want [short], got %v", valid)

	}

	if len(skipped) != 1 || !strings.Contains(skipped[0].Reason, "too long") {

		t.Errorf("want 1 too-long skip, got %v", skipped)

	}

}

func TestSanitizeObservations_Deduplication(t *testing.T) {

	valid, skipped := sanitizeObservations("svc", []string{

		"lang:go",

		"lang:go", // duplicate

		"version:1.0",

		"lang:go", // duplicate again

	})

	if len(valid) != 2 {

		t.Errorf("want 2 valid (deduped), got %d: %v", len(valid), valid)

	}

	if len(skipped) != 2 {

		t.Errorf("want 2 skipped duplicates, got %d", len(skipped))

	}

	for _, s := range skipped {

		if s.Reason != "duplicate within batch" {

			t.Errorf("unexpected reason %q", s.Reason)

		}

	}

}

func TestSanitizeObservations_Trimming(t *testing.T) {

	valid, skipped := sanitizeObservations("svc", []string{"  lang:go  ", " lang:go "})

	// Both trim to "lang:go" — first is valid, second is duplicate

	if len(valid) != 1 || valid[0] != "lang:go" {

		t.Errorf("want [lang:go], got %v", valid)

	}

	if len(skipped) != 1 || skipped[0].Reason != "duplicate within batch" {

		t.Errorf("want 1 duplicate skip, got %v", skipped)

	}

}

func TestSanitizeObservations_Empty_Input(t *testing.T) {

	valid, skipped := sanitizeObservations("svc", nil)

	if len(valid) != 0 || len(skipped) != 0 {

		t.Errorf("want empty slices for nil input, got valid=%v skipped=%v", valid, skipped)

	}

}
