// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "strings"

// UnirICS junta los objetos iCalendar de un calendario —CalDAV guarda uno por
// evento— en un único VCALENDAR. Es lo que importan Google Calendar y Outlook,
// que no aceptan una carpeta con un .ics por evento.
//
// No reinterpreta nada: cada componente (VEVENT, VTODO, VTIMEZONE…) se copia
// línea a línea, con sus plegados. Las propiedades del VCALENDAR de cada objeto
// (VERSION, PRODID, METHOD) sobran, porque el nuevo lleva las suyas, y una zona
// horaria repetida sale una sola vez.
func UnirICS(nombre string, objetos []string) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Sytue//Sytue Calendar//EN\r\nCALSCALE:GREGORIAN\r\n")
	if nombre != "" {
		b.WriteString("X-WR-CALNAME:" + escaparTextoICS(nombre) + "\r\n")
	}
	zonas := map[string]bool{}
	for _, obj := range objetos {
		var (
			comp  []string
			nivel int
			tzid  string
		)
		for linea := range strings.SplitSeq(strings.ReplaceAll(obj, "\r\n", "\n"), "\n") {
			mayus := strings.ToUpper(linea)
			if nivel == 0 {
				// Fuera de un componente solo hay envoltorio y propiedades del calendario.
				if strings.HasPrefix(mayus, "BEGIN:") && mayus != "BEGIN:VCALENDAR" {
					comp, nivel, tzid = []string{linea}, 1, ""
				}
				continue
			}
			comp = append(comp, linea)
			switch {
			case strings.HasPrefix(mayus, "BEGIN:"):
				nivel++
			case strings.HasPrefix(mayus, "END:"):
				nivel--
			case nivel == 1 && strings.HasPrefix(mayus, "TZID:"):
				tzid = linea[len("TZID:"):]
			}
			if nivel > 0 {
				continue
			}
			if strings.EqualFold(comp[0], "BEGIN:VTIMEZONE") {
				if zonas[tzid] {
					continue
				}
				zonas[tzid] = true
			}
			for _, l := range comp {
				b.WriteString(l + "\r\n")
			}
		}
	}
	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

// escaparTextoICS escapa un valor TEXT (RFC 5545 §3.3.11).
func escaparTextoICS(s string) string {
	return strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`).Replace(s)
}
