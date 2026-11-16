// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// maxAlt es `MAX_ALT_CHARS` de slides/textoAlt.ts: el techo del texto alternativo.
const maxAlt = 1000

// AltDe es `altDe` de la web: el texto alternativo de una imagen listo para leer, en una
// sola línea, recortado y con techo; "" si la caja no es una imagen o no lo lleva.
func AltDe(c Caja) string {
	if c.Tipo != "image" {
		return ""
	}
	return limpiarAlt(c.Alt)
}

// limpiarAlt es lo que guarda `textoAlt.ts`: una línea, recortada y con techo.
func limpiarAlt(s string) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) > maxAlt {
		r = r[:maxAlt]
	}
	return strings.TrimSpace(string(r))
}

// PonerAlt es `onAlt` de CampoAlt.tsx al confirmar: el texto alternativo de una imagen,
// limpio, y sin campo si queda vacío (la web no ensucia el JSON con `"alt":""`).
func (m *Modelo) PonerAlt(i int, id, alt string) {
	if c := m.caja(i, id); c != nil && c.Tipo == "image" {
		c.Alt = limpiarAlt(alt)
	}
}

// PedirAlt es el campo «Texto alternativo» de la mini-barra de la web con la imagen
// seleccionada: aquí un diálogo con el texto de ahora, porque la mini-barra se rehace con
// cada cambio y un campo dentro de ella perdería el foco al teclear. Vacío también vale:
// es quitarlo.
func (e *Editor) PedirAlt() {
	p := e.Primaria()
	if p == nil || p.Tipo != "image" || e.ctx.SoloLectura {
		return
	}
	c := fyne.CurrentApp().Driver().CanvasForObject(e.raiz)
	if c == nil {
		return
	}
	id, objetivo := p.ID, e.objetivo()
	ui.PreguntaOpcional(e.t("image.alt.label"), AltDe(*p), e.ctx.Traducir("ui.cancel"), e.ctx.Traducir("ui.save"),
		func(alt string) { e.cambiar(func() { e.m.PonerAlt(objetivo, id, alt) }, true) }).Mostrar(c)
}

// rotuloAlt es la rama de imagen de `boxLabel`: una imagen descrita se nombra por lo que
// muestra, recortada a 30 caracteres como el texto de una caja; "" si no lo lleva.
func rotuloAlt(c Caja) string {
	r := []rune(AltDe(c))
	if len(r) > 30 {
		return string(r[:29]) + "…"
	}
	return string(r)
}
