// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Disposable SDK-produced file vectors, never real credentials or application data.
func TestStorageFileGoWebVector(t *testing.T) {
	directory := os.Getenv("SYTUE_STORAGE_FILE_VECTOR_DIR")
	if directory == "" {
		t.Skip("opt-in disposable file interoperability vector")
	}
	if !filepath.IsAbs(directory) {
		t.Fatal("absolute fixture directory required")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatal("fixture directory must be empty")
	}
	f, gateway, records, _ := fileFixture(t)
	source := patternedSource{size: 16*storage.MaxCipherBlockBytes + 123, pattern: []byte("ARTIFICIAL native file <bytes> ñ 漢字")}
	request := fileRequest(source.size)
	ref, err := f.upload(context.Background(), "web-file-vector", request, source)
	if err != nil {
		t.Fatal(err)
	}
	vector := struct {
		Artificial bool                  `json:"artificial"`
		Scope      storage.Scope         `json:"scope"`
		Reference  EventRef              `json:"reference"`
		Request    StorageFileRequest    `json:"request"`
		Entries    []storage.RecordEntry `json:"entries"`
		Selected   struct {
			Offset int64  `json:"offset"`
			Bytes  []byte `json:"bytes"`
		} `json:"selected"`
	}{Artificial: true, Scope: f.scope, Reference: ref, Request: request}
	vector.Selected.Offset = storage.MaxCipherBlockBytes - 4
	vector.Selected.Bytes = make([]byte, 8)
	if _, err := source.ReadAt(vector.Selected.Bytes, vector.Selected.Offset); err != nil {
		t.Fatal(err)
	}
	for reference, record := range records.records {
		vector.Entries = append(vector.Entries, storage.RecordEntry{Reference: reference, Record: record})
	}
	for id, data := range gateway.data {
		if !storage.BlobID(id) {
			t.Fatal("invalid fixture blob")
		}
		if err := os.WriteFile(filepath.Join(directory, id), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	payload, err := json.Marshal(vector)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "vector.json"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Log("artificial Go file vector: 17 ciphertext blocks and private paged records")
}

func TestStorageFileWebGoVector(t *testing.T) {
	directory := os.Getenv("SYTUE_STORAGE_WEB_FILE_VECTOR_DIR")
	if directory == "" {
		t.Skip("opt-in disposable web file interoperability vector")
	}
	payload, err := os.ReadFile(filepath.Join(directory, "vector.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vector struct {
		Artificial bool                  `json:"artificial"`
		Scope      storage.Scope         `json:"scope"`
		Reference  EventRef              `json:"reference"`
		Request    StorageFileRequest    `json:"request"`
		Entries    []storage.RecordEntry `json:"entries"`
	}
	if json.Unmarshal(payload, &vector) != nil || !vector.Artificial || !vector.Scope.Valid() || vector.Request.SizeBytes != 16*storage.MaxCipherBlockBytes+73 {
		t.Fatal("artificial web vector required")
	}
	f, gateway, records, _ := fileFixture(t)
	f.scope = vector.Scope
	records.records = map[EventRef]storage.Record{}
	for _, entry := range vector.Entries {
		if entry.Record.Validate(vector.Scope) != nil || !entry.Reference.Valid() {
			t.Fatal("invalid private fixture record")
		}
		records.records[entry.Reference] = entry.Record
	}
	gateway.data = map[string][]byte{}
	files, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.Name() == "vector.json" {
			continue
		}
		if !storage.BlobID(file.Name()) {
			t.Fatal("invalid fixture blob filename")
		}
		data, err := os.ReadFile(filepath.Join(directory, file.Name()))
		if err != nil || len(data) > int(storage.MaxCipherBlockBytes) {
			t.Fatal("invalid fixture blob")
		}
		gateway.data[file.Name()] = data
	}
	info, err := f.download(context.Background(), vector.Reference, vector.Request.ObjectID, vector.Request.RevisionID, io.Discard)
	if err != nil || !bytes.Equal(info.Metadata, vector.Request.Metadata) || info.SizeBytes != vector.Request.SizeBytes {
		t.Fatalf("web file failed native validation: %v", err)
	}
	selected, err := f.readRange(context.Background(), vector.Reference, vector.Request.ObjectID, vector.Request.RevisionID, storage.MaxCipherBlockBytes-4, 8)
	if err != nil || !bytes.Equal(selected, []byte{1, 1, 1, 1, 2, 2, 2, 2}) {
		t.Fatalf("cross-block web range: %v", err)
	}
	t.Log("artificial Rust file vector: native full verification and cross-block range passed")
}
