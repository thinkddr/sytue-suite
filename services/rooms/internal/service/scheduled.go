// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// scheduledBatch acota cuántos programados vencidos se reclaman por transacción.
const scheduledBatch = 200

// ScheduleMessageInput son los parámetros para programar un mensaje.
type ScheduleMessageInput struct {
	ChannelID  uuid.UUID
	ParentID   *uuid.UUID
	Body       string
	FileIDs    []uuid.UUID
	SendAt     time.Time
	AuthorSub  string
	AuthorName string
}

// PostScheduled programa un mensaje para publicarse en SendAt (futuro). Valida el
// cuerpo (obligatorio salvo que haya adjuntos) y que SendAt sea futuro; el store
// valida el acceso de escritura al canal.
func (s *Service) PostScheduled(ctx context.Context, in ScheduleMessageInput) (domain.ScheduledMessage, error) {
	if s.cfg.MatrixRequired {
		return domain.ScheduledMessage{}, domain.ErrEncryptedRequired
	}
	if !in.SendAt.After(time.Now()) {
		return domain.ScheduledMessage{}, domain.ErrInvalidInput // debe ser futuro
	}
	body, err := domain.NormalizeBody(in.Body, s.cfg.MaxMessageBytes)
	if err != nil && len(in.FileIDs) == 0 {
		return domain.ScheduledMessage{}, err // vacío solo se permite con adjuntos
	}
	if err != nil {
		body = ""
	}
	return s.pstore.CreateScheduled(ctx, store.NewScheduledRecord{
		ChannelID:  in.ChannelID,
		ParentID:   in.ParentID,
		AuthorSub:  in.AuthorSub,
		AuthorName: in.AuthorName,
		Body:       body,
		FileIDs:    in.FileIDs,
		SendAt:     in.SendAt,
	})
}

// ListScheduled lista los programados del usuario (opcionalmente por canal).
func (s *Service) ListScheduled(ctx context.Context, sub string, channelID *uuid.UUID) ([]domain.ScheduledMessage, error) {
	return s.pstore.ListScheduled(ctx, sub, channelID)
}

// UpdateScheduled edita el cuerpo/adjuntos/instante de un programado del usuario.
// Valida el nuevo SendAt (futuro) y el cuerpo si vienen.
func (s *Service) UpdateScheduled(ctx context.Context, id uuid.UUID, actor string, body *string, sendAt *time.Time, fileIDs *[]uuid.UUID) (domain.ScheduledMessage, error) {
	if s.cfg.MatrixRequired {
		return domain.ScheduledMessage{}, domain.ErrEncryptedRequired
	}
	if sendAt != nil && !sendAt.After(time.Now()) {
		return domain.ScheduledMessage{}, domain.ErrInvalidInput
	}
	if body != nil {
		normalized, err := domain.NormalizeBody(*body, s.cfg.MaxMessageBytes)
		if err != nil && (fileIDs == nil || len(*fileIDs) == 0) {
			return domain.ScheduledMessage{}, err
		}
		if err != nil {
			normalized = ""
		}
		body = &normalized
	}
	return s.pstore.UpdateScheduled(ctx, id, actor, store.UpdateScheduledRecord{
		Body: body, SendAt: sendAt, FileIDs: fileIDs,
	})
}

// CancelScheduled cancela un programado del usuario.
func (s *Service) CancelScheduled(ctx context.Context, id uuid.UUID, actor string) error {
	return s.pstore.CancelScheduled(ctx, id, actor)
}

// RunScheduledWorker drena la cola de programados vencidos cada `interval` hasta
// que el contexto se cancele. El primer drenaje es inmediato (recupera lo vencido
// durante una caída). Publica cada mensaje reusando createMessageTx + el evento
// chat.message.created (el relay del outbox lo difunde a la sala del canal, igual
// que un envío normal). Cross-tenant con SKIP LOCKED: varias réplicas sin doble
// envío. Un interval <= 0 lo normaliza a 20 s.
func (s *Service) RunScheduledWorker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 20 * time.Second
	}
	log.From(ctx).Info("worker de mensajes programados iniciado", "interval", interval.String())
	s.drainScheduled(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.drainScheduled(ctx)
		}
	}
}

// drainScheduled reclama y publica lotes hasta vaciar los programados vencidos.
func (s *Service) drainScheduled(ctx context.Context) {
	if s.cfg.MatrixRequired {
		return
	}
	for {
		n, err := s.pstore.ClaimDueScheduled(ctx, time.Now(), scheduledBatch, s.publishScheduled)
		if err != nil {
			log.From(ctx).Warn("drenaje de programados falló", "error", err)
			return
		}
		if n < scheduledBatch {
			return // vaciado
		}
	}
}

// publishScheduled escribe el evento chat.message.created del mensaje ya insertado
// en el outbox usando la MISMA tx del worker (exactly-once: el evento solo viaja si
// la tx hace commit). Mismo sobre {channel_id, data} que Service.publish, para que
// NewFanoutHandler lo enrute a la sala del canal como un mensaje normal.
func (s *Service) publishScheduled(ctx context.Context, q pg.Querier, m domain.Message) error {
	env := eventEnvelope{ChannelID: m.ChannelID, Data: m}
	ev, err := events.NewEvent(EventChannel, EventMessageCreated, m.Tenant, env)
	if err != nil {
		return err
	}
	return events.Publish(ctx, q, ev)
}
