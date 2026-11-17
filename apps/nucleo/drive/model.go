// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const (
	itemKind          = "org.sytue.drive.item.v1"
	bodyKind          = "org.sytue.drive.body.v1"
	entryKind         = "org.sytue.drive.entry.v1"
	inlineSourceBytes = 16 << 10
)

// Summaries and their complete original JSON live only inside private records.
type summary struct {
	Kind           string     `json:"kind"`
	SourceKind     string     `json:"source_kind"`
	SourceID       string     `json:"source_id"`
	Epoch          string     `json:"epoch,omitempty"`
	RawSHA256      string     `json:"raw_sha256"`
	BaselineSHA256 string     `json:"baseline_sha256,omitempty"`
	Seed           bool       `json:"seed,omitempty"`
	ParentID       string     `json:"parent_id,omitempty"`
	Name           string     `json:"name,omitempty"`
	SizeBytes      int64      `json:"size_bytes,omitempty"`
	ContentType    string     `json:"content_type,omitempty"`
	Checksum       string     `json:"checksum,omitempty"`
	Version        int        `json:"version,omitempty"`
	IsRoot         bool       `json:"is_root,omitempty"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
}

type bodySummary struct {
	Kind       string `json:"kind"`
	SourceKind string `json:"source_kind"`
	SourceID   string `json:"source_id"`
	Epoch      string `json:"epoch,omitempty"`
	Purpose    string `json:"purpose"`
	SHA256     string `json:"sha256"`
	SizeBytes  int64  `json:"size_bytes"`
}

type bodyPointer struct {
	ObjectID  string            `json:"object_id"`
	Reference storage.Reference `json:"reference"`
	Origin    bodyOrigin        `json:"origin,omitzero"`
}

// Copies and version snapshots retain an explicitly authenticated immutable
// content identity. The origin is always flattened; it cannot form alias chains.
type bodyOrigin struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Epoch string `json:"epoch,omitempty"`
}

func (pointer bodyPointer) source(item summary, purpose string) (summary, error) {
	if pointer.Origin != (bodyOrigin{}) {
		origin := pointer.Origin
		if purpose != "binary" || !binaryKind(origin.Kind) || !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: origin.Kind, ID: origin.ID}) || origin.Epoch != "" && !storage.MigrationEpoch(origin.Epoch) {
			return summary{}, ErrSource
		}
		item.SourceKind, item.SourceID, item.Epoch = origin.Kind, origin.ID, origin.Epoch
	}
	if pointer.ObjectID != bodyID(item.SourceKind, item.SourceID, purpose) || !pointer.Reference.Valid() {
		return summary{}, ErrSource
	}
	return item, nil
}

type entry struct {
	Kind        string       `json:"kind"`
	SourceKind  string       `json:"source_kind"`
	SourceID    string       `json:"source_id"`
	Raw         []byte       `json:"raw,omitempty"`
	Metadata    *bodyPointer `json:"metadata,omitempty"`
	Binary      *bodyPointer `json:"binary,omitempty"`
	observation *observation
}

func sourceKind(kind string) bool {
	switch kind {
	case "folder", "file", "version", "share", "favorite", "request", "provenance":
		return true
	default:
		return false
	}
}

func digest(data []byte) string              { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func objectID(kind, id string) string        { return "drive_" + digest([]byte(kind+"\x00"+id)) }
func bodyID(kind, id, purpose string) string { return objectID(kind+"_"+purpose, id) }
func validHash(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func binaryKind(kind string) bool { return kind == "file" || kind == "version" }

func sourceRowValid(row storage.LegacySourceRow, epoch string) bool {
	return sourceKind(row.Kind) && storage.SourceCursorValid(storage.LegacySourceCursor{Kind: row.Kind, ID: row.ID}) && (epoch == "" || storage.MigrationEpoch(epoch)) && len(row.Payload) <= storage.MaxLegacySourceBytes && json.Valid(row.Payload)
}

func summarize(row storage.LegacySourceRow, epoch string) (summary, error) {
	result := summary{Kind: itemKind, SourceKind: row.Kind, SourceID: row.ID, Epoch: epoch, RawSHA256: digest(row.Payload), Seed: epoch != ""}
	if epoch != "" {
		result.BaselineSHA256 = result.RawSHA256
	}
	if !sourceRowValid(row, epoch) {
		return summary{}, ErrSource
	}
	var raw struct {
		ID          string     `json:"id"`
		ParentID    *string    `json:"parent_id"`
		FolderID    string     `json:"folder_id"`
		FileID      string     `json:"file_id"`
		Name        string     `json:"name"`
		SizeBytes   int64      `json:"size_bytes"`
		ContentType string     `json:"content_type"`
		Checksum    string     `json:"checksum"`
		Version     int        `json:"version"`
		IsRoot      bool       `json:"is_root"`
		UpdatedAt   time.Time  `json:"updated_at"`
		CreatedAt   time.Time  `json:"created_at"`
		DeletedAt   *time.Time `json:"deleted_at"`
	}
	if json.Unmarshal(row.Payload, &raw) != nil {
		return summary{}, ErrSource
	}
	if (row.Kind == "folder" || binaryKind(row.Kind)) && raw.ID != row.ID {
		return summary{}, ErrSource
	}
	if binaryKind(row.Kind) && (raw.SizeBytes < 0 || raw.SizeBytes > storage.MaxFileBytes) {
		return summary{}, ErrSource
	}
	switch row.Kind {
	case "folder":
		result.Name, result.IsRoot = raw.Name, raw.IsRoot
		if raw.ParentID != nil {
			result.ParentID = *raw.ParentID
		}
	case "file":
		result.ParentID, result.Name, result.SizeBytes, result.ContentType, result.Version = raw.FolderID, raw.Name, raw.SizeBytes, raw.ContentType, raw.Version
	case "version":
		result.ParentID, result.SizeBytes, result.ContentType, result.Version = raw.FileID, raw.SizeBytes, raw.ContentType, raw.Version
	}
	result.UpdatedAt, result.DeletedAt = raw.UpdatedAt, raw.DeletedAt
	if result.UpdatedAt.IsZero() {
		result.UpdatedAt = raw.CreatedAt
	}
	if binaryKind(row.Kind) {
		result.Checksum = strings.ToLower(raw.Checksum)
		if result.Checksum != "" && !validHash(result.Checksum) {
			return summary{}, ErrSource
		}
	}
	return result, nil
}

func decodeSummary(version storage.ObjectVersion) (summary, error) {
	var value summary
	if json.Unmarshal(version.Commit.Metadata, &value) != nil || value.Kind != itemKind || !sourceKind(value.SourceKind) || !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: value.SourceKind, ID: value.SourceID}) || !validHash(value.RawSHA256) || value.Epoch != "" && !storage.MigrationEpoch(value.Epoch) || value.BaselineSHA256 != "" && !validHash(value.BaselineSHA256) || value.Checksum != "" && !validHash(value.Checksum) || version.ObjectID != objectID(value.SourceKind, value.SourceID) || value.SizeBytes < 0 || value.SizeBytes > storage.MaxFileBytes {
		return summary{}, ErrSource
	}
	if !value.validLineage() {
		return summary{}, ErrSource
	}
	return value, nil
}

func (value summary) validLineage() bool {
	if value.Epoch == "" {
		return value.BaselineSHA256 == "" && !value.Seed
	}
	return validHash(value.BaselineSHA256) && (!value.Seed || value.BaselineSHA256 == value.RawSHA256)
}
