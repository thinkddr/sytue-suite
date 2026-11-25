// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"image/color"
	"math"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// TestElPuntoDeLlamadaSeVeEnLosDosTemas (ROOM-03): el punto de «hay llamada» de la fila
// del canal llega a 3:1 (WCAG 1.4.11) sobre la fila elegida del carril, en claro y en
// oscuro, con el cromo «papel» de las apps. Con `success` se quedaba a ~2,8:1 en oscuro.
func TestElPuntoDeLlamadaSeVeEnLosDosTemas(t *testing.T) {
	tema.ElegirCromo(tema.CromoPapel)
	t.Cleanup(func() { tema.ElegirCromo(""); tema.Elegir(""); tema.FormaDeLosGuardianes() })
	for _, forma := range []string{"marco", "carril"} {
		for _, variante := range []string{"claro", "oscuro"} {
			tema.ElegirForma(forma)
			tema.Elegir(variante)
			v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
			f := NuevaFilaCanal(traductor(t), Canal{ID: "c1", Nombre: "equipo", EnLlamada: true}, nil, true, func() {}, func() {})
			w := test.NewTempWindow(t, f)
			w.Resize(MedidaFilaCanal().Max(fyne.NewSize(260, 0)))
			fondo := tokens.SobreSRGB(tema.Color("sidebar-active-bg", v), tema.Color(tokenFondoMenu(), v))
			if c := contrasteDe(f.vivo.FillColor.(color.NRGBA), fondo); c < 3 {
				t.Errorf("%s/%s: el punto de llamada queda a %.2f:1 sobre la fila elegida", forma, variante, c)
			}
		}
	}
}

// tokenFondoMenu es lo que hay detrás del menú lateral en la forma en vigor.
func tokenFondoMenu() string {
	if tema.EsMarco() {
		return "frame-from"
	}
	return "sidebar-bg"
}

func contrasteDe(a, b color.NRGBA) float64 {
	l := func(c color.NRGBA) float64 {
		ch := func(x uint8) float64 {
			f := float64(x) / 255
			if f <= 0.03928 {
				return f / 12.92
			}
			return math.Pow((f+0.055)/1.055, 2.4)
		}
		return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
	}
	la, lb := l(a), l(b)
	return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
}
