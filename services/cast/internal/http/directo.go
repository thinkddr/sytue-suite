// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package casthttp

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
	"git.sytue.eu/sytue/one/services/cast/internal/directo"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
)

// El directo (CAST-ENCARGO §5.2 y §6): `/studio/stream-key` (GET y :rotate),
// `/lives`, `/lives/{id}/publish`, `/lives/{id}/end` con sesión, y
// `/webhooks/livekit` en la raíz, sin sesión pero firmado por LiveKit.

// maxWebhookBytes acota el cuerpo de un webhook de LiveKit (1 MiB).
const maxWebhookBytes = 1 << 20

// montarDirecto enchufa el directo. Sin directo configurado se monta la
// implementación nula (503).
func montarDirecto(api, publico, raiz chi.Router, d Directo) {
	if d == nil {
		d = sinDirecto{}
	}
	d.Montar(api, publico, raiz)
}

// directoHTTP es el directo detrás de la interfaz Directo: traduce peticiones y
// respuestas y deja las reglas en `internal/directo`.
type directoHTTP struct {
	d *directo.Servicio
	h *Handlers
}

// NuevoDirecto envuelve el directo para el router. Con nil devuelve nil, que el
// router entiende como «sin directo».
func NuevoDirecto(d *directo.Servicio) Directo {
	if d == nil {
		return nil
	}
	return &directoHTTP{d: d, h: &Handlers{svc: d.Nucleo()}}
}

// Montar registra las rutas del directo.
func (x *directoHTTP) Montar(api, _, raiz chi.Router) {
	api.Get("/studio/stream-key", x.claveDeEmision)
	api.Post("/studio/stream-key:rotate", x.rotarClave)
	api.Post("/lives", x.programar)
	api.Get("/lives/{id}/publish", x.publicacion)
	api.Post("/lives/{id}/end", x.terminar)
	raiz.Post("/webhooks/livekit", x.webhook)
}

// Play rellena lo del directo para quien ya pasó la ACL de `play`.
func (x *directoHTTP) Play(ctx context.Context, v domain.Video, quien service.Quien) (*LivePlay, error) {
	c, err := x.d.Play(ctx, v, quien)
	if err != nil {
		return nil, err
	}
	return livePlayDe(c), nil
}

func livePlayDe(c *directo.Conexion) *LivePlay {
	if c == nil {
		return nil
	}
	return &LivePlay{ServerURL: c.ServerURL, Token: c.Token, Room: c.Room, Identity: c.Identity}
}

type claveDTO struct {
	RTMPURL   string `json:"rtmp_url"`
	StreamKey string `json:"stream_key"`
	IngressID string `json:"ingress_id"`
}

func claveDTOde(c directo.ClaveDeEmision) claveDTO {
	return claveDTO{RTMPURL: c.RTMPURL, StreamKey: c.StreamKey, IngressID: c.IngressID}
}

// claveDeEmision — GET /api/v1/studio/stream-key: la clave de OBS de quien llama
// (se crea al primer GET). Solo a su dueño: el token dice quién es.
func (x *directoHTTP) claveDeEmision(w http.ResponseWriter, r *http.Request) {
	c, err := x.d.StreamKey(r.Context(), quienDe(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, claveDTOde(c))
}

// rotarClave — POST /api/v1/studio/stream-key:rotate.
func (x *directoHTTP) rotarClave(w http.ResponseWriter, r *http.Request) {
	c, err := x.d.RotarClave(r.Context(), quienDe(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, claveDTOde(c))
}

type programaDTO struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Visibility  string `json:"visibility"`
	Source      string `json:"source"`
}

// programar — POST /api/v1/lives {title, description?, visibility?, source} →
// {video, publish?}: con source=webrtc trae el token de emisor.
func (x *directoHTTP) programar(w http.ResponseWriter, r *http.Request) {
	var in programaDTO
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, emision, err := x.d.Programar(r.Context(), quienDe(r), directo.Programa{
		Title: in.Title, Description: in.Description, Visibility: domain.Visibility(in.Visibility), Source: domain.LiveSource(in.Source),
	})
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	out := map[string]any{"video": x.h.aDTO(v, soloDueno())}
	if emision != nil {
		out["publish"] = livePlayDe(emision)
	}
	httpx.JSON(w, r, http.StatusCreated, out)
}

// publicacion — GET /api/v1/lives/{id}/publish → {server_url, token, room, identity}.
func (x *directoHTTP) publicacion(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := x.d.Publicacion(r.Context(), quienDe(r), id)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, livePlayDe(c))
}

// terminar — POST /api/v1/lives/{id}/end → el vídeo ya `ended`.
func (x *directoHTTP) terminar(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, err := x.d.Terminar(r.Context(), quienDe(r), id)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, x.h.aDTO(v, soloDueno()))
}

// webhook — POST /webhooks/livekit. La autenticidad la prueba la firma del
// cuerpo (Authorization: un JWT HS256 con el api_secret de LiveKit), no Sytue ID:
// LiveKit no porta tokens nuestros. Un aviso mal firmado es 401; uno que no se
// pudo aplicar, 500, para que LiveKit lo reintente.
func (x *directoHTTP) webhook(w http.ResponseWriter, r *http.Request) {
	cuerpo, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBytes))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The webhook body could not be read."))
		return
	}
	if err := x.d.Webhook(r.Context(), r.Header.Get("Authorization"), cuerpo); err != nil {
		if errors.Is(err, directo.ErrWebhookInvalido) {
			log.From(r.Context()).Warn("cast: webhook de LiveKit rechazado", "error", logsafe.Text(err))
			httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("Unauthenticated webhook.").Wrap(err))
			return
		}
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]string{"status": "ok"})
}
