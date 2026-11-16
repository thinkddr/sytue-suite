// SPDX-License-Identifier: Apache-2.0
package filesdk

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestManifestWireRemainsCompatibleWithExistingPrivateFiles(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "drive"}
	// Static legacy wire, not re-encoded through the new struct for comparison.
	golden := []byte(`{"kind":"org.sytue.file.v1","request":{"object_id":"object","revision_id":"revision","size_bytes":134217729,"metadata":"e30="},"sha256":"` + strings.Repeat("a", 64) + `","root":{"kind":"org.sytue.file.page.v1","level":1,"index":0,"nodes":[{"event":{"room_id":"!file:example.test","event_id":"$first"}},{"event":{"room_id":"!file:example.test","event_id":"$last"}}]}}`)
	var file Manifest
	if err := json.Unmarshal(golden, &file); err != nil {
		t.Fatal(err)
	}
	if err := file.Validate(scope, "object", "revision"); err != nil {
		t.Fatal("legacy manifest denied", err)
	}
	wire, err := json.Marshal(file)
	if err != nil || !bytes.Equal(wire, golden) {
		t.Fatal("private file wire changed", err)
	}
	for _, change := range []string{"object", "revision", "metadata", "hash", "kind", "scope", "root-level", "missing-page", "branch-as-leaf", "foreign-reference"} {
		t.Run(change, func(t *testing.T) {
			var mutated Manifest
			if err := json.Unmarshal(golden, &mutated); err != nil {
				t.Fatal(err)
			}
			expected := scope
			switch change {
			case "object":
				mutated.Request.ObjectID = "other"
			case "revision":
				mutated.Request.RevisionID = "other"
			case "metadata":
				mutated.Request.Metadata = []byte("not-json")
			case "hash":
				mutated.SHA256 = strings.Repeat("A", 64)
			case "kind":
				mutated.Kind = "other"
			case "scope":
				expected.Product = "invalid"
			case "root-level":
				mutated.Root.Level = 0
			case "missing-page":
				mutated.Root.Nodes = mutated.Root.Nodes[:1]
			case "branch-as-leaf":
				mutated.Root.Nodes[0].Block = &Descriptor{PlainSHA256: strings.Repeat("a", 64)}
			case "foreign-reference":
				mutated.Root.Nodes[0].Event.RoomID = "invalid"
			}
			if mutated.Validate(expected, "object", "revision") == nil {
				t.Fatal("changed manifest accepted", change)
			}
		})
	}
}

func TestManifestUsesExactPartialPagesAndFullSharedFileLimit(t *testing.T) {
	request := storage.FileRequest{SizeBytes: storage.MaxCipherBlockBytes*16 + 1}
	hash := strings.Repeat("a", 64)
	last := Page{Kind: PageKind, Level: 0, Index: 1, Nodes: []Link{{Block: &Descriptor{PlainSHA256: hash}}}}
	if ValidatePage(last, request, 0, 1) != nil {
		t.Fatal("last partial leaf denied")
	}
	for _, change := range []string{"extra", "wrong-position", "missing", "event-in-leaf", "bad-hash"} {
		clone := Page{Kind: PageKind, Level: 0, Index: 1, Nodes: []Link{{Block: &Descriptor{PlainSHA256: hash}}}}
		switch change {
		case "extra":
			clone.Nodes = append(clone.Nodes, clone.Nodes[0])
		case "wrong-position":
			clone.Index = 0
		case "missing":
			clone.Nodes = nil
		case "event-in-leaf":
			ref := storage.Reference{RoomID: "!file:example.test", EventID: "$extra"}
			clone.Nodes[0].Event = &ref
		case "bad-hash":
			clone.Nodes[0].Block.PlainSHA256 = "bad"
		}
		if ValidatePage(clone, request, 0, 1) == nil {
			t.Fatal("partial page corruption accepted", change)
		}
	}
	// Validate the 128 TiB contract using only its bounded root, not an allocation.
	request.SizeBytes = storage.MaxFileBytes
	root := Page{Kind: PageKind, Level: 5}
	for range 16 {
		ref := storage.Reference{RoomID: "!file:example.test", EventID: "$page"}
		root.Nodes = append(root.Nodes, Link{Event: &ref})
	}
	if BlockCount(request.SizeBytes) != 1<<24 || RootLevel(BlockCount(request.SizeBytes)) != 5 || ValidatePage(root, request, 5, 0) != nil {
		t.Fatal("shared maximum file contract changed")
	}
	for _, bad := range []int64{-1, storage.MaxFileBytes + 1} {
		request.SizeBytes = bad
		if ValidatePage(root, request, 5, 0) == nil || BlockCount(bad) != 0 || BlockBytes(bad, 0) != -1 {
			t.Fatal("invalid size accepted")
		}
	}
	if RootLevel(0) != -1 || RootLevel(MaxBlocks+1) != -1 || PageSpan(-1) != 0 || PageSpan(Levels) != 0 {
		t.Fatal("invalid geometry did not fail closed")
	}
	if BlockCount(0) != 1 || BlockBytes(0, 0) != 0 || BlockBytes(0, 1) != -1 {
		t.Fatal("empty file changed")
	}
}

func TestManifestBlockDescriptorRetainsMaintainedSDKWireForEveryProduct(t *testing.T) {
	plain := []byte("artificial private file content across all apps")
	sum := sha256.Sum256(plain)
	hash := hex.EncodeToString(sum[:])
	for _, product := range storage.Products() {
		t.Run(product, func(t *testing.T) {
			scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: product}
			block, cipher, err := PrepareBlock(scope, "object", "revision", 0, bytes.NewReader(plain), int64(len(plain)))
			if err != nil {
				t.Fatal(err)
			}
			defer clear(cipher)
			manifest := Manifest{Kind: ManifestKind, Request: storage.FileRequest{ObjectID: "object", RevisionID: "revision", SizeBytes: int64(len(plain)), Metadata: []byte("{}")}, SHA256: hash, Root: Page{Kind: PageKind, Nodes: []Link{{Block: Describe(block, hash)}}}}
			raw, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			defer clear(raw)
			var decoded Manifest
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatal(err)
			}
			if err := decoded.Validate(scope, "object", "revision"); err != nil {
				t.Fatal(err)
			}
			restored := decoded.Root.Nodes[0].Block.At(scope, "object", "revision", 0)
			if !reflect.DeepEqual(restored.File, block.File) || restored.Blob != block.Blob {
				t.Fatal("descriptor lost maintained SDK fields")
			}
			if err := DecryptBlockInPlace(scope, "object", "revision", 0, restored, cipher); err != nil || !bytes.Equal(cipher, plain) {
				t.Fatal("shared manifest could not restore SDK file", err)
			}
		})
	}
}
