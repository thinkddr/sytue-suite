// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestStorageBlockOnlyUploadsCiphertextAndNeverExposesUnauthenticatedData(t *testing.T) {
	var saved []byte
	var publicBody []byte
	requests := 0
	alter := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method == http.MethodPut {
			if r.Header.Get("Content-Type") != "application/octet-stream" {
				t.Error("private MIME leaked")
			}
			saved, _ = io.ReadAll(r.Body)
			publicBody = bytes.Clone(saved)
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/cipher/blobs/drive/")
			hash := sha256.Sum256(saved)
			_ = json.NewEncoder(w).Encode(storage.BlobReceipt{ID: id, Product: "drive", SizeBytes: int64(len(saved)), SHA256: hex.EncodeToString(hash[:])})
			return
		}
		data := bytes.Clone(saved)
		if alter {
			data[len(data)-1] ^= 1
		}
		_, _ = w.Write(data)
	}))
	defer server.Close()
	gateway := api.NewDrive(server.URL, nil, server.Client())
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "drive"}
	plain := []byte("private document, filename, index and content must remain on the client")
	block, err := uploadStorageBlock(t.Context(), gateway, scope, "object", "revision", 0, bytes.NewReader(plain), int64(len(plain)))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(publicBody, plain) || bytes.Contains(publicBody, []byte(block.File.Key.Key)) {
		t.Fatal("gateway saw plaintext or a content key")
	}
	got, err := downloadStorageBlock(t.Context(), gateway, scope, block, "object", "revision", 0)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatal("roundtrip", err)
	}
	before := requests
	for _, field := range []string{"tenant", "subject", "product", "object", "revision", "part"} {
		other := scope
		object, revision, part := "object", "revision", 0
		switch field {
		case "tenant":
			other.Tenant = "other"
		case "subject":
			other.Subject = "bob"
		case "product":
			other.Product = "mail"
		case "object":
			object = "other"
		case "revision":
			revision = "other"
		case "part":
			part = 1
		}
		if data, err := downloadStorageBlock(t.Context(), gateway, other, block, object, revision, part); err == nil || len(data) != 0 {
			t.Fatal("block escaped its context", field)
		}
	}
	if requests != before {
		t.Fatal("scope substitution reached gateway")
	}
	alter = true
	if data, err := downloadStorageBlock(t.Context(), gateway, scope, block, "object", "revision", 0); err == nil || len(data) != 0 {
		t.Fatal("viewer received unauthenticated plaintext")
	}
	if _, err := uploadStorageBlock(t.Context(), gateway, scope, "object", "revision", 0, bytes.NewReader(plain), 1); err == nil {
		t.Fatal("source silently truncated")
	}
}

func TestStorageBlockRequiresVerifiedReadyClientBeforeUploading(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	client, err := newClient(storageConfig(t, server, "drive"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.UploadStorageBlock(t.Context(), api.NewDrive(server.URL, nil, server.Client()), "object", "revision", 0, strings.NewReader("private"), 7); err == nil || requests != 0 {
		t.Fatal("unverified client reached gateway", err)
	}
}

type earlyFileTransport func(*http.Request) (*http.Response, error)

func (f earlyFileTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestStorageCipherBufferIsDetachedBeforeAnEarlyTransportCanReadAgain(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "drive"}
	block, data, err := prepareStorageBlock(scope, "object", "revision", 0, strings.NewReader("private"), 7)
	if err != nil {
		t.Fatal(err)
	}
	var body io.ReadCloser
	client := api.NewDrive("https://drive.invalid", nil, &http.Client{Transport: earlyFileTransport(func(r *http.Request) (*http.Response, error) {
		body = r.Body
		response, _ := json.Marshal(block.Blob)
		return &http.Response{StatusCode: http.StatusCreated, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(response)), Request: r}, nil
	})})
	if err := putPreparedStorageBlock(t.Context(), client, block, data); err != nil {
		t.Fatal(err)
	}
	clear(data)
	var later [16]byte
	if n, err := body.Read(later[:]); n != 0 || err != io.EOF {
		t.Fatal("transport retained a reader over a cleared/reusable buffer")
	}
}
