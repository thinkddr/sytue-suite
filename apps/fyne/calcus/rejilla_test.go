// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/driver/software"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// escala es a la que se comprueban los píxeles: DOS, por lo mismo que en `ui` —a ×1 el
// pintor tiene que elegir una fila para un medio píxel y se mediría el redondeo, no el
// widget.
const escala float32 = 2

// pinta captura los objetos sobre un lienzo blanco, sin relleno y a `escala`.
func pinta(t *testing.T, w, h float32, objs ...fyne.CanvasObject) image.Image {
	t.Helper()
	fondo := fcanvas.NewRectangle(color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	fondo.Resize(fyne.NewSize(w, h))
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetContent(container.NewWithoutLayout(append([]fyne.CanvasObject{fondo}, objs...)...))
	c.SetScale(escala)
	c.Resize(fyne.NewSize(w, h))
	return c.Capture()
}

func px(img image.Image, x, y float32) color.NRGBA {
	r, g, b, a := img.At(int(x*escala), int(y*escala)).RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

// cerca compara con un canal de margen: el alfa se cuantiza a un byte y el redondeo
// puede discrepar en uno, pero no en catorce.
func cerca(a, b color.NRGBA) bool {
	d := func(x, y uint8) bool { return int(x)-int(y) > 1 || int(y)-int(x) > 1 }
	return !d(a.R, b.R) && !d(a.G, b.G) && !d(a.B, b.B)
}

// rejillaDePrueba devuelve una rejilla ya dimensionada, lista para capturar.
func rejillaDePrueba(w, h float32) *Rejilla {
	r := NuevaRejilla(NuevoLibro())
	r.Resize(fyne.NewSize(w, h))
	return r
}

// guarda deja el PNG donde diga `SYTUE_CAPTURA`. Es como se saca la imagen que se mira
// al lado de la captura de la web, y el mismo gesto que `pantalla/demo_test.go`.
func guarda(t *testing.T, img image.Image) {
	t.Helper()
	destino := os.Getenv("SYTUE_CAPTURA")
	if destino == "" {
		return
	}
	f, err := os.Create(destino)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("captura escrita en %s", destino)
}

// TestLaCuadriculaCaeDondeLaWebLaPinta es el guardián de geometría: la línea que
// separa dos columnas tiene que estar en el ÚLTIMO píxel de la celda —`border-r` con
// `box-sizing: border-box`— y no en el primero de la siguiente.
//
// Los números no salen de este código: 40 + 2×96 − 1 es `HEAD_W + 2·CELL_W − 1` con
// las constantes de SheetCanvas.tsx, y el color es `--editor-grid-line` aplanado.
func TestLaCuadriculaCaeDondeLaWebLaPinta(t *testing.T) {
	r := rejillaDePrueba(400, 200)
	img := pinta(t, 400, 200, r)
	guarda(t, img)

	// El borde derecho de B1: dentro de la celda, en su último píxel.
	const borde = AnchoCabecera + 2*AnchoCelda - tema.Borde
	const mediaFila = AltoCelda + AltoCelda/2
	if got := px(img, borde, mediaFila); !cerca(got, LineaRejilla) {
		t.Errorf("la línea de la cuadrícula pinta %v en x=%v y `--editor-grid-line` sobre papel es %v",
			got, borde, LineaRejilla)
	}
	if got := px(img, borde-1, mediaFila); !cerca(got, Papel) {
		t.Errorf("el píxel de dentro de la celda pinta %v y el papel es %v: la línea ocupa más de lo que dice", got, Papel)
	}
	if got := px(img, borde+1, mediaFila); !cerca(got, Papel) {
		t.Errorf("el píxel de la celda siguiente pinta %v y el papel es %v: la línea se ha corrido a la celda de al lado", got, Papel)
	}

	// Y el borde de abajo de la fila 1, que es la misma cuenta en el otro eje. Se mira
	// bajo B1 y no bajo A1: A1 es la celda ancla y su marco de selección tapa los dos
	// últimos píxeles de la celda, que es justo donde va este borde.
	const bordeH = 2*AltoCelda - tema.Borde
	if got := px(img, AnchoCabecera+AnchoCelda+AnchoCelda/2, bordeH); !cerca(got, LineaRejilla) {
		t.Errorf("la línea horizontal pinta %v en y=%v y debería ser %v", got, bordeH, LineaRejilla)
	}
}

// TestLasCabecerasSonPegajosasYLlevanSuFondo comprueba las dos bandas y la esquina:
// `bg-bg-subtle` con el `border-border` de la web, y que siguen en su sitio con la
// rejilla desplazada (que es lo que significa `sticky`).
func TestLasCabecerasSonPegajosasYLlevanSuFondo(t *testing.T) {
	r := rejillaDePrueba(400, 200)
	fondo := tema.Color("bg-subtle", tema.Claro)

	for _, paso := range []struct {
		nombre     string
		desX, desY float32
	}{
		{"sin desplazar", 0, 0},
		{"desplazada", 150, 90},
	} {
		r.desX, r.desY = paso.desX, paso.desY
		img := pinta(t, 400, 200, r)
		if got := px(img, 10, 10); !cerca(got, fondo) {
			t.Errorf("%s: la esquina pinta %v y `bg-subtle` es %v", paso.nombre, got, fondo)
		}
		if got := px(img, 200, 10); !cerca(got, fondo) {
			t.Errorf("%s: la banda de columnas pinta %v y `bg-subtle` es %v", paso.nombre, got, fondo)
		}
		if got := px(img, 10, 120); !cerca(got, fondo) {
			t.Errorf("%s: la banda de filas pinta %v y `bg-subtle` es %v", paso.nombre, got, fondo)
		}
		// Y el `border-b` de la banda, en su último píxel.
		if got := px(img, 200, AltoCelda-tema.Borde); !cerca(got, tema.Color("border", tema.Claro)) {
			t.Errorf("%s: el borde de la banda pinta %v y el token `border` es %v",
				paso.nombre, got, tema.Color("border", tema.Claro))
		}
	}
}

// TestLaCeldaAnclaLlevaElMarcoDeDosPixeles es la sombra `inset 0 0 0 2px` de la web:
// dos píxeles POR DENTRO de la celda, en el acento de marca, y el fondo intacto.
//
// Que el fondo siga siendo el papel es la mitad importante: si el marco fuera una
// `ui.Caja` —rellena— taparía el formato condicional de la celda.
//
// Los 2 px van escritos aquí a mano, y no como `GrosorSeleccion`. Es a propósito y es
// lo que este test tenía mal la primera vez: con la constante, bajarla a 1 movía
// TAMBIÉN los puntos donde se mira, el test seguía verde y el marco medía la mitad.
// Un guardián que saca su vector de lo que vigila no vigila nada. El 2 sale de
// `SEL_STYLE` en SheetCanvas.tsx, y que siga ahí lo comprueba
// `TestLasClasesDeLaRejillaSiguenSiendoLasMismas`.
func TestLaCeldaAnclaLlevaElMarcoDeDosPixeles(t *testing.T) {
	const grosorWeb float32 = 2 // `inset 0 0 0 2px`

	r := rejillaDePrueba(400, 200)
	r.Seleccionar(1, 1, false) // B2
	img := pinta(t, 400, 200, r)

	x := AnchoCabecera + AnchoCelda // borde izquierdo de B2
	y := 2 * AltoCelda              // borde superior de B2
	// superficies.css usa --editor-select = primary-500; colors.css lo fija en #1a9aa3.
	acento := color.NRGBA{R: 26, G: 154, B: 163, A: 255}
	// Los DOS píxeles de cada listón, uno a uno: el de fuera y el de dentro.
	for _, p := range []struct {
		nombre string
		x, y   float32
	}{
		{"el listón de arriba, píxel de fuera", x + AnchoCelda/2, y + 0.5},
		{"el listón de arriba, píxel de dentro", x + AnchoCelda/2, y + grosorWeb - 0.5},
		{"el de abajo, píxel de fuera", x + AnchoCelda/2, y + AltoCelda - 0.5},
		{"el de abajo, píxel de dentro", x + AnchoCelda/2, y + AltoCelda - grosorWeb + 0.5},
		{"el de la izquierda, píxel de fuera", x + 0.5, y + AltoCelda/2},
		{"el de la izquierda, píxel de dentro", x + grosorWeb - 0.5, y + AltoCelda/2},
		{"el de la derecha, píxel de fuera", x + AnchoCelda - 0.5, y + AltoCelda/2},
		{"el de la derecha, píxel de dentro", x + AnchoCelda - grosorWeb + 0.5, y + AltoCelda/2},
	} {
		if got := px(img, p.x, p.y); !cerca(got, acento) {
			t.Errorf("%s pinta %v y el acento es %v: el marco no llega a los %v px de la web",
				p.nombre, got, acento, grosorWeb)
		}
	}
	if got := px(img, x+AnchoCelda/2, y+AltoCelda/2); !cerca(got, Papel) {
		t.Errorf("el fondo de la celda ancla pinta %v y tiene que seguir siendo el papel (%v)", got, Papel)
	}
	// Y son DOS, no tres: el tercero ya es papel.
	if got := px(img, x+grosorWeb+0.5, y+AltoCelda/2); !cerca(got, Papel) {
		t.Errorf("a %v px del borde se sigue pintando %v: el marco mide más de %v",
			grosorWeb, got, grosorWeb)
	}
}

// TestElRangoSeTineConElDocePorCiento es el `color-mix(in srgb, ACCENT 12%, papel)` de
// la web, y de paso comprueba que la celda ancla NO se tiñe: lleva el marco y conserva
// su fondo.
func TestElRangoSeTineConElDocePorCiento(t *testing.T) {
	r := rejillaDePrueba(400, 200)
	r.Seleccionar(0, 0, false)
	r.Seleccionar(2, 2, true) // A1:C3
	img := pinta(t, 400, 200, r)

	// SheetCanvas: color-mix(in srgb, #1a9aa3 12%, #fff).
	quiero := color.NRGBA{R: 228, G: 243, B: 244, A: 255}
	// El centro de B2, que está en el rango y no es el ancla.
	x := AnchoCabecera + AnchoCelda + AnchoCelda/2
	y := 2*AltoCelda + AltoCelda/2
	if got := px(img, x, y); !cerca(got, quiero) {
		t.Errorf("el rango pinta %v y el 12 %% del acento sobre papel es %v", got, quiero)
	}
	// El ancla (A1) conserva el papel.
	if got := px(img, AnchoCabecera+AnchoCelda/2, AltoCelda+AltoCelda/2); !cerca(got, Papel) {
		t.Errorf("la celda ancla pinta %v y tenía que conservar el papel (%v)", got, Papel)
	}
	// La cabecera seleccionada mezcla el acento sobre SU fondo, incluso al congelar filas.
	// superficie-hoja.css mezcla editor-select con la fuerza de la selección (12 % en
	// claro) sobre bg-subtle #e8efef, sin depender del fondo del lienzo:
	// color-mix(#1a9aa3 12%, #e8efef).
	quieroCab := color.NRGBA{R: 207, G: 229, B: 230, A: 255}
	if got := px(img, AnchoCabecera+AnchoCelda+6, AltoCelda/2); !cerca(got, quieroCab) {
		t.Errorf("la cabecera dentro del rango pinta %v y se esperaba %v", got, quieroCab)
	}
	if got := px(img, AnchoCabecera+AnchoCelda+6, AltoCelda-1); !cerca(got, color.NRGBA{R: 26, G: 154, B: 163, A: 255}) {
		t.Errorf("la arista de la cabecera seleccionada no lleva el acento: %v", got)
	}
}

// TestElTecladoMueveComoEnLaWeb: flechas mueven, Mayús extiende, Inicio y Fin saltan a
// los extremos de la fila y Escape deshace la extensión.
func TestElTecladoMueveComoEnLaWeb(t *testing.T) {
	r := rejillaDePrueba(400, 200)
	pulsa := func(k fyne.KeyName) { r.TypedKey(&fyne.KeyEvent{Name: k}) }

	pulsa(fyne.KeyDown)
	pulsa(fyne.KeyRight)
	if got := r.Seleccion(); got != (Celda{C: 1, F: 1}) {
		t.Errorf("tras abajo y derecha la celda ancla es %v y debería ser B2", got)
	}

	r.KeyDown(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	pulsa(fyne.KeyRight)
	pulsa(fyne.KeyDown)
	c1, f1, c2, f2 := r.Rango()
	if c1 != 1 || f1 != 1 || c2 != 2 || f2 != 2 {
		t.Errorf("con Mayús el rango es %d,%d..%d,%d y debería ser B2:C3", c1, f1, c2, f2)
	}
	if got := r.Seleccion(); got != (Celda{C: 1, F: 1}) {
		t.Errorf("extender el rango ha movido la celda ancla a %v", got)
	}
	r.KeyUp(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})

	pulsa(fyne.KeyEscape)
	if c1, f1, c2, f2 = r.Rango(); c2 != c1 || f2 != f1 {
		t.Errorf("Escape tenía que dejar solo la celda ancla y el rango es %d,%d..%d,%d", c1, f1, c2, f2)
	}

	// Fin es `lastUsedCell(fila)`: la última celda con algo de la fila, no la columna Z;
	// y en una fila vacía no se mueve.
	pulsa(fyne.KeyEnd)
	if got := r.Seleccion(); got != (Celda{C: 1, F: 1}) {
		t.Errorf("Fin en una fila vacía ha movido la celda a %v", got)
	}
	r.L.Escribir(4, 1, "x")
	pulsa(fyne.KeyEnd)
	if got := r.Seleccion().C; got != 4 {
		t.Errorf("Fin lleva a la columna %d y la última con algo es la E", got)
	}
	pulsa(fyne.KeyHome)
	if got := r.Seleccion().C; got != 0 {
		t.Errorf("Inicio lleva a la columna %d y debería ser la A", got)
	}
	// Y no se sale de la rejilla.
	pulsa(fyne.KeyLeft)
	pulsa(fyne.KeyUp)
	if got := r.Seleccion(); got != (Celda{C: 0, F: 0}) {
		t.Errorf("la selección se ha salido de la rejilla: %v", got)
	}
}

// TestEscribirUnaCeldaYQueLaFormulaLaEvalueElNucleo es la prueba de que el motor es el
// compartido y de que la celda guarda el texto CRUDO: la celda con `=` enseña el
// resultado y la barra de fórmulas, la fórmula.
func TestEscribirUnaCeldaYQueLaFormulaLaEvalueElNucleo(t *testing.T) {
	c := NuevoLienzo(NuevoLibro())
	c.Rejilla.Resize(fyne.NewSize(400, 200))

	c.Rejilla.Seleccionar(0, 0, false)
	c.Rejilla.Editar("")
	c.Rejilla.ed.PonerTexto("2")
	c.Rejilla.Confirmar(0, 1)
	c.Rejilla.Editar("")
	c.Rejilla.ed.PonerTexto("3")
	c.Rejilla.Confirmar(0, 1)
	c.Rejilla.Editar("=SUM(A1:A2)")
	c.Rejilla.Confirmar(0, 0)

	if got := c.Libro.Texto(0, 2); got != "5" {
		t.Errorf("A3 enseña %q y `=SUM(A1:A2)` con 2 y 3 son 5", got)
	}
	if got := c.Libro.Crudo(0, 2); got != "=SUM(A1:A2)" {
		t.Errorf("A3 guarda %q y el texto crudo tiene que ser la fórmula", got)
	}
	c.Rejilla.Seleccionar(0, 2, false)
	if got := c.Barra.ed.Texto(); got != "=SUM(A1:A2)" {
		t.Errorf("la barra de fórmulas enseña %q y debería enseñar la fórmula", got)
	}
	if got := c.Barra.Rotulo(); got != "A3" {
		t.Errorf("el rótulo de la celda activa dice %q y debería decir A3", got)
	}

	// Escape en la celda no guarda.
	c.Rejilla.Editar("")
	c.Rejilla.ed.PonerTexto("99")
	c.Rejilla.Cancelar()
	if got := c.Libro.Crudo(0, 2); got != "=SUM(A1:A2)" {
		t.Errorf("Escape ha guardado: A3 vale %q", got)
	}
}

// TestEnSoloLecturaNoSeEscribe es el estado de permiso denegado —un lector, o un
// documento que el lienzo no sabe reescribir—: se mira y se copia, no se escribe.
func TestEnSoloLecturaNoSeEscribe(t *testing.T) {
	c := NuevoLienzo(NuevoLibro())
	c.Rejilla.Resize(fyne.NewSize(400, 200))
	c.Libro.Escribir(0, 0, "dato")
	c.SoloLectura(true)

	c.Rejilla.Editar("x")
	if c.Rejilla.Editando() {
		t.Error("en solo lectura se ha abierto la celda para escribir")
	}
	c.Rejilla.Escribir("otro")
	c.Rejilla.BorrarRango()
	if got := c.Libro.Crudo(0, 0); got != "dato" {
		t.Errorf("A1 vale %q en solo lectura: se ha escrito igualmente", got)
	}
}

// TestElDesplazamientoNoSeSaleDeLaHoja: la rueda no puede dejar la rejilla en blanco
// ni por arriba ni por abajo.
func TestElDesplazamientoNoSeSaleDeLaHoja(t *testing.T) {
	r := rejillaDePrueba(400, 200)
	r.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.Delta{DX: 100, DY: 100}})
	if r.desX != 0 || r.desY != 0 {
		t.Errorf("desplazar hacia arriba desde el origen ha dejado la vista en %v,%v", r.desX, r.desY)
	}
	r.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.Delta{DX: -99999, DY: -99999}})
	maxX := float32(Columnas)*AnchoCelda + AnchoCabecera - 400
	maxY := float32(Filas)*AltoCelda + AltoCelda - 200
	if r.desX != maxX || r.desY != maxY {
		t.Errorf("el tope es %v,%v y la vista se ha quedado en %v,%v", maxX, maxY, r.desX, r.desY)
	}
}

// TestLaRejillaSeAnunciaConSuCeldaActiva: sin esto, moverse con las flechas no dice
// nada a un lector de pantalla. Es el `aria-activedescendant` de la web.
func TestLaRejillaSeAnunciaConSuCeldaActiva(t *testing.T) {
	r := rejillaDePrueba(400, 200)
	if got := r.AccessibilityLabel(); got != "A1" {
		t.Errorf("la rejilla se anuncia como %q y la celda activa es A1", got)
	}
	r.L.Escribir(1, 2, "42")
	r.Seleccionar(1, 2, false)
	if got := r.AccessibilityLabel(); got != "B3: 42" {
		t.Errorf("la rejilla se anuncia como %q y debería decir «B3: 42»", got)
	}
}

// TestElTextoQueNoCabeSeRecortaEnLaCaja: la celda de la web lleva `overflow-hidden`, no
// puntos suspensivos —una hoja de cálculo que enseña «12345…» miente sobre el dato—, y
// el navegador corta a mitad de glifo. Aquí el texto va ENTERO dentro de una caja del
// tamaño de la de relleno de la celda, que lo recorta.
func TestElTextoQueNoCabeSeRecortaEnLaCaja(t *testing.T) {
	r := rejillaDePrueba(400, 200)
	largo := "1234567890123456789012345678901234567890"
	r.L.Escribir(0, 0, largo)
	r.Refresh()

	cp := r.r.celdas[0]
	if cp.texto.Text != largo {
		t.Fatalf("el texto se ha cortado a %q: tiene que ir entero y recortarlo la caja", cp.texto.Text)
	}
	if got, quiero := cp.caja.Size(), fyne.NewSize(AnchoCelda-tema.Borde, AltoCelda-tema.Borde); got != quiero {
		t.Errorf("la caja que recorta mide %v y la de relleno de la celda %v", got, quiero)
	}
	if got := cp.caja.Position(); got != fyne.NewPos(AnchoCabecera, AltoCelda) {
		t.Errorf("la caja que recorta está en %v", got)
	}
	var s fyne.Scrollable = cp.caja // sin esto Fyne no recorta
	_ = s
}

// TestDosLineasCentradasEnsenanMediaCadaUna: con `wrap` y `valign: middle`, un texto de
// dos líneas se centra en la celda (48 px de líneas en 23 de caja) y la web enseña la
// mitad de abajo de la primera y la de arriba de la segunda.
func TestDosLineasCentradasEnsenanMediaCadaUna(t *testing.T) {
	c := lienzoDePrueba()
	c.Cargar(`{"active":0,"sheets":[{"name":"H","cells":{"A1":"Presupuesto anual"},` +
		`"styles":{"A1":{"wrap":true,"valign":"middle"}}}]}`)
	pinta(t, 560, 300, c)
	cp := c.Rejilla.r.celdas[0]
	if n := len(partirLineas(cp.texto, "Presupuesto anual", AnchoCelda-tema.Borde-2*MargenCelda)); n != 2 {
		t.Fatalf("el texto de prueba se parte en %d líneas y tiene que ser en dos", n)
	}
	if !cp.texto.Visible() || !cp.otra.Visible() {
		t.Fatalf("se ven %v y %v: tienen que verse las dos líneas del medio", cp.texto.Visible(), cp.otra.Visible())
	}
	alto := AltoCelda - tema.Borde
	// La caja de línea de la primera empieza (23 − 48) / 2 = −12,5 por encima de la caja.
	if y := cp.texto.Position().Y; y >= 0 {
		t.Errorf("la primera línea no asoma por arriba de la caja: empieza en %v", y)
	}
	if y := cp.otra.Position().Y; y <= 0 || y >= alto {
		t.Errorf("la segunda línea empieza en %v, fuera de la caja de %v", y, alto)
	}
}

// TestElLienzoEnteroSePinta monta barra y rejilla con datos y los pinta.
//
// No es una captura de adorno: `tema.Color` entra en pánico con un nombre de token
// desconocido AL PINTAR, no al compilar, así que un widget puede pasar todos los tests
// de medida y reventar la primera vez que alguien lo enseña. Esto lo pinta entero.
//
// Con `SYTUE_CAPTURA=/ruta/hoja.png` deja además el PNG, que es lo que se mira al lado
// de la captura de la web.
func TestElLienzoEnteroSePinta(t *testing.T) {
	const w, h float32 = 560, 300
	c := NuevoLienzo(NuevoLibro())
	for f, fila := range [][2]string{
		{"Enero", "1200"},
		{"Febrero", "980"},
		{"Marzo", "1435"},
		{"Abril", "1120"},
		{"Total", "=SUM(B1:B4)"},
	} {
		c.Libro.Escribir(0, f, fila[0])
		c.Libro.Escribir(1, f, fila[1])
	}
	c.Rejilla.Seleccionar(1, 0, false)
	c.Rejilla.Seleccionar(1, 3, true) // B1:B4 marcado
	c.Resize(fyne.NewSize(w, h))

	img := pinta(t, w, h, c)
	guarda(t, img)

	// La barra vive en el cromo (`bg-surface`) y la rejilla sobre el papel: si estas
	// dos bandas se confunden, es que el lienzo no se ha apilado.
	if got, quiero := px(img, w-4, AltoBarra/2), tema.Color("surface", tema.Claro); !cerca(got, quiero) {
		t.Errorf("la barra de fórmulas pinta %v y `bg-surface` es %v", got, quiero)
	}
	if got := px(img, AnchoCabecera+AnchoCelda*2+10, AltoBarra+AltoCelda*3); !cerca(got, Papel) {
		t.Errorf("la rejilla pinta %v donde debería estar el papel (%v)", got, Papel)
	}
	if got := c.Libro.Texto(1, 4); got != "4735" {
		t.Errorf("el total dice %q y 1200+980+1435+1120 son 4735", got)
	}
}

// BenchmarkRejilla es la medida que faltaba: una hoja se desplaza en DOS ejes y eso no
// lo había medido nadie.
//
// Las cuatro variantes contestan la única pregunta que importa de una virtualización:
// ¿cuesta lo mismo con 2.600 celdas que con 130.000, y en el origen que desplazada?
// Si el coste creciera con el tamaño de la hoja, esto no estaría virtualizando nada.
func BenchmarkRejilla(b *testing.B) {
	for _, caso := range []struct {
		nombre     string
		cols, fils int
		desX, desY float32
	}{
		{"26x100_origen", 26, 100, 0, 0},
		{"26x100_desplazada", 26, 100, 1200, 1200},
		{"26x5000_origen", 26, 5000, 0, 0},
		{"26x5000_desplazada", 26, 5000, 1200, 60000},
	} {
		b.Run(caso.nombre, func(b *testing.B) {
			r := NuevaRejilla(NuevoLibro())
			r.Cols, r.Fils = caso.cols, caso.fils
			for f := range min(caso.fils, 200) {
				r.L.Escribir(f%caso.cols, f, "12345")
			}
			tam := fyne.NewSize(1200, 800)
			r.Resize(tam)
			r.desX, r.desY = caso.desX, caso.desY
			rr := r.CreateRenderer()
			rr.Layout(tam) // fuera de la medida: la primera vez crea los charcos
			b.ResetTimer()
			for b.Loop() {
				rr.Layout(tam)
			}
		})
	}

	// Y el fotograma COMPLETO de un desplazamiento: recolocar más avisar a cada
	// primitiva. Es lo que cuesta de verdad mover la rueda, y así el número de arriba
	// no se lee como si lo fuera.
	b.Run("fotograma_de_desplazamiento", func(b *testing.B) {
		r := NuevaRejilla(NuevoLibro())
		r.Fils = 5000
		for f := range 200 {
			r.L.Escribir(f%r.Cols, f, "12345")
		}
		tam := fyne.NewSize(1200, 800)
		r.Resize(tam)
		rr := r.CreateRenderer()
		rr.Layout(tam)
		b.ResetTimer()
		for i := 0; b.Loop(); i++ {
			r.desY = float32(i % 1000)
			rr.Refresh()
		}
	})
}

// TestElAroDeLaBarraEsUnRebordeYNoUnBloque es el fallo que solo se ve mirando el PNG.
//
// `ui.Aro` es un rectángulo RELLENO del color del anillo y da por hecho que el control
// tiene encima una caja opaca que solo le deja asomar el reborde. El campo de la
// fórmula es `bg-transparent` en la web, así que sin pintarle su fondo el aro salía
// como un bloque azul que se tragaba la fórmula entera.
//
// Visto en rojo quitando el rectángulo `campo` del renderer: el centro del campo
// pintaba el azul del aro en vez de la superficie.
func TestElAroDeLaBarraEsUnRebordeYNoUnBloque(t *testing.T) {
	const w, h float32 = 560, 60
	c := NuevoLienzo(NuevoLibro())
	c.Libro.Escribir(0, 0, "=SUM(B1:B4)")
	c.Rejilla.Seleccionar(0, 0, false)
	c.Resize(fyne.NewSize(w, h))
	c.Barra.enfocado = true
	c.Barra.Refresh()
	img := pinta(t, w, h, c)
	guarda(t, img)

	// El campo empieza tras el rótulo y la «fx»; se mira bien adentro, donde no hay
	// letra que valga.
	dentroX := w - MargenBarra - 12
	medio := (AltoBarra - tema.Borde) / 2
	if got, quiero := px(img, dentroX, medio), tema.Color("surface", tema.Claro); !cerca(got, quiero) {
		t.Errorf("el centro del campo enfocado pinta %v y la superficie es %v:"+
			" el aro se ha vuelto a pintar como un bloque", got, quiero)
	}
	// Y el reborde SÍ está, justo por fuera de la caja del campo.
	arriba := medio - AltoEntradaFx/2 - 1
	if got, quiero := px(img, dentroX, arriba), tema.Color("focus-ring", tema.Claro); !cerca(got, quiero) {
		t.Errorf("por fuera del campo enfocado se pinta %v y el aro es %v: no hay reborde", got, quiero)
	}
}

// TestLaPreviaDelAutorrellenoEsDiscontinuaComoEnChromium: `outline: 1px dashed` con
// `outline-offset: -2px` en una celda de 91×24, medido en el Chromium del frontal a ×1 y
// a ×2: 18 trazos de 3 px arriba (huecos de 2,06) y 5 a cada lado (huecos de 1,75), con
// un trazo en cada esquina.
func TestLaPreviaDelAutorrellenoEsDiscontinuaComoEnChromium(t *testing.T) {
	ts := discontinuo(fyne.NewPos(0, 0), fyne.NewSize(89, 22), 1)
	var arriba, izquierda []tramo
	for _, x := range ts {
		switch {
		case x.y == 0 && x.h == 1:
			arriba = append(arriba, x)
		case x.x == 0 && x.w == 1:
			izquierda = append(izquierda, x)
		}
	}
	if len(arriba) != 18 || len(izquierda) != 5 {
		t.Fatalf("salen %d trazos arriba y %d a la izquierda; Chromium pinta 18 y 5", len(arriba), len(izquierda))
	}
	if d := arriba[1].x - (3 + 35.0/17); d > 0.01 || d < -0.01 {
		t.Errorf("el segundo trazo empieza en %v y en Chromium en %v", arriba[1].x, 3+35.0/17)
	}
	if fin := arriba[17].x + arriba[17].w; fin < 88.99 || fin > 89.01 {
		t.Errorf("el último trazo acaba en %v y tiene que tocar la esquina (89)", fin)
	}
	if d := izquierda[1].y - (3 + 1.75); d > 0.01 || d < -0.01 {
		t.Errorf("el segundo trazo del lado empieza en %v y en Chromium en 4,75", izquierda[1].y)
	}
}

// TestElDesplegableSeEnciendeBajoElPuntero: el «▾» de la validación de lista lleva
// `hover:bg-selected hover:text-text`; la rejilla sigue el puntero y lo repinta al
// entrar y al salir, y el resto de la celda no cambia.
func TestElDesplegableSeEnciendeBajoElPuntero(t *testing.T) {
	c := lienzoDePrueba()
	r := c.Rejilla
	r.Seleccionar(0, 0, false)
	c.Mandar(CmdValAnadir, `{"range":"A1","values":["Sí","No"]}`)
	pos, tam, ok := r.botonValores()
	if !ok {
		t.Fatal("la celda con una lista no tiene su «▾»")
	}
	// Un punto del fondo del botón, lejos de la flecha y de su `border-l`.
	muestra := func() color.NRGBA { return px(pinta(t, 560, 300, c), pos.X+tam.Width-3, AltoBarra+pos.Y+3) }
	fuera := muestra()
	r.MouseMoved(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: pos.AddXY(tam.Width/2, tam.Height/2)}})
	encima := muestra()
	if r.boton != -1 || cerca(fuera, encima) {
		t.Errorf("con el puntero encima el «▾» sigue %v (botón %d)", encima, r.boton)
	}
	r.MouseOut()
	if r.boton != 0 || !cerca(muestra(), fuera) {
		t.Error("al salir el puntero el «▾» no vuelve a su color")
	}
}

// TestLosMarcosDeLaFormulaQuedanBajoLoInmovilizado: en la web la fila inmovilizada
// (`z-20`) tapa el marco de una referencia (`z-10`) que se ha desplazado por debajo; aquí
// el marco se recorta donde empieza lo que se desplaza.
func TestLosMarcosDeLaFormulaQuedanBajoLoInmovilizado(t *testing.T) {
	c := lienzoDePrueba()
	r := c.Rejilla
	c.Libro.Fijar(0, 1)
	r.refsFx = []formula.Referencia{{Rect: formula.Rect{C1: 0, R1: 1, C2: 0, R2: 2}}} // A2:A3
	pinta(t, 560, 300, c)
	r.Desplazar(0, AltoCelda) // A2 pasa por debajo de la fila 1
	pinta(t, 560, 300, c)
	_, finY := r.vista().finFijas()
	m := r.r.marcosFx[0]
	if !m.fondo.Visible() || m.fondo.Position().Y < finY {
		t.Errorf("el fondo del marco empieza en %v y lo inmovilizado acaba en %v", m.fondo.Position().Y, finY)
	}
	for i, b := range m.bordes {
		if b.Visible() && b.Position().Y < finY {
			t.Errorf("el listón %d del marco asoma sobre la fila inmovilizada (y = %v)", i, b.Position().Y)
		}
	}
}
