// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package noteshttp

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// Compartir un cuaderno o una nota con UNA persona de la organización (store/personas.go):
//
//	GET    /notebooks/{id}/people          con quién está compartido (su dueño)
//	PUT    /notebooks/{id}/people/{sub}    {permission: read|write} — compartir o cambiar el nivel
//	DELETE /notebooks/{id}/people/{sub}    dejar de compartir
//	(lo mismo bajo /notes/{id}/people, para quien escribió la nota)
//	GET    /shared/notes                   «Compartido conmigo»: las notas que me han dado
//
// La persona llega por su `sub`: la web lo saca del correo en su BFF con
// `directory:resolve`, igual que Drive, y la app nativa con el mismo directorio. Y aquí
// se vuelve a preguntar a Sytue ID que sea de la organización antes de guardarlo.
// Compartir por primera vez avisa a quien recibe (`notes.shared`); cambiar el nivel no.

// maxSujeto acota el `sub` de la ruta: es un identificador, no texto libre.
const maxSujeto = 255

// VentanaAviso es cada cuánto, como mucho, se avisa a una persona de que se le ha
// compartido LO MISMO: retirar y volver a compartir no le llena la campana.
const VentanaAviso = 12 * time.Hour

func sujetoDeRuta(r *http.Request) (string, error) {
	s := strings.TrimSpace(chi.URLParam(r, "sub"))
	if s == "" || len(s) > maxSujeto {
		return "", httpx.ErrBadRequest.WithCode("invalid_subject")
	}
	return s, nil
}

// permisoDe lee `{permission}` del cuerpo: `write` es editar, `read` o nada es leer.
func permisoDe(w http.ResponseWriter, r *http.Request) (editar, ok bool) {
	var cuerpo struct {
		Permission string `json:"permission"`
	}
	if err := httpx.DecodeJSON(w, r, &cuerpo); err != nil {
		httpx.Fail(w, r, err)
		return false, false
	}
	switch cuerpo.Permission {
	case "", domain.PermisoLeer:
		return false, true
	case domain.PermisoEditar:
		return true, true
	}
	httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_permission").WithMessage("permission must be read or write."))
	return false, false
}

func personasJSON(w http.ResponseWriter, r *http.Request, ps []domain.Persona, err error) {
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"people": ps})
}

// PersonasDeCuaderno — GET /notebooks/{id}/people.
func (h *Handlers) PersonasDeCuaderno(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ps, err := h.store.PersonasDeCuaderno(r.Context(), id, sub(r))
	personasJSON(w, r, ps, err)
}

// PersonasDeNota — GET /notes/{id}/people.
func (h *Handlers) PersonasDeNota(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ps, err := h.store.PersonasDeNota(r.Context(), id, sub(r))
	personasJSON(w, r, ps, err)
}

// CompartirCuaderno — PUT /notebooks/{id}/people/{sub}.
func (h *Handlers) CompartirCuaderno(w http.ResponseWriter, r *http.Request) {
	h.compartir(w, r, h.store.CompartirCuaderno, hubnotify.EnlaceCuaderno, func(ctx context.Context, id uuid.UUID, sub string) error {
		_, err := h.store.PersonasDeCuaderno(ctx, id, sub)
		return err
	})
}

// CompartirNota — PUT /notes/{id}/people/{sub}.
func (h *Handlers) CompartirNota(w http.ResponseWriter, r *http.Request) {
	h.compartir(w, r, h.store.CompartirNota, hubnotify.EnlaceNotaCompartida, func(ctx context.Context, id uuid.UUID, sub string) error {
		_, err := h.store.PersonasDeNota(ctx, id, sub)
		return err
	})
}

type compartirFn func(ctx context.Context, id uuid.UUID, sub, sujeto string, editar bool) (bool, string, error)

// compartir es lo común de los dos. `esSuyo` comprueba ANTES que lo que se comparte es
// de quien lo pide, para no preguntarle a Sytue ID por nadie en nombre de quien no puede.
func (h *Handlers) compartir(w http.ResponseWriter, r *http.Request, fn compartirFn, enlace func(pub, org, id string) string,
	esSuyo func(context.Context, uuid.UUID, string) error,
) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sujeto, err := sujetoDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	editar, ok := permisoDe(w, r)
	if !ok {
		return
	}
	if err := esSuyo(r.Context(), id, sub(r)); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if !h.esDeLaOrganizacion(w, r, sujeto) {
		return
	}
	nueva, titulo, err := fn(r.Context(), id, sub(r), sujeto, editar)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	org := tenancy.MustFrom(r.Context()).Slug
	if nueva && h.avisados.Primera(org+"|"+id.String()+"|"+sujeto) {
		c, _ := authn.FromContext(r.Context())
		h.hub.SendAsync(r.Context(), hubnotify.Compartido(org, sujeto, "notes", "notes.shared",
			cmp.Or(strings.TrimSpace(titulo), "Notepads"), cmp.Or(strings.TrimSpace(c.Name), c.Email),
			enlace(h.cfg.PublicURL, org, id.String())))
	}
	permiso := domain.PermisoLeer
	if editar {
		permiso = domain.PermisoEditar
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"subject": sujeto, "permission": permiso, "created": nueva})
}

// esDeLaOrganizacion comprueba en Sytue ID, con la sesión de quien comparte, que el
// `sub` es de alguien de SU organización (`directory:resolve`). El cliente ya lo sacó
// del directorio, pero el que decide es el servicio: un permiso a un identificador
// inventado —o de otra organización— no se guarda. Contesta él si no lo es.
func (h *Handlers) esDeLaOrganizacion(w http.ResponseWriter, r *http.Request, sujeto string) bool {
	_, _, err := peticiones.Persona(r.Context(), h.cfg.IDBaseURL, authn.Token(r), sujeto)
	switch {
	case err == nil:
		return true
	case errors.Is(err, peticiones.ErrSinPersona):
		httpx.Fail(w, r, httpx.ErrNotFound.WithCode("unknown_person"))
	default:
		httpx.Fail(w, r, httpx.ErrUnavailable.Wrap(err))
	}
	return false
}

// DejarDeCompartirCuaderno — DELETE /notebooks/{id}/people/{sub}.
func (h *Handlers) DejarDeCompartirCuaderno(w http.ResponseWriter, r *http.Request) {
	h.dejarDeCompartir(w, r, h.store.DejarDeCompartirCuaderno)
}

// DejarDeCompartirNota — DELETE /notes/{id}/people/{sub}.
func (h *Handlers) DejarDeCompartirNota(w http.ResponseWriter, r *http.Request) {
	h.dejarDeCompartir(w, r, h.store.DejarDeCompartirNota)
}

func (h *Handlers) dejarDeCompartir(w http.ResponseWriter, r *http.Request, fn func(context.Context, uuid.UUID, string, string) error) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sujeto, err := sujetoDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := fn(r.Context(), id, sub(r), sujeto); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.NoContent(w)
}

// NotasCompartidas — GET /shared/notes.
func (h *Handlers) NotasCompartidas(w http.ResponseWriter, r *http.Request) {
	ns, err := h.store.NotasCompartidas(r.Context(), sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if ns == nil {
		ns = []domain.NotaCompartida{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"notes": ns})
}
