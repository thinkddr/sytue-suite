// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tokens

import (
	"image/color"
	"testing"
)

// TestLasMedidasDeFormaSonLasDeLaTabla: las medidas de Marco y de Carril salen de
// theme.css (y, los títulos de Marco, de la regla de la hoja en appearance.css). Los
// esperados son la tabla del comentario «Tokens de forma» de theme.css, copiada a mano:
// si salieran del lector, el test se comprobaría a sí mismo.
//
// VISTO EN ROJO sin leer la regla `.sytue-sheet` de appearance.css: en Marco los títulos
// se quedan en 700/600 y escala 1, que son los de fuera de la hoja.
func TestLasMedidasDeFormaSonLasDeLaTabla(t *testing.T) {
	px := func(n float64) Medida { return Medida{Valor: n, Unidad: "px"} }
	rem := func(n float64) Medida { return Medida{Valor: n, Unidad: "rem"} }
	sin := func(n float64) Medida { return Medida{Valor: n} }
	for _, c := range []struct {
		nombre        string
		marco, carril Medida
	}{
		{"radius-control", px(4), px(4)},
		{"radius-nav", px(999), px(9)},
		{"radius-card", px(16), px(14)},
		{"radius-row", px(10), px(0)},
		{"radius-sheet", px(16), px(0)},
		{"sheet-gap", px(10), px(0)},
		{"nav-pad", rem(0.875), rem(0.625)},
		{"nav-indicator", px(0), px(3)},
		{"nav-active-weight", sin(650), sin(500)},
		{"rail-indicator", px(3), px(0)},
		{"row-grow", px(1), px(0)},
		{"row-inset", px(6), px(0)},
		{"row-bar", px(0), px(2)},
		{"title-weight", sin(800), sin(700)},
		{"subtitle-weight", sin(700), sin(600)},
		{"title-scale", sin(1.1), sin(1)},
	} {
		if got := MedidaEnForma(c.nombre, false); got != c.marco {
			t.Errorf("Marco: %s = %+v, quiero %+v", c.nombre, got, c.marco)
		}
		if got := MedidaEnForma(c.nombre, true); got != c.carril {
			t.Errorf("Carril: %s = %+v, quiero %+v", c.nombre, got, c.carril)
		}
	}
}

// TestLaFormaEsUnEjeAparte: Marco es la hoja sin atributo y la forma por defecto; Carril
// se elige. La forma trae sus propios tokens de color —el ítem activo del menú, y en
// Marco el marco y la hoja— de los bloques finales de appearance.css, y no toca el
// color de la suite ni el tema. Los esperados, a mano de esos bloques.
//
// VISTO EN ROJO sin apilar los bloques de la forma: `frame-from` no existe y entra en
// pánico.
func TestLaFormaEsUnEjeAparte(t *testing.T) {
	t.Cleanup(func() { ElegirForma(""); ElegirColor("") })
	if EsCarril() {
		t.Fatal("la forma por defecto es Marco y sale Carril")
	}
	// Marco: el marco es el color del menú; en traslúcido, un degradado del primario
	// (900 → 700), y la hoja, `surface-raised` al 82 %.
	for _, c := range []struct {
		nombre                    string
		claro, oscuro, traslucido color.NRGBA
	}{
		{"frame-from", rgb(0x0c5f68), rgb(0x0d2a2e), rgb(0x083b40)},
		{"frame-to", rgb(0x0c5f68), rgb(0x0d2a2e), rgb(0x0c636b)},
		{"sheet-bg", Color("bg", Light), Color("bg", Dark), color.NRGBA{247, 251, 251, 209}},
		// El ítem activo es el velo del menú, con su texto: los mismos en las dos formas.
		{"nav-active-bg", color.NRGBA{255, 255, 255, 36}, color.NRGBA{127, 211, 216, 41}, color.NRGBA{255, 255, 255, 36}},
		{"nav-active-fg", rgb(0xffffff), rgb(0xffffff), rgb(0xffffff)},
	} {
		for v, quiero := range []color.NRGBA{c.claro, c.oscuro, c.traslucido} {
			if got := Color(c.nombre, Variant(v)); got != quiero {
				t.Errorf("Marco, apariencia %d: %s = %v, quiero %v", v, c.nombre, got, quiero)
			}
		}
	}
	primario := Color("primary", Light)
	if !ElegirForma("carril") || !EsCarril() {
		t.Fatal("elegir Carril no ha cambiado nada")
	}
	if ElegirForma("carril") {
		t.Error("elegir la forma que ya está dice que ha cambiado: se repintaría sin motivo")
	}
	if Color("primary", Light) != primario || Color("nav-active-bg", Light) != (color.NRGBA{255, 255, 255, 36}) {
		t.Error("cambiar de forma ha tocado el color, o Carril ha perdido su ítem activo")
	}
	// Con otro color de la suite, la forma sigue siendo la elegida, y al revés.
	ElegirColor("cobalto")
	if !EsCarril() || Color("sidebar-bg", Light) != rgb(0x3355d6) {
		t.Error("cambiar de color ha cambiado la forma, o no ha cambiado el color")
	}
	if !ElegirForma("marco") || EsCarril() || Color("frame-from", Light) != rgb(0x3355d6) {
		t.Errorf("Marco en cobalto: el marco pinta %v y es el carril de cobalto", Color("frame-from", Light))
	}
	if ElegirForma("") || ElegirForma("hexagonal") {
		t.Error("el vacío, o una forma que no existe, tienen que dejar Marco sin cambiar nada")
	}
}

// TestTodosLosJuegosDeLaHojaSeLeen: al arrancar solo se lee lo de la casa, así que una
// hoja con un bloque ilegible en otro color o en la otra forma ya no impide arrancar: lo
// dice este test. Lee cada color que la hoja declara, en las dos formas, y el de un
// producto.
func TestTodosLosJuegosDeLaHojaSeLeen(t *testing.T) {
	colores := append([]string{""}, Colores()...)
	if len(colores) < 3 {
		t.Fatalf("la hoja declara %v: faltan el cobalto o el de cada app", Colores())
	}
	for _, carril := range []bool{false, true} {
		for _, c := range colores {
			p := pedido{color: c, carril: carril}
			if c == ColorDeCadaApp {
				p.app = "mail"
			}
			crudas, err := parseCrudasDe(oscuroElegido, p, coloresCSS, aparienciaCSS)
			if err != nil {
				t.Errorf("%+v: %v", p, err)
				continue
			}
			for v := range crudas {
				if len(crudas[v]) != len(paletas[Light]) && !carril {
					t.Errorf("%+v, apariencia %d: %d tokens y la casa tiene %d", p, v, len(crudas[v]), len(paletas[Light]))
				}
			}
		}
	}
}
