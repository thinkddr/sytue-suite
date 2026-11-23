// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// bovedaConMarca apunta lo que le llega del manejador y contesta `err`.
type bovedaConMarca struct {
	pqVaultService
	err     error
	reset   *service.PQVaultExpect
	copia   string
	claim   service.PQVaultCopyClaim
	retirar string
}

func (b *bovedaConMarca) PutPQVaultKey(ctx context.Context, _ string, _ recoverywrap.PQVaultWrapper, _ bool, reset *service.PQVaultExpect) (service.PQVaultState, error) {
	b.reset, b.copia = reset, store.PQVaultCopyFrom(ctx)
	return service.PQVaultState{}, b.err
}

func (b *bovedaConMarca) AppendPQVaultRecord(ctx context.Context, _, _, _ string, _ json.RawMessage) (service.PQVaultRecord, error) {
	b.copia = store.PQVaultCopyFrom(ctx)
	return service.PQVaultRecord{}, b.err
}

func (b *bovedaConMarca) ClaimPQVaultCopy(_ context.Context, _ string, claim service.PQVaultCopyClaim) (service.PQVaultState, error) {
	b.claim = claim
	return service.PQVaultState{}, b.err
}

func (b *bovedaConMarca) ReleasePQVaultCopy(_ context.Context, _, id string) (service.PQVaultState, error) {
	b.retirar = id
	return service.PQVaultState{}, b.err
}

func pedirBoveda(t *testing.T, handler http.HandlerFunc, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	tn, err := tenancy.New("acme")
	if err != nil {
		t.Fatal(err)
	}
	header, err := storage.EncodeScopeHeader(storage.Scope{Tenant: "acme", Subject: "alice", Product: "drive"})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.Header.Set(storage.ScopeHeader, header)
	r.Header.Set("Content-Type", "application/json")
	rc := chi.NewRouteContext()
	rc.URLParams.Add("product", "drive")
	ctx := context.WithValue(store.WithOwner(tenancy.Into(r.Context(), tn), "alice"), chi.RouteCtxKey, rc)
	w := httptest.NewRecorder()
	handler(w, r.WithContext(ctx))
	return w
}

func codigo(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return body.Error.Code
}

// Vaciar viaja con la expectativa y la marca de quien vacía; sin expectativa (un
// nativo ya instalado) se vacía como antes, sin comparar. Los dos rechazos llegan
// con su código.
func TestPQVaultResetNeedsExpectation(t *testing.T) {
	b := &bovedaConMarca{}
	h := &Handlers{pqVault: b}
	wrapper := `{"v":1}`
	if w := pedirBoveda(t, h.PutPQVaultKey, http.MethodPut, "/api/v1/pq-vault/drive/keys", `{"wrapper":`+wrapper+`,"set_active":true,"reset":true}`); w.Code != http.StatusOK || b.reset == nil || !b.reset.Unchecked {
		t.Fatalf("vaciar sin expectativa (cliente anterior): %d %+v", w.Code, b.reset)
	}
	b.err = domain.ErrPQVaultCopyInProgress
	if w := pedirBoveda(t, h.PutPQVaultKey, http.MethodPut, "/api/v1/pq-vault/drive/keys", `{"wrapper":`+wrapper+`,"set_active":true,"reset":true}`); w.Code != http.StatusConflict || codigo(t, w) != "pq_vault_copy_in_progress" {
		t.Fatalf("vaciar sin expectativa durante una copia ajena: %d", w.Code)
	}
	b.err = nil
	if w := pedirBoveda(t, h.PutPQVaultKey, http.MethodPut, "/api/v1/pq-vault/drive/keys", `{"wrapper":`+wrapper+`,"set_active":true,"expect":{"vault_id":"!pqv_a","version":1}}`); w.Code != http.StatusBadRequest {
		t.Fatalf("expectativa sin vaciar: %d", w.Code)
	}
	w := pedirBoveda(t, h.PutPQVaultKey, http.MethodPut, "/api/v1/pq-vault/drive/keys", `{"wrapper":`+wrapper+`,"set_active":true,"reset":true,"expect":{"vault_id":"!pqv_a","version":7},"copy_id":"marca"}`)
	if w.Code != http.StatusOK || b.reset == nil || *b.reset != (service.PQVaultExpect{VaultID: "!pqv_a", Version: 7}) || b.copia != "marca" {
		t.Fatalf("vaciar con expectativa: %d %+v %q", w.Code, b.reset, b.copia)
	}
	for err, code := range map[error]string{domain.ErrPQVaultChanged: "pq_vault_changed", domain.ErrPQVaultCopyInProgress: "pq_vault_copy_in_progress"} {
		b.err = err
		w := pedirBoveda(t, h.PutPQVaultKey, http.MethodPut, "/api/v1/pq-vault/drive/keys", `{"wrapper":`+wrapper+`,"set_active":true,"reset":true,"expect":{"vault_id":"","version":0}}`)
		if w.Code != http.StatusConflict || codigo(t, w) != code {
			t.Fatalf("%v: %d %s", err, w.Code, w.Body.String())
		}
	}
}

// Publicar lleva la marca de quien copia; poner y retirar la marca validan lo
// que reciben.
func TestPQVaultCopyEndpoints(t *testing.T) {
	b := &bovedaConMarca{}
	h := &Handlers{pqVault: b}
	if w := pedirBoveda(t, h.AppendPQVaultRecord, http.MethodPost, "/api/v1/pq-vault/drive/records", `{"vault_id":"!pqv_a","transaction_id":"t","payload":{},"copy_id":"marca"}`); w.Code != http.StatusCreated || b.copia != "marca" {
		t.Fatalf("publicar con marca: %d %q", w.Code, b.copia)
	}
	b.err = domain.ErrPQVaultCopyInProgress
	if w := pedirBoveda(t, h.AppendPQVaultRecord, http.MethodPost, "/api/v1/pq-vault/drive/records", `{"vault_id":"!pqv_a","transaction_id":"t","payload":{}}`); w.Code != http.StatusConflict || codigo(t, w) != "pq_vault_copy_in_progress" || b.copia != "" {
		t.Fatalf("publicar durante una copia ajena: %d %s", w.Code, w.Body.String())
	}
	b.err = nil
	if w := pedirBoveda(t, h.PutPQVaultCopy, http.MethodPut, "/api/v1/pq-vault/drive/copy", `{"id":"m","holder":"native-linux","kind":"matrix-copy","epoch":"e","ttl_seconds":300}`); w.Code != http.StatusOK || b.claim.TTL != 5*time.Minute || b.claim.Holder != "native-linux" {
		t.Fatalf("poner la marca: %d %+v", w.Code, b.claim)
	}
	for _, ttl := range []string{"0", "86400"} {
		if w := pedirBoveda(t, h.PutPQVaultCopy, http.MethodPut, "/api/v1/pq-vault/drive/copy", `{"id":"m","holder":"h","kind":"matrix-copy","epoch":"e","ttl_seconds":`+ttl+`}`); w.Code != http.StatusBadRequest {
			t.Fatalf("caducidad %s: %d", ttl, w.Code)
		}
	}
	if w := pedirBoveda(t, h.DeletePQVaultCopy, http.MethodDelete, "/api/v1/pq-vault/drive/copy?id=m", ""); w.Code != http.StatusOK || b.retirar != "m" {
		t.Fatalf("retirar: %d %q", w.Code, b.retirar)
	}
	for _, q := range []string{"", "?id=a&id=b", "?id=a&otro=1"} {
		if w := pedirBoveda(t, h.DeletePQVaultCopy, http.MethodDelete, "/api/v1/pq-vault/drive/copy"+q, ""); w.Code != http.StatusBadRequest {
			t.Fatalf("retirar %q: %d", q, w.Code)
		}
	}
}
