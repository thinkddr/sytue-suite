// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package events implementa el bus de eventos de Sytue One SOBRE PostgreSQL
// (sin Redis/Valkey, docs/CONVENTIONS.md §3): publicación transaccional vía
// patrón outbox + entrega en tiempo real vía LISTEN/NOTIFY.
//
// Modelo:
//   - Publish escribe el evento en la tabla `events_outbox` dentro de la MISMA
//     transacción que el cambio de negocio (atomicidad exactly-once de escritura)
//     y emite un NOTIFY para despertar a los consumidores.
//   - Subscribe abre una conexión dedicada con LISTEN y entrega los eventos al
//     manejador. Un relay (Dispatcher) garantiza entrega aunque el NOTIFY se
//     pierda (poll de respaldo sobre el outbox).
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Channel es el canal lógico (mapea a un canal PostgreSQL NOTIFY).
type Channel string

// channelName devuelve el nombre del canal Postgres (prefijo de la suite).
func (c Channel) channelName() string { return "sytue_" + string(c) }

// Event es un evento de dominio publicado en el bus.
type Event struct {
	// ID es el identificador único del evento (UUID).
	ID string `json:"id"`
	// Channel es el canal lógico de publicación.
	Channel Channel `json:"channel"`
	// Type es el tipo de evento, en notación punteada (p. ej. "drive.file.created").
	Type string `json:"type"`
	// Tenant es el slug del tenant propietario (vacío para eventos de sistema).
	Tenant string `json:"tenant,omitempty"`
	// Payload es la carga útil arbitraria del evento (JSON).
	Payload json.RawMessage `json:"payload,omitempty"`
	// CreatedAt es el instante de creación (UTC).
	CreatedAt time.Time `json:"created_at"`
}

// NewEvent construye un Event con ID y timestamp generados. payload se serializa
// a JSON; pasa nil para un evento sin carga.
func NewEvent(channel Channel, eventType, tenant string, payload any) (Event, error) {
	if channel == "" || eventType == "" {
		return Event{}, fmt.Errorf("events: channel y type son obligatorios")
	}
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return Event{}, fmt.Errorf("events: serializar payload: %w", err)
		}
		raw = b
	}
	return Event{
		ID:        uuid.NewString(),
		Channel:   channel,
		Type:      eventType,
		Tenant:    tenant,
		Payload:   raw,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// Handler procesa un evento entregado. Si devuelve error, el Dispatcher
// reintentará la entrega (el outbox se marca como no procesado).
type Handler func(ctx context.Context, ev Event) error
