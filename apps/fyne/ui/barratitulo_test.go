// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fmt"
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	temafyne "fyne.io/fyne/v2/theme"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLaBarraDeTituloLlevaElColorDeLaBarra es el pedido del dueño (2026-09-25): la
// barra de título del sistema lleva el color que PINTA la barra de la suite, en claro
// y en oscuro, y lo cambia en caliente cuando cambia el tema.
//
// Compara con el píxel pintado y no con el nombre del token. VISTO EN ROJO pasando
// `tema.Color("topbar-bg", v)` en `pintarTitulo`, y quitando la llamada de `Layout`.
func TestLaBarraDeTituloLlevaElColorDeLaBarra(t *testing.T) {
	type puesto struct {
		c      color.NRGBA
		oscuro bool
	}
	var puestos []puesto
	ventana, colorear := ventanaNativaDe, colorearTitulo
	ventanaNativaDe = func(fyne.CanvasObject) uintptr { return 1 }
	colorearTitulo = func(_ uintptr, c color.NRGBA, oscuro bool) { puestos = append(puestos, puesto{c, oscuro}) }
	t.Cleanup(func() { ventanaNativaDe, colorearTitulo = ventana, colorear; tema.Elegir("") })

	tema.Elegir("claro")
	m, b := cromoPrueba(t, anchoPrueba, 200)
	for _, variante := range []string{"claro", "oscuro"} {
		tema.Elegir(variante)
		b.Refresh() // lo que hace Fyne con cada widget al cambiar el tema
		img := pinta(t, anchoPrueba, 200, m)
		// Fuera de la esquina de marca: la barra de título sigue a la barra, no al carril.
		barra := px(img, int((tema.AnchoLateral+4)*escala), int(AltoBarraSuite()/2*escala))
		if len(puestos) == 0 {
			t.Fatalf("en %s no se le ha pasado ningún color al sistema", variante)
		}
		p := puestos[len(puestos)-1]
		if p.c != barra || p.oscuro != (variante == "oscuro") {
			t.Errorf("en %s la barra de título lleva %v (oscuro=%v) y la barra pinta %v", variante, p.c, p.oscuro, barra)
		}
	}
	// Sin repetirse: una llamada al sistema por color, no una por Layout.
	if len(puestos) != 2 || puestos[0].c == puestos[1].c {
		t.Errorf("se esperaban dos colores distintos, uno por tema, y se pusieron %v", puestos)
	}
}

// TestLaVentanaDiceDeQueAppEs: en X11 GLFW pone como clase de la ventana su TÍTULO, y
// Plasma no la unía a su .desktop (visto en un Plasma 6.7 real: el número de avisos no
// salía en el lanzador hasta ponerle la clase a mano). La barra le dice al sistema el
// identificador de la app una vez por ventana nativa, no en cada Layout. VISTO EN ROJO
// sin la llamada y llamándola en cada pintado.
func TestLaVentanaDiceDeQueAppEs(t *testing.T) {
	var ids []string
	ventana, colorear, ident := ventanaNativaDe, colorearTitulo, identificar
	ventanaNativaDe = func(fyne.CanvasObject) uintptr { return 7 }
	colorearTitulo = func(uintptr, color.NRGBA, bool) {}
	identificar = func(h uintptr, id string) { ids = append(ids, fmt.Sprint(h, " ", id)) }
	t.Cleanup(func() { ventanaNativaDe, colorearTitulo, identificar = ventana, colorear, ident; tema.Elegir("") })

	m, b := cromoPrueba(t, anchoPrueba, 200)
	for _, variante := range []string{"claro", "oscuro"} {
		tema.Elegir(variante)
		b.Refresh()
		pinta(t, anchoPrueba, 200, m)
	}
	quiero := fmt.Sprint(7, " ", fyne.CurrentApp().Metadata().ID)
	if len(ids) != 1 || ids[0] != quiero {
		t.Fatalf("se ha identificado la ventana %q; se esperaba una vez %q", ids, quiero)
	}
}

// TestLaBarraDeTituloNoSigueAlSistemaSiLaAppEligioOtraApariencia: en Windows, al cambiar
// el tema del sistema, Fyne vuelve a poner en la ventana el modo oscuro del SISTEMA
// después de repintar los widgets. Con la app en claro y el sistema pasando a oscuro,
// lo último que se le dice al sistema tiene que seguir siendo claro, y dicho DESPUÉS de
// ese repintado.
//
// VISTO EN ROJO con el `tituloPuesto` de antes (ventana y color): el color no cambia,
// no se vuelve a decir nada y la barra se queda con el oscuro que puso Fyne.
func TestLaBarraDeTituloNoSigueAlSistemaSiLaAppEligioOtraApariencia(t *testing.T) {
	var oscuros []bool
	ventana, colorear, sistema := ventanaNativaDe, colorearTitulo, varianteSistema
	ventanaNativaDe = func(fyne.CanvasObject) uintptr { return 1 }
	colorearTitulo = func(_ uintptr, _ color.NRGBA, oscuro bool) { oscuros = append(oscuros, oscuro) }
	variante := temafyne.VariantLight
	varianteSistema = func() fyne.ThemeVariant { return variante }
	t.Cleanup(func() { ventanaNativaDe, colorearTitulo, varianteSistema = ventana, colorear, sistema; tema.Elegir("") })

	tema.Elegir("claro")
	_, b := cromoPrueba(t, anchoPrueba, 200)
	b.Refresh()
	antes := len(oscuros)
	variante = temafyne.VariantDark // el sistema pasa a oscuro; la app sigue en claro
	b.Refresh()
	fyne.DoAndWait(func() {})
	if len(oscuros) < antes+2 {
		t.Fatalf("tras cambiar el sistema se le dijo %d veces el color (quiero la de ahora y otra después de Fyne)", len(oscuros)-antes)
	}
	if oscuros[len(oscuros)-1] {
		t.Error("la barra de título se queda en oscuro con la app en claro")
	}
}
