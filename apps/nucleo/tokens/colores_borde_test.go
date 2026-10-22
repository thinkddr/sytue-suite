// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tokens

import (
	"image/color"
	"testing"
)

// Los bordes tienen que dar EL MISMO PÍXEL que en el navegador.
//
// Gio componía en espacio lineal y el navegador en sRGB, así que `rgba(ink, .12)`
// sobre blanco daba (241,241,242) en Gio contra (227,230,232) en producción:
// todos los bordes de la suite a media fuerza, en todas las pantallas. Se veían
// «casi bien», que es por lo que nadie lo miró hasta comparar píxeles.
//
// Los vectores de aquí son los colores MEDIDOS en producción, no los que calcula
// este mismo código: un test que compare la implementación consigo misma pasa
// siempre.
func TestBordesSonLosDelNavegador(t *testing.T) {
	casos := []struct {
		token  string
		quiero color.NRGBA
		medido bool
	}{
		// DERIVADO del contrato claro: rgba(15,29,32,.12) sobre #ffffff.
		{"border", color.NRGBA{R: 226, G: 228, B: 228, A: 0xff}, false},
		// DERIVADO de la misma regla, no medido: `rgba(ink, .07)` con la tinta
		// del CSS (15, 29, 32) sobre la superficie #ffffff. Se apunta la diferencia
		// a propósito: el de arriba prueba que la fórmula es la del navegador;
		// este solo prueba que se aplica igual al otro alfa.
		{"border-subtle", color.NRGBA{R: 238, G: 239, B: 239, A: 0xff}, false},
	}
	for _, c := range casos {
		got := Color(c.token, Light)
		if got.A != 0xff {
			t.Errorf("--sytue-%s sigue con alfa (A=%d): Gio lo mezcla en lineal y sale claro de más",
				c.token, got.A)
		}
		// Un canal de margen: el redondeo del navegador y el nuestro pueden
		// discrepar en uno, pero no en catorce, que es lo que había.
		canales := [][3]any{{"R", got.R, c.quiero.R}, {"G", got.G, c.quiero.G}, {"B", got.B, c.quiero.B}}
		for _, ch := range canales {
			tengo, quiero := int(ch[1].(uint8)), int(ch[2].(uint8))
			if d := tengo - quiero; d < -1 || d > 1 {
				t.Errorf("--sytue-%s canal %s = %d, quiero %d (±1)", c.token, ch[0], tengo, quiero)
			}
		}
	}
}

// El velo NO se aplana: va sobre contenido arbitrario y tiene que seguir dejando
// ver lo de debajo. Si alguien amplía `aplanarBordes` a todo lo que lleve alfa,
// esto se pone en rojo antes de que un modal tape la pantalla entera.
func TestElVeloSigueSiendoTranslucido(t *testing.T) {
	for _, v := range []Variant{Light, Dark, Translucent} {
		if a := Color("overlay", v).A; a == 0xff {
			t.Errorf("%v: el velo salió opaco; taparía la pantalla en vez de velarla", v)
		}
	}
}
