// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"bufio"
	"io"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
)

// maxBuildLogLines acota las líneas devueltas del log de build: sin filtro son las
// primeras N; con filtro, las primeras N que casan. Es la cota de memoria de la
// respuesta (solo se acumulan las líneas devueltas), complementaria a la cota de
// entrada maxBuildLogBytes (LimitReader sobre el objeto S3).
const maxBuildLogLines = 5000

// maxLogLineBytes acota una sola línea del scanner (líneas muy largas de un log de
// build: trazas, JSON en una línea). Evita que una línea patológica dispare la
// memoria; bufio.Scanner sin este tope aborta en líneas > 64 KiB.
const maxLogLineBytes = 1 << 20 // 1 MiB

// filterLogLines lee r línea a línea y devuelve las que casan el filtro (streaming:
// solo acumula las líneas devueltas). q casa por subcadena case-insensitive; level
// (ya sea INFO/WARN/ERROR… de Kaniko/logrus) casa como token de la línea. q y level
// combinan en AND. maxLines <= 0 o por encima del tope => maxBuildLogLines.
func filterLogLines(r io.Reader, q, level string, maxLines int) string {
	if maxLines <= 0 || maxLines > maxBuildLogLines {
		maxLines = maxBuildLogLines
	}
	q = buscar.Plegar(q)
	level = strings.ToUpper(strings.TrimSpace(level))

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxLogLineBytes)

	var b strings.Builder
	n := 0
	for n < maxLines && sc.Scan() {
		line := sc.Text()
		if q != "" && !strings.Contains(buscar.Plegar(line), q) {
			continue
		}
		if level != "" && !lineHasLevel(line, level) {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
		n++
	}
	return b.String()
}

// lineHasLevel indica si line contiene level (ya en mayúsculas) como TOKEN,
// delimitado por caracteres no alfabéticos: así "INFO" casa "INFO[0007] paso" pero
// no "INFORMATION". Best-effort: los logs de build no tienen un esquema fijo.
func lineHasLevel(line, level string) bool {
	up := strings.ToUpper(line)
	from := 0
	for {
		i := strings.Index(up[from:], level)
		if i < 0 {
			return false
		}
		i += from
		before := i == 0 || !isASCIILetter(up[i-1])
		after := i+len(level) == len(up) || !isASCIILetter(up[i+len(level)])
		if before && after {
			return true
		}
		from = i + 1
	}
}

func isASCIILetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}
