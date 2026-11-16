// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"unicode/utf8"
)

var ErrUploadMissing = errors.New("storage: upload journal missing; verify the catalog before treating the operation as complete")

const (
	MaxFileBytes         = int64(1) << 47
	MaxFileMetadataBytes = 8 << 10
)

// FileRequest contains PRIVATE metadata. Only the maintained SDK's encrypted
// records carry it; the opaque blob service never receives these fields.
type FileRequest struct {
	ObjectID   string   `json:"object_id"`
	RevisionID string   `json:"revision_id"`
	SizeBytes  int64    `json:"size_bytes"`
	Metadata   []byte   `json:"metadata"`
	Previous   []string `json:"previous,omitempty"`
}

func (r FileRequest) Validate(scope Scope) error {
	if !scope.Valid() || r.SizeBytes < 0 || r.SizeBytes > MaxFileBytes || len(r.Metadata) > MaxFileMetadataBytes || !utf8.Valid(r.Metadata) || !json.Valid(r.Metadata) {
		return ErrRecord
	}
	return (Record{Version: 1, Scope: scope, ObjectID: r.ObjectID, RevisionID: r.RevisionID, Role: "manifest", Payload: []byte(`{}`), Previous: r.Previous}).Validate(scope)
}

// MatchesExact preserves opaque private metadata and original predecessor order.
// It compares application data only; callers still validate scope and SDK readback.
func (request FileRequest) MatchesExact(actual FileRequest) bool {
	return request.ObjectID == actual.ObjectID && request.RevisionID == actual.RevisionID && request.SizeBytes == actual.SizeBytes && bytes.Equal(request.Metadata, actual.Metadata) && slices.Equal(request.Previous, actual.Previous)
}
