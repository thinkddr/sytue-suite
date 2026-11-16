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

// TestElTextoAlternativoSeEditaDesdeLaMiniBarra: con una imagen seleccionada, la mini-barra
// lleva «Texto alternativo de la imagen» (encendido si ya lo tiene), que lo pregunta con el
// texto de ahora; lo aceptado se guarda limpio y vacío lo quita, como CampoAlt de la web.
func TestElTextoAlternativoSeEditaDesdeLaMiniBarra(t *testing.T) {
	e, ctx, w := montar(t, conImagenDescrita)
	botonAlt := func() *editor.BotonCinta {
		for _, o := range e.miniBarra.Objects {
			if b, ok := o.(*editor.BotonCinta); ok && b.Rotulo == e.t("image.alt.label") {
				return b
			}
		}
		return nil
	}
	if botonAlt() != nil {
		t.Error("sin imagen seleccionada la mini-barra lleva el texto alternativo")
	}
	e.SeleccionarSolo("i")
	b := botonAlt()
	if b == nil || !b.Activo {
		t.Fatalf("con la imagen descrita seleccionada, el botón: %+v", b)
	}
	b.Tapped(&fyne.PointEvent{})
	campo, ok := w.Canvas().Focused().(*ui.Entrada)
	if !ok || campo.Texto() != "Mapa de las oficinas de Madrid y Lisboa" {
		t.Fatalf("el diálogo no pregunta con el texto de ahora: %T", w.Canvas().Focused())
	}
	campo.PonerTexto("  Oficinas\nde Madrid ")
	campo.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if got := e.m.caja(0, "i").Alt; got != "Oficinas de Madrid" || ctx.Guardado != editor.Sucio {
		t.Errorf("se guarda %q (chip %v)", got, ctx.Guardado)
	}
	e.m.PonerAlt(0, "i", "   ")
	if strings.Contains(e.Serializar(), `"alt"`) {
		t.Error("vacío tiene que quitar el campo, no dejar \"alt\":\"\"")
	}
	e.SeleccionarSolo("j")
	if b := botonAlt(); b == nil || b.Activo {
		t.Errorf("la imagen sin alt lleva el botón apagado: %+v", b)
	}
}
