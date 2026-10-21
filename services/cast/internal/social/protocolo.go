// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package social

import (
	"time"

	"github.com/google/uuid"
)

// Tramas del servidor al cliente (CAST-ENCARGO §5.2).
const (
	// TramaMensaje: un mensaje nuevo en el chat.
	TramaMensaje = "message"
	// TramaBorrado: un mensaje retirado por moderación.
	TramaBorrado = "deleted"
	// TramaEspectadores: cuántas conexiones tiene el chat de este vídeo.
	TramaEspectadores = "viewers"
	// TramaError: algo que el cliente hizo mal (cuerpo vacío, demasiado deprisa).
	TramaError = "error"
)

// Órdenes del cliente al servidor.
const (
	// OrdenEnviar publica un mensaje: {"type":"send","body":"…"}.
	OrdenEnviar = "send"
)

// Trama es lo que el servidor manda por el WebSocket. Los campos van a la
// misma altura que en el REST (`id`, `author`, `body`, `created_at`), así el
// cliente pinta igual un mensaje del historial y uno que llega en directo.
type Trama struct {
	Type      string     `json:"type"`
	ID        *uuid.UUID `json:"id,omitempty"`
	Author    *Autor     `json:"author,omitempty"`
	Body      string     `json:"body,omitempty"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	Count     *int       `json:"count,omitempty"`
	Code      string     `json:"code,omitempty"`
	Message   string     `json:"message,omitempty"`
	// cerrar: tras escribir esta trama, el escritor cierra la conexión con 1008.
	// Va en la misma cola para que el cliente lea el motivo ANTES del cierre.
	cerrar bool
}

// Orden es lo que el cliente manda.
type Orden struct {
	Type string `json:"type"`
	Body string `json:"body,omitempty"`
}

func tramaMensaje(m Mensaje) Trama {
	id, autor, cuando := m.ID, m.Author, m.CreatedAt
	return Trama{Type: TramaMensaje, ID: &id, Author: &autor, Body: m.Body, CreatedAt: &cuando}
}

func tramaEspectadores(n int) Trama {
	return Trama{Type: TramaEspectadores, Count: &n}
}

func tramaError(code, message string) Trama {
	return Trama{Type: TramaError, Code: code, Message: message}
}
