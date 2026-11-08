// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # El ERP sobre el cromo «papel» (ADR 0080)
//
// En papel la hoja es blanca y `bg` es el gris del marco: lo que la web deja sin fondo
// tiene que pintarse con la hoja, no con `bg`.

// enPapel pone Marco y papel para el test y los devuelve al acabar.
func enPapel(t *testing.T) tema.Variante {
	t.Helper()
	tema.ElegirForma("marco")
	tema.ElegirCromo(tema.CromoPapel)
	t.Cleanup(func() {
		tema.ElegirCromo("")
		tema.FormaDeLosGuardianes()
	})
	return tema.De(fyne.CurrentApp().Settings().ThemeVariant())
}

// pixeles pinta `o` a `m` y devuelve quién lee un píxel en coordenadas de la suite.
func pixeles(t *testing.T, o fyne.CanvasObject, m fyne.Size) func(x, y float32) color.NRGBA {
	t.Helper()
	w := test.NewWindow(o)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(m)
	img := w.Canvas().Capture()
	escala := float32(img.Bounds().Dx()) / m.Width
	return func(x, y float32) color.NRGBA {
		return color.NRGBAModel.Convert(img.At(int(x*escala), int(y*escala))).(color.NRGBA)
	}
}

func distancia(a, b color.NRGBA) int {
	d := func(x, y uint8) int { return max(int(x)-int(y), int(y)-int(x)) }
	return max(d(a.R, b.R), d(a.G, b.G), d(a.B, b.B))
}

// VISTO EN ROJO (2026-11-06): el velo de «Mías» era un rectángulo cuadrado y pintaba
// `selected` en la esquina de fuera del perfil redondeado; y el grupo iba relleno de
// `bg`, gris dentro de la hoja blanca.
func TestElConmutadorMiasNoTapaSuBordeYSeAsientaEnLaHoja(t *testing.T) {
	v := enPapel(t)
	hoja := tema.Color("sheet-bg", v)
	if distancia(hoja, tema.Color("bg", v)) < 8 {
		t.Fatal("en papel la hoja y `bg` deberían distinguirse")
	}
	for _, mias := range []bool{true, false} {
		c := NuevoConmutadorMias("Tareas", "Mías", "De todos", mias, nil)
		m := c.MinSize()
		px := pixeles(t, c, m)
		// La esquina del extremo del velo, fuera de la curva del perfil, no es velo.
		x, xv := float32(1), float32(2.5)
		if !mias {
			x, xv = m.Width-1.5, m.Width-3
		}
		if p, velo := px(x, 1), px(xv, m.Height/2); distancia(p, velo) < 8 {
			t.Errorf("mías=%v: la esquina pinta el velo (%v) por fuera del borde", mias, p)
		}
		// El otro segmento, lejos del rótulo: la hoja.
		xo := m.Width - 3
		if !mias {
			xo = 3
		}
		if p := px(xo, m.Height/2); distancia(p, hoja) > 4 {
			t.Errorf("mías=%v: el segmento sin elegir pinta %v; la hoja es %v", mias, p, hoja)
		}
	}
}

// VISTO EN ROJO (2026-11-06): las cajas con borde y sin fondo en la web —la lista de
// tareas, el plegado del banco, el paso de año y la lista de obligaciones— iban de
// `bg` y salían grises dentro de la hoja blanca.
func TestLasCajasSinFondoDeLaWebSeAsientanEnLaHoja(t *testing.T) {
	v := enPapel(t)
	hoja := tema.Color("sheet-bg", v)
	tr := traductor(t)
	const ancho = 400

	plegado, altoPlegado := NuevoBanco(tr).plegado(v, "ayuda", "¿Cómo?", ancho, nil, 0)
	tareas, altoTareas := NuevaTareas(tr).lista(v, nil, "2026-11-06")
	anio, ma := NuevaObligaciones(tr).pasoAnio(v)
	relleno := canvas.NewRectangle(color.NRGBA{})
	lista, altoLista := listaEnCaja(v, []bloque{{relleno, 40, 0}}, radioTarjeta())

	for _, c := range []struct {
		que  string
		o    fyne.CanvasObject
		m    fyne.Size
		x, y float32
	}{
		{"el plegado del banco", plegado, fyne.NewSize(ancho, altoPlegado), ancho - 6, altoPlegado / 2},
		{"la lista de tareas", tareas, fyne.NewSize(ancho, max(altoTareas, 20)), ancho / 2, max(altoTareas, 20) / 2},
		{"el paso de año", anio, ma, 3, ma.Height / 2},
		{"la lista de obligaciones", lista, fyne.NewSize(ancho, altoLista), ancho / 2, altoLista / 2},
	} {
		px := pixeles(t, c.o, c.m)
		if p := px(c.x, c.y); distancia(p, hoja) > 4 {
			t.Errorf("%s pinta %v por dentro; la hoja es %v", c.que, p, hoja)
		}
	}
}

// VISTO EN ROJO (2026-11-06): el glifo de imprimir iba en la tinta del texto, el único
// negro de una fila de fantasmas en `primary-text`, y su velo era otro.
func TestElGlifoDeImprimirVaEnLaTintaDeSusVecinos(t *testing.T) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	e := NuevoEnlaceIcono(iconos.Recibo, "Imprimir", nil)
	r := test.TempWidgetRenderer(t, e).(*enlaceIconoRenderer)
	if got, quiero := r.icono.Tinta(), tema.Color("primary-text", v); got != quiero {
		t.Errorf("el glifo va en %v; los fantasmas de la fila en %v", got, quiero)
	}
	// El velo al pasar, contra el de un fantasma de la suite en el mismo estado.
	g := ui.NuevoBoton("", nil).Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.IA, 14)
	e.MouseIn(nil)
	g.MouseIn(nil)
	pe, pg := pixeles(t, e, e.MinSize()), pixeles(t, g, g.MinSize())
	if a, b := pe(e.MinSize().Width/2, 2), pg(g.MinSize().Width/2, 2); distancia(a, b) > 4 {
		t.Errorf("al pasar por encima pinta %v; el fantasma, %v", a, b)
	}
}
