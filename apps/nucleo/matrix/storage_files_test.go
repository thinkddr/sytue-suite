// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package matrix

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type patternedSource struct {
	size    int64
	pattern []byte
}

func (s patternedSource) ReadAt(out []byte, offset int64) (int, error) {
	if offset >= s.size {
		return 0, io.EOF
	}
	wanted := int(min(int64(len(out)), s.size-offset))
	n := 0
	for n < wanted {
		start := int((offset + int64(n)) % int64(len(s.pattern)))
		n += copy(out[n:wanted], s.pattern[start:])
	}
	if n != len(out) {
		return n, io.EOF
	}
	return n, nil
}

type fileRecordFixture struct {
	records  map[EventRef]storage.Record
	txns     map[string]EventRef
	failRoot bool
	losePage bool
}

func (m *fileRecordFixture) PutStorageRecord(_ context.Context, txn string, rec storage.Record) (EventRef, error) {
	if m.records == nil {
		m.records = map[EventRef]storage.Record{}
		m.txns = map[string]EventRef{}
	}
	if ref, ok := m.txns[txn]; ok {
		if !bytes.Equal(m.records[ref].Payload, rec.Payload) {
			return EventRef{}, errors.New("transaction payload changed")
		}
		return ref, nil
	}
	var payload struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(rec.Payload, &payload)
	if m.failRoot && payload.Kind == fileKind {
		return EventRef{}, errors.New("interrupted root write")
	}
	ref := EventRef{RoomID: "!private:matrix.test", EventID: "$event" + strconv.Itoa(len(m.records)+1)}
	rec.Payload = bytes.Clone(rec.Payload)
	rec.Previous = slices.Clone(rec.Previous)
	m.records[ref] = rec
	m.txns[txn] = ref
	if m.losePage && payload.Kind == filePageKind {
		m.losePage = false
		return ref, errors.New("lost page receipt")
	}
	return ref, nil
}

func (m *fileRecordFixture) ReadStorageRecord(_ context.Context, ref EventRef, _, _ string) (storage.Record, error) {
	if rec, ok := m.records[ref]; ok {
		return rec, nil
	}
	return storage.Record{}, errors.New("missing private event")
}

type fileGatewayFixture struct {
	mu       sync.Mutex
	data     map[string][]byte
	puts     int
	gets     int
	loseNext bool
	tamper   bool
}

func (s *fileGatewayFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/cipher/blobs/drive/"), "/content")
	if r.Method == http.MethodPut {
		s.puts++
		data, _ := io.ReadAll(r.Body)
		if previous, exists := s.data[id]; exists && !bytes.Equal(previous, data) {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if s.data == nil {
			s.data = map[string][]byte{}
		}
		s.data[id] = data
		if s.loseNext {
			s.loseNext = false
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		sum := sha256.Sum256(data)
		_ = json.NewEncoder(w).Encode(storage.BlobReceipt{ID: id, Product: "drive", SizeBytes: int64(len(data)), SHA256: hex.EncodeToString(sum[:])})
		return
	}
	s.gets++
	data, ok := s.data[id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if s.tamper && len(data) > 0 {
		data = bytes.Clone(data)
		data[0] ^= 1
	}
	_, _ = w.Write(data)
}

func fileFixture(t *testing.T) (storageFileIO, *fileGatewayFixture, *fileRecordFixture, string) {
	t.Helper()
	gateway := &fileGatewayFixture{}
	server := httptest.NewServer(gateway)
	t.Cleanup(server.Close)
	directory := t.TempDir()
	journal, err := offline.OpenWithSecrets(directory, &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	records := &fileRecordFixture{}
	return storageFileIO{scope: storage.Scope{Tenant: "acme", Subject: "alice", Product: "drive"}, gateway: api.NewDrive(server.URL, nil, server.Client()), records: records, journal: journal}, gateway, records, directory
}

func fileRequest(size int64) StorageFileRequest {
	return StorageFileRequest{ObjectID: "object", RevisionID: "revision", SizeBytes: size, Metadata: []byte(`{"name":"private name.pdf","mime":"application/pdf","integer":9007199254740993}`)}
}

func TestStorageFilePagedRoundtripHasBoundedManifestsAndOpaqueProvider(t *testing.T) {
	f, gateway, records, _ := fileFixture(t)
	records.losePage = true
	source := patternedSource{size: storage.MaxCipherBlockBytes*fileFanout + 123, pattern: bytes.Repeat([]byte("private content belongs only on authorized devices;"), 128)}
	request := fileRequest(source.size)
	if _, err := f.upload(t.Context(), "upload", request, source); err == nil {
		t.Fatal("lost page receipt accepted")
	}
	ref, err := f.upload(t.Context(), "upload", request, source)
	if err != nil {
		t.Fatal(err)
	}
	if gateway.puts != fileFanout+1 {
		t.Fatal("resuming a page receipt reuploaded completed blocks", gateway.puts)
	}
	if len(records.records) != 3 {
		t.Fatal("expected two leaf pages and an inline parent root", len(records.records))
	}
	got, err := f.download(t.Context(), ref, request.ObjectID, request.RevisionID, io.Discard)
	if err != nil || !bytes.Equal(got.Metadata, request.Metadata) {
		t.Fatal("large file or exact metadata did not roundtrip", err)
	}
	if count, err := f.journal.Count(storage.PendingUploadsNamespace); err != nil || count != 0 {
		t.Fatal("verified upload left pending work", count, err)
	}
	for id, data := range gateway.data {
		if !storage.BlobID(id) || bytes.Contains(data, []byte("private content belongs")) {
			t.Fatal("provider received identifying/plaintext data")
		}
	}
	for _, record := range records.records {
		if record.Validate(f.scope) != nil {
			t.Fatal("manifest exceeds SDK record bound")
		}
	}
	beforeRange := gateway.gets
	rangeBytes, err := f.readRange(t.Context(), ref, request.ObjectID, request.RevisionID, storage.MaxCipherBlockBytes-3, 10)
	var expected [10]byte
	_, _ = source.ReadAt(expected[:], storage.MaxCipherBlockBytes-3)
	if err != nil || !bytes.Equal(rangeBytes, expected[:]) || gateway.gets-beforeRange != 2 {
		t.Fatal("range did not authenticate just its two blocks", err)
	}
	gateway.tamper = true
	if _, err := f.download(t.Context(), ref, request.ObjectID, request.RevisionID, io.Discard); err == nil {
		t.Fatal("corrupt ciphertext was accepted")
	}
	before := gateway.gets
	if _, err := f.download(t.Context(), ref, request.ObjectID, "other-revision", io.Discard); err == nil || gateway.gets != before {
		t.Fatal("revision substitution reached ciphertext")
	}
}

func TestStorageFileLostReceiptResumesExactlyAndChangedInputNeverReusesIV(t *testing.T) {
	f, gateway, records, directory := fileFixture(t)
	source := []byte("a private original whose filename and content must stay encrypted")
	request := fileRequest(int64(len(source)))
	gateway.loseNext = true
	if _, err := f.upload(t.Context(), "retry", request, bytes.NewReader(source)); err == nil {
		t.Fatal("lost provider reply was accepted")
	}
	var pending storageFileUpload
	found, err := f.journal.Read(f.journalOwner(), storage.PendingUploadsNamespace, "retry", &pending)
	if !found || err != nil || pending.Pending == nil {
		t.Fatal("pending key not saved before upload", err)
	}
	before := gateway.puts
	changed := bytes.Clone(source)
	changed[0] ^= 1
	if _, err := f.upload(t.Context(), "retry", request, bytes.NewReader(changed)); !errors.Is(err, ErrStorageSourceChanged) || gateway.puts != before {
		t.Fatal("changed plaintext reused the pending key/IV", err)
	}
	key := pending.Pending.Block.File.Key.Key
	if err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(data, []byte(key)) || bytes.Contains(data, source) {
			return errors.New("private journal persisted readable material")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ref, err := f.upload(t.Context(), "retry", request, bytes.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	if len(gateway.data) != 1 || gateway.puts != 2 || len(records.records) != 1 {
		t.Fatal("retry created another ciphertext object")
	}
	var out bytes.Buffer
	if _, err := f.download(t.Context(), ref, request.ObjectID, request.RevisionID, &out); err != nil || !bytes.Equal(out.Bytes(), source) {
		t.Fatal("resumed source corrupted", err)
	}
}

func TestStorageFilePreparedResumeChecksSourceAndKeepsOriginal(t *testing.T) {
	f, gateway, records, _ := fileFixture(t)
	source := []byte("stable source")
	request := fileRequest(int64(len(source)))
	records.failRoot = true
	if _, err := f.upload(t.Context(), "prepared", request, bytes.NewReader(source)); err == nil {
		t.Fatal("missing root receipt accepted")
	}
	before := gateway.puts
	changed := bytes.Clone(source)
	changed[0] ^= 1
	records.failRoot = false
	if _, err := f.upload(t.Context(), "prepared", request, bytes.NewReader(changed)); !errors.Is(err, ErrStorageSourceChanged) || gateway.puts != before {
		t.Fatal("prepared resume accepted a changed original", err)
	}
	if _, err := f.upload(t.Context(), "prepared", request, bytes.NewReader(source)); err != nil {
		t.Fatal(err)
	}
	if string(source) != "stable source" {
		t.Fatal("uploader modified original")
	}
}

func TestStorageFileEmptyAndInvalidRootsFailClosed(t *testing.T) {
	f, gateway, records, _ := fileFixture(t)
	request := fileRequest(0)
	ref, err := f.upload(t.Context(), "empty", request, bytes.NewReader(nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.download(t.Context(), ref, request.ObjectID, request.RevisionID, io.Discard); err != nil {
		t.Fatal(err)
	}
	original := records.records[ref]
	for _, mutation := range []string{"depth", "position", "nodes", "scope", "size"} {
		rec := original
		var file storageFile
		if err := json.Unmarshal(rec.Payload, &file); err != nil {
			t.Fatal(err)
		}
		switch mutation {
		case "depth":
			file.Root.Level = 99
		case "position":
			file.Root.Index = 1
		case "nodes":
			file.Root.Nodes = nil
		case "scope":
			rec.Scope.Subject = "bob"
		case "size":
			file.Request.SizeBytes = MaxStorageFileBytes + 1
		}
		rec.Payload, _ = json.Marshal(file)
		records.records[ref] = rec
		before := gateway.gets
		if _, err := f.download(t.Context(), ref, request.ObjectID, request.RevisionID, io.Discard); err == nil || gateway.gets != before {
			t.Fatal("invalid root reached content", mutation)
		}
	}
	records.records[ref] = original
	f.journal = nil
	if _, err := f.upload(t.Context(), "unsafe", request, bytes.NewReader(nil)); err == nil {
		t.Fatal("file keys accepted without encrypted journal")
	}
}

func TestStorageFileManifestFitsRecordAtMaximumMetadataAndIdentifiers(t *testing.T) {
	scope := storage.Scope{Tenant: strings.Repeat("a", 63), Subject: strings.Repeat("b", 255), Product: "calendar"}
	request := StorageFileRequest{ObjectID: strings.Repeat("o", 128), RevisionID: strings.Repeat("r", 128), SizeBytes: storage.MaxCipherBlockBytes * fileFanout, Metadata: []byte(`"` + strings.Repeat("m", maxFileMetadataBytes-2) + `"`)}
	root := storageFilePage{Kind: filePageKind}
	for i := range fileFanout {
		block, data, err := prepareStorageBlock(scope, request.ObjectID, request.RevisionID, i, bytes.NewReader([]byte("x")), 1)
		clear(data)
		if err != nil {
			t.Fatal(err)
		}
		// Descriptor width is independent of media bytes. Exercise maximum routing
		// ID/size widths without allocating sixteen maximum-size media blocks.
		block.Blob.ID = strings.Repeat(string(rune('A'+i)), 64)
		block.Blob.SizeBytes = storage.MaxCipherBlockBytes
		root.Nodes = append(root.Nodes, storageFileLink{Block: fileDescriptor(block, digestBytes([]byte("x")))})
		request.Previous = append(request.Previous, "$"+string(rune('a'+i))+strings.Repeat("p", 253))
	}
	payload, err := json.Marshal(storageFile{Kind: fileKind, Request: request, SHA256: strings.Repeat("a", 64), Root: root})
	if err != nil {
		t.Fatal(err)
	}
	record := storage.Record{Version: 1, Scope: scope, ObjectID: request.ObjectID, RevisionID: request.RevisionID, Role: "manifest", Previous: request.Previous, Payload: payload}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := record.Validate(scope); err != nil {
		t.Fatal("legitimate maximum fields exceed private-record bound", len(encoded), err)
	}
	t.Log("maximum manifest record bytes", len(encoded))
}
