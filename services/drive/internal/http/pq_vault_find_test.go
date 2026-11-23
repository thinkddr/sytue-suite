// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// bovedaPorTransaccion solo sabe buscar por transaction_id; listar falla.
type bovedaPorTransaccion struct {
	pqVaultService
	registros map[string]service.PQVaultRecord
	pedido    string
}

func (b *bovedaPorTransaccion) FindPQVaultRecord(_ context.Context, _, _, tx string) (service.PQVaultRecord, error) {
	b.pedido = tx
	if r, ok := b.registros[tx]; ok {
		return r, nil
	}
	return service.PQVaultRecord{}, domain.ErrNotFound
}

func (b *bovedaPorTransaccion) ListPQVaultRecords(context.Context, string, string, int64, int) (service.PQVaultRecordPage, error) {
	panic("con transaction_id no se lista")
}

func pedirPorTransaccion(t *testing.T, h *Handlers, query string) (*httptest.ResponseRecorder, service.PQVaultRecordPage) {
	t.Helper()
	tn, err := tenancy.New("acme")
	if err != nil {
		t.Fatal(err)
	}
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	header, err := storage.EncodeScopeHeader(scope)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/pq-vault/mail/records?"+query, http.NoBody)
	r.Header.Set(storage.ScopeHeader, header)
	rc := chi.NewRouteContext()
	rc.URLParams.Add("product", "mail")
	ctx := context.WithValue(store.WithOwner(tenancy.Into(r.Context(), tn), "alice"), chi.RouteCtxKey, rc)
	w := httptest.NewRecorder()
	h.ListPQVaultRecords(w, r.WithContext(ctx))
	var page service.PQVaultRecordPage
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
	}
	return w, page
}

func TestRegistroDeBovedaPorTransaccion(t *testing.T) {
	b := &bovedaPorTransaccion{registros: map[string]service.PQVaultRecord{
		"ingress_secret_x": {EventID: "$pqe_AAAAAAAAAAAAAAAAAAAAAAAAAA", VaultID: "!pqv_a"},
	}}
	h := &Handlers{pqVault: b}
	w, page := pedirPorTransaccion(t, h, "vault_id=!pqv_a&transaction_id=ingress_secret_x")
	if w.Code != http.StatusOK || len(page.Entries) != 1 || page.Entries[0].EventID != "$pqe_AAAAAAAAAAAAAAAAAAAAAAAAAA" || b.pedido != "ingress_secret_x" {
		t.Fatalf("encontrado: %d %+v (%q)", w.Code, page, b.pedido)
	}
	w, page = pedirPorTransaccion(t, h, "vault_id=!pqv_a&transaction_id=otro")
	if w.Code != http.StatusOK || len(page.Entries) != 0 || page.NextCursor != "" {
		t.Fatalf("no encontrado: %d %+v", w.Code, page)
	}
	if w, _ := pedirPorTransaccion(t, h, "vault_id=!pqv_a&transaction_id=x&limit=5"); w.Code != http.StatusBadRequest {
		t.Fatalf("transaction_id con paginación: %d", w.Code)
	}
}
