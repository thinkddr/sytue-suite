// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"time"
	"unicode"
)

// SearchQuery es una consulta de búsqueda ya descompuesta en términos de texto más
// operadores estilo Slack/Mattermost. Los campos vacíos (nil/false) no filtran; los
// presentes se combinan con AND. Solo transporta la intención: la traducción a SQL
// vive en la capa store (que la ensambla de forma indexada y acotada por LIMIT, sin
// N+1).
type SearchQuery struct {
	// Terms son los términos POSITIVOS del texto libre: el cuerpo debe contener
	// TODOS, en cualquier orden y posición («reunión viernes» encuentra «el viernes
	// hay reunión»). Un término que venía entrecomillado conserva sus espacios y
	// casa la secuencia literal («informe de cierre»). Puede quedar vacío (búsqueda
	// solo por operadores, p. ej. "from:ana").
	Terms []string
	// Not son los términos EXCLUIDOS con guion (-borrador, -"copia de seguridad"):
	// el mensaje que contenga cualquiera de ellos queda fuera.
	Not []string
	// From filtra por autor: coincide por nombre (subcadena) o por sub exacto.
	From string
	// In filtra por canal: coincide por slug o por nombre (subcadena).
	In string
	// After acota created_at >= After (medianoche UTC del día, inclusivo).
	After *time.Time
	// Before acota created_at < Before (medianoche UTC del día, exclusivo). Así
	// "after:2024-01-01 before:2024-02-01" es exactamente todo enero.
	Before *time.Time
	// HasLink exige que el cuerpo lleve una URL http(s) (has:link).
	HasLink bool
	// HasFile exige al menos un adjunto de Drive (has:file).
	HasFile bool
	// Saved limita a los mensajes que guardó el PROPIO llamante (is:saved).
	Saved bool
	// Pinned limita a los mensajes fijados en su canal (is:pinned).
	Pinned bool
}

// searchDateLayout es el formato aceptado por before:/after:/on: (fecha ISO sin hora).
const searchDateLayout = "2006-01-02"

// searchToken es un token de la consulta cruda. quoted marca que venía entre
// comillas: entonces es texto literal (frase exacta) y no se lee como operador.
type searchToken struct {
	text   string
	quoted bool
}

// maxSearchTokens acota los tokens que se traducen a SQL. La consulta llega por query
// string (hasta el límite de cabecera del servidor) y cada término se convierte en una
// condición ILIKE, así que sin tope se podría fabricar una consulta con miles de
// condiciones. Descartar tokens de más nunca filtra nada: como mucho ENSANCHA el
// resultado dentro de lo que el llamante ya puede ver (el filtro de acceso y el LIMIT
// no salen del texto de la consulta). 32 es holgado para una caja de búsqueda.
const maxSearchTokens = 32

// splitSearchTokens parte la consulta por espacios RESPETANDO las comillas dobles,
// para que una frase entrecomillada viaje entera. Una comilla sin cerrar se toma
// como abierta hasta el final (una consulta a medio escribir no es un error).
func splitSearchTokens(raw string) []searchToken {
	var out []searchToken
	var cur strings.Builder
	var quoted, inQuote bool
	flush := func() {
		if cur.Len() > 0 && len(out) < maxSearchTokens {
			out = append(out, searchToken{text: cur.String(), quoted: quoted})
		}
		cur.Reset()
		quoted, inQuote = false, false
	}
	for _, r := range raw {
		switch {
		case r == '"':
			inQuote, quoted = !inQuote, true
		case unicode.IsSpace(r) && !inQuote:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

// setFlagOp activa el operador booleano key:val (has:link, has:file, is:saved,
// is:pinned) y devuelve si lo ha reconocido; con un valor desconocido devuelve false
// y el token vuelve al texto libre. key llega ya en minúsculas.
func (q *SearchQuery) setFlagOp(key, val string) bool {
	switch key + ":" + strings.ToLower(val) {
	case "has:link":
		q.HasLink = true
	case "has:file":
		q.HasFile = true
	case "is:saved":
		q.Saved = true
	case "is:pinned":
		q.Pinned = true
	default:
		return false
	}
	return true
}

// ParseSearchQuery descompone una consulta cruda en términos de texto (positivos y
// excluidos) + operadores from:/in:/before:/after:/on:/has:/is: (la clave es
// insensible a mayúsculas). Reglas:
//   - Cada palabra suelta es un término y TODOS deben aparecer (AND en el store).
//   - "frase exacta" entre comillas es UN término con espacios: casa la secuencia.
//   - -palabra (o -"frase exacta") excluye.
//   - Un operador sin valor ("from:") se ignora (ni filtra ni cae a texto).
//   - before:/after:/on: con fecha inválida se ignoran en silencio.
//   - Cualquier otro token con ':' que no sea un operador conocido —o un has:/is:
//     con valor desconocido, p. ej. una URL "http://x"— se conserva íntegro como
//     término de texto.
//
// Es una función pura (sin BD ni estado): el mismo parser lo comparten SearchAll y
// SearchMessages, y se testea aislada.
//
// ponytail: no soporta negar un operador (-from:ana); solo términos de texto. Si
// hiciera falta, el sitio es este switch con el flag ya extraído.
func ParseSearchQuery(raw string) SearchQuery {
	var q SearchQuery
	for _, tok := range splitSearchTokens(raw) {
		// Exclusión: -palabra o -"frase exacta".
		if rest, neg := strings.CutPrefix(tok.text, "-"); neg && rest != "" {
			q.Not = append(q.Not, rest)
			continue
		}
		key, val, ok := strings.Cut(tok.text, ":")
		// Lo entrecomillado es literal: "in:general" busca ese texto, no filtra.
		if !ok || tok.quoted {
			q.Terms = append(q.Terms, tok.text)
			continue
		}
		switch key = strings.ToLower(key); key {
		case "from":
			if val != "" {
				q.From = val
			}
		case "in":
			// La almohadilla se recorta: en Slack, Mattermost y Teams se escribe
			// `in:#general` —es como se nombra un canal—, pero el valor se compara
			// por subcadena contra `slug`/`name`, que NO llevan «#». Sin esto,
			// `in:#general` no casa con NADA y la búsqueda sale vacía sin decir por
			// qué: parece que el canal no tiene resultados.
			if val = strings.TrimPrefix(val, "#"); val != "" {
				q.In = val
			}
		case "before":
			// time.Parse("") también falla, así que cubre el operador sin valor.
			if d, err := time.Parse(searchDateLayout, val); err == nil {
				q.Before = &d
			}
		case "after":
			if d, err := time.Parse(searchDateLayout, val); err == nil {
				q.After = &d
			}
		case "on":
			// Un día exacto es la ventana [medianoche, medianoche+24h): reusa
			// After/Before, así que no añade ninguna condición nueva al SQL.
			if d, err := time.Parse(searchDateLayout, val); err == nil {
				end := d.AddDate(0, 0, 1)
				q.After, q.Before = &d, &end
			}
		case "has", "is":
			if !q.setFlagOp(key, val) {
				q.Terms = append(q.Terms, tok.text)
			}
		default:
			q.Terms = append(q.Terms, tok.text)
		}
	}
	return q
}
