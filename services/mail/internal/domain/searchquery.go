// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"time"
)

// SearchQuery es una consulta de búsqueda ya desmontada en criterios.
//
// POR QUÉ EXISTE: el almacén sabe filtrar por remitente, destinatario, asunto,
// adjunto, estado y fecha desde el principio, pero la caja de búsqueda mandaba
// TODO lo escrito como texto libre. Quien escribía `from:ana` no buscaba los
// correos de Ana: buscaba mensajes que contuvieran las palabras «from» y «ana»,
// y como casi ninguno las contiene, la respuesta era «sin resultados» — el peor
// fallo posible en una búsqueda, porque parece un hecho sobre tu buzón.
type SearchQuery struct {
	// Text es lo que queda después de sacar los operadores (búsqueda de texto).
	Text string
	// From, To y Subject son subcadenas; vacío = no filtra.
	From    string
	To      string
	Subject string
	// HasAttachment lo activa `has:attachment`.
	HasAttachment bool
	// Unseen/Seen/Flagged los activan `is:unread`, `is:read` e `is:starred`.
	Unseen  bool
	Seen    bool
	Flagged bool
	// After/Before acotan received_at (`after:2026-01-31`, `before:2026-03-01`).
	// Cero = sin acotar. After es inclusivo por el principio del día indicado y
	// Before excluyente por el final: `before:2026-03-01` NO trae ese 1 de marzo.
	After  time.Time
	Before time.Time
}

// ParseSearchQuery desmonta una consulta en criterios.
//
// Sintaxis: `operador:valor` y el resto texto libre. El valor admite comillas
// para frases (`subject:"informe anual"`). Un operador desconocido NO se descarta:
// vuelve al texto libre tal cual, porque quien busca `nota:reunión` está buscando
// esa palabra y no usando un operador que no existe.
//
// Es case-insensitive en el operador (`From:` vale) pero no en el valor: quien
// busca compara en minúsculas más abajo (ILIKE / FTS).
func ParseSearchQuery(q string) SearchQuery {
	var out SearchQuery
	var libre []string
	for _, tok := range tokenizeQuery(q) {
		op, val, ok := strings.Cut(tok.text, ":")
		if !ok || tok.quoted || val == "" {
			libre = append(libre, tok.text)
			continue
		}
		val = strings.Trim(val, `"`)
		switch strings.ToLower(op) {
		case "from":
			out.From = val
		case "to":
			out.To = val
		case "subject":
			out.Subject = val
		case "has":
			// `has:attachment` es el único que se entiende; `has:otracosa` no se
			// traga en silencio, vuelve al texto.
			if strings.EqualFold(val, "attachment") || strings.EqualFold(val, "attachments") {
				out.HasAttachment = true
			} else {
				libre = append(libre, tok.text)
			}
		case "is":
			switch strings.ToLower(val) {
			case "unread", "unseen":
				out.Unseen = true
			case "read", "seen":
				out.Seen = true
			case "starred", "flagged":
				out.Flagged = true
			default:
				libre = append(libre, tok.text)
			}
		case "after", "since":
			if d, err := time.Parse(time.DateOnly, val); err == nil {
				out.After = d
			} else {
				libre = append(libre, tok.text)
			}
		case "before", "until":
			if d, err := time.Parse(time.DateOnly, val); err == nil {
				// Final del día indicado excluido: `before:2026-03-01` deja fuera
				// ese día entero, que es lo que espera quien lo escribe.
				out.Before = d
			} else {
				libre = append(libre, tok.text)
			}
		default:
			libre = append(libre, tok.text)
		}
	}
	out.Text = strings.Join(libre, " ")
	return out
}

// token es una palabra suelta o una frase entrecomillada de la consulta.
type token struct {
	text string
	// quoted marca la frase que venía ENTERA entre comillas ("a: b"), que es texto
	// literal y no un operador aunque lleve dos puntos dentro.
	quoted bool
}

// tokenizeQuery parte la consulta respetando las comillas: `subject:"a b" c` son
// dos tokens. Una comilla sin cerrar acaba al final de la cadena en lugar de
// descartar lo escrito.
func tokenizeQuery(q string) []token {
	var out []token
	var cur strings.Builder
	enComillas := false
	empezoConComilla := false
	descargar := func() {
		if cur.Len() > 0 {
			out = append(out, token{text: cur.String(), quoted: empezoConComilla})
			cur.Reset()
		}
		empezoConComilla = false
	}
	for _, r := range q {
		switch {
		case r == '"':
			if !enComillas && cur.Len() == 0 {
				empezoConComilla = true
			}
			enComillas = !enComillas
			// La comilla se conserva para que el valor de un operador la lleve y
			// pueda recortarse después (`subject:"a b"` → valor `"a b"` → `a b`).
			cur.WriteRune(r)
		case (r == ' ' || r == '\t' || r == '\n') && !enComillas:
			descargar()
		default:
			cur.WriteRune(r)
		}
	}
	descargar()
	return out
}
