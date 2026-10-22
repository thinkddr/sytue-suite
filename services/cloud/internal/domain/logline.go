// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"time"

	"github.com/google/uuid"
)

// LogLine es UNA línea de log de runtime de una app del cliente.
//
// Vive en domain porque la cruzan tres capas que no deben conocerse entre sí: la
// produce runtime (leyendo el pod), la persiste store y la devuelve la API. Las
// etiquetas JSON SON el contrato de GET /projects/{id}/logs.
type LogLine struct {
	// ID vale uuid.Nil mientras la línea no está persistida (en la ingesta solo
	// se leen TS y Text); al buscarla ya trae el id que desempata el cursor keyset.
	ID uuid.UUID `json:"id"`
	// DeploymentID identifica QUÉ despliegue la escribió: en la búsqueda por
	// proyecto se mezclan varios (producción + previews) y sin esto no se sabe
	// cuál habló.
	DeploymentID uuid.UUID `json:"deployment_id"`
	// TS es el instante que puso el contenedor, no el de la ingesta.
	TS time.Time `json:"ts"`
	// Text es la línea tal cual, sin el prefijo de tiempo del runtime.
	Text string `json:"text"`
}
