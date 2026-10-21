// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"cmp"
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// # Pedir acceso de edición
//
// La franja de solo lectura de Docus, Calcus y Diapos lleva un botón. Quien solo
// puede leer o comentar lo pulsa y el dueño recibe un aviso del Hub y un correo; el
// aviso abre el documento, donde la franja del dueño le ofrece «Conceder» con un
// clic. Al conceder, el canal de coedición le cambia el rol a quien lo pidió y su
// franja se quita sola, sin recargar (collab.Hub.PublishRole).

// peticionesStore es lo que esto necesita del store: tablas que no son del
// contrato documental (domain.Store), como connectorStore.
type peticionesStore interface {
	PedirEdicion(ctx context.Context, p peticiones.Peticion) (bool, error)
	PeticionesEdicion(ctx context.Context, docID string) ([]peticiones.Peticion, error)
	ResolverPeticionEdicion(ctx context.Context, docID, sujeto, concede string) error
	HayPeticionEdicion(ctx context.Context, docID, sujeto string) (bool, error)
}

// pedirEdicion — POST /documents/{id}/edit-requests. Idempotente: pedirlo otra vez
// no vuelve a avisar al dueño.
func (h *Handler) pedirEdicion(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	doc, role, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleViewer)
	if err != nil {
		fail(w, r, err)
		return
	}
	// Ya puede: no hay nada que pedir, y la respuesta le dice a la franja que se quite.
	if role.CanWrite() {
		httpx.JSON(w, r, http.StatusOK, map[string]any{"role": role, "requested": false})
		return
	}
	claims, _ := authnClaims(r)
	nueva, err := h.peticiones.PedirEdicion(r.Context(), peticiones.De(claims, doc.ID))
	if err != nil {
		fail(w, r, err)
		return
	}
	if nueva {
		quien := peticiones.De(claims, doc.ID).Quien()
		t, _ := tenancy.From(r.Context())
		h.avisador().Avisar(r.Context(), peticiones.Aviso{
			Tenant: t.Slug, Para: doc.OwnerSub, Source: origenDe(doc.Kind), Type: "docs.access.requested",
			Titulo: quien + " asks to edit “" + doc.Title + "”", URL: hubnotify.EnlaceDocumento(h.publicURL, t.Slug, doc.ID, string(doc.Kind)),
			Recurso: doc.ID, Params: map[string]string{"actor": quien, "name": doc.Title}, Token: authn.Token(r),
			Asunto: quien + " asks to edit “" + doc.Title + "”",
			Cuerpo: quien + " is asking for permission to edit “" + doc.Title + "”. Open the document to grant it with one click:",
		})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"role": role, "requested": true})
}

// listarPeticiones — GET /documents/{id}/edit-requests. Solo el dueño.
func (h *Handler) listarPeticiones(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if _, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleOwner); err != nil {
		fail(w, r, err)
		return
	}
	ps, err := h.peticiones.PeticionesEdicion(r.Context(), docID(r))
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"requests": ps})
}

// concederPeticion — POST /documents/{id}/edit-requests/{sub}/grant. Solo el dueño, y
// solo a quien lo pidió: el `sub` de la ruta tiene que tener su petición.
func (h *Handler) concederPeticion(w http.ResponseWriter, r *http.Request) {
	h.resolverPeticion(w, r, true)
}

// rechazarPeticion — DELETE /documents/{id}/edit-requests/{sub}.
func (h *Handler) rechazarPeticion(w http.ResponseWriter, r *http.Request) {
	h.resolverPeticion(w, r, false)
}

func (h *Handler) resolverPeticion(w http.ResponseWriter, r *http.Request, concede bool) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	doc, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleOwner)
	if err != nil {
		fail(w, r, err)
		return
	}
	quien := chi.URLParam(r, "sub")
	por := ""
	if concede {
		por = subj.Sub
	}
	if err := h.peticiones.ResolverPeticionEdicion(r.Context(), doc.ID, quien, por); err != nil {
		fail(w, r, err)
		return
	}
	if concede {
		// La franja de quien lo pidió se quita sola: su conexión de coedición pasa a editor.
		h.hub.PublishRole(r.Context(), doc.ID, quien, domain.RoleEditor)
		t, _ := tenancy.From(r.Context())
		actor := ""
		if c, ok := authnClaims(r); ok {
			actor = cmp.Or(strings.TrimSpace(c.Name), c.Email)
		}
		h.avisador().Avisar(r.Context(), peticiones.Aviso{
			Tenant: t.Slug, Para: quien, Source: origenDe(doc.Kind), Type: "docs.access.granted",
			Titulo: "You can now edit “" + doc.Title + "”", URL: hubnotify.EnlaceDocumento(h.publicURL, t.Slug, doc.ID, string(doc.Kind)),
			Recurso: doc.ID, Params: map[string]string{"actor": actor, "name": doc.Title},
		})
	}
	httpx.NoContent(w)
}

// pedidaPor dice si esta persona tiene una petición pendiente, para que la franja la
// enseñe como enviada al volver a abrir. Un fallo aquí no tumba la lectura del documento.
func (h *Handler) pedidaPor(ctx context.Context, docID, sub string) bool {
	if h.peticiones == nil {
		return false
	}
	hay, err := h.peticiones.HayPeticionEdicion(ctx, docID, sub)
	return err == nil && hay
}

// avisador es el de platform/pkg/peticiones con el Hub, el correo y Sytue ID de Docs.
func (h *Handler) avisador() peticiones.Avisador {
	return peticiones.Avisador{Hub: h.hubNotifier, Correo: h.correo, IDBaseURL: h.idBaseURL}
}
