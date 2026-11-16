// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
)

// DuracionColores es el `transition-colors duration-150` de los controles: una fila del
// menú, una del lateral o su acción no SALTAN de color al pasar el ratón, lo cambian en
// 150 ms.
const DuracionColores = 150 * time.Millisecond

// CurvaColores es la de `transition-*` en la web: theme.css reasigna
// `--default-transition-timing-function` a `--ease-standard`, así que el hover, el foco y
// el pulsado van con la misma curva que las entradas (§1.5) y no con la que Tailwind trae
// de serie. Lo vigila `TestLasTransicionesDeColorLlevanLaCurvaDeLaHoja`.
var CurvaColores = CurvaEstandar

// transicion es un color que va a su destino en DuracionColores en vez de saltar: el
// `transition-colors` de la hoja, para un relleno.
//
// El primer color se pone sin animar —la fila nace con su color, no «llega» a él—, y un
// destino nuevo a mitad de camino arranca desde donde iba el anterior, que es lo que hace
// el navegador cuando el ratón cruza deprisa.
//
// ponytail: una animación de Fyne por cambio; con el ratón barriendo un menú son dos o
// tres a la vez, de 150 ms. Si una lista enorme las encadena, aquí va un único reloj.
type transicion struct {
	puesta         bool
	visto, destino color.NRGBA
	anim           *fyne.Animation
	pintar         func(color.NRGBA)
}

// hacia lleva el color a `c` y llama a `pintar` en cada fotograma.
func (t *transicion) hacia(c color.NRGBA, pintar func(color.NRGBA)) {
	t.pintar = pintar
	if !t.puesta || MenosMovimiento() {
		t.puesta, t.visto, t.destino = true, c, c
		t.parar()
		pintar(c)
		return
	}
	if c == t.destino {
		pintar(t.visto) // un Refresh sin cambio de estado repinta lo que se ve
		return
	}
	t.parar()
	desde := t.visto
	t.destino = c
	t.anim = fyne.NewAnimation(DuracionColores, func(p float32) {
		t.visto = mezclaColor(desde, c, p)
		t.pintar(t.visto)
	})
	t.anim.Curve = CurvaColores
	t.anim.Start()
}

func (t *transicion) parar() {
	if t.anim != nil {
		t.anim.Stop()
		t.anim = nil
	}
}

// mezclaColor interpola dos colores en sRGB PREMULTIPLICADO, que es como interpola CSS
// un `transition-colors`: de `transparent` a un velo, el tono no pasa por el negro
// del `rgba(0,0,0,0)`, solo sube el alfa.
func mezclaColor(a, b color.NRGBA, p float32) color.NRGBA {
	if p <= 0 {
		return a
	}
	if p >= 1 {
		return b
	}
	aA, bA := float32(a.A)/255, float32(b.A)/255
	alfa := aA + (bA-aA)*p
	if alfa <= 0 {
		return color.NRGBA{}
	}
	canal := func(x, y uint8) uint8 {
		v := (float32(x)*aA + (float32(y)*bA-float32(x)*aA)*p) / alfa
		return uint8(min(max(v+0.5, 0), 255))
	}
	return color.NRGBA{R: canal(a.R, b.R), G: canal(a.G, b.G), B: canal(a.B, b.B), A: uint8(alfa*255 + 0.5)}
}
