// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package baseshttp

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/bases/internal/store"
)

const (
	// maxCuerpoPublico es el de un envío de formulario: un registro cabe en 60 KB.
	maxCuerpoPublico = 96 << 10
	// dificultadAltcha es el número máximo de la prueba de trabajo (decenas de
	// milisegundos en un móvil): frena el envío masivo sin estorbar a una persona.
	dificultadAltcha = 100_000
	vidaReto         = 5 * time.Minute
)

// montarFormularios registra, en el árbol SIN sesión, lo que llama el BFF en nombre de
// quien abre el enlace de un formulario. NO se publica en el ingress, que solo enruta
// `/api/v1` (solo la web lo llama, por dentro): el token es la única credencial y el cupo
// del enlace, el techo.
func (h *Handlers) montarFormularios(r chi.Router) {
	r.Route("/_bases/v1/forms/{token}", func(f chi.Router) {
		f.Use(httpx.MaxBodyBytes(maxCuerpoPublico))
		f.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: h.limitePublico}))
		f.Get("/", h.verFormulario)
		f.Get("/altcha", h.retoFormulario)
		f.Post("/", h.enviarFormulario)
	})
}

func (h *Handlers) verFormulario(w http.ResponseWriter, r *http.Request) {
	f, err := h.Store.FormularioPorToken(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, f)
}

func (h *Handlers) retoFormulario(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	// Solo se da un reto a un enlace que existe y sigue vivo: sin esto cualquiera
	// pediría retos con tokens inventados.
	if _, err := h.Store.FormularioPorToken(r.Context(), token); err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	reto, err := h.Altcha.CrearReto(store.AmbitoFormulario(token), dificultadAltcha, vidaReto)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("altcha_failed"))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, reto)
}

type envioFormulario struct {
	Values map[string]json.RawMessage `json:"values"`
	Altcha string                     `json:"altcha"`
}

func (h *Handlers) enviarFormulario(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	var in envioFormulario
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// La prueba de trabajo va ANTES de tocar la base: un envío sin ella no cuesta ni una
	// consulta de escritura. Su ámbito ata el reto a ESTE enlace y su firma se acepta una vez.
	if err := h.Altcha.Comprobar(r.Context(), store.AmbitoFormulario(token), dificultadAltcha/2, in.Altcha); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("altcha_invalid"))
		return
	}
	if err := h.Store.EnviarFormulario(r.Context(), token, in.Values); err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- gestión (con sesión, propietario) ---

type entradaEnlace struct {
	MaxResponses int        `json:"max_responses"`
	ExpiresAt    *time.Time `json:"expires_at"`
}

func (h *Handlers) crearEnlace(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	vid, err2 := id(r, "vid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in entradaEnlace
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	e, token, err := h.Store.CrearEnlaceFormulario(r.Context(), a, vid, in.MaxResponses, in.ExpiresAt)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	// El token se entrega UNA vez, aquí, y no se guarda en claro.
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusCreated, struct {
		store.EnlaceFormulario
		Token string `json:"token"`
	}{e, token})
}

func (h *Handlers) listarEnlaces(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	vid, err2 := id(r, "vid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	es, err := h.Store.ListarEnlacesFormulario(r.Context(), a, vid)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, es)
}

func (h *Handlers) revocarEnlace(w http.ResponseWriter, r *http.Request) {
	a, err := actor(r)
	eid, err2 := id(r, "eid")
	if err = firstErr(err, err2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.hecho(w, r, h.Store.RevocarEnlaceFormulario(r.Context(), a, eid))
}
