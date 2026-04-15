// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package embeddings

import (
	"encoding/binary"
	"math"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type dbDocEmbedding struct {
	DocID       string    `gorm:"primaryKey"`
	ContentHash string    `gorm:"not null"`
	// Vector stores []float32 as little-endian IEEE 754 bytes.
	// This field is STALE when ContentHash no longer matches sha256hex(current doc content)
	// or when ModelKey differs from the active provider. Re-indexed automatically on next scan.
	Vector      []byte    `gorm:"not null"`
	ModelKey    string    `gorm:"not null;index"`
	UpdatedAt   time.Time `gorm:"not null"`
}

type dbEntityEmbedding struct {
	EntityName string    `gorm:"primaryKey"`
	ObsHash    string    `gorm:"not null"`
	// Vector stores []float32 as little-endian IEEE 754 bytes.
	// This field is STALE when ObsHash no longer matches sha256hex(EntityText(current entity))
	// or when ModelKey differs from the active provider. Re-indexed on next mutation.
	Vector     []byte    `gorm:"not null"`
	ModelKey   string    `gorm:"not null;index"`
	UpdatedAt  time.Time `gorm:"not null"`
}

// VectorStore persists and retrieves float32 embedding vectors in the existing DB.
type VectorStore struct {
	db *gorm.DB
}

// NewVectorStore runs auto-migration for the two embedding tables and returns a VectorStore.
func NewVectorStore(db *gorm.DB) (*VectorStore, error) {
	if err := db.AutoMigrate(&dbDocEmbedding{}, &dbEntityEmbedding{}); err != nil {
		return nil, err
	}
	return &VectorStore{db: db}, nil
}

// encodeVector serialises []float32 as little-endian IEEE 754 bytes (4 bytes per element).
func encodeVector(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

// decodeVector deserialises little-endian IEEE 754 bytes back to []float32.
func decodeVector(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

// DocEmbedding is a decoded document embedding row.
type DocEmbedding struct {
	DocID       string
	ContentHash string
	Vector      []float32
}

// EntityEmbedding is a decoded entity embedding row.
type EntityEmbedding struct {
	EntityName string
	ObsHash    string
	Vector     []float32
}

// UpsertDoc stores or replaces a document embedding.
func (vs *VectorStore) UpsertDoc(docID, contentHash, modelKey string, vector []float32) error {
	row := dbDocEmbedding{
		DocID:       docID,
		ContentHash: contentHash,
		Vector:      encodeVector(vector),
		ModelKey:    modelKey,
		UpdatedAt:   time.Now().UTC(),
	}
	return vs.db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}

// UpsertEntity stores or replaces an entity embedding.
func (vs *VectorStore) UpsertEntity(entityName, obsHash, modelKey string, vector []float32) error {
	row := dbEntityEmbedding{
		EntityName: entityName,
		ObsHash:    obsHash,
		Vector:     encodeVector(vector),
		ModelKey:   modelKey,
		UpdatedAt:  time.Now().UTC(),
	}
	return vs.db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}

// LoadDocEmbeddings returns all doc embeddings for the given modelKey.
func (vs *VectorStore) LoadDocEmbeddings(modelKey string) ([]DocEmbedding, error) {
	var rows []dbDocEmbedding
	if err := vs.db.Where("model_key = ?", modelKey).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]DocEmbedding, len(rows))
	for i, r := range rows {
		out[i] = DocEmbedding{DocID: r.DocID, ContentHash: r.ContentHash, Vector: decodeVector(r.Vector)}
	}
	return out, nil
}

// LoadEntityEmbeddings returns all entity embeddings for the given modelKey.
func (vs *VectorStore) LoadEntityEmbeddings(modelKey string) ([]EntityEmbedding, error) {
	var rows []dbEntityEmbedding
	if err := vs.db.Where("model_key = ?", modelKey).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]EntityEmbedding, len(rows))
	for i, r := range rows {
		out[i] = EntityEmbedding{EntityName: r.EntityName, ObsHash: r.ObsHash, Vector: decodeVector(r.Vector)}
	}
	return out, nil
}

// DeleteDocByID removes a single doc embedding row by its docID.
func (vs *VectorStore) DeleteDocByID(docID string) error {
	return vs.db.Where("doc_id = ?", docID).Delete(&dbDocEmbedding{}).Error
}
