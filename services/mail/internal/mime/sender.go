// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mime

import "bytes"

// EnsureSenderHeader garantiza que el mensaje crudo lleva una cabecera `Sender:`
// con `addr` (RFC 5322 §3.6.2), reemplazando cualquiera que trajera el cliente.
//
// Es la frontera de confianza del «enviar-en-nombre-de» (delegación): el servidor
// —tras VERIFICAR que el usuario tiene permiso `send` sobre la identidad del
// From— sella el remitente real (el asistente) en el Sender, sin fiarse de lo que
// componga el cliente. Para el «enviar-como» de un buzón compartido no se llama:
// el From ya ES la identidad compartida y no procede un Sender distinto.
//
// Se antepone al bloque de cabeceras (el orden no es significativo, RFC 5322
// §3.6) y se descartan las líneas `Sender:` previas y sus continuaciones. No
// altera el resto del mensaje ni el cuerpo, así que la firma DKIM posterior
// (que NO cubre Sender) sigue siendo válida.
func EnsureSenderHeader(raw []byte, addr string) []byte {
	if addr == "" {
		return raw
	}
	// Separar cabeceras y cuerpo por la primera línea en blanco (CRLF o LF).
	sep := []byte("\r\n\r\n")
	headEnd := bytes.Index(raw, sep)
	if headEnd < 0 {
		sep = []byte("\n\n")
		headEnd = bytes.Index(raw, sep)
	}
	head, rest := raw, []byte(nil)
	if headEnd >= 0 {
		head = raw[:headEnd]
		rest = raw[headEnd:]
	}
	head = stripHeader(head, "sender:")

	var out bytes.Buffer
	out.Grow(len(raw) + len(addr) + 12)
	out.WriteString("Sender: ")
	out.WriteString(addr)
	out.WriteString("\r\n")
	out.Write(head)
	out.Write(rest)
	return out.Bytes()
}

// stripHeader elimina del bloque de cabeceras las líneas cuyo campo case-insensitive
// es `nameLower` (con ':'), incluidas sus líneas de continuación plegadas (las que
// empiezan por espacio/tab, RFC 5322 §2.2.3).
func stripHeader(head []byte, nameLower string) []byte {
	lines := splitKeepLines(head)
	var out bytes.Buffer
	skipping := false
	for _, ln := range lines {
		if len(ln) > 0 && (ln[0] == ' ' || ln[0] == '\t') {
			if skipping {
				continue // continuación de una cabecera descartada
			}
			out.Write(ln)
			continue
		}
		skipping = hasPrefixFold(ln, nameLower)
		if skipping {
			continue
		}
		out.Write(ln)
	}
	return out.Bytes()
}

// splitKeepLines parte en líneas conservando el terminador (\n) de cada una.
func splitKeepLines(b []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i := 0; i < len(b); i++ {
		if b[i] == '\n' {
			lines = append(lines, b[start:i+1])
			start = i + 1
		}
	}
	if start < len(b) {
		lines = append(lines, b[start:])
	}
	return lines
}

// hasPrefixFold indica si b empieza por el prefijo ASCII (case-insensitive) p.
func hasPrefixFold(b []byte, p string) bool {
	if len(b) < len(p) {
		return false
	}
	for i := 0; i < len(p); i++ {
		c := b[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != p[i] {
			return false
		}
	}
	return true
}
