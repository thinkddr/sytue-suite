// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/compat"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// Webhooks SALIENTES (Ola 15): un consumidor del outbox (events.Dispatcher, en
// cmd/chat) invoca OutgoingWebhookHandler por cada mensaje de USUARIO. Si casa un
// webhook saliente, se hace un POST **SSRF-safe** al callback; si la respuesta trae
// {text}, se publica de vuelta en el canal como autor hook:<id> (badge BOT).
//
// SSRF — DOBLE BARRERA (defensa en profundidad):
//  1. validateOutgoingURL: antes de la petición, valida esquema http/https y RESUELVE
//     el host; rechaza si CUALQUIER IP resuelta es privada/loopback/link-local/ULA/
//     CGNAT/multicast/unspecified.
//  2. dialer: en el momento de conectar, vuelve a resolver y valida, y conecta a la IP
//     EXACTA validada (cierra la ventana TOCTOU / DNS-rebinding entre (1) y el connect).
//
// Además: sin proxy (no exfiltrar por env), redirects PROHIBIDOS (ningún salto a
// privadas), timeout corto y cap del cuerpo de respuesta. Nunca se registra el token.

const (
	outgoingTimeout      = 5 * time.Second
	outgoingMaxRespBytes = 64 << 10 // cap del cuerpo de la respuesta del callback
	outgoingAttempts     = 2        // reintentos acotados por webhook (idempotente)
	outgoingRetryDelay   = 300 * time.Millisecond
)

// ssrfSafeTransport construye el *http.Transport con la barrera 2 (el dialer que
// re-resuelve, valida TODAS las IPs y conecta a la ya validada). Sin proxy (no
// exfiltrar el destino por HTTP(S)_PROXY). Lo comparten el cliente de webhooks
// salientes (sin redirects) y el de unfurl (redirects acotados que re-validan cada
// salto por este mismo dialer). Centralizar el dialer mantiene el denylist SSRF en
// UN solo sitio.
func ssrfSafeTransport() *http.Transport {
	base := &net.Dialer{Timeout: outgoingTimeout, KeepAlive: outgoingTimeout}
	return &http.Transport{
		Proxy:                 nil, // sin proxy: no exfiltrar destino por HTTP(S)_PROXY
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   outgoingTimeout,
		ExpectContinueTimeout: time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("ssrf: sin IPs para %q", host)
			}
			for _, ipa := range ips {
				if ssrfhttp.IsBlockedIP(ipa.IP) {
					return nil, fmt.Errorf("ssrf: destino no permitido")
				}
			}
			// Conectar a la IP concreta ya validada (no re-resolver por SO).
			return base.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
	}
}

// ssrfSafeClient construye el cliente HTTP de los webhooks salientes con la barrera
// del dialer (barrera 2) y sin seguir redirects.
func ssrfSafeClient() *http.Client {
	return &http.Client{
		Timeout: outgoingTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("ssrf: redirects no permitidos")
		},
		Transport: ssrfSafeTransport(),
	}
}

// validateOutgoingURL es la barrera 1: esquema http/https y todas las IPs del host
// públicas. Se ejecuta ANTES de la petición.
func validateOutgoingURL(ctx context.Context, rawURL string) error {
	if !domain.ValidCallbackURL(rawURL) {
		return errors.New("ssrf: URL inválida")
	}
	host := hostOf(rawURL)
	if host == "" {
		return errors.New("ssrf: sin host")
	}
	// Un literal IP se valida directo; un nombre se resuelve.
	if ip := net.ParseIP(host); ip != nil {
		if ssrfhttp.IsBlockedIP(ip) {
			return errors.New("ssrf: IP no permitida")
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, outgoingTimeout)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("ssrf: no se resuelve el host: %w", err)
	}
	if len(ips) == 0 {
		return errors.New("ssrf: host sin IPs")
	}
	for _, ipa := range ips {
		if ssrfhttp.IsBlockedIP(ipa.IP) {
			return errors.New("ssrf: destino no permitido")
		}
	}
	return nil
}

// hostOf extrae el hostname de una URL ya validada por ValidCallbackURL.
func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// outgoingPayload es la vista mínima del mensaje que viaja en el evento (el `data`
// del sobre es un domain.Message serializado).
type outgoingPayload struct {
	ID         uuid.UUID `json:"id"`
	ChannelID  uuid.UUID `json:"channel_id"`
	AuthorSub  string    `json:"author_sub"`
	AuthorName string    `json:"author_name"`
	Body       string    `json:"body"`
	Type       string    `json:"type"`
	CreatedAt  time.Time `json:"created_at"`
}

// outgoingRequest es el cuerpo JSON enviado al callback (estilo Mattermost). El token
// de verificación va también en la cabecera. Los campos `json:"-"` NO viajan en el
// cuerpo nativo (que se mantiene byte a byte como antes): solo alimentan el dialecto
// Slack, que es form-urlencoded.
type outgoingRequest struct {
	Token        string   `json:"token"`
	ChannelID    string   `json:"channel_id"`
	MessageID    string   `json:"message_id"`
	UserID       string   `json:"user_id"`
	UserName     string   `json:"user_name"`
	Text         string   `json:"text"`
	Timestamp    int64    `json:"timestamp"`
	TriggerWords []string `json:"trigger_words,omitempty"`
	Tenant       string   `json:"-"`
	TriggerWord  string   `json:"-"`
}

// OutgoingWebhookHandler es el handler del outbox para los webhooks salientes. Ignora
// todo lo que no sea un mensaje de USUARIO recién creado; nunca re-dispara sobre
// mensajes de integración (hook:/bot:) para evitar bucles. Best-effort por webhook:
// un fallo de POST NO propaga error (no re-fan-out del evento) — solo un fallo al
// LEER los salientes (antes de cualquier POST) reintenta el evento (idempotente).
func (s *Service) OutgoingWebhookHandler() events.Handler {
	return func(ctx context.Context, ev events.Event) error {
		if ev.Channel != EventChannel || ev.Type != EventMessageCreated || ev.Tenant == "" {
			return nil
		}
		var env struct {
			ChannelID uuid.UUID       `json:"channel_id"`
			Data      outgoingPayload `json:"data"`
		}
		if err := json.Unmarshal(ev.Payload, &env); err != nil {
			return nil //nolint:nilerr // payload corrupto: descartar, no reintentar en bucle
		}
		msg := env.Data
		if msg.ChannelID == uuid.Nil {
			msg.ChannelID = env.ChannelID
		}
		if msg.ChannelID == uuid.Nil {
			return nil
		}
		// No re-disparar sobre integraciones (evita bucles) ni sobre tarjetas de sistema.
		if domain.IsBotAuthor(msg.AuthorSub) || (msg.Type != "" && msg.Type != domain.MessageTypeUser) {
			return nil
		}
		tctx := tenancy.Into(ctx, tenancy.Tenant{Slug: ev.Tenant})
		hooks, err := s.pstore.OutgoingWebhooksForChannel(tctx, msg.ChannelID)
		if err != nil {
			return err // transitorio, ANTES de cualquier POST: reintentar el evento es seguro
		}
		for _, w := range hooks {
			// Content-based triggers cannot run on private conversation bodies.
			// Existing trigger hooks remain inactive until a client-side design exists.
			if len(w.TriggerWords) == 0 && w.ChannelID != nil && *w.ChannelID == msg.ChannelID {
				s.deliverOutgoing(tctx, w, msg)
			}
		}
		return nil
	}
}

// deliverOutgoing sends only a notice. Provider responses never become messages
// through a server-side plaintext path.
func (s *Service) deliverOutgoing(ctx context.Context, w domain.OutgoingWebhook, msg outgoingPayload) {
	request, err := s.outgoingNoticeRequest(ctx, w, msg)
	if err != nil {
		log.From(ctx).Warn("webhook saliente: notification destination unavailable", "webhook_id", w.ID)
		return
	}
	if _, err := s.postOutgoing(ctx, w, request); err != nil {
		log.From(ctx).Warn("webhook saliente: entrega fallida", "webhook_id", w.ID, "error", err)
	}
}

func (s *Service) outgoingNoticeRequest(ctx context.Context, w domain.OutgoingWebhook, msg outgoingPayload) (outgoingRequest, error) {
	notice, err := s.externalMessageNotice(ctx, msg.ChannelID, msg.ID)
	if err != nil {
		return outgoingRequest{}, err
	}
	return outgoingRequest{
		Token: w.Token, ChannelID: msg.ChannelID.String(), MessageID: msg.ID.String(),
		UserName: "Sytue Rooms", Text: string(notice), Tenant: w.Tenant,
	}, nil
}

// postOutgoing valida (barrera 1) y hace el POST con reintentos acotados. Devuelve el
// `text` de la respuesta (o vacío).
func (s *Service) postOutgoing(ctx context.Context, w domain.OutgoingWebhook, reqBody outgoingRequest) (string, error) {
	if err := validateOutgoingURL(ctx, w.CallbackURL); err != nil {
		return "", err
	}
	payload, err := outgoingBody(w.Dialect, reqBody)
	if err != nil {
		return "", err
	}
	var lastErr error
	for attempt := 0; attempt < outgoingAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(outgoingRetryDelay):
			}
		}
		text, err := s.doOutgoing(ctx, w, payload)
		if err == nil {
			return text, nil
		}
		lastErr = err
	}
	return "", lastErr
}

// outgoingBody serializa el cuerpo del POST según el dialecto del webhook: JSON
// nativo (por defecto, idéntico al de siempre), el form-urlencoded de un outgoing
// webhook de Slack o la actividad de Teams — para que un bot escrito para cualquiera
// de los dos funcione sin tocarlo.
func outgoingBody(dialect string, r outgoingRequest) ([]byte, error) {
	switch dialect {
	case compat.Slack:
		// ponytail: channel_name = id del canal — el nombre exigiría una consulta extra
		// por entrega (el lookup por id pide membresía). Cambiar si algún bot lo usa.
		return []byte(compat.SlackOutgoing{
			Token:       r.Token,
			TeamDomain:  r.Tenant,
			ChannelID:   r.ChannelID,
			ChannelName: r.ChannelID,
			UserID:      r.UserID,
			UserName:    r.UserName,
			Text:        r.Text,
			TriggerWord: r.TriggerWord,
			Timestamp:   r.Timestamp,
		}.Form()), nil
	case compat.Teams:
		return compat.TeamsOutgoing{
			ID:        r.MessageID,
			Text:      r.Text,
			UserID:    r.UserID,
			UserName:  r.UserName,
			ChannelID: r.ChannelID,
			Tenant:    r.Tenant,
			Timestamp: time.Unix(r.Timestamp, 0),
		}.JSON()
	default:
		return json.Marshal(r)
	}
}

// doOutgoing makes one SSRF-safe delivery and discards the response content.
func (s *Service) doOutgoing(ctx context.Context, w domain.OutgoingWebhook, payload []byte) (string, error) {
	client := s.outgoing
	if client == nil {
		client = ssrfSafeClient()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.CallbackURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	contentType := "application/json"
	if w.Dialect == compat.Slack {
		contentType = "application/x-www-form-urlencoded"
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Sytue-Rooms-Webhook/1")
	// Token de verificación para que el receptor confirme el origen (no es un secreto
	// nuestro; se envía en claro y por eso NO se hashea en reposo).
	req.Header.Set("X-Sytue-Webhook-Token", w.Token)
	signOutgoing(req, w, payload)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, outgoingMaxRespBytes))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("callback status %d", resp.StatusCode)
	}
	return "", nil
}

// signOutgoing firma la entrega con el secreto del webhook, en el formato que el
// receptor ya sabe comprobar.
//
// Es lo que separa «este POST viene de Rooms» de «alguien vio un POST de Rooms»: el
// token va en claro en cada entrega, así que no prueba el origen de la siguiente. Un
// saliente sin secreto (los creados antes de la migración, si no se rellenaron) sale
// sin firmar: degradar es preferible a firmar con clave vacía, que se validaría sola.
func signOutgoing(req *http.Request, w domain.OutgoingWebhook, payload []byte) {
	if w.SigningSecret == "" {
		return
	}
	switch w.Dialect {
	case compat.Teams:
		req.Header.Set(compat.TeamsAuthHeader, compat.SignTeams(w.SigningSecret, payload))
	default:
		// Slack y nativo comparten formato: un bot de Bolt puesto delante de un
		// saliente nativo también puede validarlo.
		ts := time.Now().Unix()
		req.Header.Set(compat.SlackTimestampHeader, strconv.FormatInt(ts, 10))
		req.Header.Set(compat.SlackSignatureHeader, compat.SignSlack(w.SigningSecret, ts, payload))
	}
}
