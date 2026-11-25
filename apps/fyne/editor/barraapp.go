// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # La barra de la app
//
// En el teléfono el editor enseñaba la barra de la suite y, debajo, su cabecera (volver,
// título, «Guardado», «Compartir»), la cinta y la tira de pestañas: ~200 px de cromo
// antes de la primera línea del documento (auditoría del 2026-11-24). Como Mail y
// Calendar, el marco va dentro de un `ui.Armazon` con su barra móvil, que en el teléfono
// SUBE a la barra de la suite (la barra única): por debajo de `md` la cabecera no se
// pinta y su salida, el título y «Compartir» van en ella. Lo demás de la cabecera tiene
// su sitio en «Archivo»: el estado del guardado (el primer renglón), renombrar y
// descargar; y si falla el guardado, la barra lleva además el aviso que reintenta.

// Montado es lo que la app pone en su ventana: el marco dentro del armazón con su barra.
// Siempre el mismo objeto, para que la app lo pueda comparar con lo que enseña.
func (m *Marco) Montado() fyne.CanvasObject {
	if m.arm == nil {
		m.movil = ui.NuevaBarraMovil()
		m.arm = ui.NuevoArmazon(m)
		m.arm.BarraMovil = m.movil
		m.ponerBarraApp()
	}
	return m.arm
}

// enBarraApp es si la cabecera va en la barra de la app: montado en el armazón y por
// debajo de `md`, que es cuando el armazón pinta su barra.
func (m *Marco) enBarraApp(ancho float32) bool {
	return m.arm != nil && ancho > 0 && ancho < ui.Estrecho
}

// ponerBarraApp lleva a la barra de la app lo de la cabecera: la salida, el título (o
// su marcador, como el campo) y «Compartir», con el aviso de «no se ha podido guardar»
// delante si lo hay.
func (m *Marco) ponerBarraApp() {
	b := m.movil
	if b == nil {
		return
	}
	c := m.ctx
	b.EnDetalle, b.AlVolver, b.RotuloVolver = c.Volver != nil, c.Volver, c.Traducir("ui.backToList")
	b.Titulo = c.Titulo
	if b.Titulo == "" {
		b.Titulo = c.Traducir("header.title.label")
	}
	b.Acciones = nil
	if c.Guardado == FalloAlGuardar {
		reintentar := ui.NuevoBotonBarraMovil(iconos.Aviso, c.Traducir("header.save.retry"), func() {
			if c.GuardarYa != nil {
				c.GuardarYa()
			}
		})
		reintentar.Inerte = c.SinRed || c.GuardarYa == nil
		b.Acciones = append(b.Acciones, reintentar)
	}
	var compartir *ui.BotonIcono
	compartir = ui.NuevoBotonBarraMovil(iconos.Ninguno, c.Traducir("header.share"), func() {
		if c.Compartir != nil {
			c.Compartir(compartir)
		}
	})
	compartir.Glifo = iconos.TrazoEd(iconos.EdCompartir, iconos.TallaMD, "text",
		tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	compartir.Inerte = c.SinRed || c.Compartir == nil
	b.Acciones = append(b.Acciones, compartir)
	b.Refresh()
}

// opcionesBarraApp es lo que «Archivo» lleva delante con la cabecera en la barra de la
// app: el título con el estado del guardado, que es lo que decía el chip.
func (m *Marco) opcionesBarraApp() []fyne.CanvasObject {
	c := m.ctx
	guardado, _ := m.cabecera.textoChip(time.Now())
	titulo := c.Titulo
	if titulo == "" {
		titulo = c.Traducir("header.title.label")
	}
	rotulo := ui.NuevoElementoMenu(titulo+" · "+guardado, iconos.Ninguno, nil)
	rotulo.Inerte = true
	return []fyne.CanvasObject{rotulo, ui.NuevoSeparadorMenu()}
}
