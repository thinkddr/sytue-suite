// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// «Salas y recursos» dentro del formulario: ResourcePicker.tsx. Lista los recursos de la
// organización con LIBRE/OCUPADO en la franja del evento, deja añadirlos a la reserva y,
// al guardar, la App reserva cada uno (el servicio decide, de forma atómica, si queda
// aceptada o rechazada). Sin recursos en la organización la sección no se pinta.
//
//	<div class="flex flex-col gap-1.5">  <span class="text-sm font-semibold">Salas y recursos
//	  <div class="flex flex-col gap-2">
//	    <ul class="flex flex-col gap-1.5">  una fila por elegido:
//	      border-border rounded-md border px-2.5 py-2 · flex items-center gap-2
//	      icono 16 text-muted · nombre text-sm flex-1 truncate · Badge · aspa h-6 w-6
//	    <Select> «Añadir sala o recurso…» con «Nombre · 8 personas — libre», o
//	    <p class="text-text-subtle text-sm">Todos los recursos están añadidos.</p>

// selectorRecursos es el estado de la sección.
type selectorRecursos struct {
	// lista son los recursos con su disponibilidad; nil mientras se cargan.
	lista []api.Resource
	fallo bool
	// franja es la última pedida, para no repetir la consulta con la misma.
	franja string
	anadir *ui.Selector
	// ofrecidos son los recursos del selector, por posición (la 0 es el marcador).
	ofrecidos []api.Resource
}

// franjaRecursos es la del evento en instantes, con el minuto mínimo de la web.
func (f *formulario) franjaRecursos() (desde, hasta time.Time) {
	in := f.e.EntradaDe(f.p)
	hasta = in.End
	if m := in.Start.Add(time.Minute); hasta.Before(m) {
		hasta = m
	}
	return in.Start, hasta
}

// pedirRecursos consulta la disponibilidad si la franja ha cambiado.
func (f *formulario) pedirRecursos() {
	if f.rec == nil || f.a.Recursos == nil {
		return
	}
	desde, hasta := f.franjaRecursos()
	clave := desde.Format(time.RFC3339) + "|" + hasta.Format(time.RFC3339)
	if clave == f.rec.franja {
		return
	}
	f.rec.franja, f.rec.fallo = clave, false
	f.a.Recursos(desde, hasta, func(l []api.Resource, err error) {
		if f.rec.franja != clave {
			return // llegó tarde: ya se pidió otra franja
		}
		if err != nil {
			f.rec.fallo = true
		} else {
			f.rec.lista = append([]api.Resource{}, l...)
		}
		f.rehacer()
	})
}

// recursoOcupado es `isBusy` en la franja actual.
func (f *formulario) recursoOcupado(r api.Resource) bool {
	desde, hasta := f.franjaRecursos()
	return r.Ocupado(desde, hasta)
}

// bloqueRecursos es la sección entera, o nil si no hay que pintarla.
func (f *formulario) bloqueRecursos() fyne.CanvasObject {
	s := f.rec
	if s == nil || (s.lista != nil && len(s.lista) == 0) {
		return nil
	}
	p := f.p
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cabecera := alto(rotuloFuerte(p.t("event.resources", nil)), tema.TextoSM.Linea)
	grupo := func(hijo fyne.CanvasObject) fyne.CanvasObject {
		return container.New(ui.Columna{Hueco: espacio1p5}, cabecera, hijo)
	}
	switch {
	case s.fallo:
		return grupo(ui.NuevoParrafo(p.t("picker.error", nil), tema.TextoSM, ui.Normal, "text-subtle", v))
	case s.lista == nil:
		return grupo(ui.NuevoEsqueleto(0, tema.Espacio(9))) // `<Skeleton height="2.25rem" />`
	}

	porID := map[string]api.Resource{}
	for _, r := range s.lista {
		porID[r.ID] = r
	}
	var hijos []fyne.CanvasObject
	if len(f.e.Recursos) > 0 {
		filas := make([]fyne.CanvasObject, 0, len(f.e.Recursos))
		for _, id := range f.e.Recursos {
			filas = append(filas, f.filaRecurso(id, porID))
		}
		hijos = append(hijos, container.New(ui.Columna{Hueco: espacio1p5}, filas...))
	}
	s.ofrecidos = nil
	opciones := []string{p.t("picker.addPlaceholder", nil)}
	for _, r := range s.lista {
		if slices.Contains(f.e.Recursos, r.ID) {
			continue
		}
		s.ofrecidos = append(s.ofrecidos, r)
		opciones = append(opciones, p.opcionRecurso(r, f.recursoOcupado(r)))
	}
	if len(s.ofrecidos) > 0 {
		s.anadir.Opciones, s.anadir.Elegida = opciones, 0
		s.anadir.Refresh()
		hijos = append(hijos, s.anadir)
	} else {
		hijos = append(hijos, ui.NuevoParrafo(p.t("picker.allAdded", nil), tema.TextoSM, ui.Normal, "text-subtle", v))
	}
	return grupo(container.New(ui.Columna{Hueco: espacio2}, hijos...))
}

// opcionRecurso es la opción del selector: nombre, aforo en plural y estado, con la
// plantilla entera del catálogo.
func (p *Pantalla) opcionRecurso(r api.Resource, ocupado bool) string {
	estado := p.t("picker.free", nil)
	if ocupado {
		estado = p.t("picker.busy", nil)
	}
	if r.Capacity > 0 {
		return p.t("picker.optionWithCapacity", i18n.Vars{
			"name": r.Name, "capacity": p.t("resources.people", i18n.Vars{"count": r.Capacity}), "state": estado,
		})
	}
	return p.t("picker.option", i18n.Vars{"name": r.Name, "state": estado})
}

// iconoRecurso es `KIND_ICON`.
func iconoRecurso(clase string) iconos.Icono {
	if clase == api.RecursoEquipo {
		return iconos.Rejilla
	}
	return iconos.Edificio
}

// filaRecurso es el `<li>` de un recurso elegido.
func (f *formulario) filaRecurso(id string, porID map[string]api.Resource) fyne.CanvasObject {
	p := f.p
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r, conocido := porID[id]
	nombre := r.Name
	if !conocido {
		nombre = id
	}
	icono := iconos.Trazo(iconoRecurso(r.Kind), iconos.TallaSM, "text-muted", v).Objeto()
	texto := ui.NuevoParrafo(nombre, tema.TextoSM, ui.Normal, "text", v)
	texto.Lineas = 1
	insignia := ui.NuevaInsignia(p.t("picker.badgeFree", nil), ui.InsigniaExito)
	if conocido && f.recursoOcupado(r) {
		insignia = ui.NuevaInsignia(p.t("picker.badgeBusy", nil), ui.InsigniaPeligro)
	}
	rotulo := r.Name
	if !conocido {
		rotulo = p.t("picker.fallbackName", nil)
	}
	quitar := ui.NuevoBotonIcono(iconos.Cerrar, p.t("picker.remove", i18n.Vars{"name": rotulo}), func() {
		f.e.Recursos = slices.DeleteFunc(slices.Clone(f.e.Recursos), func(x string) bool { return x == id })
		f.rehacer()
	})
	// `grid h-6 w-6 rounded-md text-text-muted hover:bg-danger-bg hover:text-danger-text`.
	quitar.Lado, quitar.TamIcono, quitar.Radio = tema.Espacio(6), iconos.TallaXS, tema.Forma("radius-control")
	quitar.TrazoEncima, quitar.FondoEncima = "danger-text", "danger-bg"
	if f.e.SoloLectura {
		quitar.Inerte = true
	}
	fila := container.New(filaElastica{Hueco: espacio2, Elastico: 1}, icono, texto, insignia, quitar)
	caja := ui.NuevaCaja(tema.Color("surface-raised", v), tema.Color("border", v), tema.RadioMD)
	return container.New(&conCaja{caja: caja, X: espacio2p5, Y: espacio2}, append(caja.Objetos(), fila)...)
}

// construirRecursos arma la sección si la App sabe listar recursos.
func (f *formulario) construirRecursos() {
	if f.a.Recursos == nil {
		return
	}
	f.rec = &selectorRecursos{}
	f.rec.anadir = ui.NuevoSelector(nil, func(i int) {
		if i <= 0 || i > len(f.rec.ofrecidos) {
			return
		}
		f.e.Recursos = append(slices.Clone(f.e.Recursos), f.rec.ofrecidos[i-1].ID)
		f.rehacer()
	})
	if f.e.SoloLectura {
		f.rec.anadir.Inerte(true)
	}
}

// avisoReservas es el resumen de `bookSelectedResources`: cuántas reservas quedaron
// aceptadas y cuántas rechazadas, con las plantillas enteras del catálogo.
func avisoReservas(p *Pantalla, estados []string) (ui.TonoAviso, string) {
	reservadas, ocupadas := 0, 0
	for _, e := range estados {
		switch e {
		case api.ReservaAceptada:
			reservadas++
		case api.ReservaRechazada:
			ocupadas++
		}
	}
	hechas := p.t("booking.booked", i18n.Vars{"count": reservadas})
	if ocupadas > 0 {
		return ui.AvisoAdvertencia, p.t("booking.savedBusy", i18n.Vars{
			"booked": hechas, "busy": p.t("booking.busy", i18n.Vars{"count": ocupadas}),
		})
	}
	return ui.AvisoExito, p.t("booking.saved", i18n.Vars{"booked": hechas})
}

// reservaDe es lo que se manda al reservar un recurso para el evento `in`.
func reservaDe(uid string, in api.EventInput) api.Booking {
	b := api.Booking{
		UID: uid, Summary: in.Summary, Start: in.Start, End: in.End,
		RRule: strings.TrimPrefix(in.RRule, "RRULE:"),
	}
	if in.AllDay {
		b.AllDay = true
	} else {
		b.TZID = in.TZID
	}
	return b
}
