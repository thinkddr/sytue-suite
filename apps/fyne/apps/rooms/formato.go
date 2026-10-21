// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// En la web: apps/frontend/apps/rooms/src/lib/formato.ts y la barra «Aa» de
// Composer.tsx.
//
// La barra de formato sale al pulsar «Aa», también en móvil, y los atajos valen
// siempre: ⌘/Ctrl+B, +I, +K y ⌘/Ctrl+Mayús+X.

// MarcaFormato es `MarcaFormato` de la web.
type MarcaFormato uint8

// Las siete, en el orden de la barra.
const (
	MarcaNegrita MarcaFormato = iota
	MarcaCursiva
	MarcaTachado
	MarcaCodigo
	MarcaEnlace
	MarcaCita
	MarcaLista
)

// marcasDeBarra es `MARCAS_DE_BARRA`: marca, icono y rótulo.
var marcasDeBarra = []struct {
	marca MarcaFormato
	icono iconos.Icono
	clave string
}{
	{MarcaNegrita, iconos.Negrita, "composer.formatBold"},
	{MarcaCursiva, iconos.Cursiva, "composer.formatItalic"},
	{MarcaTachado, iconos.Tachado, "composer.formatStrike"},
	{MarcaCodigo, iconos.Codigo, "composer.formatCode"},
	{MarcaEnlace, iconos.Enlace, "composer.formatLink"},
	{MarcaCita, iconos.Cita, "composer.formatQuote"},
	{MarcaLista, iconos.Lista, "composer.formatList"},
}

// GlifoFormato es el `size={16}` de los iconos de la barra de formato y HuecoFormato
// el `gap-0.5` de su grupo.
var (
	GlifoFormato = iconos.TallaSM
	HuecoFormato = tema.Espacio(0.5)
)

var envoltorios = [...]string{MarcaNegrita: "**", MarcaCursiva: "_", MarcaTachado: "~~", MarcaCodigo: "`"}

// Envolver es `aplicarFormato` sobre la SELECCIÓN: lo que hay que teclear en su lugar
// —teclear sustituye la selección, como en cualquier editor— y cuántas posiciones
// hay que volver atrás el cursor después. Sin selección, las marcas quedan con el
// cursor EN MEDIO; una selección ya envuelta con esa marca se desenvuelve.
//
// `linea` es que la marca va al principio de la línea («> », «- »): sin selección,
// quien llama lleva antes el cursor al principio del renglón.
func Envolver(sel string, m MarcaFormato) (teclear string, atras int, linea bool) {
	switch m {
	case MarcaEnlace:
		if sel == "" {
			return "[]()", 3, false
		}
		return "[" + sel + "]()", 1, false
	case MarcaCita, MarcaLista:
		prefijo := map[MarcaFormato]string{MarcaCita: "> ", MarcaLista: "- "}[m]
		if sel == "" {
			return prefijo, 0, true
		}
		lineas := strings.Split(sel, "\n")
		todas := true
		for _, l := range lineas {
			todas = todas && strings.HasPrefix(l, prefijo)
		}
		for i, l := range lineas {
			switch {
			case todas:
				lineas[i] = strings.TrimPrefix(l, prefijo)
			case !strings.HasPrefix(l, prefijo):
				lineas[i] = prefijo + l
			}
		}
		return strings.Join(lineas, "\n"), 0, false
	case MarcaNegrita, MarcaCursiva, MarcaTachado, MarcaCodigo:
	}
	e := envoltorios[m]
	if len(sel) >= 2*len(e) && strings.HasPrefix(sel, e) && strings.HasSuffix(sel, e) {
		return sel[len(e) : len(sel)-len(e)], 0, false
	}
	if sel == "" {
		return e + e, len([]rune(e)), false
	}
	return e + sel + e, 0, false
}

// AplicarMarca pone la marca sobre la selección de la caja, o en el cursor.
//
// ponytail: la web deja SELECCIONADO el texto recién envuelto; aquí el cursor queda
// detrás, porque `widget.Entry` no deja fijar una selección desde fuera. Y sin
// selección la cita y la lista van al principio del renglón VISUAL: en un párrafo
// largo que parte línea, es el del cursor y no el del párrafo.
func (c *Compositor) AplicarMarca(m MarcaFormato) {
	if c.area == nil || c.Inerte {
		return
	}
	teclear, atras, linea := Envolver(c.area.SelectedText(), m)
	c.insertando = true
	if linea {
		c.area.TypedKey(&fyne.KeyEvent{Name: fyne.KeyHome})
	}
	for _, r := range teclear {
		c.area.TypedRune(r)
	}
	for range atras {
		c.area.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	}
	c.insertando = false
	c.enfocar()
	if c.AlCambiarTexto != nil {
		c.AlCambiarTexto(c.area.Texto())
	}
}

// MarcaDeAtajo es `marcaDeAtajo`: ⌘/Ctrl+B negrita, +I cursiva, +K enlace y
// ⌘/Ctrl+Mayús+X tachado. Con Alt no es el atajo: en algunos teclados escribe letras.
func MarcaDeAtajo(s fyne.Shortcut) (MarcaFormato, bool) {
	a, ok := s.(*desktop.CustomShortcut)
	if !ok || a.Modifier&(fyne.KeyModifierControl|fyne.KeyModifierSuper) == 0 || a.Modifier&fyne.KeyModifierAlt != 0 {
		return 0, false
	}
	if a.Modifier&fyne.KeyModifierShift != 0 {
		return MarcaTachado, a.KeyName == fyne.KeyX
	}
	switch a.KeyName {
	case fyne.KeyB:
		return MarcaNegrita, true
	case fyne.KeyI:
		return MarcaCursiva, true
	case fyne.KeyK:
		return MarcaEnlace, true
	default:
		return 0, false
	}
}

// atajo es el gancho de combinaciones del editor: el formato se lo queda.
func (c *Compositor) atajo(s fyne.Shortcut) bool {
	m, ok := MarcaDeAtajo(s)
	if ok {
		c.AplicarMarca(m)
	}
	return ok
}

// AlternarFormato enseña o esconde la barra de formato («Aa», `aria-pressed`).
func (c *Compositor) AlternarFormato() {
	c.FormatoVisible = !c.FormatoVisible
	if c.caja != nil {
		c.montarBarra()
		c.Refresh()
	}
	if c.AlFormato != nil {
		c.AlFormato(c.FormatoVisible)
	}
}
