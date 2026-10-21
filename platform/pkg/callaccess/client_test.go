// SPDX-License-Identifier: Apache-2.0
package callaccess

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestInternalDocumentCallContractKeepsCredentialInHeader(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer dedicated-secret" || r.URL.RawQuery != "" {
			t.Fatal("credential was missing or put in URL")
		}
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"version":42}`))
			return
		}
		var fields map[string]any
		if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
			t.Fatal(err)
		}
		if len(fields) != 3 || fields["document_id"] != "opaque-document" || fields["tenant"] != "acme" || fields["version"] != float64(43) {
			t.Fatalf("unexpected private content: %+v", fields)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := New(server.URL, "dedicated-secret")
	version, err := client.MinimumDocumentVersion(context.Background(), "opaque-document")
	if err != nil || version != 42 {
		t.Fatalf("fence lookup: %d %v", version, err)
	}
	if err := client.RevokeDocumentCalls(context.Background(), "opaque-document", "acme", 43); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatal("unexpected requests")
	}
}

func TestRevocationDoesNotFollowRedirectOrExposeErrorBody(t *testing.T) {
	var leaked atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { leaked.Store(true) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.URL)
		http.Error(w, "dedicated-secret private provider body", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	err := New(server.URL, "dedicated-secret").RevokeDocumentCalls(context.Background(), "id", "acme", 1)
	if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "secret") || leaked.Load() {
		t.Fatalf("unsafe redirect/error: %v leaked=%v", err, leaked.Load())
	}
	for _, origin := range []string{"https://user:password@rooms.example", "https://rooms.example/?secret=x", "https://rooms.example/path"} {
		if New(origin, "secret") != nil {
			t.Fatalf("invalid internal origin accepted: %s", origin)
		}
	}
}
