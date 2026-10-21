// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Lectura de la traza de auditoría de Sytue Notepads para la vista de auditoría
// unificada de Sytue Admin, que agrega la de todos los servicios.
//
// ACCESO PRIVILEGIADO: la traza dice quién publicó un cuaderno hacia fuera y quién
// destruyó contenido. El listado es owner/admin —`RequireRole` en el montaje— y va
// aislado por organización: `ListAudit` corre bajo `InTenantTx` y además filtra por
// tenant en el WHERE, así que RLS y predicado dicen lo mismo y un admin de A jamás
// ve la traza de B.
package noteshttp

import (
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// ListAudit devuelve las últimas entradas de la organización, de la más reciente
// a la más antigua.
//
// ponytail: sin filtros ni paginación por keyset, al contrario que Drive. Esta
// traza solo recoge cuatro acciones —publicar, despublicar, borrar un cuaderno y
// purgar una nota— así que su volumen es de unas pocas líneas al día y `limit`
// basta. El día que crezca, el camino ya está: una `ListAuditPage` con filtro en
// el store, como la de Sytue ID, en vez de filtrar en memoria aquí.
func (h *Handlers) ListAudit(w http.ResponseWriter, r *http.Request) {
	desde, err := fechaDeQuery(r, "since")
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The 'since' parameter is not a valid date."))
		return
	}
	hasta, err := fechaDeQuery(r, "until")
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The 'until' parameter is not a valid date."))
		return
	}
	entradas, err := h.store.ListAudit(r.Context(), store.FiltroAudit{
		Desde: desde, Hasta: hasta, Limit: limiteDeQuery(r, 100),
	})
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	// Lista vacía y no `null`: quien la pinta itera sin comprobar, y un `null`
	// obliga a que cada consumidor se acuerde de ese caso.
	if entradas == nil {
		entradas = []domain.AuditEntry{}
	}
	// `items` Y NO `entries`: es el sobre que ya devuelven Drive y Mail en esta
	// misma ruta, y el que lee la auditoría unificada de la Consola con UN solo
	// cliente para los cuatro servicios. Divergir aquí obligaba a que ese cliente
	// supiera que Notes es el raro.
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": entradas})
}

// fechaDeQuery lee un parámetro de fecha en RFC3339 o en AAAA-MM-DD, como los
// acepta Drive. Ausente devuelve el cero, que el store entiende como «sin
// acotar»; una fecha ilegible es un error de quien llama y se dice, en vez de
// ignorarla en silencio y devolver un rango que nadie pidió.
func fechaDeQuery(r *http.Request, clave string) (time.Time, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(clave))
	if raw == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", raw)
}
