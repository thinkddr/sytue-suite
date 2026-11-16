//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/resourcepg"
	docshttp "git.sytue.eu/sytue/one/services/docs/internal/http"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// Actual Docs Main mount + internal-only auth + SDK signature + shared Provider
// + ORIGINAL normal-RLS source SQL. The public SDK registry/directory witness is
// synthetic here; the production Rooms HTTP/Registry proof is separate.
func exerciseOriginalBootstrapHTTP(t testing.TB, ctx context.Context, st *store.Store, want resource.BootstrapSourceSnapshot) {
	t.Helper()
	fixtureSecret := strings.Repeat("a", 64)
	chain := matrixprooftest.NewUser(t, want.OwnerScope, "matrix.test", id.UserID("@sytue_"+want.OwnerScope.Subject+":matrix.test"), "DOCS_SOURCE_QUERY")
	leaf, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	current, registry := resourcetest.EnrolledSession(t, chain, leaf)
	provider, err := resource.NewSessionProvider(registry, registry)
	if err != nil {
		t.Fatal(err)
	}
	unsigned, err := resource.PrepareBootstrapSourceQuery(ctx, current, want.Resource, "!source-query:matrix.test")
	if err != nil {
		t.Fatal(err)
	}
	signed := resource.SignedBootstrapSourceQuery{BootstrapSourceQuery: unsigned, Signatures: matrixprooftest.Sign(t, chain.Device, unsigned, chain.Origin.UserID, chain.Origin.DeviceID.String())}
	verified, err := resource.VerifyBootstrapSourceQuery(ctx, signed, current)
	if err != nil {
		t.Fatal(err)
	}
	handler := docshttp.New(docshttp.Deps{Store: st, ResourceSourceSecret: fixtureSecret, ResourceSourceSessions: provider})
	router := chi.NewRouter()
	handler.Mount(router)
	server := httptest.NewServer(router)
	defer server.Close()
	endpoint, err := resourcepg.NewOriginSourceEndpoint("docs", server.URL, fixtureSecret, server.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	original, err := endpoint.BootstrapSource(verified)
	if err != nil {
		t.Fatal(err)
	}
	got, err := original.BootstrapSource(ctx, want.Resource)
	if err != nil || got != want {
		t.Fatal("HTTP original source changed tenant/owner/seq/freeze", err)
	}
	if err := original.CheckBootstrapSource(ctx, want); err != nil {
		t.Fatal(err)
	}
	changed := want
	changed.ACLVersion = "42"
	if err := original.CheckBootstrapSource(ctx, changed); err == nil {
		t.Fatal("HTTP stale ACL version accepted")
	}
	request := func(body any, secret, xff string) int {
		t.Helper()
		query, ok := body.(resource.SignedBootstrapSourceQuery)
		if !ok {
			t.Fatal("signed query fixture required")
		}
		nonce, err := httpx.FreshMetadataNonce()
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(httpx.ResourceBootstrapSourceRequest{Version: 1, Nonce: nonce, Query: query})
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+httpx.ResourceBootstrapSourcePath, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if secret != "" {
			req.Header.Set("Authorization", "Bearer "+secret)
		}
		req.Header.Set("X-Forwarded-For", xff)
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	for _, test := range []struct {
		name, secret, xff string
		want              int
	}{
		{"missing-credential", "", "", http.StatusUnauthorized},
		{"wrong-credential", "wrong", "", http.StatusUnauthorized},
		{"ingress", fixtureSecret, "203.0.113.1", http.StatusNotFound},
	} {
		if status := request(signed, test.secret, test.xff); status != test.want {
			t.Fatal("internal source boundary failed", test.name, status)
		}
	}
	noSignature := signed
	noSignature.Signatures = nil
	if status := request(noSignature, fixtureSecret, ""); status != http.StatusForbidden {
		t.Fatal("service credential substituted for SDK signature", status)
	}
	foreign := signed
	foreign.OwnerScope.Subject = "not-the-owner"
	if status := request(foreign, fixtureSecret, ""); status != http.StatusForbidden {
		t.Fatal("HTTP caller chose another owner vault", status)
	}
	registry.Retire()
	if status := request(signed, fixtureSecret, ""); status != http.StatusForbidden {
		t.Fatal("historical query revived withdrawn producer", status)
	}
	if _, err = original.BootstrapSource(ctx, want.Resource); err == nil {
		t.Fatal("retired local source intent stayed current")
	}
}
