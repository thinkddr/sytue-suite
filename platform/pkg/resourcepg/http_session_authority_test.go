// SPDX-License-Identifier: Apache-2.0
package resourcepg_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/resourcepg"
)

// Only the remote SQL/directory witness is artificial; the client, HTTP proof,
// consumed public enrollment and maintained SDK verification are production.
func TestSessionAuthorityClientUsesFreshProofAndExistingGoProvider(t *testing.T) {
	f := resourcetest.New(t, false)
	_, registry := resourcetest.EnrolledSession(t, f.Chain, f.RecipientIdentity)
	secret := strings.Repeat("a", 64)
	var mode atomic.Value
	mode.Store("valid")
	var previous resourcepg.SessionOriginResponse
	var mu sync.Mutex
	nonces := map[string]bool{}
	calls := 0
	server := httptest.NewServer(httpx.SoloDesdeDentro(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q resourcepg.SessionOriginRequest
		if r.URL.Path != "/internal/resources/sessions/docs/current" || httpx.DecodeJSONLimit(w, r, &q, 4096) != nil || !q.ValidForService("docs") {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		calls++
		if nonces[q.Nonce] {
			t.Error("client reused current/check nonce")
		}
		nonces[q.Nonce] = true
		if mode.Load().(string) == "replay" {
			httpx.JSON(w, r, http.StatusOK, previous)
			return
		}
		record, err := registry.CurrentResourceSession(r.Context(), q.Scope, q.DeviceID)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrForbidden)
			return
		}
		if mode.Load().(string) == "forged-enrollment" {
			record.ProofSHA256 = strings.Repeat("f", 64)
		}
		origin := f.Chain.Origin
		if mode.Load().(string) == "forged-SDK" {
			origin.Keys.DeviceKeys = nil
		}
		response, err := resourcepg.SignSessionOriginResponse(secret, q, resourcepg.SessionOriginResponse{Version: 1, Record: record, Origin: origin})
		if err != nil {
			t.Error(err)
			httpx.Fail(w, r, httpx.ErrUnavailable)
			return
		}
		previous = response
		httpx.JSON(w, r, http.StatusOK, response)
	})))
	defer server.Close()
	client, err := resourcepg.NewSessionAuthorityClient("docs", server.URL, secret, server.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := client.Provider()
	if err != nil {
		t.Fatal(err)
	}
	current, err := provider.Resolve(t.Context(), f.Scope, f.Actor.DeviceID)
	if err != nil || current.Binding() != registry.Record.Binding {
		t.Fatal("existing Go Provider did not verify remote SDK record", err)
	}
	if err = current.Revalidate(t.Context()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	count, unique := calls, len(nonces)
	mu.Unlock()
	if count != 4 || unique != count {
		t.Fatal("provider current/directory/check reused evidence", count, unique)
	}
	mode.Store("forged-enrollment")
	if _, err := client.CurrentResourceSessionEvidence(t.Context(), f.Scope, f.Actor.DeviceID); err == nil {
		t.Fatal("MAC-bound raw record bypassed same enrollment verifier")
	}
	mode.Store("replay")
	if err = current.Revalidate(t.Context()); err == nil {
		t.Fatal("cached current proof revived old binding")
	}
	mode.Store("forged-SDK")
	if _, err = provider.Resolve(t.Context(), f.Scope, f.Actor.DeviceID); err == nil {
		t.Fatal("valid metadata MAC replaced SDK chain verification")
	}
	mode.Store("valid")
	if _, err = client.ResourceSessionDevice(t.Context(), f.Scope, f.Actor.DeviceID, f.Chain.Self.PublicKey().String()); err == nil {
		t.Fatal("caller-selected root became pin")
	}
	registry.Retire()
	if err = current.Revalidate(t.Context()); err == nil {
		t.Fatal("retired remote session recovered from response cache")
	}
}
