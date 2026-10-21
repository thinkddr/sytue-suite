// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/compat"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// Integraciones (Ola 15): orquestación de webhooks entrantes/salientes y bots. La
// generación de tokens y su hasheo viven en domain (crypto/rand + SHA-256); el token
// en CLARO solo se devuelve una vez, al crear/rotar. Delegaciones sobre el store
// concreto (s.pstore), como los seguimientos de hilo.

// ---- Webhooks entrantes -----------------------------------------------------

// CreateIncomingWebhook crea un webhook entrante en un canal (admin del canal) y
// devuelve el token en CLARO una única vez (junto al webhook sin secreto).
func (s *Service) CreateIncomingWebhook(ctx context.Context, channelID uuid.UUID, actor, name, icon string) (domain.IncomingWebhook, string, error) {
	name = strings.TrimSpace(name)
	if name != "" && !domain.ValidName(name) {
		return domain.IncomingWebhook{}, "", domain.ErrInvalidName
	}
	plain, hash, err := domain.NewOpaqueToken()
	if err != nil {
		return domain.IncomingWebhook{}, "", err
	}
	w, err := s.pstore.CreateIncomingWebhook(ctx, channelID, actor, name, strings.TrimSpace(icon), hash)
	if err != nil {
		return domain.IncomingWebhook{}, "", err
	}
	return w, plain, nil
}

// ListIncomingWebhooks lista los webhooks entrantes de un canal (admin del canal).
func (s *Service) ListIncomingWebhooks(ctx context.Context, channelID uuid.UUID, actor string) ([]domain.IncomingWebhook, error) {
	return s.pstore.ListIncomingWebhooks(ctx, channelID, actor)
}

// DeleteIncomingWebhook borra un webhook entrante (admin de su canal).
func (s *Service) DeleteIncomingWebhook(ctx context.Context, id uuid.UUID, actor string) error {
	return s.pstore.DeleteIncomingWebhook(ctx, id, actor)
}

// RotateIncomingWebhook rota el token de un webhook entrante (admin de su canal) y
// devuelve el nuevo token en CLARO una única vez.
func (s *Service) RotateIncomingWebhook(ctx context.Context, id uuid.UUID, actor string) (domain.IncomingWebhook, string, error) {
	plain, hash, err := domain.NewOpaqueToken()
	if err != nil {
		return domain.IncomingWebhook{}, "", err
	}
	w, err := s.pstore.RotateIncomingWebhook(ctx, id, actor, hash)
	if err != nil {
		return domain.IncomingWebhook{}, "", err
	}
	return w, plain, nil
}

// PostViaIncomingWebhook publica un mensaje usando un webhook entrante: el token ES
// la autorización (sin JWT). Rehashea el token, busca el webhook CROSS-TENANT, fija
// su tenant en el contexto y publica en su canal como autor hook:<id> (badge BOT),
// emitiendo el evento que difunde el relay a la sala WS. No exige membresía; el store
// valida que el canal exista/vivo.
func (s *Service) PostViaIncomingWebhook(ctx context.Context, token, text string) (domain.Message, error) {
	if s.cfg.MatrixRequired {
		return domain.Message{}, domain.ErrEncryptedRequired
	}
	w, err := s.pstore.LookupIncomingWebhookByTokenHash(ctx, domain.HashToken(token))
	if err != nil {
		return domain.Message{}, err // ErrNotFound ⇒ token inválido
	}
	body, err := domain.NormalizeBody(text, s.cfg.MaxMessageBytes)
	if err != nil {
		return domain.Message{}, err
	}
	// Fijar el tenant del webhook para publicar bajo su RLS (el contexto público no lo trae).
	tctx := tenancy.Into(ctx, tenancy.Tenant{Slug: w.Tenant})
	name := w.Name
	if name == "" {
		name = "Webhook"
	}
	m, err := s.pstore.CreateHookMessage(tctx, w.ChannelID, domain.HookAuthorSub(w.ID), name, body)
	if err != nil {
		return domain.Message{}, err
	}
	s.publish(tctx, EventMessageCreated, m.ChannelID, m)
	return m, nil
}

// ---- Webhooks salientes -----------------------------------------------------

// CreateOutgoingWebhook crea un webhook saliente. Valida la forma de la URL
// (http/https con host; la barrera SSRF real se aplica al enviar) y exige que esté
// acotado (canal y/o palabras disparadoras). Genera el token de verificación.
func (s *Service) CreateOutgoingWebhook(ctx context.Context, channelID *uuid.UUID, actor, callbackURL string, triggerWords []string, dialect string) (domain.OutgoingWebhook, error) {
	if !domain.ValidCallbackURL(callbackURL) {
		return domain.OutgoingWebhook{}, domain.ErrInvalidInput
	}
	words := domain.NormalizeTriggerWords(triggerWords)
	if channelID == nil && len(words) == 0 {
		return domain.OutgoingWebhook{}, domain.ErrInvalidInput // debe acotar (canal o palabras)
	}
	dial, ok := compat.NormalizeOutgoing(dialect) // "" ⇒ sytue (compat total)
	if !ok {
		return domain.OutgoingWebhook{}, domain.ErrInvalidInput
	}
	token, _, err := domain.NewOpaqueToken() // verificación: se guarda/envía en claro
	if err != nil {
		return domain.OutgoingWebhook{}, err
	}
	// El secreto de firma es OTRO: el token viaja en cada entrega y por tanto no puede
	// probar el origen de la siguiente. Este no sale nunca.
	secret, err := domain.NewSigningSecret()
	if err != nil {
		return domain.OutgoingWebhook{}, err
	}
	return s.pstore.CreateOutgoingWebhook(ctx, channelID, actor, strings.TrimSpace(callbackURL), token, dial, secret, words)
}

// ListOutgoingWebhooks lista los salientes que el actor puede gestionar.
func (s *Service) ListOutgoingWebhooks(ctx context.Context, actor string) ([]domain.OutgoingWebhook, error) {
	return s.pstore.ListOutgoingWebhooks(ctx, actor)
}

// DeleteOutgoingWebhook borra un saliente (admin de su canal, o del workspace).
func (s *Service) DeleteOutgoingWebhook(ctx context.Context, id uuid.UUID, actor string) error {
	return s.pstore.DeleteOutgoingWebhook(ctx, id, actor)
}

// ---- Bots -------------------------------------------------------------------

// CreateBot crea un bot del tenant (admin del workspace) y devuelve su token Bearer
// en CLARO una única vez (junto al bot sin secreto).
func (s *Service) CreateBot(ctx context.Context, name, actor string) (domain.Bot, string, error) {
	name = strings.TrimSpace(name)
	if !domain.ValidBotName(name) {
		return domain.Bot{}, "", domain.ErrInvalidName
	}
	plain, hash, err := domain.NewBotToken()
	if err != nil {
		return domain.Bot{}, "", err
	}
	b, err := s.pstore.CreateBot(ctx, name, hash, actor)
	if err != nil {
		return domain.Bot{}, "", err
	}
	return b, plain, nil
}

// ListBots lista los bots del tenant (admin del workspace).
func (s *Service) ListBots(ctx context.Context, actor string) ([]domain.Bot, error) {
	return s.pstore.ListBots(ctx, actor)
}

// DeleteBot borra un bot (su dueño o admin del workspace).
func (s *Service) DeleteBot(ctx context.Context, id uuid.UUID, actor string) error {
	return s.pstore.DeleteBot(ctx, id, actor)
}

// AddBotToChannel añade un bot a un canal para que pueda publicar en él (admin del canal).
func (s *Service) AddBotToChannel(ctx context.Context, botID, channelID uuid.UUID, actor string) error {
	return s.pstore.AddBotToChannel(ctx, botID, channelID, actor)
}

// RemoveBotFromChannel quita a un bot de un canal (admin del canal).
func (s *Service) RemoveBotFromChannel(ctx context.Context, botID, channelID uuid.UUID, actor string) error {
	return s.pstore.RemoveBotFromChannel(ctx, botID, channelID, actor)
}

// BotFromToken resuelve la identidad de un bot a partir de su token Bearer, para el
// middleware de auth (sin JWT). Exige el prefijo bot_ y rehashea para buscar el bot
// CROSS-TENANT. Devuelve el bot con su tenant, o ErrForbidden/ErrNotFound.
func (s *Service) BotFromToken(ctx context.Context, token string) (domain.Bot, error) {
	if !strings.HasPrefix(token, domain.BotTokenPrefix) {
		return domain.Bot{}, domain.ErrForbidden
	}
	return s.pstore.LookupBotByTokenHash(ctx, domain.HashToken(token))
}
