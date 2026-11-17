// SPDX-License-Identifier: Apache-2.0
//go:build integration

package pg_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

type queueHTTPAuthority struct {
	mu      sync.Mutex
	fixture *ingressAuthorityFixture
}

func (a *queueHTTPAuthority) ReadStorageAuthority(ctx context.Context, scope storage.Scope) (matrixproof.Authority, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.fixture.ReadStorageAuthority(ctx, scope)
}

func (a *queueHTTPAuthority) VerifyStorageAction(ctx context.Context, action matrixproof.Action, proof matrixproof.SignedAction) (matrixproof.Authorization, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.fixture.VerifyStorageAction(ctx, action, proof)
}

func queueHTTPJSON(t *testing.T, server *httptest.Server, scope storage.Scope, path string, input, output any) {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/v1/private-storage/ingress/"+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	header, err := storage.EncodeScopeHeader(scope)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(storage.ScopeHeader, header)
	request.Header.Set("Authorization", "Bearer synthetic-queue-session")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		t.Fatal("queue HTTP operation failed", path, response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(output); err != nil {
		t.Fatal(err)
	}
}

func queueHTTPProof(t *testing.T, server *httptest.Server, request ingress.ControlRequest, keys *matrixprooftest.Fixture) ingress.ControlProof {
	t.Helper()
	var action matrixproof.Action
	queueHTTPJSON(t, server, request.Scope, "challenge", request, &action)
	proof := matrixproof.SignedAction{Action: action}
	proof.Signatures = matrixprooftest.Sign(t, keys.Device, action, keys.Origin.UserID, keys.Origin.DeviceID.String())
	return ingress.ControlProof{Request: request, Proof: proof}
}

// The native HTTP client runs in its own module, preserving the deliberate
// separation between desktop keyring dependencies and the platform module.
// Only disposable routing and public signed transcripts cross that boundary.
func TestPrivateIngressHTTPClientUsesDurableNonceClaimBodyAndAcknowledgment(t *testing.T) {
	h, cfg, reader, enrollment := newIngressControlFixture(t)
	authority := &queueHTTPAuthority{fixture: reader}
	cfg.Authority = authority
	queue := ingressQueueFixture(h, cfg)
	handlers := httpx.NewPrivateIngressHandlers(httpx.PrivateIngressHTTPConfig{Product: "mail", Port: queue, MaxBodyBytes: 1 << 20, MapError: func(err error) *httpx.Error {
		if shared := httpx.PrivateIngressError(err); shared != nil {
			return shared
		}
		return httpx.ErrInternal
	}, Identity: func(context.Context) (httpx.PrivateSourceIdentity, bool) {
		return httpx.PrivateSourceIdentity{Tenant: cfg.Scope.Tenant, Subject: cfg.Scope.Subject}, true
	}})
	router := chi.NewRouter()
	router.Route("/api/v1", handlers.Mount)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-queue-session" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		router.ServeHTTP(w, r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: cfg.Scope.Tenant})))
	}))
	defer server.Close()
	enrolled := queueHTTPProof(t, server, enrollment, reader.keys)
	var result ingress.ControlResult
	queueHTTPJSON(t, server, cfg.Scope, "apply", enrolled, &result)
	if result.Validate(enrollment, enrolled.Proof.Action) != nil {
		t.Fatal("HTTP public enrollment changed")
	}
	h.seal(t)
	request := ingress.ControlRequest{Version: 1, Scope: cfg.Scope, Epoch: cfg.Epoch.String(), Kind: "task.claim", Target: enrollment.Target, Claim: &ingress.ClaimRequest{TaskID: h.task.ID, Token: "http_claim", LeaseSeconds: 300}}
	proof := queueHTTPProof(t, server, request, reader.keys)
	queueHTTPJSON(t, server, cfg.Scope, "apply", proof, &result)
	if result.Validate(request, proof.Proof.Action) != nil || result.Claim == nil {
		t.Fatal("HTTP signed claim changed")
	}
	claim := *result.Claim
	proof.Claim = &claim
	receipt := ingressReceipt(claim)
	ack := ingress.ControlRequest{Version: 1, Scope: cfg.Scope, Epoch: cfg.Epoch.String(), Kind: "task.ack", Target: claim.Target, Acknowledgment: &ingress.AcknowledgmentRequest{Claim: claim, Receipt: receipt}}
	acknowledged := queueHTTPProof(t, server, ack, reader.keys)
	vector, err := json.Marshal(struct{ Proof, ACK ingress.ControlProof }{proof, acknowledged})
	if err != nil {
		t.Fatal(err)
	}
	pgtest.RunNativeFixture(t.Context(), t, pgtest.NativeFixture{Module: "../../../apps/nucleo", Package: "./api", Test: "TestPrivateIngressRemoteFixture", Marker: "INGRESS_PG_HTTP_NATIVE_PASS", Timeout: 2 * time.Minute, Env: map[string]string{"SYTUE_INGRESS_FIXTURE": "1", "SYTUE_INGRESS_FIXTURE_URL": server.URL, "SYTUE_INGRESS_FIXTURE_VECTOR": string(vector)}})
	page, err := queue.Pending(tenancy.Into(t.Context(), tenancy.Tenant{Slug: cfg.Scope.Tenant}), cfg.Scope, cfg.Epoch.String(), "", 10)
	if err != nil || len(page.Tasks) != 0 {
		t.Fatal("native ACK did not commit in PostgreSQL", err)
	}
	t.Log("INGRESS_REAL_PG_HTTP_NATIVE_PASS retained nonce, exact ciphertext, signed claim and ACK; live SDK publication remains a separate proof")
}
