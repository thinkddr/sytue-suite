// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ical

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-ical"
)

// Partir una serie sin perder lo que contestaron los invitados (Sytue Office).
//
// «Solo este» y «este y los siguientes» sacan la ocurrencia (o la serie desde ella) a un
// evento NUEVO, con otro UID, hecho con lo que leyó el formulario al abrirse. Lo que los
// invitados contestaron entre medias —a esa ocurrencia, que crea su excepción, o a la
// serie— se quedaba en el evento de antes, que deja de tener esa ocurrencia: no seguía a
// la persona al evento nuevo. DividirConRespuestas lo lleva, igual que
// ConservarRespuestas al guardar el evento entero.

// DividirConRespuestas devuelve `nuevo` —el evento que sale de partir la serie, hecho con
// lo que leyó el formulario— con lo que contestaron los invitados según `guardado` (la
// serie tal como está ahora en el almacén):
//
//   - «solo este» (siguientes=false): cada invitado de `nuevo` que la persona no tocó
//     lleva el PARTSTAT que tiene en ESA ocurrencia (la que empieza en `ocurrencia`): el
//     de su excepción si la tiene —la que creó una respuesta mientras tanto— o, si no, el
//     del maestro;
//   - «este y los siguientes»: los del maestro, y las excepciones de las ocurrencias desde
//     `ocurrencia` pasan a la serie nueva (con su UID y su SEQUENCE), cada una con lo que
//     se contestó en ella, si la serie nueva se repite igual y a la misma hora (si no,
//     apuntarían a ocurrencias que no existen, como en CarryOver); con `serie`, lo que la
//     persona cambió en la serie les llega ([LlevarALasExcepciones]);
//   - quien la persona quitó, añadió o cambió de ROLE queda como ella lo dejó, también en
//     esas excepciones ([ConservarRespuestas]);
//   - si `nuevo` no trae organizador (el formulario no lo manda al partir), lleva el de la
//     serie: es a quien contestan los invitados.
//
// Un `guardado` vacío o que no se deja leer no tiene nada que llevar: vuelve `nuevo`.
func DividirConRespuestas(nuevo, guardado string, ocurrencia time.Time, siguientes, serie bool) (string, error) {
	if strings.TrimSpace(guardado) == "" {
		return nuevo, nil
	}
	calNuevo, err := ical.NewDecoder(strings.NewReader(nuevo)).Decode()
	if err != nil {
		return "", fmt.Errorf("ical: decodificar el recurso nuevo: %w", err)
	}
	calGuardado, err := ical.NewDecoder(strings.NewReader(guardado)).Decode()
	if err != nil {
		return nuevo, nil //nolint:nilerr // lo guardado ilegible no puede bloquear la edición
	}
	mN, mG := masterComponent(calNuevo), masterComponent(calGuardado)
	if mN == nil || mG == nil {
		return nuevo, nil
	}
	// El formulario no manda el organizador de lo que sale de la serie (`copiaContenido`):
	// sin él, las respuestas al evento nuevo no tendrían a quién ir ni se aplicarían.
	organizador := mN.Props.Get(ical.PropOrganizer) == nil && mG.Props.Get(ical.PropOrganizer) != nil
	if organizador {
		mN.Props[ical.PropOrganizer] = copiaPropiedades(mG.Props[ical.PropOrganizer])
	}
	if !siguientes {
		fuente := mG
		for _, c := range calGuardado.Children {
			if excepcionRecortada(c, ocurrencia, false) {
				fuente = c
			}
		}
		if !ponerRespuestas(mN, asistentesDe(fuente)) && !organizador {
			return nuevo, nil
		}
		return codificar(calNuevo)
	}
	if mismaRepeticion(mN, mG, ocurrencia) {
		uid := mN.Props.Get(ical.PropUID)
		for _, c := range calGuardado.Children {
			if !excepcionRecortada(c, ocurrencia, true) {
				continue
			}
			ex := copiaComponente(c)
			ex.Props[ical.PropUID] = []ical.Prop{copiaPropiedad(*uid)}
			if s := mN.Props.Get(ical.PropSequence); s != nil {
				ex.Props[ical.PropSequence] = []ical.Prop{copiaPropiedad(*s)}
			} else {
				delete(ex.Props, ical.PropSequence)
			}
			if serie {
				llevarCambios(mG, mN, ex)
			}
			calNuevo.Children = append(calNuevo.Children, ex)
		}
	}
	out, err := codificar(calNuevo)
	if err != nil {
		return "", err
	}
	return ConservarRespuestas(out, guardado)
}

// mismaRepeticion dice si la serie nueva (maestro n) genera, desde `ocurrencia`, las
// mismas ocurrencias que la guardada (maestro g): empieza en esa ocurrencia, en la misma
// zona y se repite con el mismo patrón (sin contar COUNT ni UNTIL).
func mismaRepeticion(n, g *ical.Component, ocurrencia time.Time) bool {
	inicio, _, err := PropertyTime(n.Props, ical.PropDateTimeStart)
	if err != nil || !inicio.Equal(ocurrencia) || componentAllDay(n) != componentAllDay(g) {
		return false
	}
	if !componentAllDay(n) && zonaSemantica(n.Props.Get(ical.PropDateTimeStart)) != zonaSemantica(g.Props.Get(ical.PropDateTimeStart)) {
		return false
	}
	pn, pg := patronDeLaRegla(n), patronDeLaRegla(g)
	return pn != "" && pn == pg
}

// patronDeLaRegla es la regla del VEVENT por lo que significa ([reglaSemantica]) sin su
// final (COUNT, UNTIL); "" si no se repite.
func patronDeLaRegla(ev *ical.Component) string {
	if ev.Props.Get(ical.PropRecurrenceRule) == nil {
		return ""
	}
	var partes []string
	for _, p := range strings.Split(reglaSemantica(ev), ";") {
		if !strings.HasPrefix(p, "COUNT=") && !strings.HasPrefix(p, "UNTIL=") {
			partes = append(partes, p)
		}
	}
	return strings.Join(partes, ";")
}

func codificar(cal *ical.Calendar) (string, error) {
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return "", fmt.Errorf("ical: re-serializar el recurso: %w", err)
	}
	return buf.String(), nil
}
