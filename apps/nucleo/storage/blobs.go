// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package storage

import "strings"

// MaxCipherBlockBytes bounds memory while clients authenticate each media block.
// Large objects use an encrypted manifest of blocks, not an unbounded allocation.
const MaxCipherBlockBytes int64 = 8 << 20

// BlobReceipt contains only public routing/integrity metadata. It never carries
// an attachment key, filename, MIME type or the content's plaintext checksum.
type BlobReceipt struct {
	ID        string `json:"id"`
	Product   string `json:"product"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

func BlobID(id string) bool {
	return len(id) >= 26 && len(id) <= 64 && strings.IndexFunc(id, func(c rune) bool { return (c < 'A' || c > 'Z') && (c < '2' || c > '7') }) < 0
}

func (r BlobReceipt) Valid(product, id string) bool {
	return Product(product) && BlobID(id) && r.Product == product && r.ID == id && r.SizeBytes >= 0 && r.SizeBytes <= MaxCipherBlockBytes && len(r.SHA256) == 64 && strings.IndexFunc(r.SHA256, func(c rune) bool { return (c < '0' || c > '9') && (c < 'a' || c > 'f') }) < 0
}
