// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"image"
	"image/color"
	"math"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// a4 es una página A4 en puntos PDF, que es lo que manda el motor.
func a4() Pagina {
	return Pagina{Numero: 1, Ancho: 595, Alto: 842, Rotacion: 0, Caja: Rect{0, 0, 595, 842}}
}

// TestElPapelMideLaPaginaPorElZoom es la medida de la hoja, y los números NO salen de
// [MedidaPapel]: salen de la web.
//
// `PdfApp.tsx` pone `style={{ width: pagina.ancho * zoom, height: pagina.alto * zoom }}`
// y nada más — sin redondear y sin sumar el borde, porque el borde de la web va por
// dentro (`box-sizing: border-box`). Así que un A4 al 150 % son 892,5 × 1263 px de CSS
// EXACTOS, con su medio píxel incluido.
//
// El medio píxel es el caso que importa y por eso está en la tabla: redondearlo aquí
// congelaría a ×1 un error que una pantalla HiDPI arregla sola (regla 3 de `ui.go`).
func TestElPapelMideLaPaginaPorElZoom(t *testing.T) {
	t.Parallel()
	casos := []struct {
		zoom              float64
		anchoWeb, altoWeb float32
	}{
		{1, 595, 842},
		{0.5, 297.5, 421},
		// 595 × 1,5 = 892,5: medio píxel, que es exacto a ×2 (1785 filas).
		{1.5, 892.5, 1263},
		{2, 1190, 1684},
	}
	for _, c := range casos {
		ancho, alto := MedidaPapel(a4(), c.zoom)
		if ancho != c.anchoWeb || alto != c.altoWeb {
			t.Errorf("zoom %g: papel %g×%g, la web pone %g×%g",
				c.zoom, ancho, alto, c.anchoWeb, c.altoWeb)
		}
	}
}

// TestLaHojaReservaElHuecoDelNumero comprueba que bajo el papel caben el `gap-1.5` y la
// línea del número, que es lo que la web apila con `flex-col gap-1.5`.
//
// Los dos sumandos se escriben aquí con su valor de la HOJA DE ESTILOS y no llamando a
// `tema.Espacio(1.5)` ni a `tema.TextoXS.Linea`: un test que saca sus puntos de medida
// de la constante que vigila sale verde con el defecto puesto, que es el modo de fallo
// que ya ha mordido tres veces en esta ola.
//
//	gap-1.5 → 1,5 × 0,25rem → 1,5 × 0,25 × 16 = 6 px
//	text-xs → line-height 1rem → 16 px
func TestLaHojaReservaElHuecoDelNumero(t *testing.T) {
	t.Parallel()
	const gapWeb, lineaXSWeb = 6.0, 16.0

	_, altoPapel := MedidaPapel(a4(), 1)
	hoja := MedidaHoja(a4(), 1)

	if quiero := altoPapel + gapWeb + lineaXSWeb; hoja.Height != quiero {
		t.Errorf("la hoja mide %g de alto; el papel (%g) + gap-1.5 (%g) + text-xs (%g) son %g",
			hoja.Height, altoPapel, float32(gapWeb), float32(lineaXSWeb), quiero)
	}
	// El ancho de la hoja es el del papel: el número va centrado debajo y no ensancha.
	if anchoPapel, _ := MedidaPapel(a4(), 1); hoja.Width != anchoPapel {
		t.Errorf("la hoja mide %g de ancho; el papel mide %g", hoja.Width, anchoPapel)
	}
}

// TestElPapelEsBlancoYNoUnToken es el guardián de color, y vigila el cambio que
// parecería una mejora: sustituir el blanco literal por `tema.Color("surface", v)`.
//
// La web pone `bg-white` en la hoja. Con `surface`, en tema oscuro el papel saldría
// gris carbón y un documento —tinta negra sobre blanco— quedaría ilegible.
//
// # Lo que este test SÍ cubre y lo que NO
//
// Cubre dos cosas: que lo pintado es blanco, y que el token con el que alguien lo
// sustituiría NO es blanco en oscuro, que es lo que hace detectable la sustitución.
//
// NO captura en variante oscura, y decirlo importa: `tema.De` lee la variante de
// `fyne.CurrentApp()`, y la app de prueba de la suite no la cambia a media ejecución.
// La primera versión de este test SÍ decía «en los dos temas» y hacía dos subpruebas
// idénticas —declaraba un `fyne.Theme` por caso y no lo aplicaba nunca—, o sea que
// anunciaba el doble de lo que medía. Un guardián que miente sobre su alcance es peor
// que no tenerlo, porque el siguiente da por cubierto lo que no lo está.
func TestElPapelEsBlancoYNoUnToken(t *testing.T) {
	h := NuevaHoja(a4(), 0.2, HojaSinDibujar, "x", nil)
	w := ventanaSinRelleno(h)
	defer w.Close()
	w.Resize(MedidaHoja(a4(), 0.2))

	// El centro del papel. A zoom 0,2 el A4 son 119×168,4, así que (40, 40) cae dentro
	// del papel y lejos del rótulo, que va centrado.
	r, g, b, _ := w.Canvas().Capture().At(40, 40).RGBA()
	if r>>8 < 250 || g>>8 < 250 || b>>8 < 250 {
		t.Errorf("el papel pinta (%d,%d,%d); la web pone bg-white", r>>8, g>>8, b>>8)
	}

	if ColorPapel != (color.NRGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Errorf("ColorPapel es %v y `bg-white` es blanco opaco", ColorPapel)
	}
	// Si algún día `surface` en oscuro fuera blanco, este test dejaría de distinguir
	// una cosa de la otra y habría que buscarle otro apoyo. Mejor enterarse aquí.
	if s := tema.Color("surface", tema.Oscuro); s.R > 240 && s.G > 240 && s.B > 240 {
		t.Errorf("surface en oscuro es %v, casi blanco: este guardián ya no distingue "+
			"el blanco literal del token, hay que replantearlo", s)
	}
}

// TestLaHojaDiceQueNoSabeDibujarLaPagina es el guardián de la promesa del producto: una
// página que no se sabe dibujar tiene que DECIRLO, no salir en blanco.
//
// Se comprueba contando tinta: si el aviso no se pinta, el papel es blanco uniforme y
// esto sale en rojo. Es exactamente el fallo que se quiere impedir —un documento que
// parece vacío en vez de decir que no se puede mostrar—, así que el guardián y el
// defecto son la misma cosa vista del derecho y del revés.
func TestLaHojaDiceQueNoSabeDibujarLaPagina(t *testing.T) {
	// Un zoom que deje sitio al rótulo: a 0,2 el papel mide 119 px y el aviso no cabe.
	const zoom = 1.0
	aviso := "Esta página usa algo que todavía no sabemos dibujar."

	conAviso := tintaSobreElPapel(t, NuevaHoja(a4(), zoom, HojaSinDibujar, aviso, nil), zoom)
	sinAviso := tintaSobreElPapel(t, NuevaHoja(a4(), zoom, HojaSinDibujar, "", nil), zoom)

	t.Logf("MEDIDO: con aviso %d píxeles de tinta sobre el papel; sin aviso %d", conAviso, sinAviso)
	if conAviso <= sinAviso {
		t.Fatalf("el aviso no pinta nada (%d con, %d sin): una página que no se sabe "+
			"dibujar estaría saliendo en blanco y callada", conAviso, sinAviso)
	}
}

// TestElAvisoParteEnLineasYNoSeSaleDelPapel es el defecto que tenía la hoja: el aviso
// iba en UNA línea, y a zoom bajo —que es como se ve un documento entero— la frase es
// más ancha que el papel y se salía por los dos lados, encima de lo que hubiera al
// lado.
//
// Se mide donde el defecto existía (zoom 0,2: el A4 son 119 px de ancho) y en el zoom
// natural, que es donde la frase cabe entera y NO debe partirse: sin esa segunda mitad,
// un reparto que partiera siempre por la mitad pasaría el guardián.
func TestElAvisoParteEnLineasYNoSeSaleDelPapel(t *testing.T) {
	// Sin t.Parallel: mide texto, y el conformador de Fyne comparte búfer entre llamadas
	// (visto con -race: harfbuzz.(*Buffer).Clear desde dos pruebas a la vez).
	aviso := "Esta página usa algo que todavía no sabemos dibujar."
	casos := []struct {
		zoom      float64
		minLineas int
	}{
		{0.2, 2}, // el papel mide 119: la frase no cabe de una
		{1.0, 1}, // a tamaño natural cabe, y partirla sería inventarse un salto
	}
	for _, c := range casos {
		h := NuevaHoja(a4(), c.zoom, HojaSinDibujar, aviso, nil)
		w := ventanaSinRelleno(h)
		w.Resize(MedidaHoja(a4(), c.zoom))
		r, ok := test.WidgetRenderer(h).(*hojaRenderer)
		if !ok {
			w.Close()
			t.Fatal("el renderer de la hoja no es hojaRenderer")
		}
		ancho, _ := MedidaPapel(a4(), c.zoom)
		if len(r.aviso) < c.minLineas {
			t.Errorf("zoom %g: el aviso ocupa %d línea(s) y hacen falta al menos %d en "+
				"un papel de %g px", c.zoom, len(r.aviso), c.minLineas, ancho)
		}
		if c.zoom == 1.0 && len(r.aviso) != 1 {
			t.Errorf("zoom 1: el aviso cabe entero y sale en %d líneas", len(r.aviso))
		}
		for i, l := range r.aviso {
			x, an := l.Position().X, ui.Medir(l.Text, tema.TextoXS, ui.Normal).Width
			if x < 0 || x+an > ancho {
				t.Errorf("zoom %g: la línea %d va de x=%g a %g y el papel mide %g: se sale",
					c.zoom, i, x, x+an, ancho)
			}
		}
		w.Close()
	}
}

// TestLasMarcasCaenDondeDiceLaConversion comprueba que una marca se coloca en el sitio
// que [RectEnSVG] calcula, multiplicado por el zoom — y no en (0,0), que es donde acaba
// todo lo que se olvida de colocar.
//
// El caso es una página ROTADA: con /Rotate 0 un error de ejes se disimula, y es
// justamente con rotación donde la conversión tiene algo que hacer.
func TestLasMarcasCaenDondeDiceLaConversion(t *testing.T) {
	// Sin t.Parallel: mide texto, y el conformador de Fyne comparte búfer entre llamadas
	// (visto con -race: harfbuzz.(*Buffer).Clear desde dos pruebas a la vez).
	const zoom = 2.0
	p := Pagina{Numero: 1, Ancho: 842, Alto: 595, Rotacion: 90, Caja: Rect{0, 0, 595, 842}}
	marca := Marca{Rect: Rect{100, 200, 300, 500}, Clase: MarcaCensura}

	h := NuevaHoja(p, zoom, HojaSinDibujar, "", []Marca{marca})
	w := ventanaSinRelleno(h)
	defer w.Close()
	w.Resize(MedidaHoja(p, zoom))

	r, ok := test.WidgetRenderer(h).(*hojaRenderer)
	if !ok || len(r.marcas) != 1 {
		t.Fatalf("esperaba una marca montada, hay %d", len(r.marcas))
	}
	c := RectEnSVG(p, marca.Rect)
	pos, tam := r.marcas[0].Position(), r.marcas[0].Size()
	quiero := fyne.NewPos(float32(c.X*zoom), float32(c.Y*zoom))
	if math.Abs(float64(pos.X-quiero.X)) > 0.01 || math.Abs(float64(pos.Y-quiero.Y)) > 0.01 {
		t.Errorf("la marca está en %v; RectEnSVG×%g la pone en %v", pos, zoom, quiero)
	}
	if tam.Width <= 0 || tam.Height <= 0 {
		t.Errorf("la marca mide %v: un rectángulo de lado no positivo no se pinta", tam)
	}
}

// ventanaSinRelleno monta una ventana de prueba SIN el relleno que Fyne le pone por
// defecto, igual que hace la aplicación de verdad (`w.SetPadded(false)` en `cmd/`).
//
// No es cosmético y costó una captura: con relleno, Fyne mete el widget hacia dentro y
// deja alrededor el FONDO DE LA VENTANA, que en el tema de prueba es oscuro. O sea que
// una captura del mismo tamaño que el widget sale con bandas oscuras arriba, a la
// izquierda y abajo, y cualquier recuento de píxeles se traga esas bandas como si
// fueran tinta del widget: el guardián del aviso medía 4 278 píxeles de «tinta» sobre
// un papel en blanco, y ni uno era mío.
func ventanaSinRelleno(o fyne.CanvasObject) fyne.Window {
	w := test.NewWindow(o)
	w.SetPadded(false)
	return w
}

// tintaSobreElPapel cuenta los píxeles del papel que NO son blancos, sin mirar el borde
// ni la franja del número.
func tintaSobreElPapel(t *testing.T, h *Hoja, zoom float64) int {
	t.Helper()
	w := ventanaSinRelleno(h)
	defer w.Close()
	w.Resize(MedidaHoja(h.pagina, zoom))
	img := w.Canvas().Capture()

	ancho, alto := MedidaPapel(h.pagina, zoom)
	b := img.Bounds()
	// Dos píxeles por dentro del borde, para no contarlo.
	zona := image.Rect(2, 2, int(ancho)-2, int(alto)-2).Intersect(b)
	n := 0
	for y := zona.Min.Y; y < zona.Max.Y; y++ {
		for x := zona.Min.X; x < zona.Max.X; x++ {
			if !esBlanco(img.At(x, y)) {
				n++
			}
		}
	}
	return n
}

func esBlanco(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r>>8 >= 250 && g>>8 >= 250 && b>>8 >= 250
}
