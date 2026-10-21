// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package ws implementa la mensajería en tiempo real de Sytue Rooms sobre
// WebSocket (coder/websocket, MIT). El servidor difunde a los suscriptores de
// cada canal los eventos de mensajes/reacciones (originados en la API REST o en
// otros clientes) y los efímeros de "está escribiendo..." y presencia.
//
// Estado durable en PostgreSQL; el fan-out CROSS-RÉPLICA de los eventos
// durables (mensajes, reacciones) se hace por LISTEN/NOTIFY sobre el outbox
// (platform/pkg/events). Los frames EFÍMEROS (typing/presencia) cruzan réplicas
// por pub/sub de Dragonfly (platform/pkg/cache), fuera del bus NOTIFY: su
// volumen lo multiplicaría x5-20. Sin DRAGONFLY_URL degradan a intra-réplica.
// El Hub es un multiplexor en memoria por proceso.
//
// Clean-room: protocolo JSON propio sobre RFC 6455; sin código de terceros AGPL.
package ws

import (
	"encoding/json"

	"github.com/google/uuid"
)

// Tipos de mensaje del cliente al servidor.
const (
	// ClientSubscribe se une a la difusión en tiempo real de un canal.
	ClientSubscribe = "subscribe"
	// ClientUnsubscribe abandona la difusión de un canal.
	ClientUnsubscribe = "unsubscribe"
	// ClientTyping indica que el usuario está escribiendo en un canal (efímero).
	ClientTyping = "typing"
	// ClientPresence anuncia el estado de presencia del usuario (online|away).
	ClientPresence = "presence"
	// ClientPing es un keep-alive de aplicación (opcional; el ping WS lo cubre).
	ClientPing = "ping"
)

// Tipos de mensaje del servidor al cliente.
const (
	// ServerReady confirma la conexión y porta el subject del usuario.
	ServerReady = "ready"
	// ServerMessage anuncia un mensaje nuevo (o respuesta de hilo).
	ServerMessage = "message"
	// ServerMessageEdited anuncia la edición de un mensaje.
	ServerMessageEdited = "message_edited"
	// ServerMessageDeleted anuncia el borrado de un mensaje.
	ServerMessageDeleted = "message_deleted"
	// ServerReaction anuncia un cambio de reacción (added|removed).
	ServerReaction = "reaction"
	// ServerPinChanged anuncia un cambio de fijado (pinned|unpinned).
	ServerPinChanged = "pin"
	// ServerCardChanged anuncia el estado nuevo de la tarjeta de un mensaje
	// (encuesta votada, tarea marcada): el payload lleva el recuento COMPLETO.
	ServerCardChanged = "card"
	// ServerChannelMemberAdded anuncia que alguien entró al canal —o que cambió su
	// rol—: el payload lleva el miembro completo y el cliente hace UPSERT por subject.
	ServerChannelMemberAdded = "channel_member_added"
	// ServerChannelMemberRemoved anuncia que alguien salió (o fue expulsado) del canal.
	ServerChannelMemberRemoved = "channel_member_removed"
	// ServerChannelDeleted anuncia que el canal se ha borrado: el cliente lo quita
	// de la barra lateral y lo cierra si lo tenía abierto.
	ServerChannelDeleted = "channel_deleted"
	// ServerTyping difunde el indicador de escritura a la sala.
	ServerTyping = "typing"
	// ServerPresence difunde el cambio de presencia de un usuario.
	ServerPresence = "presence"
	// ServerCallStarted anuncia que ha empezado una llamada en el canal. Sustituye
	// al sondeo cada ~25 s con el que el otro extremo se enteraba antes: quien
	// llama quiere que suene ahora, no dentro de medio minuto.
	ServerCallStarted = "call_started"
	// ServerCallEnded anuncia que la llamada del canal ha terminado.
	ServerCallEnded = "call_ended"
	// ServerError comunica un error legible al cliente.
	ServerError = "error"
	// ServerPong responde a un ping de aplicación.
	ServerPong = "pong"
)

// ClientFrame es un mensaje entrante del cliente (sobre JSON).
type ClientFrame struct {
	Type      string    `json:"type"`
	ChannelID uuid.UUID `json:"channel_id,omitempty"`
	// Status para presence: "online" | "away".
	Status string `json:"status,omitempty"`
}

// ServerFrame es un mensaje saliente del servidor (sobre JSON). Payload lleva el
// recurso afectado ya serializado (mensaje, reacción, etc.).
type ServerFrame struct {
	Type      string          `json:"type"`
	ChannelID *uuid.UUID      `json:"channel_id,omitempty"`
	Subject   string          `json:"subject,omitempty"`
	Name      string          `json:"name,omitempty"`
	Status    string          `json:"status,omitempty"`
	Code      string          `json:"code,omitempty"`
	Message   string          `json:"message,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// errorFrame construye un ServerFrame de error.
func errorFrame(code, message string) ServerFrame {
	return ServerFrame{Type: ServerError, Code: code, Message: message}
}
