// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// # La señal de cambios a quienes ven un documento
//
// Lo que escribe una persona en un documento ajeno —renombrarlo, comentarlo, una
// versión, editarlo en vivo— cambia los recientes de Docus, Calcus o Diapos de su
// propietario y de las demás personas con permiso: les llega la señal de cambios del
// widget (ADR 0078), como en Calendar. Hasta el 2026-11-25 solo la recibía quien
// escribía, y la edición en vivo ni eso.
//
// - Por REST (`/documents/{id}/…`), conQuienesVen suma a la señal de la escritura en
//   curso (hubnotify.TrasEscribir) al propietario y a las personas con permiso: una
//   lectura del documento y otra de su ACL, solo si hay señal y una vez por petición.
// - La edición en vivo (`/ops` por REST y las operaciones del WebSocket) va a ritmo de
//   tecla y por eso va sin TrasEscribir: avisarEnVivo manda la señal a quien escribe y a
//   quienes ven el documento como mucho UNA vez por documento y VentanaEnVivo, antes de
//   leer nada. El resto de la sesión de edición la recoge el latido del widget.

// VentanaEnVivo es cada cuánto, como mucho, la edición en vivo de un documento manda la
// señal de cambios. En memoria y por réplica, como hubnotify.UnaVez.
const VentanaEnVivo = time.Minute

// quienesVenDocumento son el propietario del documento y las personas con un permiso
// sobre él. Ni grupos, ni toda la organización, ni enlaces: no se avisan uno a uno.
func (h *Handler) quienesVenDocumento(ctx context.Context, id string) (*domain.Document, []string, error) {
	doc, err := h.store.GetDocument(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	perms, err := h.store.ListPermissions(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	quienes := []string{doc.OwnerSub}
	for _, p := range perms {
		if p.PrincipalKind == domain.PrincipalUser && p.PrincipalID != "" {
			quienes = append(quienes, p.PrincipalID)
		}
	}
	return doc, quienes, nil
}

// conQuienesVen es el middleware de `/documents/{id}`: en una escritura con señal, suma
// a quienes ven el documento. Se mira ANTES de escribir: borrado, ya no se lee.
func (h *Handler) conQuienesVen(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := chi.URLParam(r, "id"); id != "" {
			ctx := r.Context()
			hubnotify.AvisarQuienesVen(ctx, "docs/"+id, func() ([]string, error) {
				_, quienes, err := h.quienesVenDocumento(ctx, id)
				return quienes, err
			})
		}
		next.ServeHTTP(w, r)
	})
}

// avisarEnVivo es la señal de la edición en vivo de `id` por `quien` (arriba).
func (h *Handler) avisarEnVivo(ctx context.Context, quien, id string) {
	if h.hubNotifier == nil || h.enVivo == nil {
		return
	}
	t, err := tenancy.From(ctx)
	if err != nil || !h.enVivo.Primera(t.Slug+"\x00"+id) {
		return
	}
	doc, quienes, err := h.quienesVenDocumento(ctx, id)
	if err != nil {
		log.From(ctx).Warn("docs: quiénes ven el documento editado en vivo", "error", err)
		return
	}
	h.hubNotifier.AvisarCambio(ctx, t.Slug, origenDe(doc.Kind), append(quienes, quien)...)
}

// aplicarOps es domain.Service.ApplyOps con la señal de la edición en vivo.
func (h *Handler) aplicarOps(ctx context.Context, subj domain.Subject, id string, ops []domain.Op, baseSeq *int64) ([]domain.SequencedOp, int64, error) {
	seqd, seq, err := h.svc.ApplyOps(ctx, subj, id, ops, baseSeq)
	if err == nil && len(seqd) > 0 {
		h.avisarEnVivo(ctx, subj.Sub, id)
	}
	return seqd, seq, err
}
