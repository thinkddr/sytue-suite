// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package caldav

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/emersion/go-ical"
)

// CarryOver devuelve el recurso `nuevo` con lo que la API REST no sabe expresar y el
// recurso guardado (`anterior`) sí tenía.
//
// El PUT REST regenera el iCalendar desde los campos del evento (EncodeEvent), y en esos
// campos no hay recordatorios: los VALARM se perdían, y `replaceReminders` —que los saca
// del ICS— borraba sus filas. La web no lo sufre porque edita por CalDAV y reinyecta los
// VALARM tal cual (`ical.ts`); cualquier cliente REST —la suite en Gio— le quitaba a la
// persona los recordatorios que había puesto en la web, sin decir nada.
//
// Se conservan:
//   - los VALARM del VEVENT maestro, si el nuevo no trae los suyos. Con `propios` el nuevo
//     trae SUS recordatorios editables (Recordatorios), aunque sean cero: entonces solo se
//     conservan los que la API no sabe expresar —un aviso por correo, un disparo a una hora
//     fija…—, que no se enseñan ni se pueden quitar desde ella;
//   - los VEVENT de excepción (RECURRENCE-ID), pero SOLO si la serie sigue siendo la misma
//     (mismo RRULE y mismo DTSTART): con otra regla o a otra hora, esas excepciones
//     apuntarían a ocurrencias que ya no existen.
func CarryOver(nuevo, anterior string, propios bool) (string, error) {
	if strings.TrimSpace(anterior) == "" {
		return nuevo, nil
	}
	calNuevo, err := ical.NewDecoder(strings.NewReader(nuevo)).Decode()
	if err != nil {
		return "", fmt.Errorf("ical: decodificar el recurso nuevo: %w", err)
	}
	calAnterior, err := ical.NewDecoder(strings.NewReader(anterior)).Decode()
	if err != nil {
		// Lo guardado no se deja leer: no hay nada que conservar, y el cambio no se pierde.
		return nuevo, nil //nolint:nilerr // un recurso viejo ilegible no puede bloquear la edición
	}
	maestroNuevo, maestroAnterior := masterComponent(calNuevo), masterComponent(calAnterior)
	if maestroNuevo == nil || maestroAnterior == nil {
		return nuevo, nil
	}

	tocado := false
	if propios || !tieneHijo(maestroNuevo, ical.CompAlarm) {
		for _, hijo := range maestroAnterior.Children {
			if hijo.Name != ical.CompAlarm {
				continue
			}
			if _, editable := minutosEditables(hijo); propios && editable {
				continue // lo sustituyen los que trae el nuevo
			}
			maestroNuevo.Children = append(maestroNuevo.Children, hijo)
			tocado = true
		}
	}
	if mismaSerie(maestroNuevo, maestroAnterior) && !tieneExcepciones(calNuevo) {
		for _, hijo := range calAnterior.Children {
			if hijo.Name == ical.CompEvent && hijo.Props.Get(ical.PropRecurrenceID) != nil {
				calNuevo.Children = append(calNuevo.Children, hijo)
				tocado = true
			}
		}
	}
	if !tocado {
		return nuevo, nil
	}
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(calNuevo); err != nil {
		return "", fmt.Errorf("ical: re-serializar el recurso: %w", err)
	}
	return buf.String(), nil
}

func tieneHijo(c *ical.Component, nombre string) bool {
	for _, hijo := range c.Children {
		if hijo.Name == nombre {
			return true
		}
	}
	return false
}

func tieneExcepciones(cal *ical.Calendar) bool {
	for _, hijo := range cal.Children {
		if hijo.Name == ical.CompEvent && hijo.Props.Get(ical.PropRecurrenceID) != nil {
			return true
		}
	}
	return false
}

// mismaSerie: los dos maestros repiten con la misma regla y desde el mismo inicio.
func mismaSerie(a, b *ical.Component) bool {
	valor := func(c *ical.Component, nombre string) string {
		if p := c.Props.Get(nombre); p != nil {
			return p.Value
		}
		return ""
	}
	regla := valor(a, ical.PropRecurrenceRule)
	return regla != "" && regla == valor(b, ical.PropRecurrenceRule) &&
		valor(a, ical.PropDateTimeStart) == valor(b, ical.PropDateTimeStart)
}
