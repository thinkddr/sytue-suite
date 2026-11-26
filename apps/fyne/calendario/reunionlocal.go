// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Crear el enlace de la reunión desde el formulario en la edición sin cuenta (Sytue
// Office, la otra mitad de A-13). Con PistaEnlace el formulario tiene un campo libre
// para el enlace (no hay salas de Sytue Rooms); con Pantalla.Reunion, además, un botón
// que la crea con lo que haya en el calendario elegido:
//
//   - en uno de Google, «Añadir Google Meet»; en uno de Microsoft, «Reunión de Teams»: el
//     enlace lo crea el proveedor al subir el evento, así que el formulario dice que se
//     creará y, al guardar, la app lo pide (Reunion.Pedir); el enlace llega con la
//     sincronización;
//   - en los demás, si hay una cuenta de mensajería con Teams o Meet, la reunión se crea
//     ya con ella (Reunion.Crear) y su enlace se pega en el campo.
//
// Donde no hay con qué, queda el campo libre de siempre. Sin Reunion (nil), todo como
// hasta ahora; la edición conectada no lo mira.

// Reunion es lo que la app da para crear el enlace de la reunión desde el formulario.
type Reunion struct {
	// Boton es el rótulo del botón en ese calendario; "" si allí no hay con qué crearla.
	Boton func(calendario string) string
	// Pendiente, si el enlace de ese calendario lo crea su proveedor al guardar, es la
	// frase que lo dice; "" si se crea ya (Crear).
	Pendiente func(calendario string) string
	// Crear la crea ya, con el título y las horas del evento, y da su enlace.
	Crear func(ctx context.Context, calendario, titulo string, inicio, fin time.Time) (string, error)
	// Pedir apunta, al guardar, que el proveedor del calendario cree la reunión del evento
	// uid (el formulario lo llama justo antes de Guardar).
	Pedir func(calendario, uid string)
}

// reunionLocal es Reunion si se usa: con el campo libre del enlace.
func (p *Pantalla) reunionLocal() *Reunion {
	if p.PistaEnlace != "" && p.Reunion != nil && p.Reunion.Boton != nil {
		return p.Reunion
	}
	return nil
}

// botonReunion es el rótulo del botón en el calendario elegido, o "" si no va: sin
// Reunion, en consulta, con un enlace ya puesto o donde no hay con qué crearla.
func (f *formulario) botonReunion() string {
	r := f.p.reunionLocal()
	if r == nil || f.e.SoloLectura || strings.TrimSpace(f.e.Videollamada) != "" {
		return ""
	}
	return r.Boton(f.e.CalendarioID)
}

// pendienteReunion es la frase de un calendario cuyo proveedor crea la reunión al
// guardar, o "" si se crea ya.
func (f *formulario) pendienteReunion() string {
	if r := f.p.reunionLocal(); r != nil && r.Pendiente != nil {
		return r.Pendiente(f.e.CalendarioID)
	}
	return ""
}

// bloqueReunion es lo que va bajo el campo del enlace: el botón que crea la reunión, o la
// frase de que la creará el proveedor al guardar con «Quitar», y el error si no se pudo.
// nil si no hay nada.
func (f *formulario) bloqueReunion() fyne.CanvasObject {
	rotulo := f.botonReunion()
	if rotulo == "" {
		return nil
	}
	p := f.p
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var hijos []fyne.CanvasObject
	if f.reunionPedida {
		quitar := ui.NuevoBoton(p.t("common.remove", nil), func() {
			f.reunionPedida = false
			f.rehacer()
		}).Con(ui.Fantasma, ui.TallaSM)
		quitar.Rotulo = p.t("event.conferenceRemoveAria", nil)
		hijos = append(hijos, ui.NuevoParrafo(f.pendienteReunion(), tema.TextoSM, ui.Normal, "text-muted", v).A(f.util()), encajar(quitar))
	} else {
		r := rotulo
		if f.creandoReunion {
			r = p.t("event.conferenceCreating", nil)
		}
		crear := ui.NuevoBoton(r, f.crearReunion).Con(ui.Secundario, ui.TallaSM)
		crear.Inerte = f.creandoReunion || (f.a.Reunion == nil && f.pendienteReunion() == "")
		hijos = append(hijos, encajar(crear))
	}
	if f.errorReunion {
		hijos = append(hijos, ui.NuevoParrafo(p.t("event.conferenceError", nil), tema.TextoXS, ui.Normal, "danger-text", v))
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(1)}, hijos...)
}

// crearReunion es el botón: en un calendario cuyo proveedor la crea al guardar, la deja
// pedida; si no, la crea ya y pega su enlace en el campo.
func (f *formulario) crearReunion() {
	r := f.p.reunionLocal()
	if r == nil || f.creandoReunion {
		return
	}
	f.errorReunion = false
	if f.pendienteReunion() != "" {
		f.reunionPedida = true
		f.rehacer()
		return
	}
	if f.a.Reunion == nil {
		return
	}
	f.leerFechas()
	titulo := strings.TrimSpace(f.titulo.Texto())
	if titulo == "" {
		titulo = f.p.t("event.conferenceDefaultSummary", nil)
	}
	f.creandoReunion = true
	f.rehacer()
	cal := f.e.CalendarioID
	// La reunión va a la hora real del evento: los campos dicen la de pared en su zona.
	inicio, fin := f.e.Inicio, f.e.Fin
	if !f.e.TodoElDia {
		loc := f.e.zonaDe(f.p)
		inicio, fin = enLaPared(inicio, f.p.zona(), loc), enLaPared(fin, f.p.zona(), loc)
	}
	f.a.Reunion(cal, titulo, inicio, fin, func(enlace string, err error) {
		f.creandoReunion = false
		if err != nil || strings.TrimSpace(enlace) == "" {
			f.errorReunion = true
		} else if f.enlace != nil {
			f.enlace.PonerTexto(enlace)
		}
		f.rehacer()
	})
}

// alCambiarEnlace es el campo libre: con algo escrito el botón se va (y lo pedido al
// proveedor se olvida: la persona puso el suyo), y vuelve si se vacía.
func (f *formulario) alCambiarEnlace(s string) {
	antes := strings.TrimSpace(f.e.Videollamada) != ""
	f.e.Videollamada = strings.TrimSpace(s)
	if f.p.reunionLocal() == nil {
		return
	}
	ahora := f.e.Videollamada != ""
	if ahora {
		f.reunionPedida = false
	}
	if antes != ahora {
		f.errorReunion = false
		f.rehacer()
	}
}

// alCambiarCalendarioReunion: lo pedido a un proveedor no vale en el calendario de otro.
func (f *formulario) alCambiarCalendarioReunion() {
	f.reunionPedida, f.errorReunion = false, false
}

// pedirReunion, al guardar, apunta lo pedido al proveedor del calendario elegido.
func (f *formulario) pedirReunion() {
	r := f.p.reunionLocal()
	if r == nil || !f.reunionPedida || r.Pedir == nil || f.botonReunion() == "" {
		return
	}
	if f.e.UID == "" {
		f.e.UID = nuevoUID()
	}
	r.Pedir(f.e.CalendarioID, f.e.UID)
}
