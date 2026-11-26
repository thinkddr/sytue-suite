// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ical

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/emersion/go-ical"
)

// ResponderAsistente escribe en el iCalendar de un objeto la respuesta (PARTSTAT) del
// asistente con ese correo en todos sus VEVENT (el maestro y sus excepciones), y quita
// el RSVP pendiente. Devuelve el objeto reescrito y si ese correo estaba invitado: si no
// lo estaba, `data` vuelve tal cual. El resto del objeto se conserva entero.
func ResponderAsistente(data, correo, partstat string) (string, bool, error) {
	correo = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.ToLower(correo), "mailto:")))
	partstat = strings.ToUpper(strings.TrimSpace(partstat))
	if correo == "" || partstat == "" || strings.ContainsAny(partstat, "\r\n;:,\"") {
		return data, false, fmt.Errorf("ics: respuesta sin correo o con un PARTSTAT inválido")
	}
	cal, err := ical.NewDecoder(strings.NewReader(data)).Decode()
	if err != nil {
		return data, false, fmt.Errorf("ics: decodificar objeto: %w", err)
	}
	encontrado := false
	for _, child := range cal.Children {
		if child.Name != ical.CompEvent {
			continue
		}
		for i := range child.Props[ical.PropAttendee] {
			p := &child.Props[ical.PropAttendee][i]
			email := strings.ToLower(strings.TrimSpace(p.Value))
			email = strings.TrimPrefix(email, "mailto:")
			if email != correo {
				continue
			}
			p.Params.Set("PARTSTAT", partstat)
			p.Params.Del("RSVP")
			encontrado = true
		}
	}
	if !encontrado {
		return data, false, nil
	}
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return data, false, err
	}
	return buf.String(), true, nil
}
