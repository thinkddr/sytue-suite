// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package casthttp

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
	"git.sytue.eu/sytue/one/services/cast/internal/social"
)

// montarSocial enchufa comentarios y chat con sesión (CAST-ENCARGO §5.2) bajo
// `api` (/api/v1). Sin social configurado, las rutas contestan 503.
func montarSocial(api chi.Router, h *Handlers) {
	if h.social == nil {
		api.HandleFunc("/videos/{id}/comments", noHaySocial)
		api.HandleFunc("/videos/{id}/comments/*", noHaySocial)
		api.HandleFunc("/videos/{id}/chat", noHaySocial)
		api.HandleFunc("/videos/{id}/chat/*", noHaySocial)
		return
	}
	api.Get("/videos/{id}/comments", h.Comentarios)
	api.Post("/videos/{id}/comments", h.Comentar)
	api.Delete("/videos/{id}/comments/{cid}", h.BorrarComentario)
	api.Get("/videos/{id}/chat", h.HistorialChat)
	api.Delete("/videos/{id}/chat/{mid}", h.BorrarChat)
}

// montarSocialPublico: la lectura de comentarios sin sesión (§5.3), bajo /public
// y /api/v1/public.
func montarSocialPublico(publico chi.Router, h *Handlers) {
	if h.social == nil {
		publico.HandleFunc("/videos/{id}/comments", noHaySocial)
		return
	}
	publico.Get("/videos/{id}/comments", h.Comentarios)
}

// montarSocialRaiz: el WebSocket del chat va fuera de /api/v1 (ruta /ws en el
// ingress), sin timeout y con el token por query.
func montarSocialRaiz(raiz chi.Router, h *Handlers) {
	if h.social == nil {
		raiz.HandleFunc("/ws/*", noHaySocial)
		return
	}
	raiz.Get("/ws/videos/{id}/chat", h.ChatWS)
}

func noHaySocial(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("Comments and chat are not available on this deployment."))
}

// socialDe convierte a quien pregunta en el `Quien` de lo social.
func socialDe(q service.Quien) social.Quien {
	return social.Quien{Sub: q.Sub, Tenant: q.Tenant, Nombre: q.Nombre, Usuario: q.Usuario, Admin: q.Admin, Anonimo: q.Anonimo, IP: q.IP}
}

// Comentarios — GET /api/v1/videos/{id}/comments y GET /public/videos/{id}/comments.
func (h *Handlers) Comentarios(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	filas, siguiente, err := h.social.Comentarios(r.Context(), socialDe(quienDe(r)), id, r.URL.Query().Get("cursor"), limiteDe(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if filas == nil {
		filas = []social.Comentario{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"comments": filas, "next_cursor": siguiente})
}

// Comentar — POST /api/v1/videos/{id}/comments {"body":"…","reply_to":"…"}.
func (h *Handlers) Comentar(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		Body    string     `json:"body"`
		ReplyTo *uuidParam `json:"reply_to"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := h.social.Comentar(r.Context(), socialDe(quienDe(r)), id, in.Body, in.ReplyTo.ptr())
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, c)
}

// BorrarComentario — DELETE /api/v1/videos/{id}/comments/{cid}.
func (h *Handlers) BorrarComentario(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cid, err := idDeRuta(r, "cid")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.social.BorrarComentario(r.Context(), socialDe(quienDe(r)), id, cid); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.NoContent(w)
}

// HistorialChat — GET /api/v1/videos/{id}/chat?since=RFC3339.
func (h *Handlers) HistorialChat(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var desde time.Time
	if s := r.URL.Query().Get("since"); s != "" {
		if desde, err = time.Parse(time.RFC3339Nano, s); err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid 'since' timestamp."))
			return
		}
	}
	msgs, err := h.social.HistorialChat(r.Context(), socialDe(quienDe(r)), id, desde)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if msgs == nil {
		msgs = []social.Mensaje{}
	}
	// El recuento de espectadores no va aquí: lo manda el WebSocket (`viewers`) a
	// quien está conectado, y ponerlo costaría resolver el vídeo dos veces.
	httpx.JSON(w, r, http.StatusOK, map[string]any{"messages": msgs})
}

// BorrarChat — DELETE /api/v1/videos/{id}/chat/{mid}.
func (h *Handlers) BorrarChat(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	mid, err := idDeRuta(r, "mid")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.social.BorrarChat(r.Context(), socialDe(quienDe(r)), id, mid); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.NoContent(w)
}
