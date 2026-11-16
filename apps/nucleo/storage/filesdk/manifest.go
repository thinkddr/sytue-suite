// SPDX-License-Identifier: Apache-2.0
package filesdk

import (
	"encoding/hex"

	"maunium.net/go/mautrix/crypto/attachment"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// The same private wire format and bounded tree are used by every SDK adapter.
// Only encrypted records/frames carry these values: descriptors contain keys.
const (
	Fanout       = 16
	Levels       = 6
	MaxBlocks    = 1 << 24
	ManifestKind = "org.sytue.file.v1"
	PageKind     = "org.sytue.file.page.v1"
)

// Descriptor omits context already authenticated by the containing manifest.
// At restores that exact context; callers still validate it before gateway IO.
type Descriptor struct {
	Blob        storage.BlobReceipt      `json:"blob"`
	File        attachment.EncryptedFile `json:"matrix_file"`
	PlainSHA256 string                   `json:"plain_sha256"`
}

func Describe(block Block, plainSHA string) *Descriptor {
	return &Descriptor{Blob: block.Blob, File: block.File, PlainSHA256: plainSHA}
}

func (b Descriptor) At(scope storage.Scope, object, revision string, part int) Block {
	return Block{Scope: scope, ObjectID: object, RevisionID: revision, Part: part, Blob: b.Blob, File: b.File}
}

type Link struct {
	Block *Descriptor        `json:"block,omitempty"`
	Event *storage.Reference `json:"event,omitempty"`
}
type Page struct {
	Kind  string `json:"kind"`
	Level int    `json:"level"`
	Index int    `json:"index"`
	Nodes []Link `json:"nodes"`
}
type Manifest struct {
	Kind    string              `json:"kind"`
	Request storage.FileRequest `json:"request"`
	SHA256  string              `json:"sha256"`
	Root    Page                `json:"root"`
}

// BlockCount returns zero for a size outside the shared file contract.
func BlockCount(size int64) int {
	if size < 0 || size > storage.MaxFileBytes {
		return 0
	}
	if size == 0 {
		return 1
	}
	return int(1 + (size-1)/storage.MaxCipherBlockBytes)
}

func BlockBytes(size int64, index int) int64 {
	if index < 0 || index >= BlockCount(size) {
		return -1
	}
	return min(storage.MaxCipherBlockBytes, max(int64(0), size-int64(index)*storage.MaxCipherBlockBytes))
}

func PageSpan(level int) int {
	if level < 0 || level >= Levels {
		return 0
	}
	span := 1
	for range level + 1 {
		span *= Fanout
	}
	return span
}

func RootLevel(count int) int {
	if count < 1 || count > MaxBlocks {
		return -1
	}
	level := 0
	for count > PageSpan(level) {
		level++
	}
	return level
}

func ValidHash(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 32 && hex.EncodeToString(raw) == value
}

// ValidatePage checks exact positions and fanout, including the final partial
// page. It never accepts caller-supplied shape in place of the file byte count.
func ValidatePage(page Page, request storage.FileRequest, level, index int) error {
	if request.SizeBytes < 0 || request.SizeBytes > storage.MaxFileBytes {
		return storage.ErrRecord
	}
	count := BlockCount(request.SizeBytes)
	if level < 0 || level >= Levels || index < 0 || index > count || page.Kind != PageKind || page.Level != level || page.Index != index {
		return storage.ErrRecord
	}
	span := PageSpan(level)
	if index > (count-1)/span {
		return storage.ErrRecord
	}
	first := index * span
	childSpan := span / Fanout
	wanted := 1 + (min(span, count-first)-1)/childSpan
	if len(page.Nodes) != wanted {
		return storage.ErrRecord
	}
	for _, node := range page.Nodes {
		if level == 0 {
			if node.Block == nil || node.Event != nil || !ValidHash(node.Block.PlainSHA256) {
				return storage.ErrRecord
			}
		} else if node.Block != nil || node.Event == nil || !node.Event.Valid() {
			return storage.ErrRecord
		}
	}
	return nil
}

func (f Manifest) Validate(scope storage.Scope, object, revision string) error {
	if f.Kind != ManifestKind || f.Request.Validate(scope) != nil || f.Request.ObjectID != object || f.Request.RevisionID != revision || !ValidHash(f.SHA256) {
		return storage.ErrRecord
	}
	return ValidatePage(f.Root, f.Request, RootLevel(BlockCount(f.Request.SizeBytes)), 0)
}
