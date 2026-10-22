// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"net/http"
	"strconv"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// auditoriaStore es lo que esto necesita del store: se declara aparte del contrato
// documental (domain.Store) como connectorStore y peticionesStore.
type auditoriaStore interface {
	AuditoriaOps(ctx context.Context, docID string, antesDe int64, ventana int) (domain.PaginaAuditoria, error)
}

// ventanaAuditoriaPorDefecto es cuántas operaciones resume una página si no se pide otra cosa.
const ventanaAuditoriaPorDefecto = 500

// auditoriaOps — GET /documents/{id}/audit?before_seq=&limit=. Quién cambió qué y
// cuándo, para el PROPIETARIO: ni editores ni lectores la ven (403). Cada entrada es el
// resumen de un grupo de operaciones —autor, tipo, nombre de la clave, minuto y cuántas
// fueron—; el contenido de lo escrito no sale nunca. Se pagina con before_seq y `limit`,
// que es el número de operaciones que resume la página (500 por defecto, tope en
// store.MaxVentanaAuditoria).
func (h *Handler) auditoriaOps(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if _, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleOwner); err != nil {
		fail(w, r, err)
		return
	}
	q := r.URL.Query()
	antesDe, err := strconv.ParseInt(q.Get("before_seq"), 10, 64)
	if q.Get("before_seq") != "" && (err != nil || antesDe < 0) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("before_seq must be a non-negative integer."))
		return
	}
	ventana := ventanaAuditoriaPorDefecto
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > store.MaxVentanaAuditoria {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("limit must be between 1 and "+strconv.Itoa(store.MaxVentanaAuditoria)+"."))
			return
		}
		ventana = n
	}
	pag, err := h.auditoria.AuditoriaOps(r.Context(), docID(r), antesDe, ventana)
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, pag)
}
