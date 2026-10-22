// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// cajaSuelta es una `Caja` con cáscara, para poder pintarla sola. Sin cáscara no se
// pinta: el recorrido de Fyne solo desciende en `*fyne.Container` y en `fyne.Widget`.
type cajaSuelta struct {
	widget.BaseWidget
	c   *Caja
	tam fyne.Size
}

func nuevaCajaSuelta(c *Caja, tam fyne.Size) *cajaSuelta {
	o := &cajaSuelta{c: c, tam: tam}
	o.ExtendBaseWidget(o)
	o.Resize(tam)
	return o
}

func (o *cajaSuelta) CreateRenderer() fyne.WidgetRenderer { return &cajaSueltaR{o: o} }

type cajaSueltaR struct{ o *cajaSuelta }

func (r *cajaSueltaR) MinSize() fyne.Size           { return r.o.tam }
func (r *cajaSueltaR) Objects() []fyne.CanvasObject { return r.o.c.Objetos() }
func (r *cajaSueltaR) Destroy()                     {}
func (r *cajaSueltaR) Layout(t fyne.Size)           { r.o.c.Redimensionar(t) }
func (r *cajaSueltaR) Refresh()                     {}

// TestElInerteNoApilaDosCapasTranslucidas es el guardián de `atenuar`, y está escrito
// sobre el defecto MEDIDO que lo motivó.
//
// `disabled:opacity-50` se aplica en la web UNA vez sobre el grupo. Bajando el alfa
// color a color, los dos rectángulos de una `Caja` se componen por separado y el
// interior sale `0,75 × relleno + 0,25 × borde`. Medido antes del arreglo, con una
// entrada inerte sobre blanco y el pintor de software a ×2:
//
//	franja del borde   (193,200,203) contra (193,201,204)   error  0 / −1 / −1
//	interior           (224,228,229) contra (255,255,255)   error −31 / −27 / −26
//
// La franja ya salía bien porque los tokens `border*` llegan APLANADOS de
// `apps/nucleo/tokens`: ahí no había dos translúcidas. El fallo estaba en el relleno.
// Este test exige los dos a cero, así que volver a bajar el alfa lo pone en rojo.
func TestElInerteNoApilaDosCapasTranslucidas(t *testing.T) {
	v := tema.Claro
	superficie, borde := tema.Color("surface", v), tema.Color("border-strong", v)
	caja := NuevaCaja(atenuar(superficie, true), atenuar(borde, true), tema.RadioMD)
	tam := fyne.NewSize(60, 30)
	img := pinta(t, tam.Width+10, tam.Height+10, nuevaCajaSuelta(caja, tam))

	// La web: el control entero, y el resultado al 50 % sobre la superficie.
	web := func(x color.NRGBA) color.NRGBA {
		m := func(f, fondo uint8) uint8 { return uint8(float64(f)*0.5 + float64(fondo)*0.5 + 0.5) }
		return color.NRGBA{R: m(x.R, superficie.R), G: m(x.G, superficie.G), B: m(x.B, superficie.B), A: 255}
	}
	y := 15 * int(escala)
	if got := px(img, 0, y); !cerca(got, web(borde)) {
		t.Errorf("la franja del borde inerte pinta %v y la web da %v", got, web(borde))
	}
	if got := px(img, 30*int(escala), y); !cerca(got, web(superficie)) {
		t.Errorf("el interior inerte pinta %v y la web da %v: se están apilando dos capas translúcidas", got, web(superficie))
	}
}

// TestUnColorYaTransparenteNoGanaFondoAlAtenuarse: sin esta salvaguarda, mezclar un
// color de alfa cero con la superficie devuelve la superficie OPACA, y un botón
// fantasma inerte —que en reposo no tiene relleno— pasaría a tener fondo blanco encima
// de lo que sea que esté debajo.
func TestUnColorYaTransparenteNoGanaFondoAlAtenuarse(t *testing.T) {
	if got := atenuar(color.NRGBA{}, true); got.A != 0 {
		t.Errorf("atenuar un color transparente devuelve %v y tiene que seguir siendo transparente", got)
	}
	// Y sin inercia no toca nada, ni siquiera el alfa.
	c := tema.Color("border-strong", tema.Claro)
	if got := atenuar(c, false); got != c {
		t.Errorf("atenuar sin inercia ha devuelto %v en vez de %v", got, c)
	}
}
