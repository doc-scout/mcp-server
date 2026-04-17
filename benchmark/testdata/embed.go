// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package testdata

import "embed"

//go:embed synthetic-org ground_truth.json questions.json
var FS embed.FS
