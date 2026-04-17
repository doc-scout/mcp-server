// Copyright 2026 Leonan Carvalho

// SPDX-License-Identifier: AGPL-3.0-only

// Package health exposes a lightweight /healthz handler for HTTP deployments.

// It is intentionally unauthenticated so load balancers and Kubernetes probes

// can reach it without a bearer token.

package health

import (
	"encoding/json"
	"net/http"
	"time"
)

// StatusProvider is satisfied by anything that can report scanner/graph state.

// In production this is the scanner.Scanner + memory.MemoryService combo; in

// tests it is a simple stub.

type StatusProvider interface {

	// HealthStatus returns the current health snapshot.

	// ready is true once the first scan has completed (repos > 0).

	HealthStatus() Status
}

// Status is the JSON body returned by /healthz.

type Status struct {

	// Status is "ok" when the server is ready to serve requests, "starting"

	// before the first scan has completed.

	Status string `json:"status"`

	StartedAt time.Time `json:"started_at"`

	RepoCount int `json:"repos"`

	Entities int64 `json:"entities"`
}

// Handler returns an http.HandlerFunc that serves /healthz.

// The response is always JSON. HTTP 200 means ready; 503 means starting.

func Handler(p StatusProvider) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		s := p.HealthStatus()

		w.Header().Set("Content-Type", "application/json")

		if s.Status != "ok" {

			w.WriteHeader(http.StatusServiceUnavailable)

		}

		_ = json.NewEncoder(w).Encode(s)

	}

}
