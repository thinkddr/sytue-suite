// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package casthttp

import (
	"context"
	"net/http"
	"strings"

	"github.com/coder/websocket"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
	"git.sytue.eu/sytue/one/services/cast/internal/social"
)

// ChatWS — GET /ws/videos/{id}/chat?access_token=… (CAST-ENCARGO §5.2).
//
// Valida el token (cabecera o query: el navegador no pone cabeceras en el
// handshake), resuelve el acceso al vídeo UNA vez y entra en la sala. La conexión
// se cierra sola cuando el token caduca; al reconectar, el token nuevo dice lo que
// vale ahora. Este fichero lo REEMPLAZA entero el agente backend-social si lo necesita.
func (h *Handlers) ChatWS(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	raw := bearerDe(r)
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("access_token"))
	}
	if raw == "" {
		httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("Missing token."))
		return
	}
	claims, err := h.verifier.Verify(r.Context(), raw)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("Invalid or expired token.").Wrap(err))
		return
	}
	t, ok := claims.TenantOrEmpty()
	if !ok {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("The token does not carry a valid organisation."))
		return
	}
	quien := service.Quien{Sub: claims.Subject, Tenant: t.Slug, Nombre: claims.Name, Admin: claims.HasAnyRole("owner", "admin"), Token: raw}

	// El acceso se resuelve ANTES de aceptar: quien no ve el vídeo recibe un 404
	// normal, no un socket que se cierra.
	v, p, err := h.svc.Resolver(r.Context(), quien, id)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// El socket llega a cast.<zona> mientras la página vive en <org>.<zona>:
		// el host propio no cubre el Origin del navegador. Ver httpx.WSOriginPatterns.
		OriginPatterns: httpx.WSOriginPatterns(r, h.cfg.AllowedOrigins, t.Slug),
	})
	if err != nil {
		log.From(r.Context()).Debug("cast/chat: no se pudo aceptar la conexión", "error", err)
		return
	}

	video := social.Video{ID: v.ID, Tenant: v.Tenant, OwnerSub: v.OwnerSub, OwnerName: v.OwnerName, Titulo: v.Title, ComentariosActivos: v.CommentsEnabled, ChatActivo: v.ChatEnabled}
	cliente := h.social.NuevoCliente(conn, socialDe(quien), video, social.Permisos{Ver: p.Ver, Editar: p.Editar, Moderar: p.Moderar})
	// Desligado del request: la conexión sobrevive a la petición que la abrió.
	cliente.Servir(context.WithoutCancel(r.Context()), claims.Expira)
}

func bearerDe(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefijo = "Bearer "
	if len(h) > len(prefijo) && strings.EqualFold(h[:len(prefijo)], prefijo) {
		return strings.TrimSpace(h[len(prefijo):])
	}
	return ""
}
