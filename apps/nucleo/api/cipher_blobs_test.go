// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestCipherRequestsPinOriginalScope(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "original-owner", Product: "mail"}
	id := strings.Repeat("A", 26)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		got, err := storage.DecodeScopeHeader(r.Header.Get(storage.ScopeHeader))
		if err != nil || got != scope || r.URL.Path != "/api/v1/cipher/blobs/mail/"+id && r.URL.Path != "/api/v1/cipher/blobs/mail/"+id+"/content" {
			t.Error("request lost original owner scope", err)
		}
		switch {
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/content"):
			_, _ = io.WriteString(w, "opaque")
		default:
			_ = json.NewEncoder(w).Encode(storage.BlobReceipt{ID: id, Product: "mail", SizeBytes: 6, SHA256: strings.Repeat("a", 64)})
		}
	}))
	defer server.Close()
	client := NewDrive(server.URL, &sesionFalsa{tokens: []string{"artificial"}}, server.Client())
	if _, err := client.PutCipherBlob(t.Context(), scope, id, strings.NewReader("opaque"), 6); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CipherBlob(t.Context(), scope, id); err != nil {
		t.Fatal(err)
	}
	body, err := client.ReadCipherBlob(t.Context(), scope, id)
	if err != nil {
		t.Fatal(err)
	}
	_ = body.Close()
	if err := client.DeleteCipherBlob(t.Context(), scope, id); err != nil {
		t.Fatal(err)
	}
	invalid := scope
	invalid.Subject = ""
	if _, err := client.PutCipherBlob(t.Context(), invalid, id, strings.NewReader("opaque"), 6); err == nil {
		t.Fatal("invalid scope uploaded")
	}
	if _, err := client.CipherBlob(t.Context(), invalid, id); err == nil {
		t.Fatal("invalid scope read")
	}
	if _, err := client.ReadCipherBlob(t.Context(), invalid, id); err == nil {
		t.Fatal("invalid scope downloaded")
	}
	if err := client.DeleteCipherBlob(t.Context(), invalid, id); err == nil {
		t.Fatal("invalid scope deleted")
	}
	if calls != 4 {
		t.Fatal("invalid scope reached network", calls)
	}
}
