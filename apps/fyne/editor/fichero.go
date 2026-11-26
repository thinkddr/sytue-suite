// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// Guardar en un FICHERO DEL DISCO, solo en la edición local (PLAN-EDICION-LOCAL O-04 y
// O-07). En la conectada el documento vive en el servicio y se guarda solo; en la local
// también se guarda solo en la biblioteca, pero la persona puede querer, como en cualquier
// procesador de textos, que esté además en un fichero suyo: «Guardar» (Ctrl/Cmd+S) lo
// escribe en el fichero del que salió y «Guardar como…» elige otro.
//
// Todo es ADITIVO: sin [Fichero] (la edición conectada) el contexto no lleva los
// ganchos, el menú Archivo no enseña las entradas y Ctrl/Cmd+S hace lo de siempre.

// Fichero es lo que el binario de la edición local engancha en cada documento abierto
// (`docus.App.Fichero`, `calcus.App.Fichero`, `diapos.App.Fichero`). Reciben el contexto
// del documento (su título, `Notificar`) y `antes`, que vuelca lo pendiente al almacén
// (puede ser nil: no había nada); quien recibe la llamada lo corre fuera del hilo de la
// interfaz antes de leer el documento.
type Fichero struct {
	Guardar     func(ctx *Contexto, id string, antes func() error)
	GuardarComo func(ctx *Contexto, id string, antes func() error)
	// Rótulos de las dos entradas del menú Archivo, ya traducidos.
	RotuloGuardar, RotuloGuardarComo string
}

// Enganchar pone en ctx «Guardar» y «Guardar como…» del documento id. `antes` se pide
// en cada pulsación: es lo pendiente EN ESE MOMENTO. Con f nil no hace nada.
func (f *Fichero) Enganchar(ctx *Contexto, id string, antes func() func() error) {
	if f == nil || ctx == nil {
		return
	}
	pendiente := func() func() error {
		if antes == nil {
			return nil
		}
		return antes()
	}
	ctx.RotuloGuardarFichero, ctx.RotuloGuardarComo = f.RotuloGuardar, f.RotuloGuardarComo
	if f.Guardar != nil {
		ctx.GuardarFichero = func() { f.Guardar(ctx, id, pendiente()) }
	}
	if f.GuardarComo != nil {
		ctx.GuardarComo = func() { f.GuardarComo(ctx, id, pendiente()) }
	}
}

// OcultarCompartirSinGancho quita «Compartir» de la cabecera, de la cinta y del menú
// compacto cuando el documento no tiene a quién compartirse ([Contexto.Compartir] nil),
// en vez de dejarlo inerte (PLAN-EDICION-LOCAL O-02). Lo pone la edición local, donde
// nunca lo hay; en la conectada queda en false y todo sale como siempre (sin red, o sin
// permiso, el botón inerte sigue diciendo que existe).
var OcultarCompartirSinGancho bool

// sinCompartir dice si «Compartir» no se enseña en este documento.
func (c *Contexto) sinCompartir() bool {
	return OcultarCompartirSinGancho && (c == nil || c.Compartir == nil)
}

// esGuardar es Ctrl/Cmd+S, sin Mayúsculas ni Alt.
func esGuardar(s *desktop.CustomShortcut) bool {
	return s != nil && s.KeyName == fyne.KeyS && s.Modifier == fyne.KeyModifierShortcutDefault
}

// AtajoGuardarFichero es Ctrl/Cmd+S cuando el documento se guarda en un fichero: dice si
// lo ha atendido. Sin [Contexto.GuardarFichero] no lo atiende, y el lienzo hace lo suyo
// (en Docus, guardar ya en el almacén).
func (c *Contexto) AtajoGuardarFichero(s *desktop.CustomShortcut) bool {
	if !esGuardar(s) || c == nil || c.GuardarFichero == nil {
		return false
	}
	c.GuardarFichero()
	return true
}

// atajoGuardar es Ctrl/Cmd+S en la ventana del marco, para cuando el foco no lo tiene el
// lienzo (la cinta, un panel). Sin gancho no hace nada, que es lo de antes: la ventana no
// tenía Ctrl/Cmd+S.
var atajoGuardar = &desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierShortcutDefault}
