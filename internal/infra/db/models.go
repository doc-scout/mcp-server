// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package db

import "time"

type dbEntity struct {
	Name       string `gorm:"primaryKey"`
	EntityType string `gorm:"index"`
}

type dbRelation struct {
	ID           uint   `gorm:"primaryKey;autoIncrement"`
	FromEntity   string `gorm:"index;column:from_node"`
	ToEntity     string `gorm:"index;column:to_node"`
	RelationType string `gorm:"index"`
	Confidence   string `gorm:"default:authoritative"`
}

type dbObservation struct {
	ID         uint   `gorm:"primaryKey;autoIncrement"`
	EntityName string `gorm:"index;column:entity_name"`
	Content    string
}

type dbDocContent struct {
	ID        uint   `gorm:"primaryKey;autoIncrement"`
	RepoName  string `gorm:"index;uniqueIndex:idx_repo_path"`
	Path      string `gorm:"uniqueIndex:idx_repo_path"`
	SHA       string
	FileType  string `gorm:"index"`
	Content   string `gorm:"type:text"`
	IndexedAt time.Time
}
