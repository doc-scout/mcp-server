// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package tools

import (
	"fmt"
	"strings"
)

const (
	// minObsLen is the minimum character length for a valid observation (after trimming).
	minObsLen = 2
	// maxObsLen is the maximum character length for a valid observation.
	// Observations longer than this are likely hallucinated blobs or accidental pastes.
	maxObsLen = 500
)

// SkippedObservation records a rejected observation along with the reason it was filtered.
type SkippedObservation struct {
	EntityName  string `json:"entity_name"`
	Observation string `json:"observation"`
	Reason      string `json:"reason"`
}

// sanitizeObservations filters and deduplicates a list of observation strings.
// It applies the following rules in order:
//  1. Trim leading/trailing whitespace.
//  2. Reject empty or whitespace-only strings.
//  3. Reject strings shorter than minObsLen characters.
//  4. Reject strings longer than maxObsLen characters.
//  5. Deduplicate: skip strings already seen within this batch (case-sensitive).
//
// Returns (valid, skipped) — valid is the cleaned list ready for storage,
// skipped records every rejection with its reason.
func sanitizeObservations(entityName string, raw []string) (valid []string, skipped []SkippedObservation) {
	seen := make(map[string]struct{}, len(raw))

	for _, obs := range raw {
		trimmed := strings.TrimSpace(obs)

		switch {
		case trimmed == "":
			skipped = append(skipped, SkippedObservation{
				EntityName:  entityName,
				Observation: obs,
				Reason:      "empty or whitespace-only",
			})
		case len(trimmed) < minObsLen:
			skipped = append(skipped, SkippedObservation{
				EntityName:  entityName,
				Observation: trimmed,
				Reason:      fmt.Sprintf("too short (min %d chars)", minObsLen),
			})
		case len(trimmed) > maxObsLen:
			skipped = append(skipped, SkippedObservation{
				EntityName:  entityName,
				Observation: trimmed[:80] + "…",
				Reason:      fmt.Sprintf("too long (%d chars, max %d)", len(trimmed), maxObsLen),
			})
		default:
			if _, dup := seen[trimmed]; dup {
				skipped = append(skipped, SkippedObservation{
					EntityName:  entityName,
					Observation: trimmed,
					Reason:      "duplicate within batch",
				})
			} else {
				seen[trimmed] = struct{}{}
				valid = append(valid, trimmed)
			}
		}
	}
	return valid, skipped
}
