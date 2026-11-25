// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"sync"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLaSombraDeNuevePartesEsLaMismaQueLaEntera: la sombra estirada desde su plantilla
// tiene que ser, byte a byte, la que sale de pintarla entera, en los cinco niveles, los dos
// temas, con y sin recorte del interior, con radios de nada a «píldora» y con medidas
// fraccionarias.
//
// VISTO EN ROJO (2026-11-24): con la plantilla 30 px más estrecha de lo que pide
// tamPlantilla y sin la comprobación de la columna y la fila del medio, ya el primer caso
// (`xs`, 200×120) sale distinto de la entera.
func TestLaSombraDeNuevePartesEsLaMismaQueLaEntera(t *testing.T) {
	// El último no es diádico: se pinta entero y tiene que dar lo mismo.
	tamanos := []fyne.Size{
		{Width: 200, Height: 120},
		{Width: 37.5, Height: 300.25},
		{Width: 640, Height: 48},
		{Width: 901.75, Height: 503},
		{Width: 145.703, Height: 260},
	}
	propias := []tema.CapaSombra{
		{DesplX: -3, DesplY: 8.5, Desenfoque: 20, Color: color.NRGBA{R: 15, G: 23, B: 42, A: 40}},
		{DesplY: 1, Desenfoque: 3, Color: color.NRGBA{R: 15, G: 23, B: 42, A: 25}},
	}
	estiradas := 0
	for n := tema.SombraXS; n <= tema.SombraXL; n++ {
		for _, v := range []tema.Variante{tema.Claro, tema.Oscuro} {
			capas := tema.Sombra(n, v)
			if n == tema.SombraXS && v == tema.Oscuro {
				capas = propias
			}
			for _, radio := range []float32{0, 8, 999} {
				for _, tam := range tamanos {
					for _, sin := range []bool{false, true} {
						quiero := pintarSombra(capas, radio, tam, sin)
						if _, ok := tamPlantilla(capas, radio, tam); ok {
							estiradas++
						}
						got := pintarONueve(capas, radio, tam, sin)
						if got.Rect != quiero.Rect || !bytes.Equal(got.Pix, quiero.Pix) {
							t.Fatalf("nivel %d, variante %d, radio %g, %v, sin interior %v: la de nueve partes no es la entera",
								n, v, radio, tam, sin)
						}
					}
				}
			}
		}
	}
	// Sin esto el guardián pasaría pintando siempre entero.
	if estiradas < 100 {
		t.Errorf("solo %d casos salieron de una plantilla: el guardián no mira las nueve partes", estiradas)
	}
}

// TestLaSombraDeNuevePartesNoDependeDelTamano: lo que se pinta de verdad es la plantilla,
// que no crece con el panel.
func TestLaSombraDeNuevePartesNoDependeDelTamano(t *testing.T) {
	capas := tema.Sombra(tema.SombraMD, tema.Claro)
	a, okA := tamPlantilla(capas, tema.RadioLG, fyne.NewSize(600, 400))
	b, okB := tamPlantilla(capas, tema.RadioLG, fyne.NewSize(1800, 1200))
	if !okA || !okB || a != b {
		t.Errorf("las plantillas de 600×400 y 1800×1200 son %v (%v) y %v (%v): tienen que ser la misma", a, okA, b, okB)
	}
}

// TestLasSombrasIgualesCompartenImagen: dos paneles con la misma sombra, rehechos, no la
// vuelven a pintar; se llevan la misma imagen, que es la de pintarla entera.
func TestLasSombrasIgualesCompartenImagen(t *testing.T) {
	sombras.vaciar()
	tam := fyne.NewSize(280, 132)
	a := NuevaSombra(tema.SombraSM, tema.RadioLG)
	a.Ajustar(fyne.Position{}, tam, tema.Claro)
	b := NuevaSombra(tema.SombraSM, tema.RadioLG)
	b.Ajustar(fyne.Position{}, tam, tema.Claro)
	if a.img.Image != b.img.Image {
		t.Fatal("dos sombras iguales llevan dos imágenes: la caché no se usa")
	}
	quiero := pintarSombra(tema.Sombra(tema.SombraSM, tema.Claro), tema.RadioLG, tam, false)
	if !bytes.Equal(a.img.Image.(*image.NRGBA).Pix, quiero.Pix) {
		t.Error("la imagen de la caché no es la de pintarla entera")
	}
	// Otra variante, otro recorte u otro radio son otra sombra.
	c := NuevaSombra(tema.SombraSM, tema.RadioLG).SinInterior()
	c.Ajustar(fyne.Position{}, tam, tema.Claro)
	d := NuevaSombra(tema.SombraSM, tema.RadioLG)
	d.Ajustar(fyne.Position{}, tam, tema.Oscuro)
	if c.img.Image == a.img.Image || d.img.Image == a.img.Image {
		t.Error("el recorte del interior o la variante no cuentan en la clave")
	}
}

// TestLaCacheDeSombrasNoPasaDeSuPresupuesto: el LRU cuenta bytes, saca la menos usada y
// no se guarda lo que solo ya no cabe.
//
// VISTO EN ROJO (2026-11-24): sin el bucle de expulsión de guardar, a la undécima imagen
// retiene 1 126 400 bytes de 1 048 576.
func TestLaCacheDeSombrasNoPasaDeSuPresupuesto(t *testing.T) {
	c := nuevaCacheSombras(1 << 20)
	img := func() *image.NRGBA { return image.NewNRGBA(image.Rect(0, 0, 160, 160)) } // 100 KiB
	clave := func(i int) []byte { return fmt.Appendf(nil, "k%d", i) }
	for i := range 32 {
		c.guardar(clave(i), img())
		if i == 5 {
			c.buscar(clave(0)) // la 0 se usa: pasa delante
		}
		if c.usados > c.presupuesto {
			t.Fatalf("tras %d imágenes la caché retiene %d bytes de %d", i+1, c.usados, c.presupuesto)
		}
	}
	if len(c.entradas) != 10 {
		t.Errorf("caben 10 de 100 KiB en 1 MiB y hay %d", len(c.entradas))
	}
	if c.buscar(clave(31)) == nil || c.buscar(clave(1)) != nil {
		t.Error("el LRU no conserva la última ni saca las viejas")
	}
	c.guardar([]byte("enorme"), image.NewNRGBA(image.Rect(0, 0, 1024, 1024)))
	if c.buscar([]byte("enorme")) != nil || c.buscar(clave(31)) == nil {
		t.Error("una imagen mayor que el presupuesto se ha guardado y ha echado a las demás")
	}
	total := 0
	for e := c.raiz.sig; e != &c.raiz; e = e.sig {
		total += bytesSombra(e.img)
	}
	if total != c.usados {
		t.Errorf("la cuenta de bytes dice %d y las entradas suman %d", c.usados, total)
	}
}

// TestLaCacheDeSombrasAguantaVariasGorutinas: con `-race`, pedir las mismas sombras desde
// varias gorutinas no corre ni da imágenes distintas de la de referencia.
func TestLaCacheDeSombrasAguantaVariasGorutinas(t *testing.T) {
	sombras.vaciar()
	capas := tema.Sombra(tema.SombraMD, tema.Claro)
	tams := []fyne.Size{{Width: 300, Height: 200}, {Width: 320, Height: 200}, {Width: 64, Height: 64}}
	quiero := make([]*image.NRGBA, len(tams))
	for i, tam := range tams {
		quiero[i] = pintarSombra(capas, tema.RadioLG, tam, false)
	}
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for i := range 30 {
				k := (g + i) % len(tams)
				if got := imagenSombra(capas, tema.RadioLG, tams[k], false); !bytes.Equal(got.Pix, quiero[k].Pix) {
					t.Errorf("gorutina %d: la sombra %v no es la de referencia", g, tams[k])
					return
				}
			}
		})
	}
	wg.Wait()
}
