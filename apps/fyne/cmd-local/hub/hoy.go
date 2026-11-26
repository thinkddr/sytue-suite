// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/apps/hub"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/indice"
)

// # «Hoy» en la portada (HU-04)
//
// Lo del día, leído de los almacenes de Calendar y Notepads y del índice de Mail
// (`indice.LeerHoy`): los eventos de hoy, los recordatorios que suenan hoy, las tareas
// por hacer y el correo sin leer. Cada fila abre su app, como lo reciente. No es el
// «Hoy» del Hub conectado (`hub.Hoy`), que sale del servidor.

// MaxTareasPortada son las tareas que se ven en la tarjeta; las demás, en Notepads.
const MaxTareasPortada = 3

// FilaHoy es una línea de la tarjeta, antes de pintarla.
type FilaHoy struct {
	Elemento indice.Elemento
	Cuando   string // «09:00–09:30», «Todo el día», «3 sin leer»
	Resumen  string
}

// hora es la hora local corta.
func hora(l i18n.Locale, t time.Time) string { return i18n.FormatTime(l, t.Local()) }

// FilasHoy son las líneas de la tarjeta, en orden: eventos, recordatorios, tareas (sin
// las que ya salen por su recordatorio) y el correo.
func FilasHoy(l i18n.Locale, h indice.Hoy) []FilaHoy {
	tx := func(clave string, vars ...string) string { return texto(l, clave, vars...) }
	var out []FilaHoy
	for _, e := range h.Eventos {
		cuando := tx("hoy.todoElDia")
		if !e.TodoElDia {
			cuando = hora(l, e.Fecha)
			if e.Fin.After(e.Fecha) {
				cuando += "–" + hora(l, e.Fin)
			}
		}
		out = append(out, FilaHoy{Elemento: e.Elemento, Cuando: cuando, Resumen: e.Detalle})
	}
	vistos := map[string]bool{}
	for _, r := range h.Recordatorios {
		vistos[r.ID] = true
		out = append(out, FilaHoy{Elemento: r, Cuando: hora(l, r.Fecha), Resumen: tx("hoy.recordatorio")})
	}
	// Las tareas que ya se ven, como fila propia o por su recordatorio.
	puestas, solas := 0, 0
	for _, t := range h.Tareas {
		switch {
		case vistos[t.ID]:
			puestas++
		case solas < MaxTareasPortada:
			solas++
			puestas++
			out = append(out, FilaHoy{Elemento: t, Resumen: tx("hoy.tarea")})
		}
	}
	if resto := h.TotalTareas - puestas; resto > 0 {
		out = append(out, FilaHoy{Elemento: indice.Elemento{Producto: "notes", Titulo: tx("hoy.masTareas", "n", strconv.Itoa(resto))}})
	}
	if h.SinLeer > 0 {
		resumen := ""
		if len(h.Correos) > 0 {
			c := h.Correos[0]
			resumen = strings.Join(noVacios(c.Detalle, c.Titulo), " · ")
		}
		titulo := tx("hoy.sinLeer.uno")
		if h.SinLeer != 1 {
			titulo = tx("hoy.sinLeer", "n", strconv.Itoa(h.SinLeer))
		}
		out = append(out, FilaHoy{Elemento: indice.Elemento{Producto: "mail", Titulo: titulo}, Resumen: resumen})
	}
	return out
}

func noVacios(ss ...string) []string {
	var out []string
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// pintarHoy rehace la tarjeta con lo leído. En el hilo de la interfaz.
func (p *Portada) pintarHoy() {
	p.mu.Lock()
	h := p.hoy
	p.mu.Unlock()
	l := p.T.Locale()
	p.rotuloHoy.Text = strings.ToUpper(p.tx("hoy") + " · " + i18n.FormatWeekdayDayMonth(l, p.Ahora()))
	p.rotuloHoy.Refresh()
	hoy := FilasHoy(l, h)
	filas := make([]fyne.CanvasObject, 0, len(hoy))
	for _, f := range hoy {
		e := f.Elemento
		fila := hub.NuevaFilaReciente(cmpOr(e.Titulo, "—"), f.Resumen, func() { p.AbrirHoy(e) })
		if m, ok := iconos.ProductoDe(e.Producto); ok {
			fila.Marca, fila.Origen, fila.Conocido = m, m.NombreCorto(), true
		}
		fila.Cuando = f.Cuando
		filas = append(filas, fila)
	}
	if len(filas) == 0 {
		v := ui.NuevoVacio(p.tx("hoy.vacio"), p.tx("hoy.vacio.texto"))
		v.Compacto = true
		filas = []fyne.CanvasObject{v}
	}
	p.cajaHoy.Objects = filas
	p.cajaHoy.Refresh()
}

// AbrirHoy abre una fila de «Hoy»: un evento, una nota o una tarea en su app por su
// identificador; el resumen del correo o de las tareas, la portada de su app.
func (p *Portada) AbrirHoy(e indice.Elemento) {
	if e.ID != "" {
		p.Abrir(e)
		return
	}
	if a, ok := p.instalada(e.Producto); ok {
		p.abrirApp(a, "")
		return
	}
	p.Abrir(e)
}

// Hoy es lo último leído de hoy.
func (p *Portada) Hoy() indice.Hoy {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hoy
}
