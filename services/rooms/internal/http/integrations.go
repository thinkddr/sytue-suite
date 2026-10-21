// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/compat"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
)

// intHandlers agrupa los endpoints de INTEGRACIONES (Ola 15): webhooks
// entrantes/salientes y bots. Usa el *service.Service concreto (no la interfaz
// chatService) para no engordar esa superficie mockeable; su seguridad se cubre con
// tests dedicados (SSRF, hash de token, auth de bot).
type intHandlers struct {
	svc *service.Service
	// publicURL es la base pública del servicio (cfg.PublicURL, sin barra final): con
	// ella se compone la URL completa del webhook entrante (base + /hooks/{token}) que
	// se muestra una vez al crear/rotar.
	publicURL string
	// internalSecret autentica el endpoint interno chat←meet (POST /internal/call-ended).
	// Vacío => la ruta no se monta (ver NewRouter).
	internalSecret string
	// channelMailDomain es cfg.ChannelMailDomain: viaja con el secreto de un entrante
	// para que quien lo enseña componga su dirección de correo. Vacío = apagado.
	channelMailDomain string
	// postIncomingFn es la costura de publicación del webhook entrante. nil (siempre
	// en producción) ⇒ el servicio real; los tests del handler la sustituyen porque
	// *service.Service es concreto y exigiría una BD.
	//
	// ponytail: un campo func en vez de una interfaz nueva — un único método.
	postIncomingFn func(ctx context.Context, token, text string) error
}

// secretoEntrante es lo que vuelve al crear o rotar un entrante: el token y la URL en
// claro y, si el correo a canal está encendido, su dominio (`channel_mail_domain`), con
// el que la web y las apps componen `<canal>+<token>@<dominio>`. Sin él no se ofrece
// la dirección: enseñar una que rebota sería peor que no enseñar ninguna.
func (ih *intHandlers) secretoEntrante(hook domain.IncomingWebhook, token string) map[string]any {
	out := map[string]any{"webhook": hook, "token": token, "url": ih.hookURL(token)}
	if ih.channelMailDomain != "" {
		out["channel_mail_domain"] = ih.channelMailDomain
	}
	return out
}

// hookURL compone la URL pública completa de un webhook entrante para un token.
func (ih *intHandlers) hookURL(token string) string {
	return ih.publicURL + "/hooks/" + token
}

// ---------------------------------------------------------------------------
// Endpoint PÚBLICO: POST /hooks/{token}  (SIN JWT — el token ES la autorización)
// ---------------------------------------------------------------------------

type hookPostRequest struct {
	Text string `json:"text"`
}

// maxHookBodyBytes acota el cuerpo del webhook entrante ANTES de mirarlo (el
// dialecto se detecta sobre los bytes, así que no puede delegarse en DecodeJSON).
// Holgado para tarjetas de Slack/Teams y muy por debajo de lo que aguanta el
// mensaje resultante (NormalizeBody recorta después con MaxMessageBytes).
const maxHookBodyBytes = 256 << 10

// PostIncoming publica un mensaje mediante un webhook entrante. No hay JWT: el token
// de la ruta es la autorización (rate-limitado por token+IP en el middleware). El
// servicio rehashea el token, lo busca cross-tenant y publica en el canal ligado.
// AUTODETECTA el dialecto del cuerpo (nativo, Slack o Teams) para que cualquier
// herramienta que ya sepa hablar con un webhook de Slack/Teams funcione pegando la URL.
func (ih *intHandlers) PostIncoming(w http.ResponseWriter, r *http.Request) {
	ih.postIncoming(w, r, "")
}

// PostIncomingSlack fuerza el dialecto Slack (alias /hooks/slack/{token}): útil
// cuando el emisor manda solo {"text":"…"} con mrkdwn y espera el cuerpo `ok`.
func (ih *intHandlers) PostIncomingSlack(w http.ResponseWriter, r *http.Request) {
	ih.postIncoming(w, r, compat.Slack)
}

// PostIncomingTeams fuerza el dialecto Teams (alias /hooks/teams/{token}).
func (ih *intHandlers) PostIncomingTeams(w http.ResponseWriter, r *http.Request) {
	ih.postIncoming(w, r, compat.Teams)
}

// PostIncomingGoogleChat fuerza el dialecto Google Chat (alias /hooks/googlechat/{token}):
// un {"text"} con formato de Chat se traduce en vez de pasar como texto llano.
func (ih *intHandlers) PostIncomingGoogleChat(w http.ResponseWriter, r *http.Request) {
	ih.postIncoming(w, r, compat.GoogleChat)
}

// postIncoming es el cuerpo común: lee (acotado), decide dialecto, traduce y publica.
//
// SEGURIDAD: el token de la ruta sigue siendo la ÚNICA autorización y el canal lo
// fija el webhook. El campo `channel` del payload de Slack se IGNORA a propósito
// (ver compat/slack.go): permitirlo dejaría a cualquiera con un token postear en
// cualquier canal del tenant.
func (ih *intHandlers) postIncoming(w http.ResponseWriter, r *http.Request, forced string) {
	token := strings.TrimSpace(chi.URLParam(r, "token"))
	if token == "" {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxHookBodyBytes))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The request body is too large."))
		return
	}
	contentType := r.Header.Get("Content-Type")
	dialect := forced
	if dialect == "" {
		dialect = compat.Detect(contentType, raw)
	}

	var text string
	if dialect == compat.Native {
		// Camino NATIVO intacto: misma decodificación estricta y mismos errores que antes.
		var req hookPostRequest
		r.Body = io.NopCloser(bytes.NewReader(raw))
		if err := httpx.DecodeJSON(w, r, &req); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		text = req.Text
	} else if text, err = compat.Translate(dialect, contentType, raw); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The webhook body could not be parsed."))
		return
	}

	if err := ih.publishIncoming(r.Context(), token, text); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	// Los clientes de Slack/Teams comprueban el cuerpo literal («ok» / «1») y marcan
	// la entrega como fallida si no lo ven; el nativo mantiene su JSON.
	if ct, ack := compat.Ack(dialect); ack != "" {
		w.Header().Set("Content-Type", ct)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, ack)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"ok": true})
}

// publishIncoming publica por el servicio (o por la costura de test).
func (ih *intHandlers) publishIncoming(ctx context.Context, token, text string) error {
	if ih.postIncomingFn != nil {
		return ih.postIncomingFn(ctx, token, text)
	}
	_, err := ih.svc.PostViaIncomingWebhook(ctx, token, text)
	return err
}

// hookRateKey deriva la clave de rate-limit del endpoint público: hash del token
// (prefijo, para no meter el secreto en la tabla de límites) + IP del cliente.
func hookRateKey(r *http.Request) string {
	token := strings.TrimSpace(chi.URLParam(r, "token"))
	digest := domain.HashToken(token)
	if len(digest) > 16 {
		digest = digest[:16]
	}
	return "hook:" + digest + ":" + clientIP(r)
}

// clientIP deriva la IP del cliente (respeta X-Forwarded-For del proxy de confianza),
// para la clave de rate-limit por IP del endpoint público.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if xr := r.Header.Get("X-Real-Ip"); xr != "" {
		return xr
	}
	return r.RemoteAddr
}

// ---------------------------------------------------------------------------
// Auth de BOT: Bearer bot_… ⇒ identidad bot:<id> + tenant. Alcance MÍNIMO: se monta
// solo sobre la ruta de publicación de mensajes.
// ---------------------------------------------------------------------------

// botAuth valida un token Bearer de bot y fija la identidad sintética bot:<id> y el
// tenant en el contexto, de modo que los handlers/store existentes (PostMessage,
// requireWriteAccess sobre channel_members) funcionen sin cambios. NO acepta JWT: esta
// ruta es exclusiva de bots.
func (ih *intHandlers) botAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := bearerFromHeader(r)
		if tok == "" || !strings.HasPrefix(tok, domain.BotTokenPrefix) {
			httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("Missing bot token."))
			return
		}
		bot, err := ih.svc.BotFromToken(r.Context(), tok)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("Invalid bot token."))
			return
		}
		claims := authn.Claims{Subject: domain.BotAuthorSub(bot.ID), Name: bot.Name, Tenant: bot.Tenant}
		ctx := authn.Into(r.Context(), claims)
		ctx = tenancy.Into(ctx, tenancy.Tenant{Slug: bot.Tenant})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ---------------------------------------------------------------------------
// Gestión (autenticada con JWT; admin del canal/workspace). Mutaciones por POST/DELETE.
// ---------------------------------------------------------------------------

type createIncomingRequest struct {
	Name string `json:"name,omitempty"`
	Icon string `json:"icon,omitempty"`
}

// CreateIncoming crea un webhook entrante en el canal (admin). Devuelve el token en
// CLARO una única vez.
func (ih *intHandlers) CreateIncoming(w http.ResponseWriter, r *http.Request) {
	channelID, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req createIncomingRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	hook, token, err := ih.svc.CreateIncomingWebhook(r.Context(), channelID, actor, req.Name, req.Icon)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, ih.secretoEntrante(hook, token))
}

// ListIncoming lista los webhooks entrantes de un canal (admin).
func (ih *intHandlers) ListIncoming(w http.ResponseWriter, r *http.Request) {
	channelID, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	hooks, err := ih.svc.ListIncomingWebhooks(r.Context(), channelID, actor)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if hooks == nil {
		hooks = []domain.IncomingWebhook{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"webhooks": hooks})
}

// DeleteIncoming borra un webhook entrante (admin de su canal).
func (ih *intHandlers) DeleteIncoming(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	if err := ih.svc.DeleteIncomingWebhook(r.Context(), id, actor); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// RotateIncoming rota el token de un webhook entrante (admin). Devuelve el nuevo token.
func (ih *intHandlers) RotateIncoming(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	hook, token, err := ih.svc.RotateIncomingWebhook(r.Context(), id, actor)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, ih.secretoEntrante(hook, token))
}

type createOutgoingRequest struct {
	ChannelID    string   `json:"channel_id,omitempty"`
	CallbackURL  string   `json:"callback_url"`
	TriggerWords []string `json:"trigger_words,omitempty"`
	// Dialect: "sytue" (por defecto), "slack" para hablar con bots escritos para los
	// outgoing webhooks de Slack (POST form-urlencoded + respuesta {text}) o "teams"
	// para los escritos contra la actividad de Bot Framework.
	Dialect string `json:"dialect,omitempty"`
}

// CreateOutgoing crea un webhook saliente (admin del canal, o del workspace si es
// tenant-wide). El token de verificación y el secreto de firma viajan en el objeto
// devuelto: el segundo hay que pegarlo en el bot para que valide el origen.
func (ih *intHandlers) CreateOutgoing(w http.ResponseWriter, r *http.Request) {
	var req createOutgoingRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var channelID *uuid.UUID
	if c := strings.TrimSpace(req.ChannelID); c != "" {
		parsed, perr := uuid.Parse(c)
		if perr != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid channel_id."))
			return
		}
		channelID = &parsed
	}
	actor, _ := caller(r)
	hook, err := ih.svc.CreateOutgoingWebhook(r.Context(), channelID, actor, strings.TrimSpace(req.CallbackURL), req.TriggerWords, req.Dialect)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"webhook": hook})
}

// ListOutgoing lista los salientes que el actor puede gestionar.
func (ih *intHandlers) ListOutgoing(w http.ResponseWriter, r *http.Request) {
	actor, _ := caller(r)
	hooks, err := ih.svc.ListOutgoingWebhooks(r.Context(), actor)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if hooks == nil {
		hooks = []domain.OutgoingWebhook{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"webhooks": hooks})
}

// DeleteOutgoing borra un saliente (admin de su canal, o del workspace).
func (ih *intHandlers) DeleteOutgoing(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	if err := ih.svc.DeleteOutgoingWebhook(r.Context(), id, actor); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

type createBotRequest struct {
	Name string `json:"name"`
}

// CreateBot crea un bot del tenant (admin del workspace). Devuelve su token Bearer en
// CLARO una única vez.
func (ih *intHandlers) CreateBot(w http.ResponseWriter, r *http.Request) {
	var req createBotRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	bot, token, err := ih.svc.CreateBot(r.Context(), req.Name, actor)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"bot": bot, "token": token})
}

// ListBots lista los bots del tenant (admin del workspace).
func (ih *intHandlers) ListBots(w http.ResponseWriter, r *http.Request) {
	actor, _ := caller(r)
	bots, err := ih.svc.ListBots(r.Context(), actor)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if bots == nil {
		bots = []domain.Bot{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"bots": bots})
}

// DeleteBot borra un bot (su dueño o admin del workspace).
func (ih *intHandlers) DeleteBot(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	if err := ih.svc.DeleteBot(r.Context(), id, actor); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// AddBotChannel añade un bot a un canal para que pueda publicar (admin del canal).
func (ih *intHandlers) AddBotChannel(w http.ResponseWriter, r *http.Request) {
	botID, err := parseID(r, "botID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	channelID, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	if err := ih.svc.AddBotToChannel(r.Context(), botID, channelID, actor); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// RemoveBotChannel quita a un bot de un canal (admin del canal).
func (ih *intHandlers) RemoveBotChannel(w http.ResponseWriter, r *http.Request) {
	botID, err := parseID(r, "botID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	channelID, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	if err := ih.svc.RemoveBotFromChannel(r.Context(), botID, channelID, actor); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}
