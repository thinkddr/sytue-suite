// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package collab

import (
	"encoding/json"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// ProtocolVersion versiona el contrato del canal, no el seq del documento.
// Cero conserva clientes ya publicados sin este campo (contrato v1).
const ProtocolVersion = 1

func compatibleVersion(version int) bool { return version == 0 || version == ProtocolVersion }

// Protocolo WebSocket de colaboración (JSON sobre coder/websocket).
//
// Cliente -> Servidor (ClientMessage.Type):
//   - "hello"   : el cliente anuncia su last_seq conocido; el servidor responde
//                 con un replay de las operaciones posteriores (sincronización).
//   - "ops"     : el cliente envía operaciones CRDT locales para secuenciar.
//   - "cursor"  : presencia efímera (posición de cursor/selección); no durable.
//   - "ping"    : keep-alive del cliente.
//
// Servidor -> Cliente (ServerMessage.Type):
//   - "welcome" : confirma la conexión, el rol efectivo y el seq actual del doc.
//   - "ops"     : operaciones secuenciadas (propias confirmadas o de otros).
//   - "ack"     : confirma la última seq asignada a un lote del propio cliente.
//   - "presence": lista de participantes / cursores.
//   - "role"    : el rol de UNA persona ha cambiado (el dueño concedió o quitó un
//                 permiso). Solo lo reciben las conexiones de esa persona: es lo que
//                 desbloquea la franja de solo lectura sin recargar.
//   - "error"   : error legible (código + mensaje).

// ClientMessageType enumera los tipos de mensaje entrantes.
type ClientMessageType string

const (
	MsgHello  ClientMessageType = "hello"
	MsgCData  ClientMessageType = "ops"
	MsgCursor ClientMessageType = "cursor"
	MsgPing   ClientMessageType = "ping"
)

// ClientMessage es un mensaje del cliente.
type ClientMessage struct {
	Type    ClientMessageType `json:"type"`
	Version int               `json:"version,omitempty"`
	// LastSeq (hello): última seq que el cliente ya tiene aplicada.
	LastSeq int64 `json:"last_seq,omitempty"`
	// Ops (ops): operaciones CRDT locales a secuenciar.
	Ops []domain.Op `json:"ops,omitempty"`
	// Cursor (cursor): payload opaco de presencia (posición/selección).
	Cursor json.RawMessage `json:"cursor,omitempty"`
	// Proteccion (ops): contraseñas de hoja/libro, fuera de las operaciones para que
	// no entren en el registro (domain.Proteccion).
	Proteccion *domain.Proteccion `json:"proteccion,omitempty"`
}

// ServerMessageType enumera los tipos de mensaje salientes.
type ServerMessageType string

const (
	MsgWelcome  ServerMessageType = "welcome"
	MsgOps      ServerMessageType = "ops"
	MsgAck      ServerMessageType = "ack"
	MsgPresence ServerMessageType = "presence"
	MsgRole     ServerMessageType = "role"
	MsgError    ServerMessageType = "error"
)

// ServerMessage es un mensaje del servidor.
type ServerMessage struct {
	Type    ServerMessageType `json:"type"`
	Version int               `json:"version,omitempty"`
	// Welcome y role: rol del cliente (y seq actual del documento en welcome).
	Role string `json:"role,omitempty"`
	Seq  int64  `json:"seq,omitempty"`
	// Ops/replay: operaciones secuenciadas.
	Ops []domain.SequencedOp `json:"ops,omitempty"`
	// Presence: subs y cursores presentes.
	Presence []Presence `json:"presence,omitempty"`
	// Cursor: presencia de UN participante (difusión de "cursor").
	Cursor *Presence `json:"cursor,omitempty"`
	// Error.
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// Presence describe la presencia efímera de un participante.
type Presence struct {
	Sub    string          `json:"sub"`
	Name   string          `json:"name,omitempty"`
	Cursor json.RawMessage `json:"cursor,omitempty"`
}

// errorMessage construye un ServerMessage de error.
func errorMessage(code, msg string) ServerMessage {
	return ServerMessage{Type: MsgError, Code: code, Message: msg}
}
