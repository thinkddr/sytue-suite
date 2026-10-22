// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// InteractiveArea incluye la cabecera del driver en las ventanas secundarias.
// El negro de Presentar no debe tapar su fondo claro, pero sí llegar a ambos
// bordes y bajo el indicador de inicio después de girar.
func TestElFondoDePresentarRespetaLaCabeceraNativa(t *testing.T) {
	for _, segura := range []fyne.Position{{X: 0, Y: 80}, {X: 44, Y: 48}, {X: 0, Y: 48}} {
		total := fyne.NewSize(390, 260)
		cabecera := canvas.NewRectangle(color.White)
		cabecera.Resize(total)
		fondo := canvas.NewRectangle(color.Black)
		raiz := container.NewWithoutLayout(fondo)
		raiz.Move(segura)
		colocarFondoDeVentana(fondo, segura, total)
		for _, escala := range []float32{1, 3} {
			img := pintaA(t, escala, total.Width, total.Height, cabecera, raiz)
			for _, p := range []fyne.Position{{X: 2, Y: 2}, {X: 195, Y: segura.Y - 1}} {
				if got := px(img, int(p.X*escala), int(p.Y*escala)); got != (color.NRGBA{R: 255, G: 255, B: 255, A: 255}) {
					t.Errorf("la presentación tapa la cabecera en %v: %v", p, got)
				}
			}
			for _, p := range []fyne.Position{{X: 2, Y: segura.Y + 1}, {X: 388, Y: segura.Y + 1}, {X: 195, Y: 258}} {
				if got := px(img, int(p.X*escala), int(p.Y*escala)); got != (color.NRGBA{A: 255}) {
					t.Errorf("el fondo deja una franja después de girar en %v: %v", p, got)
				}
			}
		}
	}
}

// Un fondo plano debe cubrir también una página larga sin pedir una textura de
// página completa. El pintor GL sube cada Raster a Size×escala y no limita esa
// subida al máximo de la GPU; el pintor de software no tiene ese límite.
func TestElFondoSolidoNoGastaUnaTexturaDelAltoDeLaPagina(t *testing.T) {
	const retina, presupuesto = float32(3), float32(4096) // presupuesto del guardián, no límite atribuido a un dispositivo
	f := NuevoFondoSuite(tema.Claro)
	for _, v := range []tema.Variante{tema.Claro, tema.Oscuro, tema.Claro} {
		f.PonerVariante(v)
		for _, alto := range []float32{32, 6000, 32} {
			f.Resize(fyne.NewSize(8, alto))
			for _, o := range test.WidgetRenderer(f).Objects() {
				if !o.Visible() {
					continue
				}
				var textura fyne.Size
				switch x := o.(type) {
				case *canvas.Raster:
					textura = x.Size()
				case *canvas.Image:
					if x.Image != nil {
						b := x.Image.Bounds()
						textura = fyne.NewSize(float32(b.Dx()), float32(b.Dy()))
					}
				}
				if max(textura.Width, textura.Height)*retina > presupuesto {
					t.Errorf("fondo %v, página de %g: textura %v a ×%g excede el presupuesto de %g px", v, alto, textura, retina, presupuesto)
				}
			}
			// Se pinta el inicio, el centro y el final, con el mismo color opaco y
			// sin depender de una captura de toda la página ni de su fondo blanco.
			for _, y := range []float32{0, max(alto/2-4, 0), alto - 8} {
				f.Move(fyne.NewPos(0, -y))
				img := pintaA(t, 1, 8, 8, f)
				for _, p := range [][2]int{{0, 0}, {4, 4}, {7, 7}} {
					if got, quiero := px(img, p[0], p[1]), tema.Color("bg", v); got != quiero || got.A != 255 {
						t.Errorf("fondo %v, alto=%g, y=%g, píxel=%v: %v; quiero %v opaco", v, alto, y, p, got, quiero)
					}
				}
			}
			f.Move(fyne.Position{})
		}
	}
}

func TestElFondoSuiteInterpolaLosTresStopsEnSRGB(t *testing.T) {
	// Colores inyectados: el guardián no compara contra colores calculados por el
	// widget ni repite las tintas de branding en el código de producción.
	c := tintasFondoSuite{
		azul:  color.NRGBA{R: 10, G: 20, B: 30, A: 255},
		medio: color.NRGBA{R: 110, G: 120, B: 130, A: 255},
		menta: color.NRGBA{R: 210, G: 220, B: 230, A: 255},
	}
	g := geometriaDelFondoSuite(100, 100)
	for _, caso := range []struct {
		x, y float64
		c    color.NRGBA
	}{
		{0, 0, color.NRGBA{R: 10, G: 20, B: 30, A: 255}},
		{22, 22, color.NRGBA{R: 60, G: 70, B: 80, A: 255}},
		{44, 44, color.NRGBA{R: 110, G: 120, B: 130, A: 255}},
		{72, 72, color.NRGBA{R: 160, G: 170, B: 180, A: 255}},
		{100, 100, color.NRGBA{R: 210, G: 220, B: 230, A: 255}},
	} {
		if got := g.color(caso.x, caso.y, c); got != caso.c {
			t.Errorf("stop en (%g,%g): %v; quiero %v", caso.x, caso.y, got, caso.c)
		}
	}

	// A 112°, el lado izquierdo cambia de tono al pasar de apaisado a vertical.
	// Valores fijos, calculados fuera del pintor con azul10 → medio110 al 44%.
	for _, caso := range []struct {
		w, h float64
		c    color.NRGBA
	}{
		{200, 100, color.NRGBA{R: 29, G: 39, B: 49, A: 255}},
		{100, 200, color.NRGBA{R: 61, G: 71, B: 81, A: 255}},
	} {
		g = geometriaDelFondoSuite(caso.w, caso.h)
		if got := g.color(0, caso.h/2, c); got != caso.c {
			t.Errorf("fondo %g×%g, centro izquierdo: %v; quiero %v", caso.w, caso.h, got, caso.c)
		}
	}
}

func TestElFondoSuiteComponeLaElipseHastaEl38PorCiento(t *testing.T) {
	base := color.NRGBA{R: 100, G: 100, B: 100, A: 255}
	c := tintasFondoSuite{azul: base, medio: base, menta: base, calido: color.NRGBA{R: 200, A: 128}}
	g := geometriaDelFondoSuite(100, 100)
	for _, caso := range []struct {
		x, y float64
		c    color.NRGBA
	}{
		{88, 32, color.NRGBA{R: 150, G: 50, B: 50, A: 255}},
		// La mitad del radio horizontal: rx=124,4508, radio38%=47,2913.
		{64.354349, 32, color.NRGBA{R: 125, G: 75, B: 75, A: 255}},
		{40.708698, 32, base},
		{0, 0, base},
	} {
		if got := g.color(caso.x, caso.y, c); got != caso.c {
			t.Errorf("elipse en (%g,%g): %v; quiero %v", caso.x, caso.y, got, caso.c)
		}
	}
}

func TestElFondoSuiteConservaDimensionesYRefrescaLaApariencia(t *testing.T) {
	f := NuevoFondoSuite(tema.Claro)
	r := test.WidgetRenderer(f).(*fondoSuiteRenderer)
	raster := r.raster
	for _, v := range []tema.Variante{tema.Claro, tema.Oscuro, tema.Translucido, tema.Claro} {
		f.PonerVariante(v)
		if r.v != v || r.raster != raster {
			t.Fatalf("cambiar apariencia ha perdido la variante o el ráster: %v", r.v)
		}
		for _, tam := range []fyne.Size{fyne.NewSize(200, 100), fyne.NewSize(100, 200)} {
			f.Resize(tam)
			if raster.Size() != tam || f.MinSize() != (fyne.Size{}) {
				t.Errorf("el fondo no llena %v o exige un mínimo: %v / %v", tam, raster.Size(), f.MinSize())
			}
			if v != tema.Translucido {
				img := pintaA(t, 1, tam.Width, tam.Height, f)
				quiero := tema.Color("bg", v)
				for _, p := range [][2]int{{0, 0}, {int(tam.Width) - 1, int(tam.Height) - 1}} {
					if got := color.NRGBAModel.Convert(img.At(p[0], p[1])); got != quiero {
						t.Errorf("fondo sólido %v en %v: %v, quiero %v", v, p, got, quiero)
					}
				}
			} else {
				img := r.pintar(int(tam.Width), int(tam.Height))
				if img.Bounds().Dx() != int(tam.Width) || img.Bounds().Dy() != int(tam.Height) {
					t.Errorf("el raster pierde las dimensiones pedidas: %v", img.Bounds())
				}
				if img.At(0, 0) == img.At(int(tam.Width)-1, int(tam.Height)-1) {
					t.Error("la apariencia translúcida sigue siendo un fondo plano")
				}
			}
		}
	}
	// También se admite editar la variante pública y después refrescar el widget.
	f.Variante = tema.Oscuro
	f.Refresh()
	if r.v != tema.Oscuro || r.base != tema.Color("bg", tema.Oscuro) {
		t.Error("Refresh no recoge la variante nueva")
	}
	if img := r.pintar(0, 0); !img.Bounds().Empty() {
		t.Errorf("un lienzo vacío pinta %v", img.Bounds())
	}
}

func TestElArmazonUsaElFondoSuiteYLoConservaAlRepintar(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	tema.Elegir("traslucido")
	a := NuevoArmazon(iconoFalso())
	a.Resize(fyne.NewSize(800, 600))
	r := test.WidgetRenderer(a).(*armazonRenderer)
	fondo := r.fondo
	if r.Objects()[0] != fondo || fondo.Variante != tema.Translucido || fondo.Size() != a.Size() {
		t.Fatal("el lienzo translúcido no llena la raíz del armazón")
	}
	a.Refresh()
	if r.fondo != fondo {
		t.Fatal("repintar contenido ha recreado el lienzo")
	}
	tema.Elegir("oscuro")
	a.Refresh()
	if r.fondo != fondo || fondo.Variante != tema.Oscuro {
		t.Error("cambiar tema no conserva y refresca el lienzo del armazón")
	}
}

func TestElLienzoTraslucidoCoincideConChromium(t *testing.T) {
	// Screenshot real de --workspace-canvas a320×240, DSF1, Chromium remoto
	// 2026-10-04, con los colores de la paleta petróleo. Estas muestras no llaman al
	// resolver ni a la interpolación.
	r := test.WidgetRenderer(NuevoFondoSuite(tema.Translucido)).(*fondoSuiteRenderer)
	img := r.pintar(320, 240)
	for _, caso := range []struct {
		x, y int
		css  color.NRGBA
	}{
		{0, 0, color.NRGBA{R: 182, G: 216, B: 219, A: 255}},
		{319, 0, color.NRGBA{R: 225, G: 231, B: 214, A: 255}},
		{0, 239, color.NRGBA{R: 196, G: 223, B: 222, A: 255}},
		{319, 239, color.NRGBA{R: 235, G: 232, B: 210, A: 255}},
		{159, 119, color.NRGBA{R: 211, G: 227, B: 221, A: 255}},
		{281, 76, color.NRGBA{R: 229, G: 223, B: 183, A: 255}},
	} {
		got := color.NRGBAModel.Convert(img.At(caso.x, caso.y)).(color.NRGBA)
		if !cercaDe(got, caso.css, 1) {
			t.Errorf("lienzo en (%d,%d)=%v; captura Chromium=%v", caso.x, caso.y, got, caso.css)
		}
	}
}
