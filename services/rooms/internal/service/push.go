// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/cache"
	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// Web Push — ENVIADOR (Ola 18). Cuando el outbox produce un mensaje de usuario que menciona
// a alguien o es un DM, se le envía un Web Push cifrado (webpush.go) a sus suscripciones,
// para avisarle con la app CERRADA (el Service Worker lo recibe y muestra la notificación).
// No BLOQUEA el post: corre en el consumidor exactly-once del outbox (main.go), como los
// webhooks salientes (Ola 15). Best-effort: los fallos de envío se registran/limpian pero
// nunca reintentan el evento (no re-disparar webhooks). Respeta silenciado/no-molestar del
// canal en la consulta (store.PushTargetsForMessage); el filtrado FINO por ventana horaria
// «No molestar» queda en el cliente (el servidor no conoce la zona horaria del usuario).

const (
	// pushTimeout acota cada POST al servicio de push (no colgar el lote del outbox).
	pushTimeout = 10 * time.Second
	// pushMaxTargets acota las suscripciones por mensaje. Los destinatarios son
	// @mencionados + peer de DM (pocos); el tope es una red de seguridad.
	// ponytail: cap fijo; un @channel a un canal enorme iría por una cola dedicada.
	pushMaxTargets = 50
)

// pushStore define la superficie de persistencia consumida por el enviador de push y avisos de Hub.
type pushStore interface {
	PushTargetsForMessage(ctx context.Context, channelID uuid.UUID, authorSub string, mentions []string) ([]domain.PushTarget, error)
	DeletePushSubscription(ctx context.Context, subject, endpoint string) error
	APNSTargetsForMessage(ctx context.Context, channelID uuid.UUID, authorSub string, mentions []string) ([]domain.APNSTarget, error)
	DeleteAPNSDevice(ctx context.Context, subject, token string) error
	NotificationRecipientsForMessage(ctx context.Context, channelID uuid.UUID, authorSub string, mentions []string) ([]domain.NotificationRecipient, error)
	SavePushSubscription(ctx context.Context, subject string, sub domain.PushSubscription) error
	SaveAPNSDevice(ctx context.Context, subject, token string) error
}

func (s *Service) pushStore() pushStore {
	if s.pushSt != nil {
		return s.pushSt
	}
	if s.pstore != nil {
		return s.pstore
	}
	return nil
}

// errPushGone marca que el servicio de push reportó la suscripción como caduca (404/410):
// hay que borrarla.
var errPushGone = errors.New("push: suscripción caduca")

// PushSender guarda el par VAPID y el cliente HTTP SSRF-safe para entregar Web Push.
type PushSender struct {
	vapid  *vapidKeys
	client *http.Client
}

// NewPushSender construye el enviador desde la config. Sin claves VAPID => (nil, nil):
// push deshabilitado (dev/tests sin push), sin error. Con solo una de las dos => error
// explícito. Reusa el cliente SSRF-safe de los webhooks salientes (sin redirects, timeout,
// denylist de IPs privadas: un endpoint de push malicioso no puede sondear la red interna).
func NewPushSender(cfg config.Config) (*PushSender, error) {
	if cfg.VAPIDPublic == "" && cfg.VAPIDPrivate == "" {
		return nil, nil //nolint:nilnil // push deshabilitado: ausencia intencional, no error.
	}
	if cfg.VAPIDPublic == "" || cfg.VAPIDPrivate == "" {
		return nil, errors.New("push: define SYTUE_VAPID_PUBLIC y SYTUE_VAPID_PRIVATE juntas (o ninguna)")
	}
	v, err := parseVAPIDKeys(cfg.VAPIDPublic, cfg.VAPIDPrivate, cfg.VAPIDSubject)
	if err != nil {
		return nil, err
	}
	return &PushSender{vapid: v, client: ssrfSafeClient()}, nil
}

// SetPushSender inyecta el enviador de push (lo construye main.go con logging). nil =
// deshabilitado.
func (s *Service) SetPushSender(p *PushSender) { s.push = p }

// SetEfimero conecta la caché de Dragonfly. Opcional, como el emisor de push:
// sin ella el servicio funciona igual, solo que regenera las miniaturas de las
// vistas previas en cada petición en vez de reusarlas.
func (s *Service) SetEfimero(c *cache.Client) { s.efimero = c }

// PushEnabled indica si el Web Push está configurado (hay par VAPID).
func (s *Service) PushEnabled() bool { return s.push != nil }

// VAPIDPublicKey devuelve la clave pública VAPID en base64url (applicationServerKey del
// navegador). Vacía si el push no está configurado.
func (s *Service) VAPIDPublicKey() string {
	if s.push == nil {
		return ""
	}
	return s.push.vapid.pubB64
}

// SubscribePush registra (upsert) una suscripción Web Push del usuario. Valida la forma y el
// endpoint (https + no privado, misma barrera SSRF que los webhooks salientes) antes de
// persistir.
func (s *Service) SubscribePush(ctx context.Context, subject string, sub domain.PushSubscription) error {
	if !domain.ValidPushSubscription(sub) {
		return domain.ErrInvalidInput
	}
	// Claves decodificables al tamaño esperado (auth 16 bytes, p256dh punto de 65 bytes).
	if auth, err := b64urlDecode(sub.Auth); err != nil || len(auth) != 16 {
		return domain.ErrInvalidInput
	}
	if p256dh, err := b64urlDecode(sub.P256dh); err != nil || len(p256dh) != 65 {
		return domain.ErrInvalidInput
	}
	if err := validatePushEndpoint(ctx, sub.Endpoint); err != nil {
		return domain.ErrInvalidInput
	}
	if ps := s.pushStore(); ps != nil {
		return ps.SavePushSubscription(ctx, subject, sub)
	}
	return nil
}

// UnsubscribePush borra la suscripción (subject, endpoint) del usuario.
func (s *Service) UnsubscribePush(ctx context.Context, subject, endpoint string) error {
	if endpoint == "" {
		return domain.ErrInvalidInput
	}
	if ps := s.pushStore(); ps != nil {
		return ps.DeletePushSubscription(ctx, subject, endpoint)
	}
	return nil
}

// MessageCreatedHandler es el handler del consumidor exactly-once del outbox: encadena los
// webhooks salientes (Ola 15) y el Web Push (Ola 18) sobre cada mensaje creado. Corre los
// salientes PRIMERO: solo ellos reintentan el evento (fallo transitorio ANTES de cualquier
// efecto), así el push nunca se re-ejecuta por un reintento suyo (sin duplicar avisos ni
// re-POSTear webhooks). El push es best-effort y jamás propaga error.
func (s *Service) MessageCreatedHandler() events.Handler {
	outgoing := s.OutgoingWebhookHandler()
	return func(ctx context.Context, ev events.Event) error {
		if err := outgoing(ctx, ev); err != nil {
			return err // reintenta el evento; el push aún NO ha corrido
		}
		// Puentes, WhatsApp y push, best-effort: un fallo aquí no reintenta el evento, que
		// volvería a entregar los salientes de arriba.
		s.deliverBridgesForEvent(ctx, ev)
		s.deliverWhatsAppForEvent(ctx, ev)
		s.deliverPushForEvent(ctx, ev)
		return nil
	}
}

// pushPayload es la vista mínima del mensaje que el enviador necesita del evento del outbox.
type pushPayload struct {
	ID         uuid.UUID `json:"id"`
	ChannelID  uuid.UUID `json:"channel_id"`
	AuthorSub  string    `json:"author_sub"`
	AuthorName string    `json:"author_name"`
	Body       string    `json:"body"`
	Type       string    `json:"type"`
	Mentions   []string  `json:"mentions"`
}

// deliverPushForEvent envía Web Push / APNs / Hub notifications por un mensaje recién creado a sus @mencionados y (si
// DM) al otro miembro. Best-effort: nunca devuelve error.
func (s *Service) deliverPushForEvent(ctx context.Context, ev events.Event) {
	if s.push == nil && s.apns == nil && s.hubNotifier == nil {
		return
	}
	if ev.Channel != EventChannel || ev.Type != EventMessageCreated || ev.Tenant == "" {
		return
	}
	var env struct {
		ChannelID uuid.UUID   `json:"channel_id"`
		Data      pushPayload `json:"data"`
	}
	if err := json.Unmarshal(ev.Payload, &env); err != nil {
		return
	}
	msg := env.Data
	if msg.ChannelID == uuid.Nil {
		msg.ChannelID = env.ChannelID
	}
	if msg.ChannelID == uuid.Nil {
		return
	}
	// Ni integraciones (hook:/bot:) ni tarjetas de sistema disparan push.
	if domain.IsBotAuthor(msg.AuthorSub) || (msg.Type != "" && msg.Type != domain.MessageTypeUser) {
		return
	}
	tctx := tenancy.Into(ctx, tenancy.Tenant{Slug: ev.Tenant})
	if s.push != nil {
		s.entregarWebPush(tctx, msg)
	}
	if s.apns != nil {
		s.despertarIOS(tctx, msg)
	}
	if s.hubNotifier != nil {
		s.entregarHubNotification(tctx, ev.Tenant, msg)
	}
}

// entregarHubNotification emite notificaciones unificadas hacia Sytue Hub. Best-effort.
func (s *Service) entregarHubNotification(ctx context.Context, tenant string, msg pushPayload) {
	ps := s.pushStore()
	if s.hubNotifier == nil || ps == nil {
		return
	}
	recipients, err := ps.NotificationRecipientsForMessage(ctx, msg.ChannelID, msg.AuthorSub, msg.Mentions)
	if err != nil {
		log.From(ctx).Warn("hubnotify: no se pudieron resolver los destinatarios", "error", err)
		return
	}
	if len(recipients) == 0 {
		return
	}
	if len(recipients) > pushMaxTargets {
		recipients = recipients[:pushMaxTargets]
	}

	for _, r := range recipients {
		s.hubNotifier.SendAsync(ctx, avisoDeMensaje(s.cfg.PublicURL, tenant, msg, r))
	}
}

// avisoDeMensaje es el aviso del Hub de un mensaje a uno de sus destinatarios: el
// mismo en vivo que en la reconstrucción de avisos (AvisosHistoricos).
func avisoDeMensaje(publicURL, tenant string, msg pushPayload, r domain.NotificationRecipient) hubnotify.Notification {
	return hubnotify.Notification{
		Tenant:  tenant,
		Subject: r.Subject,
		Source:  "rooms",
		Type:    "rooms.message",
		Level:   "info",
		Title:   "Sytue Rooms",
		Body:    hubnotify.TextoAvisoRooms,
		URL:     hubnotify.EnlaceMensaje(publicURL, tenant, msg.ChannelID.String(), msg.ID.String()),
		Params:  map[string]string{"actor": "Sytue Rooms"},
	}
}

// entregarWebPush avisa a los navegadores suscritos (RFC 8291). Best-effort.
func (s *Service) entregarWebPush(ctx context.Context, msg pushPayload) {
	ps := s.pushStore()
	if ps == nil {
		return
	}
	targets, err := ps.PushTargetsForMessage(ctx, msg.ChannelID, msg.AuthorSub, msg.Mentions)
	if err != nil {
		log.From(ctx).Warn("push: no se pudieron leer las suscripciones", "error", err)
		return
	}
	if len(targets) == 0 {
		return
	}
	if len(targets) > pushMaxTargets {
		targets = targets[:pushMaxTargets]
	}
	payload := pushMessageJSON(msg)
	for _, t := range targets {
		if err := s.sendOnePush(ctx, t, payload); err != nil {
			if errors.Is(err, errPushGone) {
				if derr := ps.DeletePushSubscription(ctx, t.Subject, t.Endpoint); derr != nil {
					log.From(ctx).Warn("push: no se pudo limpiar suscripción caduca", "error", derr)
				}
				continue
			}
			log.From(ctx).Warn("push: entrega fallida", "error", err)
		}
	}
}

// despertarIOS manda el aviso VACÍO a los aparatos de iOS de los mismos destinatarios
// (ver apns.go: a Apple no le llega ni el texto ni el autor). Best-effort, como el Web
// Push: un fallo aquí nunca tumba el evento del outbox.
func (s *Service) despertarIOS(tctx context.Context, msg pushPayload) {
	ps := s.pushStore()
	if ps == nil {
		return
	}
	targets, err := ps.APNSTargetsForMessage(tctx, msg.ChannelID, msg.AuthorSub, msg.Mentions)
	if err != nil {
		log.From(tctx).Warn("apns: no se pudieron leer los aparatos", "error", err)
		return
	}
	if len(targets) > apnsMaxTargets {
		targets = targets[:apnsMaxTargets]
	}
	for _, t := range targets {
		if err := s.sendOneAPNS(tctx, t); err != nil {
			if errors.Is(err, errPushGone) {
				if derr := ps.DeleteAPNSDevice(tctx, t.Subject, t.Token); derr != nil {
					log.From(tctx).Warn("apns: no se pudo limpiar el token caduco", "error", derr)
				}
				continue
			}
			log.From(tctx).Warn("apns: no se pudo despertar el aparato", "error", err)
		}
	}
}

// pushMessageJSON never exposes authors, excerpts or encrypted payloads to the
// notification service or operating system. Content is opened inside Rooms.
func pushMessageJSON(msg pushPayload) []byte {
	// Deep-link que el ChatApp ya sabe abrir (`?channel=&message=`): salta al mensaje.
	q := url.Values{"channel": {msg.ChannelID.String()}, "message": {msg.ID.String()}}
	payload, _ := json.Marshal(map[string]string{
		"title": "Sytue Rooms",
		"body":  hubnotify.TextoAvisoRooms,
		"url":   "/?" + q.Encode(),
		"tag":   "sytue-rooms-" + msg.ChannelID.String(),
	})
	return payload
}

// sendOnePush cifra el payload para una suscripción y hace el POST al servicio de push.
// Devuelve errPushGone si el endpoint reporta la suscripción caduca (404/410).
func (s *Service) sendOnePush(ctx context.Context, t domain.PushTarget, payload []byte) error {
	if err := validatePushEndpoint(ctx, t.Endpoint); err != nil {
		return err
	}
	p256dh, err := b64urlDecode(t.P256dh)
	if err != nil {
		return fmt.Errorf("push: p256dh inválida: %w", err)
	}
	auth, err := b64urlDecode(t.Auth)
	if err != nil {
		return fmt.Errorf("push: auth inválida: %w", err)
	}
	body, err := encryptPush(rand.Reader, p256dh, auth, payload)
	if err != nil {
		return err
	}
	authz, err := s.push.vapid.authorizationHeader(t.Endpoint)
	if err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, pushTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, t.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("TTL", "86400")
	req.Header.Set("Urgency", "normal")
	req.Header.Set("Authorization", authz)

	resp, err := s.push.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	return classifyPushStatus(resp.StatusCode)
}

// classifyPushStatus traduce el status del servicio de push: 2xx = entregado; 404/410 =
// suscripción caduca (errPushGone, hay que borrarla); el resto = fallo (best-effort).
func classifyPushStatus(code int) error {
	switch {
	case code == http.StatusNotFound || code == http.StatusGone:
		return errPushGone
	case code >= 200 && code < 300:
		return nil
	default:
		return fmt.Errorf("push: endpoint status %d", code)
	}
}

// validatePushEndpoint exige https y reusa la barrera SSRF de los webhooks salientes
// (resuelve el host y rechaza IPs privadas/loopback/link-local): un endpoint de push
// controlado por el usuario no debe poder apuntar a la red interna.
func validatePushEndpoint(ctx context.Context, endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return errors.New("push: endpoint debe ser https")
	}
	return validateOutgoingURL(ctx, endpoint)
}
