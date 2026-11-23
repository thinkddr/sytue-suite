// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// EL ANCLA EN LA BÓVEDA (ADR 0086).
//
// Con el Drive de la persona en la bóveda post-cuántica, este servicio no puede dejar
// ahí el fichero de un documento (hogar_en_drive.go: el documento nace sin ancla). Lo
// escribe el NAVEGADOR, cifrado, y aquí solo llega el ancla: «vault:<id del fichero
// en la bóveda>». Nunca el contenido, y nada de lo que hace este servicio con un
// «drive:<id>» (la papelera de Drive, abrir desde Drive) lo toma por un fichero suyo:
// `ficheroDe` solo reconoce el prefijo «drive:».
//
// Las reglas, y por qué:
//
//   - Solo el DUEÑO: la bóveda es la suya; un editor que guarda en la suya deja una
//     copia, no el fichero del documento.
//   - Solo desde sin ancla o desde otra de la bóveda. Un «drive:» no se sustituye: la
//     papelera lo usa para llevar el fichero del Drive del servidor con el documento,
//     y ese fichero sigue existiendo (congelado) aunque el Drive esté en la bóveda.
//   - Comparar y cambiar (`expect`): dos pestañas que guardan a la vez no se pisan el
//     ancla; la segunda recibe 409 y su fichero queda en la bóveda sin anclar.
//   - El id es un uuid canónico: lo que la bóveda genera (crypto.randomUUID) o lo que
//     conservó al copiar el Drive del servidor.
//
// No cambia `updated_at`: anclar no es editar, y la lista no debe reordenarse por ello.

// prefijoBoveda es el del ancla a un fichero del Drive de la bóveda.
const prefijoBoveda = "vault:"

// anclaBovedaStore es lo que este fichero necesita de la persistencia.
type anclaBovedaStore interface {
	// AnclarEnBoveda pone `despues` como ancla del documento `id` si su ancla sigue
	// siendo `antes` ("" = sin ancla). domain.ErrNotFound si ya no lo era (o si el
	// documento ya no está); domain.ErrConflict si otro documento vivo tiene esa ancla.
	AnclarEnBoveda(ctx context.Context, id, antes, despues string) error
}

var (
	errAnclaNoValida = httpx.ErrBadRequest.WithCode("vault_anchor_invalid").
				WithMessage(`"source_ref" must be "vault:" followed by a file id.`)
	errAnclaDeDrive = httpx.ErrConflict.WithCode("vault_anchor_not_allowed").
			WithMessage("This document is anchored to a file in Sytue Drive.")
	errAnclaCambiada = httpx.ErrConflict.WithCode("vault_anchor_changed").
				WithMessage("The document's file changed meanwhile. Reload and try again.")
)

// esAnclaDeBoveda dice si `ref` es «vault:<uuid canónico>».
func esAnclaDeBoveda(ref string) bool {
	id, ok := strings.CutPrefix(ref, prefijoBoveda)
	if !ok {
		return false
	}
	u, err := uuid.Parse(id)
	return err == nil && u.String() == id
}

func anclaBovedaDe(s *store.Store) anclaBovedaStore {
	if s == nil {
		return nil
	}
	return s
}

type anclaBovedaReq struct {
	SourceRef string `json:"source_ref"`
	// Expect es el ancla que quien pide cree que tiene el documento ("" = ninguna).
	Expect string `json:"expect"`
}

// anclarEnBoveda — PUT /api/v1/documents/{id}/vault-anchor.
func (h *Handler) anclarEnBoveda(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req anclaBovedaReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !esAnclaDeBoveda(req.SourceRef) {
		httpx.Fail(w, r, errAnclaNoValida)
		return
	}
	doc, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleOwner)
	if err != nil {
		fail(w, r, err)
		return
	}
	if h.anclaBoveda == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	actual := doc.SourceRef
	switch {
	case actual == req.SourceRef:
		// Ya está: repetirlo (un reintento tras perder la respuesta) no es un error.
		httpx.NoContent(w)
		return
	case actual != "" && !strings.HasPrefix(actual, prefijoBoveda):
		httpx.Fail(w, r, errAnclaDeDrive)
		return
	case actual != req.Expect:
		httpx.Fail(w, r, errAnclaCambiada)
		return
	}
	err = h.anclaBoveda.AnclarEnBoveda(r.Context(), doc.ID, actual, req.SourceRef)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		// Se autorizó un instante antes: si ya no casa, es que otro lo cambió.
		httpx.Fail(w, r, errAnclaCambiada)
	case errors.Is(err, domain.ErrConflict):
		httpx.Fail(w, r, httpx.ErrConflict.WithCode("vault_anchor_taken").
			WithMessage("Another document is already anchored to that file."))
	case err != nil:
		fail(w, r, err)
	default:
		httpx.NoContent(w)
	}
}
