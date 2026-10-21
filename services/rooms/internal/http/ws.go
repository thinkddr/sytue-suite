// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"context"
	"net/http"
	"strings"

	"github.com/coder/websocket"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
	"git.sytue.eu/sytue/one/services/rooms/internal/ws"
)

// withBearer captura el token Bearer de la cabecera Authorization y lo guarda en
// el contexto para reusarlo al llamar a Sytue Drive (propagación de identidad).
// Se compone DESPUÉS de RequireAuth (el token ya está validado).
func withBearer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tok := bearerFromHeader(r); tok != "" {
			r = r.WithContext(service.WithBearer(r.Context(), tok))
		}
		next.ServeHTTP(w, r)
	})
}

// bearerFromHeader extrae el token Bearer de la cabecera Authorization.
func bearerFromHeader(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

// wsHandler gestiona el endpoint WebSocket de tiempo real.
type wsHandler struct {
	svc      *service.Service
	hub      *ws.Hub
	verifier *authn.Verifier
	cfg      config.Config
}

// serve valida el token (cabecera o ?access_token=), acepta el WebSocket y entra
// en el bucle de la conexión. El token por query es necesario porque el
// navegador no puede fijar cabeceras en el handshake WS.
func (h *wsHandler) serve(w http.ResponseWriter, r *http.Request) {
	raw := bearerFromHeader(r)
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

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Más el host de la petición y el de la ORGANIZACIÓN del token: aquí el
		// socket llega a `rooms.<zona>` mientras la página vive en
		// `<org>.<zona>`, así que el host propio no cubre el Origin del
		// navegador. Ver httpx.WSOriginPatterns.
		OriginPatterns: httpx.WSOriginPatterns(r, h.cfg.AllowedOrigins, t.Slug),
	})
	if err != nil {
		log.From(r.Context()).Debug("ws: no se pudo aceptar la conexión", "error", err)
		return
	}

	// Contexto desligado del request HTTP para que la conexión sobreviva, pero
	// portando tenant + token (para autorizar suscripciones y resolver adjuntos).
	// Y con el plazo del token: la conexión no vive más que él (COM-08).
	ctx, cancelar := authn.HastaQueCaduque(context.WithoutCancel(r.Context()), claims)
	defer cancelar()
	ctx = tenancy.Into(ctx, t)
	ctx = service.WithBearer(ctx, raw)

	name := claims.Name
	if name == "" {
		name = claims.Email
	}
	client := ws.NewClient(h.hub, h.svc, conn, t.Slug, claims.Subject, name)
	client.Serve(ctx)
}
