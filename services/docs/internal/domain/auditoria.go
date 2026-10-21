// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "time"

// EntradaAuditoria resume lo que una persona cambió en un documento, sin su contenido.
//
// Un documento de texto genera una operación por carácter, así que la auditoría no
// lista operaciones sueltas: agrupa las de la misma persona, del mismo tipo y sobre la
// misma clave dentro del mismo minuto. SeqDesde y SeqHasta delimitan el grupo; Cuantas
// dice de cuántas operaciones se compone.
type EntradaAuditoria struct {
	SeqDesde  int64  `json:"seq_desde"`
	SeqHasta  int64  `json:"seq_hasta"`
	AuthorSub string `json:"author_sub"`
	Kind      OpKind `json:"kind"`
	// Key es el nombre de la clave (p. ej. doc:sheet), nunca su valor. Vacía en las
	// operaciones de secuencia (insert/delete).
	Key     string `json:"key,omitempty"`
	Cuantas int    `json:"count"`
	// Rutas es dónde tocaron los parches del grupo (hoja/celda, diapositiva/caja), hasta
	// cinco, sin valores. Vacía en lo que no es un parche.
	Rutas []string  `json:"paths,omitempty"`
	At    time.Time `json:"at"`
}

// PaginaAuditoria es una página de la auditoría, de la más reciente a la más antigua.
// SiguienteAntesDe es el `before_seq` de la página siguiente; 0 = no hay más.
type PaginaAuditoria struct {
	Entradas         []EntradaAuditoria `json:"entries"`
	SiguienteAntesDe int64              `json:"next_before_seq,omitempty"`
}
