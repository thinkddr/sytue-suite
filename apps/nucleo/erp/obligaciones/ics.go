// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package obligaciones

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

// Evento es una fila del calendario ya traducida: el manejador resuelve los
// rótulos en el idioma de quien descarga y aquí solo se serializa.
type Evento struct {
	UID         string
	Fecha       time.Time
	Resumen     string
	Descripcion string
	URL         string
	Categoria   string
	// AvisoDias: cuántos días antes salta el aviso. Cero = sin aviso.
	AvisoDias int
	// Repetir es la regla de repetición (RRULE), como "FREQ=WEEKLY;BYDAY=MO". Un
	// evento que se repite avisa ese mismo día a las nueve, no la víspera.
	Repetir string
}

// ICS serializa los eventos como calendario iCalendar (RFC 5545): un evento de
// día completo por vencimiento, con un aviso `AvisoDias` antes y otro la víspera.
//
// Se escribe a mano porque es un formato de texto de veinte líneas y ninguna
// dependencia hace falta para escribirlo bien: las dos reglas que importan son
// el plegado a 75 octetos y el escape de `,;\` y saltos de línea. Sin la
// primera, Apple Calendar rechaza el fichero; sin la segunda, una nota con una
// coma parte el evento en dos campos.
func ICS(nombre string, eventos []Evento, ahora time.Time) []byte {
	var b bytes.Buffer
	linea := func(s string) {
		b.WriteString(plegar(s))
		b.WriteString("\r\n")
	}
	linea("BEGIN:VCALENDAR")
	linea("VERSION:2.0")
	linea("PRODID:-//Sytue//ERP//ES")
	linea("CALSCALE:GREGORIAN")
	linea("METHOD:PUBLISH")
	linea("X-WR-CALNAME:" + escapar(nombre))
	sello := ahora.UTC().Format("20060102T150405Z")
	for _, e := range eventos {
		linea("BEGIN:VEVENT")
		linea("UID:" + e.UID)
		linea("DTSTAMP:" + sello)
		linea("DTSTART;VALUE=DATE:" + e.Fecha.Format("20060102"))
		linea("DTEND;VALUE=DATE:" + e.Fecha.AddDate(0, 0, 1).Format("20060102"))
		avisos := []string{"PT9H"}
		if e.Repetir != "" {
			linea("RRULE:" + e.Repetir)
		} else {
			avisos = avisos[:0]
			for _, dias := range []int{e.AvisoDias, 1} {
				if dias > 0 {
					avisos = append(avisos, fmt.Sprintf("-P%dD", dias))
				}
			}
		}
		linea("SUMMARY:" + escapar(e.Resumen))
		if e.Descripcion != "" {
			linea("DESCRIPTION:" + escapar(e.Descripcion))
		}
		if e.URL != "" {
			linea("URL:" + e.URL)
		}
		if e.Categoria != "" {
			linea("CATEGORIES:" + escapar(e.Categoria))
		}
		linea("TRANSP:TRANSPARENT")
		for _, disparo := range avisos {
			linea("BEGIN:VALARM")
			linea("ACTION:DISPLAY")
			linea("DESCRIPTION:" + escapar(e.Resumen))
			linea("TRIGGER:" + disparo)
			linea("END:VALARM")
		}
		linea("END:VEVENT")
	}
	linea("END:VCALENDAR")
	return b.Bytes()
}

func escapar(s string) string {
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`)
	return r.Replace(s)
}

// plegar corta la línea en trozos de como mucho 75 octetos sin partir ninguna
// secuencia UTF-8; cada continuación empieza por un espacio.
func plegar(s string) string {
	const tope = 75
	if len(s) <= tope {
		return s
	}
	var b strings.Builder
	largo := 0
	for _, r := range s {
		n := len(string(r))
		if largo+n > tope {
			b.WriteString("\r\n ")
			largo = 1
		}
		b.WriteRune(r)
		largo += n
	}
	return b.String()
}
