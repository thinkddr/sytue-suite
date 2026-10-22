// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
)

// Personas de OTRA organización en Rooms (ADR 0040): encontrarlas, listar las
// conversaciones de fuera de quien pregunta y entrar en ellas.

// anfitrionResolver es lo que necesita el cambio a la organización anfitriona.
type anfitrionResolver interface {
	Anfitrion(ctx context.Context, subject string, ref service.RefAnfitrion) (string, error)
}

// EnAnfitrion devuelve la petición en la organización ANFITRIONA cuando el recurso
// es de una conversación de otra organización donde quien llama está como persona
// de fuera; la misma petición en cualquier otro caso.
//
// LOS ROLES DEL TOKEN SE QUITAN. Son los de SU organización: un owner de la suya
// no es nada en la anfitriona, y sin esto lo sería —el vídeo da el anfitrión de
// cualquier sala y las grabaciones a quien trae «admin» en el token—. Lo que le
// abre la conversación es su fila `externo` en ese canal, no su rol.
//
// Lo usan el router de mensajería (por canal y por mensaje) y el de vídeo (por
// sala y por canal de origen), que es por lo que se exporta.
func EnAnfitrion(svc anfitrionResolver) func(r *http.Request, ref service.RefAnfitrion) (*http.Request, error) {
	return func(r *http.Request, ref service.RefAnfitrion) (*http.Request, error) {
		c, ok := authn.FromContext(r.Context())
		if !ok {
			return r, nil
		}
		anfitrion, err := svc.Anfitrion(r.Context(), c.Subject, ref)
		if err != nil || anfitrion == "" {
			return r, err
		}
		c.Roles, c.Tenant = nil, anfitrion
		ctx := authn.Into(r.Context(), c)
		ctx = tenancy.Into(ctx, tenancy.Tenant{Slug: anfitrion})
		return r.WithContext(ctx), nil
	}
}

// enAnfitrionPor es EnAnfitrion como middleware de una ruta: toma el recurso del
// parámetro de la ruta. Un parámetro que no es un UUID se deja pasar tal cual: el
// handler ya responde 400.
func enAnfitrionPor(svc anfitrionResolver, param string, ref func(uuid.UUID) service.RefAnfitrion) func(http.Handler) http.Handler {
	cambiar := EnAnfitrion(svc)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, err := uuid.Parse(chi.URLParam(r, param))
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			r, err = cambiar(r, ref(id))
			if err != nil {
				httpx.Fail(w, r, toAPIError(err))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type resolvePersonRequest struct {
	Query string `json:"query"`
}

// ResolvePerson — POST /api/v1/people:resolve — encuentra a alguien de cualquier
// organización por su @usuario o su correo, para abrirle un DM o añadirle a un
// canal. La búsqueda la hace Sytue ID con el token de quien pregunta.
func (h *Handlers) ResolvePerson(w http.ResponseWriter, r *http.Request) {
	var req resolvePersonRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	q := strings.TrimSpace(req.Query)
	if q == "" || len(q) > 320 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Enter a Sytue One username or an email address."))
		return
	}
	p, err := h.svc.ResolvePerson(r.Context(), q)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, p)
}

// ListInvitations — GET /api/v1/me/invitations — las invitaciones pendientes de otras
// organizaciones (mig 0041): lo único que se ve del canal antes de aceptar.
func (h *Handlers) ListInvitations(w http.ResponseWriter, r *http.Request) {
	sub, _ := caller(r)
	lista, err := h.svc.Invitaciones(r.Context(), sub)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if lista == nil {
		lista = []domain.Invitacion{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"invitations": lista})
}

// AcceptInvitation — POST /api/v1/me/invitations/{channelID}/accept — entra.
func (h *Handlers) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, name := caller(r)
	if err := h.svc.AceptarInvitacion(r.Context(), sub, name, id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// RejectInvitation — DELETE /api/v1/me/invitations/{channelID} — la rechaza.
func (h *Handlers) RejectInvitation(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	if err := h.svc.RechazarInvitacion(r.Context(), sub, id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ListExternalChannels — GET /api/v1/me/external-channels — las conversaciones de
// OTRAS organizaciones donde el usuario está como persona de fuera, con sus no
// leídos. Van aparte de /teams/{id}/channels porque no son de ningún equipo suyo.
func (h *Handlers) ListExternalChannels(w http.ResponseWriter, r *http.Request) {
	sub, _ := caller(r)
	channels, unread, err := h.svc.ListExternalChannels(r.Context(), sub)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if channels == nil {
		channels, unread = []domain.Channel{}, []domain.ReadState{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"channels": channels, "unread": unread})
}
