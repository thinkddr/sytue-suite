// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"slices"
	"strings"
	"time"
)

// «Coincide con…», «Mostrar como» y «los que he rechazado»: lo que decide si un evento
// ocupa la hora de quien mira. Es `lib/solapes.ts` y `ocultoPorRechazo` de la web, con
// la misma regla que el free/busy del servicio (`CountsAsBusy`).

// Disponible dice si el evento se marcó «disponible» (TRANSP:TRANSPARENT): sigue en la
// agenda pero no ocupa la hora en el free/busy ni en «buscar hueco».
func Disponible(o Ocurrencia) bool {
	return strings.EqualFold(strings.TrimSpace(o.Event.Transparency), "TRANSPARENT")
}

// respuestaDe es `miRespuesta` de lib/ical.ts: lo que ha contestado `correo` («accepted»,
// «tentative», «declined» o «pending»), o "" si no está invitado.
func respuestaDe(o Ocurrencia, correo string) string {
	correo = strings.ToLower(strings.TrimSpace(correo))
	if correo == "" {
		return ""
	}
	for _, at := range o.Event.Attendees {
		if strings.TrimPrefix(strings.ToLower(strings.TrimSpace(at.Email)), "mailto:") == correo {
			return estadoInvitado(at.PartStat)
		}
	}
	return ""
}

// OcultoPorRechazo es `ocultoPorRechazo`: con «Mostrar los eventos que he rechazado»
// apagado, lo que YO he rechazado sale de la vista (sigue guardado y en la búsqueda).
func OcultoPorRechazo(aj Ajustes, o Ocurrencia, correo string) bool {
	return !aj.Rechazados && respuestaDe(o, correo) == "declined"
}

// Solape es un evento que se pisa con el que se edita, con sus horas en la zona de la
// pantalla.
type Solape struct {
	O           Ocurrencia
	Inicio, Fin time.Time
}

// Solapes es `solapes`: las ocurrencias que se pisan con [inicio, fin), por orden de
// inicio. No cuentan el propio evento (`uid`), los de día completo, los «disponibles» ni
// los que `correo` ha rechazado; y el mismo evento visto desde dos calendarios, una vez.
// Solo se mira lo que la vista ya tiene cargado: es un aviso, no un veto.
func Solapes(eventos []Ocurrencia, inicio, fin time.Time, uid, correo string, loc *time.Location) []Solape {
	if !fin.After(inicio) {
		return nil
	}
	vistos := map[string]bool{}
	var out []Solape
	for _, o := range eventos {
		if (uid != "" && o.Event.UID == uid) || o.Event.AllDay || Disponible(o) || respuestaDe(o, correo) == "declined" {
			continue
		}
		ini, end := ocurrenciaInstantes(o, loc)
		if end.IsZero() || !ini.Before(fin) || !end.After(inicio) {
			continue
		}
		clave := o.Event.UID + "|" + ini.UTC().Format(time.RFC3339)
		if vistos[clave] {
			continue
		}
		vistos[clave] = true
		out = append(out, Solape{O: o, Inicio: ini, Fin: end})
	}
	slices.SortStableFunc(out, func(a, b Solape) int { return a.Inicio.Compare(b.Inicio) })
	return out
}
