// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Límites de los comentarios de un registro.
const (
	MaxLongComentario         = 2000 // runas; el CHECK de la tabla es 4000 caracteres
	MaxComentariosPorRegistro = 500
)

// Comentario de un registro. ParentID vacío = abre hilo; con valor, responde a ese
// comentario raíz (los hilos tienen UN nivel). AuthorName lo pone la capa HTTP, del
// directorio de Sytue ID; el servicio solo guarda el `sub`.
type Comentario struct {
	ID         string    `json:"id"`
	RecordID   string    `json:"record_id"`
	ParentID   string    `json:"parent_id,omitempty"`
	AuthorSub  string    `json:"author_sub"`
	AuthorName string    `json:"author_name,omitempty"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
	// CanDelete dice a quien lo lee si puede borrarlo. Solo pinta el botón: el borrado
	// lo vuelve a decidir el servidor (PuedeBorrarComentario).
	CanDelete bool `json:"can_delete"`
	// Deleted es una lápida: se borró, pero tiene respuestas. Sin texto ni autor.
	Deleted bool `json:"deleted,omitempty"`
}

// LimpiarComentario recorta y valida el texto de un comentario: 1..2000 runas, UTF-8
// válido y sin caracteres de control salvo el salto de línea y el tabulador.
func LimpiarComentario(s string) (string, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	if s == "" || !utf8.ValidString(s) || utf8.RuneCountInString(s) > MaxLongComentario {
		return "", ErrInvalido
	}
	for _, r := range s {
		if (r < 0x20 && r != '\n' && r != '\t') || r == 0x7f {
			return "", ErrInvalido
		}
	}
	return s, nil
}

// PuedeBorrarComentario es la regla de Docs (PR #353): lo borra quien lo escribió
// —mientras siga pudiendo comentar— y quien modera, que es editor o propietario. Un
// comentarista no borra los de otros; un lector, ninguno, ni los que escribió cuando
// podía comentar.
func PuedeBorrarComentario(r Rol, esAutor bool) bool {
	return Permite(r, AccEscribirReg) || (esAutor && Permite(r, AccComentar))
}
