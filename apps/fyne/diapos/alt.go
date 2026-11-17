// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
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

// campoAlt es CampoAlt.tsx: el campo «Texto alternativo» de la imagen elegida, en la
// mini-barra. Se escribe sin tocar el modelo y se guarda al confirmar —salir del campo,
// Intro, elegir otra caja o quitar la selección—, como el arrastre de una caja: un
// autoguardado por tecla llenaría el historial de pasos de una sola letra.
type campoAlt struct {
	entrada *ui.Entrada
	// caja es la imagen que enseña el campo, y objetivo dónde está (la diapositiva o
	// el patrón).
	caja     string
	objetivo int
	// editando es la caja con pulsaciones sin guardar ("" = nada pendiente): solo
	// entonces se guarda, y en ESA caja. Al pinchar otra imagen la selección cambia
	// antes de que el campo pierda el foco, y guardar en la elegida copiaría el texto de
	// la anterior en la nueva.
	editando string
}

// campoAltPara es el campo de la mini-barra para la imagen `p`: el MISMO objeto en cada
// recomposición, con el texto de esa imagen. Lo que llega de fuera (deshacer, otra
// persona) se refleja, pero no mientras se escribe: el modelo guarda el texto ya
// limpio y pisar el campo con él se comería el espacio que se acaba de teclear.
func (e *Editor) campoAltPara(p Caja) fyne.CanvasObject {
	a := &e.alt
	if a.entrada == nil {
		a.entrada = ui.NuevaEntrada(e.t("image.alt.placeholder")).Con(ui.CampoSM)
		a.entrada.Rotulo = e.t("image.alt.label")
		a.entrada.Ancho = tema.Espacio(48) // `w-48`
		a.entrada.AlCambiar(func(string) {
			if a.editando == "" {
				a.editando = a.caja
			}
		})
		a.entrada.AlEnfocar = func(enfocado bool) {
			if !enfocado {
				e.confirmarAlt()
			}
		}
		a.entrada.AlTeclear = func(k *fyne.KeyEvent) bool {
			if k.Name != fyne.KeyReturn && k.Name != fyne.KeyEnter {
				return false
			}
			// Intro confirma: soltar el foco es lo que guarda, como el `blur()` de la web.
			if c := fyne.CurrentApp().Driver().CanvasForObject(a.entrada); c != nil && c.Focused() == fyne.Focusable(a.entrada) {
				c.Unfocus()
			} else {
				e.confirmarAlt()
			}
			return true
		}
	}
	cambia := a.caja != p.ID
	a.caja, a.objetivo = p.ID, e.objetivo()
	if cambia || (a.editando == "" && !a.entrada.Enfocado()) {
		if a.entrada.Texto() != AltDe(p) {
			a.entrada.PonerTexto(AltDe(p))
		}
	}
	a.entrada.SoloLectura(e.ctx.SoloLectura)
	return a.entrada
}

// confirmarAlt guarda lo pendiente en la caja en la que se escribió.
func (e *Editor) confirmarAlt() {
	a := &e.alt
	id := a.editando
	if id == "" {
		return
	}
	a.editando = ""
	texto, objetivo := a.entrada.Texto(), a.objetivo
	if c := e.m.caja(objetivo, id); c == nil || c.Alt == limpiarAlt(texto) {
		e.recomponer() // nada que guardar, pero el campo enseña lo de verdad
		return
	}
	e.cambiar(func() { e.m.PonerAlt(objetivo, id, texto) }, true)
}

// confirmarAltAjeno guarda lo pendiente si la imagen en la que se escribía ya no es la
// elegida (se pinchó otra caja o se quitó la selección): es el desmontar de CampoAlt,
// porque aquí el campo perdería el foco DESPUÉS de cambiar la selección.
func (e *Editor) confirmarAltAjeno() {
	a := &e.alt
	if a.editando == "" {
		return
	}
	if p := e.Primaria(); p != nil && p.Tipo == "image" && p.ID == a.editando && e.objetivo() == a.objetivo {
		return
	}
	id, texto, objetivo := a.editando, a.entrada.Texto(), a.objetivo
	a.editando = ""
	if c := e.m.caja(objetivo, id); c != nil && c.Alt != limpiarAlt(texto) {
		e.cambiar(func() { e.m.PonerAlt(objetivo, id, texto) }, false)
	}
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
