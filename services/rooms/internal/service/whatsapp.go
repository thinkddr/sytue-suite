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
	"net/url"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// WhatsApp Business por la Cloud API de Meta. Entra por el aviso firmado de Meta
// (POST /webhooks/whatsapp/{token}) y sale por Graph con el token de la cuenta.
//
// ponytail: solo texto hacia fuera, y dentro de la ventana de 24 h que marca
// WhatsApp desde el último mensaje del cliente. Pasado ese plazo, o para escribir
// primero, WhatsApp exige una plantilla aprobada; Meta lo rechaza y el canal
// recibe el aviso con su explicación. Plantillas y adjuntos, cuando se pidan.

// El alta manual (POST /whatsapp, pegando token y secreto de app) se retiró: los
// números entran por el consentimiento de Meta (whatsapp_consent.go). Las cuentas
// ya guardadas siguen funcionando igual.

// ListWhatsApp lista los números conectados (admin del workspace).
func (s *Service) ListWhatsApp(ctx context.Context, actor string) ([]domain.WhatsAppAccount, error) {
	return s.pstore.ListWhatsAppAccounts(ctx, actor)
}

// DisconnectWhatsApp desconecta un número (admin del workspace).
func (s *Service) DisconnectWhatsApp(ctx context.Context, id uuid.UUID, actor string) error {
	return s.pstore.DeleteWhatsAppAccount(ctx, id, actor)
}

// VerifyWhatsAppSubscription responde a la comprobación de Meta al configurar el
// aviso: el «token de verificación» que se pega en Meta es el mismo de la URL.
func (s *Service) VerifyWhatsAppSubscription(ctx context.Context, token, mode, verifyToken string) bool {
	if mode != "subscribe" || token == "" || verifyToken != token {
		return false
	}
	_, err := s.pstore.LookupWhatsAppAccountByTokenHash(ctx, domain.HashToken(token))
	return err == nil
}

// ReceiveWhatsApp publishes generic activity notices without importing content.
// ErrNotFound si el token
// no existe y ErrForbidden si la firma no casa; cualquier otro error pide a Meta que
// reintente.
func (s *Service) ReceiveWhatsApp(ctx context.Context, token string, body []byte, signature string) error {
	a, err := s.pstore.LookupWhatsAppAccountByTokenHash(ctx, domain.HashToken(token))
	if err != nil {
		return err
	}
	if !a.Ready {
		return domain.ErrNotFound
	}
	if !domain.ValidWhatsAppSignature(a.AppSecret, body, signature) {
		return domain.ErrForbidden
	}
	mensajes, err := domain.ParseWhatsAppWebhook(body)
	if err != nil {
		return domain.ErrInvalidInput
	}
	tctx := tenancy.Into(ctx, tenancy.Tenant{Slug: a.Tenant})
	for _, m := range mensajes {
		// Una misma app de Meta puede llevar varios números: solo el de esta cuenta.
		if m.PhoneNumberID != "" && m.PhoneNumberID != a.PhoneNumberID {
			continue
		}
		if m.MessageID != "" {
			visto, err := s.pstore.WhatsAppSeen(tctx, m.MessageID)
			if err != nil {
				return err
			}
			if visto {
				continue
			}
		}
		chat, err := s.pstore.EnsureWhatsAppChat(tctx, a, m.WaID, "WhatsApp", m.At)
		if err != nil {
			return err
		}
		msg, err := s.pstore.CreateHookMessage(tctx, chat.ChannelID, domain.HookAuthorSub(a.ID), "WhatsApp", domain.WhatsAppInboundNotice)
		if err != nil {
			return err
		}
		s.publish(tctx, EventMessageCreated, msg.ChannelID, msg)
		if m.MessageID != "" {
			if err := s.pstore.MarkWhatsAppSeen(tctx, m.MessageID); err != nil {
				log.From(ctx).Warn("whatsapp: no se anotó el mensaje como visto", "error", err)
			}
		}
	}
	return nil
}

// deliverWhatsAppForEvent sends a generic notice and authenticated Rooms link.
// It never sends conversation content, author names or attachments to Meta.
func (s *Service) deliverWhatsAppForEvent(ctx context.Context, ev events.Event) {
	m, ok := personMessage(ev)
	if !ok {
		return
	}
	tctx := tenancy.Into(ctx, tenancy.Tenant{Slug: ev.Tenant})
	chat, a, err := s.pstore.WhatsAppChatForChannel(tctx, m.ChannelID)
	if errors.Is(err, domain.ErrNotFound) {
		return
	}
	if err != nil {
		log.From(ctx).Warn("whatsapp: no se pudo leer el canal", "channel_id", m.ChannelID, "error", err)
		return
	}
	notice, err := s.externalMessageNotice(tctx, m.ChannelID, m.ID)
	if err != nil {
		log.From(ctx).Warn("whatsapp: notification destination unavailable", "account_id", a.ID)
		return
	}
	if err := s.sendWhatsApp(tctx, a, chat.WaID, notice); err != nil {
		log.From(ctx).Warn("whatsapp: envío rechazado", "account_id", a.ID, "error", err)
		aviso, cerr := s.pstore.CreateHookMessage(tctx, m.ChannelID, domain.HookAuthorSub(a.ID), "WhatsApp", "No se pudo enviar el aviso de WhatsApp.")
		if cerr == nil {
			s.publish(tctx, EventMessageCreated, aviso.ChannelID, aviso)
		}
	}
}

// sendWhatsApp sends a notice without enabling link previews.
func (s *Service) sendWhatsApp(ctx context.Context, a domain.WhatsAppAccount, waID string, notice externalNotice) error {
	cuerpo, err := json.Marshal(map[string]any{
		"messaging_product": "whatsapp", "recipient_type": "individual", "to": waID,
		"type": "text", "text": map[string]any{"preview_url": false, "body": string(notice)},
	})
	if err != nil {
		return err
	}
	return s.graph(ctx, s.cfg.WhatsAppGraphURL+"/"+url.PathEscape(a.PhoneNumberID)+"/messages", a.AccessToken, cuerpo)
}

// graph hace un POST a Graph y devuelve solo códigos de estado o de error. El
// cuerpo de la respuesta del proveedor nunca llega a logs, errores públicos ni
// mensajes de la conversación.
func (s *Service) graph(ctx context.Context, u, accessToken string, cuerpo []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(cuerpo))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	client := s.outgoing
	if client == nil {
		client = ssrfSafeClient()
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, outgoingMaxRespBytes))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	var fallo struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &fallo) == nil && fallo.Error.Code != 0 {
		return fmt.Errorf("WhatsApp %d", fallo.Error.Code)
	}
	return fmt.Errorf("WhatsApp: HTTP %d", resp.StatusCode)
}
