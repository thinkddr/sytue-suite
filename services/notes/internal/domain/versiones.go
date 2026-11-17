// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Retención del historial de versiones de una nota: las últimas MaxVersiones y
// ninguna de hace más de RetencionVersiones. Lo que pase de cualquiera de los dos se
// poda al guardar y no se enseña al leer.
const (
	MaxVersiones       = 50
	RetencionVersiones = 30 * 24 * time.Hour
)

// TandaDeEdicion es cuánto tiempo seguido de una misma persona cuenta como UNA
// edición. El editor guarda solo cada pocos segundos: una versión por guardado
// llenaría las 50 en un rato de escribir y el historial sería un rastro de
// pulsaciones. Con la tanda, cada versión es «cómo estaba antes de ponerse a
// editar», que es lo que se busca al volver atrás.
const TandaDeEdicion = 10 * time.Minute

// NoteVersion es cómo estaba una nota antes de una tanda de edición; ReplacedBy, quien
// la editó encima. En un
// cuaderno cifrado Title y Content van vacíos y el texto está en ContentCipher,
// que el servidor guarda sin poder leerlo.
type NoteVersion struct {
	ID            uuid.UUID       `json:"id"`
	NoteID        uuid.UUID       `json:"note_id"`
	ReplacedBy    string          `json:"replaced_by"`
	Title         string          `json:"title,omitempty"`
	Content       string          `json:"content,omitempty"`
	ContentCipher string          `json:"content_cipher,omitempty"`
	Data          json.RawMessage `json:"data,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

// UltimaVersion es lo que hace falta saber de la versión más reciente para decidir
// si un guardado abre otra.
type UltimaVersion struct {
	ReplacedBy string
	CreatedAt  time.Time
}

// GuardaVersion dice si un guardado de `sub` que deja la nota con `in` debe guardar
// antes cómo estaba (`antes`). Solo cuando cambia el texto (título, contenido o
// criptograma), la nota no estaba vacía y no es la misma tanda de edición: otra
// persona, o la misma tras TandaDeEdicion desde la última versión.
func GuardaVersion(antes Note, in NoteInput, ultima *UltimaVersion, sub string, ahora time.Time) bool {
	if antes.Title == "" && antes.Content == "" && antes.ContentCipher == "" {
		return false
	}
	if antes.Title == in.Title && antes.Content == in.Content && antes.ContentCipher == in.ContentCipher {
		return false
	}
	if ultima == nil || ultima.ReplacedBy != sub {
		return true
	}
	// La tanda se cuenta desde la última edición de la nota, no desde la versión:
	// quien escribe sin parar durante una hora deja una versión, no seis.
	desde := ultima.CreatedAt
	if antes.UpdatedAt.After(desde) {
		desde = antes.UpdatedAt
	}
	return ahora.Sub(desde) >= TandaDeEdicion
}
