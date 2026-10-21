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
	"net/http"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// External destinations receive notices and authenticated Rooms links only.
// Conversation bodies, authors and attachments never enter provider payloads.

// ErrBridgeRejected: el otro lado no aceptó el mensaje de prueba al conectar.
var ErrBridgeRejected = errors.New("el destino del puente no aceptó el mensaje de prueba")

// bridgeMessage es lo que viaja por el puente.
type bridgeMessage struct {
	ID         uuid.UUID  `json:"id"`
	ChannelID  uuid.UUID  `json:"channel_id"`
	ParentID   *uuid.UUID `json:"parent_id,omitempty"`
	AuthorSub  string     `json:"author_sub"`
	AuthorName string     `json:"author_name"`
	Body       string     `json:"body"`
	Type       string     `json:"type"`
}

// CreateBridge connects a notification destination after sending a generic
// notice. The channel name and conversation content are never shared.
func (s *Service) CreateBridge(ctx context.Context, channelID uuid.UUID, actor, platform, webhookURL string) (domain.Bridge, error) {
	webhookURL = strings.TrimSpace(webhookURL)
	if !domain.ValidBridgePlatform(platform) || !domain.ValidBridgeURL(platform, webhookURL) {
		return domain.Bridge{}, domain.ErrInvalidInput
	}
	_, err := s.pstore.ChannelForAdmin(ctx, channelID, actor)
	if err != nil {
		return domain.Bridge{}, err
	}
	prueba := bridgeMessage{ChannelID: channelID}
	if err := s.sendBridge(ctx, domain.Bridge{Platform: platform, WebhookURL: webhookURL}, prueba); err != nil {
		return domain.Bridge{}, fmt.Errorf("%w: %w", ErrBridgeRejected, err)
	}
	return s.pstore.CreateBridge(ctx, channelID, actor, platform, webhookURL)
}

// ListBridges lista los puentes de un canal (admin del canal).
func (s *Service) ListBridges(ctx context.Context, channelID uuid.UUID, actor string) ([]domain.Bridge, error) {
	return s.pstore.ListBridges(ctx, channelID, actor)
}

// DeleteBridge desconecta un puente (admin de su canal).
func (s *Service) DeleteBridge(ctx context.Context, id uuid.UUID, actor string) error {
	return s.pstore.DeleteBridge(ctx, id, actor)
}

// deliverBridgesForEvent notifica un mensaje recién creado sin copiarlo al destino.
// Best-effort: nunca devuelve error, porque reintentar el evento volvería
// a disparar los webhooks salientes que ya se entregaron.
func (s *Service) deliverBridgesForEvent(ctx context.Context, ev events.Event) {
	m, ok := personMessage(ev)
	if !ok {
		return
	}
	tctx := tenancy.Into(ctx, tenancy.Tenant{Slug: ev.Tenant})
	bridges, err := s.pstore.BridgesForChannel(tctx, m.ChannelID)
	if err != nil {
		log.From(ctx).Warn("puentes: no se pudieron leer", "channel_id", m.ChannelID, "error", err)
		return
	}
	for _, b := range bridges {
		estado := ""
		if err := s.sendBridge(tctx, b, m); err != nil {
			estado = recortarError(err.Error(), 300)
			log.From(ctx).Warn("puente: entrega fallida", "bridge_id", b.ID, "platform", b.Platform, "error", estado)
		}
		if estado != b.LastError {
			_ = s.pstore.SetBridgeError(tctx, b.ID, estado)
		}
	}
}

// personMessage lee del evento un mensaje recién creado por una PERSONA. Lo que
// entró por un webhook, un bot o WhatsApp (autor hook:/bot:) ya viene de fuera y no
// vuelve a salir: repetirlo sería el eco.
func personMessage(ev events.Event) (bridgeMessage, bool) {
	if ev.Channel != EventChannel || ev.Type != EventMessageCreated || ev.Tenant == "" {
		return bridgeMessage{}, false
	}
	var env struct {
		ChannelID uuid.UUID     `json:"channel_id"`
		Data      bridgeMessage `json:"data"`
	}
	if err := json.Unmarshal(ev.Payload, &env); err != nil {
		return bridgeMessage{}, false
	}
	m := env.Data
	if m.ChannelID == uuid.Nil {
		m.ChannelID = env.ChannelID
	}
	if m.ChannelID == uuid.Nil || domain.IsBotAuthor(m.AuthorSub) ||
		(m.Type != "" && m.Type != domain.MessageTypeUser) {
		return bridgeMessage{}, false
	}
	return m, true
}

// sendBridge valida el destino (lista blanca del proveedor + barrera SSRF) y lo entrega.
func (s *Service) sendBridge(ctx context.Context, b domain.Bridge, m bridgeMessage) error {
	notice, err := s.externalMessageNotice(ctx, m.ChannelID, m.ID)
	if err != nil {
		return err
	}
	if b.InstanceID != "" {
		if s.roomsGateway == nil {
			return ErrRoomsIntegrationUnavailable
		}
		return s.roomsGateway.send(ctx, b.InstanceID, b.RemoteID, m.ChannelID, m.ID)
	}
	if !domain.ValidBridgeURL(b.Platform, b.WebhookURL) {
		return errors.New("URL de puente no válida")
	}
	if err := validateOutgoingURL(ctx, b.WebhookURL); err != nil {
		return err
	}
	destino, cuerpo, err := bridgeRequest(b.Platform, b.WebhookURL, notice)
	if err != nil {
		return err
	}
	return s.postBridge(ctx, destino, cuerpo)
}

// bridgeRequest compone la URL y el cuerpo en el formato de cada plataforma.
type externalNotice string

func (s *Service) externalMessageNotice(ctx context.Context, channel, message uuid.UUID) (externalNotice, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil {
		return "", domain.ErrForbidden
	}
	messageID := ""
	if message != uuid.Nil {
		messageID = message.String()
	}
	notice := hubnotify.AvisoMensajeRooms(s.cfg.PublicURL, tenant.Slug, channel.String(), messageID)
	if notice == "" {
		return "", ErrRoomsIntegrationUnavailable
	}
	return externalNotice(notice), nil
}

func bridgeRequest(platform, webhookURL string, notice externalNotice) (string, []byte, error) {
	switch platform {
	case domain.BridgeSlack:
		cuerpo, err := json.Marshal(map[string]any{"text": string(notice), "unfurl_links": false, "unfurl_media": false})
		return webhookURL, cuerpo, err
	case domain.BridgeGoogleChat:
		cuerpo, err := json.Marshal(map[string]string{"text": string(notice)})
		return webhookURL, cuerpo, err
	case domain.BridgeTeams:
		// Tarjeta adaptable: la entienden los flujos de Workflows y los conectores
		// clásicos. El texto va en bloques con `wrap` para que no se corte.
		cuerpo, err := json.Marshal(map[string]any{
			"type": "message",
			"attachments": []map[string]any{{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"content": map[string]any{
					"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
					"type":    "AdaptiveCard",
					"version": "1.4",
					"body": []map[string]any{
						{"type": "TextBlock", "text": string(notice), "wrap": true},
					},
				},
			}},
		})
		return webhookURL, cuerpo, err
	}
	return "", nil, fmt.Errorf("plataforma de puente desconocida: %q", platform)
}

// postBridge hace UN POST y trata cualquier 2xx como entregado (Slack responde
// «ok», Google Chat el mensaje creado y Workflows un 202 vacío).
func (s *Service) postBridge(ctx context.Context, destino string, cuerpo []byte) error {
	client := s.outgoing
	if client == nil {
		client = ssrfSafeClient()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, destino, bytes.NewReader(cuerpo))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("User-Agent", "Sytue-Rooms-Bridge/1")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, outgoingMaxRespBytes))
		return nil
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, outgoingMaxRespBytes))
	return fmt.Errorf("HTTP %d", resp.StatusCode)
}

func recortarError(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
