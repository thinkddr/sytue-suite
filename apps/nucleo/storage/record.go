// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package storage defines application data carried INSIDE maintained Matrix
// encryption. It implements no cipher, key exchange, or plaintext server fallback.
package storage

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"
)

// Leave room for the SDK's Megolm/base64 envelope and homeserver event fields
// inside Matrix's 65,536-byte PDU limit. Large content uses bounded file pages.
const MaxRecordBytes = 40 << 10

// PendingUploadsNamespace identifies resumable file jobs; logout must count it.
const PendingUploadsNamespace = "storage-file-uploads-v1"

var ErrRecord = errors.New("storage: invalid encrypted application record")

//go:embed products.txt
var productCatalog string

var productIDs = strings.Fields(productCatalog)

func Product(product string) bool { return slices.Contains(productIDs, product) }

// Products is the shared routing registry for the private storage core.
// Callers receive a copy and cannot change accepted application namespaces.
func Products() []string { return slices.Clone(productIDs) }

// NativeApp uses the existing native URL catalog: the Hub app is named One.
// Unknown products cannot become custom URL schemes supplied by a caller.
func NativeApp(product string) string {
	if !Product(product) {
		return ""
	}
	if product == "hub" {
		return "one"
	}
	return product
}

type Scope struct {
	Tenant  string `json:"tenant"`
	Subject string `json:"subject"`
	Product string `json:"product"`
}

// Valid excludes identities that cannot roundtrip through JSON or the scoped
// journal tuple. Content codecs must never silently normalize a different owner.
func (s Scope) Valid() bool {
	return s.Tenant != "" && len(s.Tenant) <= 63 && s.Subject != "" && len(s.Subject) <= 255 && Product(s.Product) && utf8.ValidString(s.Tenant) && utf8.ValidString(s.Subject) && !strings.ContainsAny(s.Tenant+s.Subject, "\x00\r\n\t")
}

// JournalOwner is the exact account/product tuple used by the shared private
// work inventory. A Subject alone must never name a private application's WAL.
func (s Scope) JournalOwner() (string, error) {
	if !s.Valid() {
		return "", ErrRecord
	}
	return s.Tenant + "\x00" + s.Subject + "\x00" + s.Product, nil
}

// Record is private application data, not Matrix state. Names, MIME types,
// indexes, task arguments and file-key descriptors belong in Payload, never in
// public routing metadata. Previous preserves concurrent revision branches;
// callers must resolve conflicts rather than use device clocks as last-write-wins.
type Record struct {
	Version    int      `json:"v"`
	Scope      Scope    `json:"scope"`
	ObjectID   string   `json:"object_id"`
	RevisionID string   `json:"revision_id"`
	Role       string   `json:"role"`
	Previous   []string `json:"previous,omitempty"`
	// JSON bytes are encoded as base64 inside the encrypted event so a generic
	// SDK/JavaScript parser cannot round domain numbers before the app sees them.
	Payload []byte `json:"payload"`
}

func Identifier(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for _, c := range value {
		switch {
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-':
		default:
			return false
		}
	}
	return true
}

func (r Record) Validate(scope Scope) error {
	if r.Version != 1 || r.Scope != scope || !scope.Valid() || !Identifier(r.ObjectID) || !Identifier(r.RevisionID) || !utf8.Valid(r.Payload) || !json.Valid(r.Payload) || len(r.Previous) > 16 {
		return ErrRecord
	}
	switch r.Role {
	case "content", "manifest", "preview", "thumbnail", "revision", "trash", "index", "task":
	default:
		return ErrRecord
	}
	seen := map[string]bool{}
	for _, prev := range r.Previous {
		if !matrixID(prev, '$') || seen[prev] {
			return ErrRecord
		}
		seen[prev] = true
	}
	raw, err := json.Marshal(r)
	if err != nil || len(raw) > MaxRecordBytes {
		return ErrRecord
	}
	return nil
}

// MatchesExact compares two valid private application records without parsing
// or normalizing Payload. It is a logical readback check, not SDK provenance
// or authority to publish. The caller must obtain actual through its scoped SDK.
func (r Record) MatchesExact(actual Record) bool {
	return r.Validate(r.Scope) == nil && actual.Validate(r.Scope) == nil && actual.ObjectID == r.ObjectID && actual.RevisionID == r.RevisionID && actual.Role == r.Role && bytes.Equal(actual.Payload, r.Payload) && slices.Equal(actual.Previous, r.Previous)
}
