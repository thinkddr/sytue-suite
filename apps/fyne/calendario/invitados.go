// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Quién organiza y qué ha contestado cada invitado: GuestList.tsx. Sale en la vista
// rápida y, en el formulario, como insignia dentro de la píldora de cada invitado. La
// respuesta va en TEXTO dentro de la insignia; el tono solo acompaña.

// estadoInvitado es `guestStatus` de lib/ical.ts: el PARTSTAT (en cualquier caja) como
// lo enseña la UI. Lo que no es sí, quizá o no —NEEDS-ACTION, DELEGATED, nada— es «a la
// espera».
func estadoInvitado(partstat string) string {
	switch strings.ToUpper(strings.TrimSpace(partstat)) {
	case "ACCEPTED":
		return "accepted"
	case "TENTATIVE":
		return "tentative"
	case "DECLINED":
		return "declined"
	}
	return "pending"
}

// tonoInvitado es `GUEST_TONE`.
var tonoInvitado = map[string]ui.Tono{
	"accepted":  ui.InsigniaExito,
	"tentative": ui.InsigniaAviso,
	"declined":  ui.InsigniaPeligro,
	"pending":   ui.InsigniaNeutra,
}

// ordenInvitado es el `ORDEN` de `sortGuests`: quien va, quizá, a la espera y no.
var ordenInvitado = map[string]int{"accepted": 0, "tentative": 1, "pending": 2, "declined": 3}

// invitadosOrdenados es `sortGuests`, estable: dentro de cada respuesta, el orden del
// evento.
func invitadosOrdenados(l []api.Attendee) []api.Attendee {
	out := slices.Clone(l)
	slices.SortStableFunc(out, func(a, b api.Attendee) int {
		return ordenInvitado[estadoInvitado(a.PartStat)] - ordenInvitado[estadoInvitado(b.PartStat)]
	})
	return out
}

// insigniaInvitado es la insignia de una respuesta, con su texto del catálogo.
func (p *Pantalla) insigniaInvitado(partstat string) *ui.Insignia {
	e := estadoInvitado(partstat)
	return ui.NuevaInsignia(p.t("guests.status."+e, nil), tonoInvitado[e])
}

// filaOrganizador es `<dl class="flex items-baseline gap-1.5">`: «Organizador» en
// `text-muted` y el nombre (o la dirección) recortado en una línea.
func (p *Pantalla) filaOrganizador(organizador string, ancho float32, esc tema.Escalon) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	rotulo := p.t("guests.organizer", nil)
	w := ui.Medir(rotulo, esc, ui.Normal).Width
	return container.New(ui.Fila{Hueco: espacio1p5},
		ui.NuevoParrafo(rotulo, esc, ui.Normal, "text-muted", v).A(w),
		ui.NuevoParrafo(organizador, esc, ui.Normal, "text", v).A(max(ancho-w-espacio1p5, 0)).Recortado(1))
}

// seccionInvitados es GuestList: el organizador, el rótulo «Invitados» y una fila por
// invitado —nombre o dirección y la insignia de su respuesta—. Nil si el evento no
// tiene ni organizador ni invitados: un evento personal no gana una sección vacía.
func (p *Pantalla) seccionInvitados(ev api.Event, ancho float32, esc tema.Escalon) fyne.CanvasObject {
	organizador := api.SinMailto(ev.Organizer)
	if organizador == "" && len(ev.Attendees) == 0 {
		return nil
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var filas []fyne.CanvasObject
	if organizador != "" {
		filas = append(filas, p.filaOrganizador(organizador, ancho, esc))
	}
	if len(ev.Attendees) > 0 {
		filas = append(filas, textoXS(p.t("event.guests", nil), "text-subtle", ancho, 1))
		lista := make([]fyne.CanvasObject, 0, len(ev.Attendees))
		for _, at := range invitadosOrdenados(ev.Attendees) {
			nombre := at.CommonName
			if nombre == "" {
				nombre = api.SinMailto(at.Email)
			}
			insignia := p.insigniaInvitado(at.PartStat)
			w := max(ancho-insignia.MinSize().Width-espacio2, 0)
			lista = append(lista, container.NewBorder(nil, nil, nil, container.NewCenter(insignia),
				ui.NuevoParrafo(nombre, esc, ui.Normal, "text", v).A(w).Recortado(1)))
		}
		filas = append(filas, container.New(ui.Columna{Hueco: tema.Espacio(1)}, lista...))
	}
	return container.New(ui.Columna{Hueco: espacio1p5}, filas...)
}
