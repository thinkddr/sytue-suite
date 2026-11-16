// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var ErrSource = errors.New("office: retained source does not match")

const (
	BinarySourceKind  = "office_bytes"
	MaxSourceRowBytes = 32 << 20
)

// ProductKind binds the three application namespaces to the existing document kinds.
func ProductKind(product string) (Kind, bool) {
	switch product {
	case "docs":
		return KindDocument, true
	case "sheets":
		return KindSpreadsheet, true
	case "slides":
		return KindPresentation, true
	}
	return "", false
}

// SourceKind is the closed historical inventory, including contributor ACLs,
// version history, protections, templates, automation and external data settings.
// Source transport does not decide whether the private application is ready.
var sourceKinds = []string{
	"document", "operation", "snapshot", "version", "permission", "share_link", "comment", "blob", "edit_request", "protection", "protection_key", "filter_view", "template", "template_log", "script", "script_version", "script_run", "brand", "link_policy", "connection", "query", "office_bytes",
}

// SourceKinds exposes the same closed inventory to importer coverage checks.
func SourceKinds() []string { return slices.Clone(sourceKinds) }

func SourceKind(kind string) bool { return slices.Contains(sourceKinds, kind) }

// SourceObjectID derives an opaque identifier from the exact SQL primary-key
// tuple. The tuple is canonicalized in Go with RawMessage, without float64.
func SourceObjectID(kind string, primary map[string]json.RawMessage) (string, error) {
	if !SourceKind(kind) || kind == BinarySourceKind || len(primary) == 0 {
		return "", ErrSource
	}
	if len(SourcePrimaryFields(kind)) != len(primary) {
		return "", ErrSource
	}
	for _, key := range SourcePrimaryFields(kind) {
		if _, ok := primary[key]; !ok {
			return "", ErrSource
		}
	}
	for key, value := range primary {
		if key == "" || !json.Valid(value) || string(value) == "null" {
			return "", ErrSource
		}
	}
	raw, err := json.Marshal(primary)
	if err != nil {
		return "", ErrSource
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte("sytue.office.source.v1\x00"))
	_, _ = digest.Write([]byte(kind))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(raw)
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func ValidateSourceRow(scope storage.Scope, kind, id string, raw []byte) error {
	if !scope.Valid() || !SourceKind(kind) || kind == BinarySourceKind || !storage.Identifier(id) || len(raw) < 2 || len(raw) > MaxSourceRowBytes {
		return ErrSource
	}
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return ErrSource
	}
	primary := make(map[string]json.RawMessage)
	for _, key := range SourcePrimaryFields(kind) {
		primary[key] = value[key]
	}
	wantID, err := SourceObjectID(kind, primary)
	if err != nil || wantID != id {
		return ErrSource
	}
	var tenant string
	if json.Unmarshal(value["tenant"], &tenant) != nil || tenant != scope.Tenant {
		return ErrSource
	}
	if kind == "document" {
		var document Document
		if json.Unmarshal(raw, &document) != nil || document.OwnerSub != scope.Subject {
			return ErrSource
		}
		want, ok := ProductKind(scope.Product)
		if !ok || document.Kind != want {
			return ErrSource
		}
	}
	return nil
}

// SourcePrimaryFields is the shared domain identity contract. SQL capture and
// authorized-device readers use these same tuples; neither invents another ID.
func SourcePrimaryFields(kind string) []string {
	var fields []string
	switch kind {
	case "document", "snapshot", "version", "share_link", "comment", "blob", "template_log", "connection", "query":
		fields = []string{"id"}
	case "operation":
		fields = []string{"document_id", "seq"}
	case "permission":
		fields = []string{"document_id", "principal_kind", "principal_id"}
	case "edit_request":
		fields = []string{"document_id", "sujeto"}
	case "protection", "template":
		fields = []string{"document_id"}
	case "protection_key":
		fields = []string{"document_id", "ambito"}
	case "filter_view":
		fields = []string{"document_id", "sub"}
	case "script", "script_run":
		fields = []string{"tenant", "id"}
	case "script_version":
		fields = []string{"tenant", "script_id", "version"}
	case "brand":
		fields = []string{"tenant", "version"}
	case "link_policy":
		fields = []string{"tenant"}
	}
	return slices.Clone(fields)
}

func BinarySourceID(kind, id string) (string, error) {
	if !SourceKind(kind) || kind == BinarySourceKind || !storage.Identifier(id) {
		return "", ErrSource
	}
	digest := sha256.Sum256([]byte(kind + "\x00" + id))
	return hex.EncodeToString(digest[:]), nil
}
