// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"unicode"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # Los atajos de teclado (`lib/atajos.ts`)
//
// Los de Apple Calendar y Google Calendar, en UNA tabla: de ella salen lo que hace cada
// tecla (Atajo) y la hoja de «?» (verAtajos). Así la hoja no puede prometer una tecla
// que no hace nada, ni una tecla quedarse sin explicar. Es la misma tabla que la web.

// accionAtajo es lo que hace una tecla: los mismos nombres que `AccionAtajo` en la web.
type accionAtajo string

const (
	atajoHoy    accionAtajo = "today"
	atajoSig    accionAtajo = "next"
	atajoAnt    accionAtajo = "prev"
	atajoMes    accionAtajo = "month"
	atajoSemana accionAtajo = "week"
	atajoDia    accionAtajo = "day"
	atajoAgenda accionAtajo = "agenda"
	atajoCrear  accionAtajo = "create"
	atajoBuscar accionAtajo = "search"
	atajoAyuda  accionAtajo = "help"
)

type filaAtajo struct {
	teclas      []rune // ya en minúscula
	accion      accionAtajo
	descripcion string // clave del catálogo
}

var gruposAtajos = []struct {
	titulo string
	atajos []filaAtajo
}{
	{"shortcuts.group.navigate", []filaAtajo{
		{[]rune{'t'}, atajoHoy, "shortcuts.today"},
		{[]rune{'n', 'j'}, atajoSig, "shortcuts.next"},
		{[]rune{'p', 'k'}, atajoAnt, "shortcuts.prev"},
	}},
	{"shortcuts.group.views", []filaAtajo{
		{[]rune{'m'}, atajoMes, "view.month"},
		{[]rune{'w'}, atajoSemana, "view.week"},
		{[]rune{'d'}, atajoDia, "view.day"},
		{[]rune{'a'}, atajoAgenda, "view.agenda"},
	}},
	{"shortcuts.group.actions", []filaAtajo{
		{[]rune{'c'}, atajoCrear, "nav.createEvent"},
		{[]rune{'/'}, atajoBuscar, "header.search"},
		{[]rune{'?'}, atajoAyuda, "shortcuts.help"},
	}},
}

// accionDeTecla es la acción de una tecla, o "" si no es un atajo.
func accionDeTecla(r rune) accionAtajo {
	r = unicode.ToLower(r)
	for _, g := range gruposAtajos {
		for _, a := range g.atajos {
			for _, k := range a.teclas {
				if k == r {
					return a.accion
				}
			}
		}
	}
	return ""
}

// Atajo son las teclas de CalendarApp.tsx. La ventana solo las entrega cuando nada tiene
// el foco —un campo o un diálogo se quedan su tecla—, que es lo que hace `esAtajoDeApp`
// en la web. Devuelve si la tecla era suya.
func (a *App) Atajo(r rune) bool {
	p := a.P
	switch accionDeTecla(r) {
	case atajoHoy:
		p.AlIrHoy()
	case atajoMes:
		p.AlCambiarVista(VistaMes)
	case atajoSemana:
		p.AlCambiarVista(VistaSemana)
	case atajoDia:
		p.AlCambiarVista(VistaDia)
	case atajoAgenda:
		p.AlCambiarVista(VistaAgenda)
	case atajoSig:
		p.navegar(1)
	case atajoAnt:
		p.navegar(-1)
	case atajoCrear:
		p.AbrirNuevoEvento()
	case atajoBuscar:
		if p.AlBuscar == nil {
			return false
		}
		p.AlBuscar()
	case atajoAyuda:
		return a.verAtajos()
	default:
		return false
	}
	return true
}

// hojaDeAtajos son los grupos de la hoja en el idioma de la pantalla.
func (p *Pantalla) hojaDeAtajos() []ui.GrupoDeAtajos {
	hoja := make([]ui.GrupoDeAtajos, 0, len(gruposAtajos))
	for _, g := range gruposAtajos {
		h := ui.GrupoDeAtajos{Titulo: p.t(g.titulo, nil)}
		for _, a := range g.atajos {
			teclas := make([]string, 0, len(a.teclas))
			for _, k := range a.teclas {
				teclas = append(teclas, string(k))
			}
			h.Atajos = append(h.Atajos, ui.AtajoDeTeclado{Teclas: teclas, Descripcion: p.t(a.descripcion, nil)})
		}
		hoja = append(hoja, h)
	}
	return hoja
}

// verAtajos abre la hoja de «?» (`ShortcutsModal`). Sin lienzo no hay dónde abrirla.
func (a *App) verAtajos() bool {
	lienzo := a.lienzo()
	if lienzo == nil {
		return false
	}
	p := a.P
	ui.MostrarHojaAtajos(lienzo, p.t("shortcuts.title", nil), p.t("common.close", nil), p.hojaDeAtajos())
	return true
}
