// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/rooms/internal/ws"
)

// PoolPublisher adapta el *pg.Pool al interfaz EventPublisher publicando cada
// evento en su propia transacción sobre el outbox (events.PublishTx).
type PoolPublisher struct {
	pool *pg.Pool
}

// NewPoolPublisher construye un PoolPublisher.
func NewPoolPublisher(pool *pg.Pool) *PoolPublisher { return &PoolPublisher{pool: pool} }

// Publish escribe el evento en el outbox de PostgreSQL.
func (p *PoolPublisher) Publish(ctx context.Context, ev events.Event) error {
	return events.PublishTx(ctx, p.pool, ev)
}

// Broadcaster es la superficie del Hub WS que consume el relay (Deliver).
type Broadcaster interface {
	Deliver(tenant string, channel uuid.UUID, frame ws.ServerFrame, except *ws.Client)
	// Expulsar da de baja en la sala a quien deja el canal (COM-08).
	Expulsar(tenant string, channel uuid.UUID, subject string)
}

// NewFanoutHandler construye el manejador del relay del outbox: traduce cada
// evento de dominio de Chat a un frame WebSocket y lo difunde a la sala del
// canal vía el Hub. Este es el fan-out CROSS-RÉPLICA por LISTEN/NOTIFY: el
// Dispatcher escucha el outbox (en TODAS las réplicas) y entrega aquí.
func NewFanoutHandler(hub Broadcaster) events.Handler {
	return func(ctx context.Context, ev events.Event) error {
		if ev.Channel != EventChannel || ev.Tenant == "" {
			return nil
		}
		var env struct {
			ChannelID uuid.UUID       `json:"channel_id"`
			Data      json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(ev.Payload, &env); err != nil {
			log.From(ctx).Warn("chat: payload de evento inválido", "type", ev.Type, "error", err)
			return nil // payload corrupto: descartar (no reintentar eternamente)
		}
		if env.ChannelID == uuid.Nil {
			return nil
		}

		frame := ws.ServerFrame{ChannelID: &env.ChannelID, Payload: env.Data}
		switch ev.Type {
		case EventMessageCreated:
			frame.Type = ws.ServerMessage
		case EventMessageEdited:
			frame.Type = ws.ServerMessageEdited
		case EventMessageDeleted:
			frame.Type = ws.ServerMessageDeleted
		case EventReactionChanged:
			frame.Type = ws.ServerReaction
		case EventPinChanged:
			frame.Type = ws.ServerPinChanged
		case EventCardChanged:
			frame.Type = ws.ServerCardChanged
		case EventChannelMemberAdded:
			frame.Type = ws.ServerChannelMemberAdded
		case EventChannelMemberRemoved:
			frame.Type = ws.ServerChannelMemberRemoved
		case EventChannelDeleted:
			frame.Type = ws.ServerChannelDeleted
		default:
			return nil
		}
		hub.Deliver(ev.Tenant, env.ChannelID, frame, nil)
		// Primero se le avisa y después deja de recibir: su suscripción se autorizó al
		// abrirla, y sin esto seguía llegándole todo lo del canal.
		if ev.Type == EventChannelMemberRemoved {
			var quien struct {
				Subject string `json:"subject"`
			}
			if json.Unmarshal(env.Data, &quien) == nil && quien.Subject != "" {
				hub.Expulsar(ev.Tenant, env.ChannelID, quien.Subject)
			}
		}
		return nil
	}
}

// MediaEventChannel es el canal de outbox por el que publica la videollamada.
// Está escrito aquí y no importado del paquete de medios porque allí es privado,
// y hacerlo público solo para esto acoplaría los dos lados por el nombre.
const MediaEventChannel events.Channel = "meet"

// NewCallFanoutHandler traduce los eventos de la VIDEOLLAMADA a frames del
// WebSocket del canal de chat, para que una llamada se anuncie en el momento.
//
// Hasta ahora el otro extremo se enteraba SONDEANDO cada ~25 segundos: quien
// llamaba esperaba media conversación a que al otro le apareciera el aviso. El
// canal ya estaba montado; lo que faltaba era cruzar los dos subsistemas, que
// viven en el mismo servicio desde la fusión pero publican en canales distintos.
//
// Solo se difunden las salas que NACEN DE UN CANAL: una reunión suelta no tiene
// canal al que avisar, y colar su evento aquí dejaría un aviso de llamada en un
// canal que no la tiene.
func NewCallFanoutHandler(hub Broadcaster) events.Handler {
	return func(ctx context.Context, ev events.Event) error {
		if ev.Channel != MediaEventChannel || ev.Tenant == "" {
			return nil
		}
		var env struct {
			SourceKind  string `json:"source_kind"`
			SourceRefID string `json:"source_ref_id"`
		}
		if err := json.Unmarshal(ev.Payload, &env); err != nil {
			log.From(ctx).Warn("rooms: payload de evento de llamada inválido", "type", ev.Type, "error", err)
			return nil
		}
		if env.SourceKind != "chat" || env.SourceRefID == "" {
			return nil
		}
		canal, err := uuid.Parse(env.SourceRefID)
		if err != nil {
			return nil //nolint:nilerr // una referencia mal formada se descarta: devolver el error haría que el relay la reintentara para siempre.
		}

		var tipo string
		switch ev.Type {
		case "meet.room.created":
			tipo = ws.ServerCallStarted
		case "meet.room.closed", "meet.room.finished":
			tipo = ws.ServerCallEnded
		default:
			return nil
		}
		hub.Deliver(ev.Tenant, canal, ws.ServerFrame{
			Type:      tipo,
			ChannelID: &canal,
			Payload:   ev.Payload,
		}, nil)
		return nil
	}
}
