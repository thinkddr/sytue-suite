// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"errors"
	"net/http"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// RutaReciboBoveda es la puerta por la que Sytue Rooms comprueba el recibo de un
// ACK del correo cuando el correo vive en la bóveda (ADR 0087): que la publicación
// es un registro de la bóveda actual de esa persona y que el destino es el de su
// clave activa. Solo desde dentro y con el secreto que Drive y Rooms ya comparten
// (ROOMS_INTERNAL_SECRET). No devuelve nada del registro: sí o no.
const RutaReciboBoveda = "/internal/pq-vault/receipt"

type verificadorReciboBoveda interface {
	VerifyPQVaultReceipt(context.Context, storage.Scope, storage.PublicationTarget, storage.Reference) error
}

// reciboBoveda — POST /internal/pq-vault/receipt {scope, target, receipt} → 204 o 404.
func reciboBoveda(svc verificadorReciboBoveda) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		var req struct {
			Scope   storage.Scope             `json:"scope"`
			Target  storage.PublicationTarget `json:"target"`
			Receipt storage.Reference         `json:"receipt"`
		}
		if httpx.DecodeJSONLimit(w, r, &req, 4096) != nil || !req.Scope.Valid() || !req.Target.Valid() || !req.Receipt.Valid() {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
		tn, err := tenancy.New(req.Scope.Tenant)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
		if svc == nil {
			httpx.Fail(w, r, httpx.ErrUnavailable)
			return
		}
		// La transacción va como el propietario del ámbito: RLS no deja ver otra bóveda.
		ctx := store.WithOwner(tenancy.Into(r.Context(), tn), req.Scope.Subject)
		switch err := svc.VerifyPQVaultReceipt(ctx, req.Scope, req.Target, req.Receipt); {
		case err == nil:
			w.WriteHeader(http.StatusNoContent)
		case errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrInvalidName):
			httpx.Fail(w, r, httpx.ErrNotFound.WithCode("pq_vault_receipt_unknown"))
		default:
			httpx.Fail(w, r, httpx.ErrUnavailable)
		}
	}
}
