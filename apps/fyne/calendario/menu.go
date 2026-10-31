// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/agenda"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// El menú de Sytue Calendar: «Crear evento» arriba, el mini-mes, «Mis calendarios» y,
// si los hay, los compartidos conmigo.
//
// El carril, su ancho y su canalón son `ui.Lateral` —la misma pieza que montan las
// otras cinco apps—; lo de dentro es de Calendar.
func (p *Pantalla) menu() fyne.CanvasObject {
	crear := ui.NuevoBoton(p.t("nav.createEvent", nil), func() { p.cajonAbierto = false; p.crearHoy() }).
		ConIcono(iconos.Mas, 0)
	crear.Ancho = ui.AnchoUtilMenu // `fullWidth`
	crear.Inerte = p.SinConexion

	mini := NuevoMiniMes(p.locale(), p.Cursor)
	mini.Zona = p.zona()
	mini.RotuloAnterior = p.t("nav.prevMonth", nil)
	mini.RotuloSiguiente = p.t("nav.nextMonth", nil)
	mini.AlElegir = func(d time.Time) {
		if p.cajonAbierto && p.AlConmutarCajon != nil {
			p.AlConmutarCajon()
		} else {
			p.cajonAbierto = false
		}
		if p.AlElegirDia != nil {
			p.AlElegirDia(d)
		}
	}
	mini.AlDesplazar = p.AlDesplazarMes

	bloques := []fyne.CanvasObject{mini, p.seccionMisCalendarios()}
	if p.AlServiciosVinculados != nil {
		bloques = append(bloques, ui.NuevaFilaLateral(p.t("linkedServices.connected", nil), iconos.Trazo(iconos.Globo, ui.GlifoMenu, "sidebar-fg", tema.De(fyne.CurrentApp().Settings().ThemeVariant())).Objeto(), func() { p.cajonAbierto = false; p.AlServiciosVinculados() }))
	}
	if len(p.Compartidos) > 0 {
		bloques = append(bloques, p.seccionCompartidos())
	}
	// El cuerpo NO desplaza por su cuenta: `ui.Lateral` deja esa decisión a quien lo
	// monta, para no traer la barra del tema de Fyne a los seis menús de la suite.
	// Aquí cabe: mini-mes (177,25) y dos secciones cortas dentro de 744 − cabecera.
	cuerpo := agenda.EnColumna(0, bloques...)

	l := ui.NuevoLateral(p.t("nav.myCalendars", nil), cuerpo).ConCabecera(crear)
	// El pie es `<ImportData>` («Trae tus calendarios»), que solo se pinta si la
	// organización tiene un origen conectado. Sin él no hay pie: ni franja ni filete
	// (en la web, `empty:hidden` en el pie de Sidebar.tsx).
	if p.Importar != nil {
		if o := p.Importar.Objeto(ui.AnchoUtilMenu, tema.De(fyne.CurrentApp().Settings().ThemeVariant())); o != nil {
			l.Pie = o
		}
	}
	return l
}

// seccionMisCalendarios es el bloque «MIS CALENDARIOS» con sus acciones de cabecero.
func (p *Pantalla) seccionMisCalendarios() *ui.SeccionLateral {
	s := ui.NuevaSeccionLateral(p.t("nav.myCalendars", nil))
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// Los tres botones del cabecero son `SidebarAction`, la misma pieza que montan
	// Mail y Notepads; antes en la web eran tres `<button>` a mano con estas clases.
	if p.PuedeGestionarRecursos && !p.SinConexion {
		edificio := iconos.Trazo(iconos.Edificio, 16, "sidebar-fg-muted", v)
		s.Acciones = append(s.Acciones,
			ui.NuevaAccionLateralCon(p.t("resources.manage", nil), edificio, p.AlGestionarRecursos))
	}
	globo := iconos.Trazo(iconos.Globo, 16, "sidebar-fg-muted", v)
	suscribir := ui.NuevaAccionLateralCon(p.t("subscribe.byUrl", nil), globo, p.AlSuscribirse)
	suscribir.Inerte = p.SinConexion
	mas := iconos.Trazo(iconos.Mas, 16, "sidebar-fg-muted", v)
	crear := ui.NuevaAccionLateralCon(p.t("cal.create", nil), mas, p.AlCrearCalendario)
	crear.Inerte = p.SinConexion
	s.Acciones = append(s.Acciones, suscribir, crear)

	if p.CargandoCalendarios {
		// El esqueleto del menú va TEÑIDO para el cromo oscuro: `bg-subtle` ahí sería
		// un destello claro sobre negro, que es lo que dice el comentario de la web.
		s.Filas = []fyne.CanvasObject{
			agenda.Bloque(0, 36, true),
			agenda.Bloque(0, 36, true),
		}
		return s
	}
	if len(p.Calendarios) == 0 {
		s.Filas = []fyne.CanvasObject{
			ui.Texto(p.t("nav.noCalendars", nil), tema.TextoSM, ui.Normal, tema.Color("sidebar-fg-muted", v)),
		}
		return s
	}
	for _, c := range p.Calendarios {
		s.Filas = append(s.Filas, p.filaDe(c, false))
	}
	return s
}

// seccionCompartidos es «COMPARTIDOS CONMIGO»: mismo camino CalDAV que los propios,
// con la insignia de permiso en TEXTO y sin kebab.
func (p *Pantalla) seccionCompartidos() *ui.SeccionLateral {
	s := ui.NuevaSeccionLateral(p.t("cal.sharedWithMe", nil))
	for _, c := range p.Compartidos {
		s.Filas = append(s.Filas, p.filaDe(c, true))
	}
	return s
}

// filaDe compone la fila de un calendario.
func (p *Pantalla) filaDe(c api.Calendar, compartido bool) *FilaCalendario {
	f := NuevaFilaCalendario(c.Title(), colorDe(c.Color), p.visible(c.ID))
	f.Suscrito = c.ReadOnly && !compartido
	f.RotuloVer = p.t("nav.showCalendar", i18n.Vars{"name": c.Title()})
	id := c.ID
	f.AlConmutar = func() {
		if p.AlConmutarCalendario != nil {
			p.AlConmutarCalendario(id)
		}
	}
	if compartido {
		// El permiso viene del servicio («reader» o «writer») y la web lo pinta con dos
		// claves y dos tonos distintos.
		if c.Permission == api.PermisoEditar {
			f.Permiso, f.PermisoEditar = p.t("nav.permWriter", nil), true
		} else {
			f.Permiso = p.t("nav.permReader", nil)
		}
		return f
	}
	if !p.SinConexion {
		f.RotuloOpciones = p.t("nav.calendarOptions", i18n.Vars{"name": c.Title()})
		f.AlAbrirMenu = func(disparador fyne.CanvasObject) {
			if p.AlMenuCalendario != nil {
				p.AlMenuCalendario(c, disparador)
			}
		}
	}
	return f
}
