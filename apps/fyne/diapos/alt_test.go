// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Una imagen con texto alternativo (`alt`, DIAP-06 de la web) tiene que abrir editable: sin
// el campo en `Caja`, releer y volver a escribir lo perdía y `Fiel` dejaba la presentación
// de solo lectura. Y su marco se anuncia por lo que muestra, como el `aria-label` de la web.
const conImagenDescrita = `{"slides":[{"id":"s","boxes":[` +
	`{"id":"i","x":10,"y":10,"w":40,"h":30,"kind":"image","html":"","src":"data:image/png;base64,AA",` +
	`"alt":"Mapa de las oficinas de Madrid y Lisboa"},` +
	`{"id":"j","x":60,"y":10,"w":30,"h":30,"kind":"image","html":"","src":"data:image/png;base64,AA"}]}]}`

func TestElTextoAlternativoSeConservaYSeAnuncia(t *testing.T) {
	m, err := Leer([]byte(conImagenDescrita))
	if err != nil {
		t.Fatal(err)
	}
	if !m.Fiel([]byte(conImagenDescrita)) {
		t.Fatalf("no es fiel: %v", m.NoFiel([]byte(conImagenDescrita)))
	}

	e, ctx, _ := montar(t, conImagenDescrita)
	if ctx.SoloLectura {
		t.Fatal("una presentación con alt abre de solo lectura")
	}
	if !strings.Contains(e.Serializar(), `"alt":"Mapa de las oficinas de Madrid y Lisboa"`) {
		t.Error("al guardar se pierde el alt")
	}
	if got, quiero := marcoDe(t, e, "i").AccessibilityLabel(), "Mapa de las oficinas de Madri…"; got != quiero {
		t.Errorf("el marco de la imagen descrita se anuncia %q y tocaba %q", got, quiero)
	}
	if got, quiero := marcoDe(t, e, "j").AccessibilityLabel(), e.t("slides.box.image"); got != quiero {
		t.Errorf("sin alt, el marco se anuncia %q y tocaba %q (su tipo)", got, quiero)
	}
}

func TestAltDe(t *testing.T) {
	casos := []struct {
		c    Caja
		want string
	}{
		{Caja{Tipo: "image", Alt: "  Logo\n de  la\tempresa "}, "Logo de la empresa"},
		{Caja{Tipo: "image"}, ""},
		{Caja{Tipo: "shape", Alt: "no es imagen"}, ""},
		{Caja{Tipo: "image", Alt: strings.Repeat("á", maxAlt+5)}, strings.Repeat("á", maxAlt)},
	}
	for _, c := range casos {
		if got := AltDe(c.c); got != c.want {
			t.Errorf("AltDe(%q) = %q, quiero %q", c.c.Alt, got, c.want)
		}
	}
}

// campoAltDe es el campo «Texto alternativo» de la mini-barra, si está.
func campoAltDe(e *Editor) *ui.Entrada {
	for _, o := range e.miniBarra.Objects {
		if c, ok := o.(*ui.Entrada); ok && c.Rotulo == e.t("image.alt.label") {
			return c
		}
	}
	return nil
}

// escribirEn enfoca el campo y escribe en él como el teclado (letra a letra).
func escribirEn(w fyne.Window, c *ui.Entrada, texto string) {
	w.Canvas().Focus(c)
	for _, r := range texto {
		c.TypedRune(r)
	}
}

// TestElTextoAlternativoSeEditaDesdeLaMiniBarra: con una imagen elegida, la mini-barra
// lleva el campo «Texto alternativo» de CampoAlt.tsx con el texto de ahora; se escribe en
// línea, Intro (o salir del campo) lo guarda limpio, y vacío lo quita.
func TestElTextoAlternativoSeEditaDesdeLaMiniBarra(t *testing.T) {
	e, ctx, w := montar(t, conImagenDescrita)
	if campoAltDe(e) != nil {
		t.Error("sin imagen seleccionada la mini-barra lleva el texto alternativo")
	}
	e.SeleccionarSolo("i")
	c := campoAltDe(e)
	if c == nil || c.Texto() != "Mapa de las oficinas de Madrid y Lisboa" {
		t.Fatalf("con la imagen descrita elegida, el campo: %+v", c)
	}
	if w.Canvas().Overlays().Top() != nil {
		t.Error("el texto alternativo no abre ningún diálogo")
	}
	c.PonerTexto("")
	escribirEn(w, c, "  Oficinas de Madrid ")
	if got := e.m.caja(0, "i").Alt; got != "Mapa de las oficinas de Madrid y Lisboa" {
		t.Errorf("mientras se escribe ya se ha guardado %q", got)
	}
	c.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if got := e.m.caja(0, "i").Alt; got != "Oficinas de Madrid" || ctx.Guardado != editor.Sucio {
		t.Errorf("Intro guarda %q (chip %v)", got, ctx.Guardado)
	}
	if w.Canvas().Focused() == fyne.Focusable(c) {
		t.Error("Intro no suelta el campo")
	}
	if c.Texto() != "Oficinas de Madrid" {
		t.Errorf("tras guardar, el campo enseña %q y no lo guardado", c.Texto())
	}
	// Vacío lo quita, al salir del campo.
	w.Canvas().Focus(c)
	c.PonerTexto("   ")
	w.Canvas().Unfocus()
	if strings.Contains(e.Serializar(), `"alt"`) {
		t.Error("vacío tiene que quitar el campo, no dejar \"alt\":\"\"")
	}
	e.SeleccionarSolo("j")
	if c := campoAltDe(e); c == nil || c.Texto() != "" {
		t.Errorf("la imagen sin alt: %+v", c)
	}
}

// TestElCampoAltConservaElFocoAlRehacerseLaMiniBarra es por lo que era un diálogo: la
// mini-barra se rehace con cada cambio, y un campo nuevo en cada recomposición perdía el
// foco y lo escrito. Ahora es el mismo campo: se rehace la barra (un cambio de otro, el
// chip de guardado) y se sigue escribiendo en él. Y si se elige otra imagen con algo a
// medias, se guarda en la suya y no en la nueva. VISTO EN ROJO con un campo nuevo por
// recomposición: el foco se quedaba en un campo que ya no estaba en la barra.
func TestElCampoAltConservaElFocoAlRehacerseLaMiniBarra(t *testing.T) {
	e, _, w := montar(t, conImagenDescrita)
	e.SeleccionarSolo("i")
	c := campoAltDe(e)
	c.PonerTexto("")
	escribirEn(w, c, "Oficinas ")
	e.Refrescar() // la mini-barra entera, otra vez
	e.cambiar(func() { e.m.PonerFondo(0, "#FFFFFF") }, true)
	if campoAltDe(e) != c {
		t.Fatal("la mini-barra se rehace con otro campo")
	}
	if w.Canvas().Focused() != fyne.Focusable(c) {
		t.Fatalf("al rehacerse la mini-barra el foco pasa a %T", w.Canvas().Focused())
	}
	escribirEn(w, c, "de Lisboa")
	if c.Texto() != "Oficinas de Lisboa" {
		t.Errorf("se pierde lo escrito: %q", c.Texto())
	}
	guarda(t, "diapos-alt-en-la-mini-barra.png", w.Canvas().Capture())

	// Elegir la otra imagen con el campo a medias: se guarda en «i», y el campo pasa a «j».
	e.SeleccionarSolo("j")
	if got := e.m.caja(0, "i").Alt; got != "Oficinas de Lisboa" {
		t.Errorf("lo pendiente no se guarda en su imagen: %q", got)
	}
	if got := e.m.caja(0, "j").Alt; got != "" {
		t.Errorf("lo de «i» se copia en «j»: %q", got)
	}
	if c.Texto() != "" {
		t.Errorf("el campo sigue con el texto de la otra imagen: %q", c.Texto())
	}
	w.Canvas().Unfocus()
	if got := e.m.caja(0, "j").Alt; got != "" {
		t.Errorf("salir del campo sin escribir guarda %q en «j»", got)
	}
}
