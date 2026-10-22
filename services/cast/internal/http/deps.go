// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package casthttp

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
)

// Las piezas que otros agentes enchufan en el router (CAST-ENCARGO §14): el
// directo es una INTERFAZ con una implementación nula, y lo social un servicio
// que puede faltar (nil), para que el servicio arranque y sirva la parte de vídeo
// bajo demanda aunque falte una. Quien las implementa REEMPLAZA entero
// directo.go / social.go y chat_ws.go.

// ErrNoDisponible: la pieza no está enchufada en este despliegue.
var ErrNoDisponible = errors.New("cast: esta función no está disponible")

// LivePlay es lo del directo que va en Play (§5.1): a qué SFU conectarse, con
// qué token (solo suscripción) y en qué room.
type LivePlay struct {
	ServerURL string `json:"server_url"`
	Token     string `json:"token"`
	Room      string `json:"room"`
	Identity  string `json:"identity"`
}

// Directo es el paquete de directo visto desde el router.
type Directo interface {
	// Montar registra sus rutas: con sesión bajo `api` (/api/v1: /studio/*, /lives/*),
	// sin sesión bajo `publico` (se monta en /public y /api/v1/public) y en la raíz
	// (`raiz`: /webhooks/livekit).
	Montar(api, publico, raiz chi.Router)
	// Play rellena lo del directo para un espectador ya autorizado (o anónimo en
	// lo público): token de LiveKit de solo suscripción.
	Play(ctx context.Context, video domain.Video, quien service.Quien) (*LivePlay, error)
}

// sinDirecto es el directo cuando no hay directo: cada ruta contesta 503.
type sinDirecto struct{}

func (sinDirecto) Montar(api, _, raiz chi.Router) {
	api.HandleFunc("/studio/*", noDisponible)
	api.HandleFunc("/lives", noDisponible)
	api.HandleFunc("/lives/*", noDisponible)
	raiz.HandleFunc("/webhooks/*", noDisponible)
}

func (sinDirecto) Play(context.Context, domain.Video, service.Quien) (*LivePlay, error) {
	return nil, ErrNoDisponible
}

func noDisponible(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("Live streaming is not available on this deployment."))
}
