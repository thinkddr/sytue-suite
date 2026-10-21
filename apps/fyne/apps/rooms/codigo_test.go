// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
)

// Un bloque ``` lleva su cabecera con el lenguaje y «Copiar», el código en la
// monoespaciada y un desplazador horizontal; «Copiar» entrega el texto entero.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con el bloque de antes —piezas sueltas en
// Figtree, sin cabecera— no hay botón ni desplazador y el alto es 22 menos.
func TestElBloqueDeCodigoLlevaCabeceraYCopiar(t *testing.T) {
	codigo := "fmt.Println(\"una línea bastante larga que no cabe en el ancho del mensaje\")\nreturn"
	m := maquetadorDe("antes\n```go\n"+codigo+"\n```", "", nil, 300)
	if len(m.codigos) != 1 || m.codigos[0].lenguaje != "go" || m.codigos[0].alto != AltoBloqueCodigo(codigo) {
		t.Fatalf("el bloque se maqueta %+v", m.codigos)
	}
	quiero := 3 + 24 + 12 + 2*18
	if d := AltoBloqueCodigo(codigo) - float32(quiero); d > 0.01 || d < -0.01 {
		t.Errorf("el bloque mide %v y la hoja da %v", AltoBloqueCodigo(codigo), quiero)
	}
	c := NuevoCuerpoRico("```go\n"+codigo+"\n```", nil)
	c.T = traductor(t)
	var copiado string
	c.AlCopiar = func(s string) { copiado = s }
	test.WidgetRenderer(c).Layout(fyne.NewSize(300, 200))
	var boton *BotonCopiar
	var scroll *container.Scroll
	var rotulo string
	for _, o := range c.objetos {
		switch x := o.(type) {
		case *BotonCopiar:
			boton = x
		case *container.Scroll:
			scroll = x
		case *canvas.Text:
			if rotulo == "" {
				rotulo = x.Text
			}
		}
	}
	if boton == nil || scroll == nil || rotulo != "go" {
		t.Fatalf("faltan piezas: botón %v, desplazador %v, rótulo %q", boton != nil, scroll != nil, rotulo)
	}
	if scroll.Direction != container.ScrollHorizontalOnly || scroll.Content.MinSize().Width <= 300 {
		t.Errorf("el código no se desplaza en horizontal: %v, %v", scroll.Direction, scroll.Content.MinSize())
	}
	test.Tap(boton)
	if copiado != codigo {
		t.Errorf("«Copiar» entrega %q", copiado)
	}
	if _, est := caraMono(); est.Monospace == false && est.Bold {
		t.Error("la cara del código no es monoespaciada")
	}
}

// La barra de un mensaje es UNA parada del tabulador aunque esté apagada: con el foco
// se enciende, las flechas mueven el botón activo e Intro lo pulsa.
//
// VISTO EN ROJO (2026-09-23, en el frontal): escondiéndola con Hide() como antes, el
// recorrido del tabulador no la encuentra.
func TestLaBarraDeUnMensajeEntraEnElTabulador(t *testing.T) {
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	f := filaDe(t, sa.P.Canal.Historial, "m1")
	b := f.barra
	if b == nil || !b.Visible() || !b.Apagada {
		t.Fatal("la barra no está en el árbol, o no nace apagada")
	}
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	lienzo.Focus(b)
	if b.Apagada || !b.Enfocada() || !f.focoBarra {
		t.Fatal("con el foco la barra sigue apagada")
	}
	b.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if b.AccessibilityLabel() != sa.P.T.T("reaction.with", map[string]any{"emoji": "❤️"}) {
		t.Errorf("tras la flecha el activo es %q", b.AccessibilityLabel())
	}
	b.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if !s.pidio("React(m1,❤️,poner)") {
		t.Errorf("Intro no reacciona: %v", s.llamadas)
	}
	lienzo.Unfocus()
	if !b.Apagada {
		t.Error("sin el foco la barra sigue encendida")
	}
}
