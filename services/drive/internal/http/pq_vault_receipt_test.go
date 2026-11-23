// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

type reciboFijo struct {
	err   error
	owner string
	slug  string
}

func (r *reciboFijo) VerifyPQVaultReceipt(ctx context.Context, _ storage.Scope, _ storage.PublicationTarget, _ storage.Reference) error {
	r.owner = store.OwnerFrom(ctx)
	if t, err := tenancy.From(ctx); err == nil {
		r.slug = t.Slug
	}
	return r.err
}

func TestReciboBovedaInterno(t *testing.T) {
	cuerpo := `{"scope":{"tenant":"acme","subject":"alice","product":"mail"},` +
		`"target":{"room_id":"!pqv_A","device_id":"pq-x"},"receipt":{"room_id":"!pqv_A","event_id":"$pqe_B"}}`
	pedir := func(svc verificadorReciboBoveda, body string) int {
		w := httptest.NewRecorder()
		reciboBoveda(svc)(w, httptest.NewRequest(http.MethodPost, RutaReciboBoveda, strings.NewReader(body)))
		return w.Code
	}
	bien := &reciboFijo{}
	if got := pedir(bien, cuerpo); got != http.StatusNoContent || bien.owner != "alice" || bien.slug != "acme" {
		t.Fatalf("recibo bueno: %d, como %q en %q", got, bien.owner, bien.slug)
	}
	if got := pedir(&reciboFijo{err: domain.ErrNotFound}, cuerpo); got != http.StatusNotFound {
		t.Fatalf("recibo que no está: %d", got)
	}
	if got := pedir(&reciboFijo{err: errors.New("base caída")}, cuerpo); got != http.StatusServiceUnavailable {
		t.Fatalf("fallo de la base: %d", got)
	}
	if got := pedir(&reciboFijo{}, `{"scope":{}}`); got != http.StatusBadRequest {
		t.Fatalf("cuerpo malo: %d", got)
	}
}
