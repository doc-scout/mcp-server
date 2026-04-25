// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	coreaudit "github.com/doc-scout/mcp-server/internal/core/audit"
)

var inMemoryCounter atomic.Int64

// OpenDB opens the database and runs AutoMigrate for all models.
// dbURL accepts: "sqlite://path.db", "postgres://...", a plain file path, or "" for in-memory SQLite.
func OpenDB(dbURL string) (*gorm.DB, error) {
	cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}

	var (
		db  *gorm.DB
		err error
	)

	switch {
	case strings.HasPrefix(dbURL, "postgres://"), strings.HasPrefix(dbURL, "postgresql://"):
		db, err = gorm.Open(postgres.Open(dbURL), cfg)
	case strings.HasPrefix(dbURL, "sqlite://"):
		path := strings.TrimPrefix(dbURL, "sqlite://")
		db, err = gorm.Open(sqlite.Open(path), cfg)
	case dbURL == "":
		name := fmt.Sprintf("file:memdb%d?mode=memory&cache=shared", inMemoryCounter.Add(1))
		db, err = gorm.Open(sqlite.Open(name), cfg)
	default:
		db, err = gorm.Open(sqlite.Open(dbURL), cfg)
	}

	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(
		&dbEntity{}, &dbRelation{}, &dbObservation{}, &dbDocContent{},
		&coreaudit.AuditEvent{},
	); err != nil {
		return nil, err
	}

	return db, nil
}
