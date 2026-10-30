// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package storage defines application data carried INSIDE maintained Matrix
// encryption. It implements no cipher, key exchange, or plaintext server fallback.
package storage

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

// Leave room for the SDK's Megolm/base64 envelope and homeserver event fields
// inside Matrix's 65,536-byte PDU limit. Large content uses bounded file pages.
const MaxRecordBytes = 40 << 10

// PendingUploadsNamespace identifies resumable file jobs; logout must count it.
const PendingUploadsNamespace = "storage-file-uploads-v1"

var ErrRecord = errors.New("storage: invalid encrypted application record")

func Product(product string) bool {
	switch product {
	case "hub", "mail", "drive", "docs", "sheets", "slides", "pdf", "calendar", "contacts", "notes", "photos", "rooms", "cast", "erp", "bases":
		return true
	default:
		return false
	}
}

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
