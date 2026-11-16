// SPDX-License-Identifier: Apache-2.0
package httpx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

type ingressHTTPProbe struct {
	PrivateIngressPort
	page  ingress.PendingPage
	row   storage.StreamSourceRow
	body  *ingressHTTPBody
	calls int
}

func (p *ingressHTTPProbe) Pending(context.Context, storage.Scope, string, string, int) (ingress.PendingPage, error) {
	p.calls++
	return p.page, nil
}

func (p *ingressHTTPProbe) Open(context.Context, storage.Scope, ingress.ControlRequest, matrixproof.SignedAction, ingress.Claim) (storage.StreamSourceRow, io.ReadCloser, error) {
	p.calls++
	return p.row, p.body, nil
}

type ingressHTTPBody struct {
	*bytes.Reader
	reads, closes int
}

func (b *ingressHTTPBody) Read(raw []byte) (int, error) { b.reads++; return b.Reader.Read(raw) }
func (b *ingressHTTPBody) Close() error                 { b.closes++; return nil }

func ingressHTTPRequest(t *testing.T, scope storage.Scope, value any) *http.Request {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/private-storage/ingress", bytes.NewReader(raw))
	header, err := storage.EncodeScopeHeader(scope)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(storage.ScopeHeader, header)
	return request.WithContext(tenancy.Into(t.Context(), tenancy.Tenant{Slug: scope.Tenant}))
}

func TestPrivateIngressEveryEndpointUsesAuthenticatedScopeBeforePort(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	for _, fault := range []string{"anonymous", "delegated", "subject", "tenant", "header", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			identity := PrivateSourceIdentity{Tenant: scope.Tenant, Subject: scope.Subject}
			known := true
			switch fault {
			case "anonymous":
				known = false
			case "delegated":
				identity.Delegado = true
			case "subject":
				identity.Subject = "mallory"
			case "tenant":
				identity.Tenant = "foreign"
			}
			port := &ingressHTTPProbe{}
			handlers := NewPrivateIngressHandlers(PrivateIngressHTTPConfig{Product: "mail", Port: port, MaxBodyBytes: 1024, Identity: func(context.Context) (PrivateSourceIdentity, bool) { return identity, known }})
			for _, handler := range []http.HandlerFunc{handlers.Challenge, handlers.Apply, handlers.Pending, handlers.Revalidate, handlers.Body, handlers.Enrollment} {
				request := ingressHTTPRequest(t, scope, ingress.PendingRequest{Epoch: "11111111-1111-4111-8111-111111111111", Limit: 10})
				if fault == "header" {
					request.Header.Set(storage.ScopeHeader, "invalid")
				}
				if fault == "cancelled" {
					ctx, cancel := context.WithCancel(request.Context())
					cancel()
					request = request.WithContext(ctx)
				}
				response := httptest.NewRecorder()
				handler(response, request)
				if response.Code < 400 || port.calls != 0 || response.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("unauthorized ingress reached domain port", response.Code, port.calls)
				}
			}
		})
	}
}

func TestPrivateIngressPendingRejectsForeignOrUnboundedResponse(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	epoch := "11111111-1111-4111-8111-111111111111"
	for _, fault := range []string{"valid", "subject", "epoch", "cursor"} {
		t.Run(fault, func(t *testing.T) {
			port := &ingressHTTPProbe{page: ingress.PendingPage{Version: 1, Scope: scope, Epoch: epoch, Tasks: []ingress.Task{}}}
			switch fault {
			case "subject":
				port.page.Scope.Subject = "mallory"
			case "epoch":
				port.page.Epoch = "22222222-2222-4222-8222-222222222222"
			case "cursor":
				port.page.Next = "phantom"
			}
			handlers := NewPrivateIngressHandlers(PrivateIngressHTTPConfig{Product: "mail", Port: port, MaxBodyBytes: 1024, Identity: func(context.Context) (PrivateSourceIdentity, bool) {
				return PrivateSourceIdentity{Tenant: scope.Tenant, Subject: scope.Subject}, true
			}})
			response := httptest.NewRecorder()
			handlers.Pending(response, ingressHTTPRequest(t, scope, ingress.PendingRequest{Epoch: epoch, Limit: 10}))
			want := http.StatusInternalServerError
			if fault == "valid" {
				want = http.StatusOK
			}
			if response.Code != want || port.calls != 1 {
				t.Fatal("pending response binding escaped guard", response.Code, port.calls)
			}
		})
	}
}

func TestPrivateIngressChangedCipherDescriptorClosesWithoutReading(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	epoch := "11111111-1111-4111-8111-111111111111"
	target := storage.PublicationTarget{RoomID: "!private:matrix.test", DeviceID: "DEVICE"}
	cipher := []byte("opaque-age-ciphertext")
	sum := sha256.Sum256(cipher)
	task := ingress.Task{Version: 1, Scope: scope, Epoch: epoch, ID: "task_01", Kind: "mail.receive", KeyID: hex.EncodeToString(sum[:]), KeyGeneration: 1, Cipher: storage.StreamSourceRow{Kind: "ingress", ID: "task_01", Size: int64(len(cipher)), SHA256: hex.EncodeToString(sum[:])}}
	claim := ingress.Claim{Version: 1, Task: task, Target: target, Token: "claim_01", Generation: 1, ExpiresAt: time.Now().Add(time.Minute)}
	request := ingress.ControlRequest{Version: 1, Scope: scope, Epoch: epoch, Kind: "task.claim", Target: target, Claim: &ingress.ClaimRequest{TaskID: task.ID, Token: claim.Token, LeaseSeconds: 300}}
	intent, err := matrixproof.IntentFor(scope, 1, epoch, request.Kind, target, request)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	proof := ingress.ControlProof{Request: request, Claim: &claim, Proof: matrixproof.SignedAction{Action: matrixproof.Action{Version: 1, Generation: 1, Scope: scope, Epoch: epoch, Kind: request.Kind, Target: target, RootKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", RequestSHA256: intent.RequestSHA256, Nonce: "nonce_01", IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()}}}
	body := &ingressHTTPBody{Reader: bytes.NewReader(cipher)}
	port := &ingressHTTPProbe{row: task.Cipher, body: body}
	port.row.Size++
	handlers := NewPrivateIngressHandlers(PrivateIngressHTTPConfig{Product: "mail", Port: port, MaxBodyBytes: 1024, Identity: func(context.Context) (PrivateSourceIdentity, bool) {
		return PrivateSourceIdentity{Tenant: scope.Tenant, Subject: scope.Subject}, true
	}})
	response := httptest.NewRecorder()
	handlers.Body(response, ingressHTTPRequest(t, scope, proof))
	if response.Code != http.StatusInternalServerError || port.calls != 1 || body.reads != 0 || body.closes != 1 {
		t.Fatal("changed descriptor exposed body or leaked stream", response.Code, port.calls, body.reads, body.closes)
	}
}
