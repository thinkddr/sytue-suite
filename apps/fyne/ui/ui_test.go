// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// enVentanaReal lo pone `ventanareal_test.go` (`-tags ventanareal`).
var enVentanaReal func(string) error

// TestMain arranca la aplicación de pruebas de Fyne con NUESTRO tema. Sin él,
// `canvas.Text` resolvería la letra de serie y todas las medidas de aquí serían las de
// otra fuente.
//
// Corre sin CGO y sin servidor gráfico: el pintor de software de Fyne es Go puro. Eso
// es lo que hace que estos guardianes puedan correr en el hook de pre-push y en el CI,
// que es donde tienen que correr.
func TestMain(m *testing.M) {
	if dir := os.Getenv("SYTUE_VENTANA_REAL"); dir != "" && enVentanaReal != nil {
		if err := enVentanaReal(dir); err != nil {
			fmt.Println("VENTANA REAL: FALLO:", err)
			os.Exit(1)
		}
		fmt.Println("VENTANA REAL: bien")
		os.Exit(0)
	}
	test.NewApp().Settings().SetTheme(tema.Sytue{})
	tema.FormaDeLosGuardianes()
	// Las pruebas pulsan el velo en el mismo instante en que se pone: sin esto las
	// tomaría por el segundo clic de un doble clic (lo prueba TestElVeloCierraYElPanelNo).
	esperaVelo = 0
	m.Run()
}

// TestElFyneDoDeOtraRutinaEsperaAlTest guarda el parche del driver de pruebas del fork
// (v2.8.1-sytue.4): el `fyne.Do` que llega de otra rutina —un reloj, una carga— se
// encola, y lo ejecuta la rutina del test en su siguiente punto de sincronía (`Capture`
// de una ventana de prueba, `test.Tap`, `test.WidgetRenderer` o un `fyne.DoAndWait`
// suyo). Antes corría en la rutina del reloj, y el que dejaba vivo un test tocaba la
// caché de renderers mientras el siguiente capturaba: una carrera que cambiaba de
// víctima con cada semilla de `-shuffle`. Si un fork nuevo pierde el parche, esto se
// pone rojo siempre, y no solo con la semilla mala.
//
// Consecuencia para quien escribe tests: para esperar a algo que va en segundo plano,
// el bucle de espera llama a `fyne.DoAndWait`; un `time.Sleep` o un canal solos ya no
// dejan pasar lo encolado.
func TestElFyneDoDeOtraRutinaEsperaAlTest(t *testing.T) {
	hecho := false
	enviado := make(chan struct{})
	go func() {
		fyne.Do(func() { hecho = true })
		close(enviado)
	}()
	<-enviado
	if hecho {
		t.Fatal("el fyne.Do de otra rutina ha corrido en ella: el driver de pruebas no es el del fork")
	}
	fyne.DoAndWait(func() {})
	if !hecho {
		t.Fatal("un fyne.DoAndWait en la rutina del test no ha ejecutado lo encolado")
	}
}

// escala es a la que se comprueban los píxeles.
//
// DOS y no uno, y es la decisión que sostiene todos los guardianes de píxel de este
// paquete: a ×1 el pintor tiene que elegir una fila para un medio píxel, igual que el
// navegador, y comparar ahí mide el redondeo y no el widget. A ×2 —Retina, HiDPI,
// cualquier teléfono— los medios píxeles de esta escala salen EXACTOS, así que una
// diferencia a ×2 sí es un fallo nuestro.
const escala float32 = 2

// pinta devuelve la captura de un lienzo BLANCO de w×h (en px de CSS) con los objetos
// encima, a `escala`. La imagen sale en píxeles de DISPOSITIVO: w×escala por h×escala.
//
// Blanco y no el fondo del tema para poder comparar con los vectores medidos en
// producción, que se midieron sobre blanco. Y sin relleno: por defecto el lienzo mete
// el `padding` del tema y desplaza todo, que es como se lee mal una captura y se saca
// la conclusión contraria.
func pinta(t *testing.T, w, h float32, objs ...fyne.CanvasObject) image.Image {
	return pintaA(t, escala, w, h, objs...)
}

// pintaA es pinta a una escala concreta. Solo la usa el guardián del trazo, que tiene
// que mirar a ×1 porque es donde el defecto existe.
func pintaA(t *testing.T, esc, w, h float32, objs ...fyne.CanvasObject) image.Image {
	t.Helper()
	fondo := fcanvas.NewRectangle(color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	fondo.Resize(fyne.NewSize(w, h))
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetContent(container.NewWithoutLayout(append([]fyne.CanvasObject{fondo}, objs...)...))
	c.SetScale(esc)
	c.Resize(fyne.NewSize(w, h))
	return c.Capture()
}

// mismaCaja compara la medida DECLARADA por un widget con la que se ha pintado, con un
// píxel de DISPOSITIVO de margen.
//
// El margen no es pereza, es lo que se puede exigir, y conviene entender por qué antes
// de apretarlo:
//
//   - Fyne cuantiza cada objeto por separado, así que una caja cuyo alto sale de sumar
//     tres piezas puede caer en un cuarto de píxel (la tarjeta: 106,25) y el pintor
//     tiene que elegir.
//   - Medir y pintar conforman ya al MISMO tamaño (los dos en px de CSS, ver el
//     `replace` del go.mod), pero el pintor coloca el origen de cada tirada en un píxel
//     de dispositivo ENTERO —`int(x)` en `DrawStringOffset`—, así que un ancho que sale
//     de medir texto difiere del pintado en menos de un píxel de dispositivo. Es
//     inofensivo —el texto va centrado y sobra sitio— pero impide la igualdad exacta.
//
// Lo que sí se exige exacto es `MinSize() == MedidaX()`, que es aritmética nuestra, y la
// geometría de rectángulos puros (TestLaGeometriaEsExactaADoble).
func mismaCaja(t *testing.T, que string, declarada fyne.Size, x0, x1, y0, y1 int) {
	t.Helper()
	pintada := fyne.NewSize(float32(x1-x0+1)/escala, float32(y1-y0+1)/escala)
	d := func(a, b float32) float32 {
		if a > b {
			return a - b
		}
		return b - a
	}
	if d(pintada.Width, declarada.Width) > 1/escala || d(pintada.Height, declarada.Height) > 1/escala {
		t.Errorf("%s dice medir %v y pinta %v px de CSS (de x %d a %d, de y %d a %d, a ×%g):"+
			" más de un píxel de dispositivo de diferencia", que, declarada, pintada, x0, x1, y0, y1, escala)
	}
}

func px(img image.Image, x, y int) color.NRGBA {
	r, g, b, a := img.At(x, y).RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

// cerca compara dos colores con un canal de margen: el redondeo del navegador y el
// nuestro pueden discrepar en uno, pero no en catorce, que es lo que se le escapó a Gio.
func cerca(a, b color.NRGBA) bool {
	d := func(x, y uint8) bool { return int(x)-int(y) > 1 || int(y)-int(x) > 1 }
	return !d(a.R, b.R) && !d(a.G, b.G) && !d(a.B, b.B)
}

// caja devuelve el rectángulo de píxeles de DISPOSITIVO donde aparece un color.
func caja(img image.Image, c color.NRGBA) (x0, x1, y0, y1 int) {
	b := img.Bounds()
	x0, x1, y0, y1 = 9999, -1, 9999, -1
	for y := range b.Dy() {
		for x := range b.Dx() {
			if cerca(px(img, x, y), c) {
				x0, x1, y0, y1 = min(x0, x), max(x1, x), min(y0, y), max(y1, y)
			}
		}
	}
	return x0, x1, y0, y1
}

// TestLaGeometriaEsExactaADoble es la medición que decide que la escala de la suite NO
// se redondee en nuestro código.
//
// A ×1 un rectángulo de 31,5 ocupa 32 filas y uno de 10,5 ocupa 11: medio píxel de más,
// porque el pintor tiene que elegir. A ×2 los mismos dan 63 y 21 clavados. Redondear en
// `tema` congelaría a ×1 un error que la pantalla arregla sola.
//
// Si esto se pone en rojo, la primera sospecha es que Fyne ha cambiado su regla de
// cuantización, y entonces hay que volver a medir ANTES de tocar ningún widget.
func TestLaGeometriaEsExactaADoble(t *testing.T) {
	filas := func(alto float32) (total, parciales int) {
		r := fcanvas.NewRectangle(color.NRGBA{A: 255})
		r.Resize(fyne.NewSize(20, alto))
		img := pinta(t, 60, 60, r)
		for y := range img.Bounds().Dy() {
			switch p := px(img, int(5*escala), y); {
			case p.R == 0:
				total++
			case p.R < 255:
				total++
				parciales++
			}
		}
		return total, parciales
	}
	for _, c := range []struct {
		css     float32
		enDoble int
		porQue  string
	}{
		{31, 62, "entero, por dos"},
		{31.5, 63, "`h-9` = 2,25rem × 14: el medio píxel cabe a ×2"},
		{10.5, 21, "`p-3` = 0,75rem × 14"},
		{7, 14, "`p-2`, que ya era entero"},
	} {
		n, parc := filas(c.css)
		if n != c.enDoble {
			t.Errorf("%v px de CSS pintan %d filas de dispositivo a ×%g y deberían ser %d (%s)",
				c.css, n, escala, c.enDoble, c.porQue)
		}
		if parc != 0 {
			t.Errorf("%v px de CSS dejan %d filas de cobertura parcial: Fyne ya interpola los lados rectos"+
				" y este paquete puede simplificarse", c.css, parc)
		}
	}
}

// TestElBordePintadoEsElDelNavegador es el guardián de la regla 4 del paquete.
//
// El vector es el MEDIDO en producción con el inspector: `--sytue-border` sobre blanco
// da (227,230,232). No es lo que calcula este código; un test que compara la
// implementación consigo misma pasa siempre.
//
// Y de paso prueba por qué `Caja` no usa `StrokeWidth`: el mismo borde como trazo de un
// píxel sale a media fuerza, porque Fyne lo pinta CENTRADO en el contorno y reparte su
// cobertura entre dos filas.
func TestElBordePintadoEsElDelNavegador(t *testing.T) {
	quiero := color.NRGBA{R: 226, G: 228, B: 228, A: 255}

	c := NuevaCaja(tema.Color("surface", tema.Claro), tema.Color("border", tema.Claro), tema.RadioMD)
	c.Redimensionar(fyne.NewSize(40, 20))
	img := pinta(t, 60, 40, c.Objetos()...)
	medio := int(10 * escala)
	// A media altura, el borde ocupa los `Borde` px de CSS de la izquierda y nada más.
	for i := range int(tema.Borde * escala) {
		if got := px(img, i, medio); !cerca(got, quiero) {
			t.Errorf("la columna %d del borde pinta %v y en producción es %v", i, got, quiero)
		}
	}
	if got, fondo := px(img, int(tema.Borde*escala), medio), tema.Color("surface", tema.Claro); !cerca(got, fondo) {
		t.Errorf("el píxel de dentro del borde pinta %v y el relleno es %v: el borde ocupa más de lo que dice", got, fondo)
	}

	// La razón de no usar el trazo de Fyne, comprobada y no supuesta.
	trazo := fcanvas.NewRectangle(color.NRGBA{})
	trazo.StrokeColor = quiero
	trazo.StrokeWidth = tema.Borde
	trazo.Resize(fyne.NewSize(40, 20))
	trazo.Move(fyne.NewPos(5, 5))
	img = pinta(t, 60, 40, trazo)
	if got := px(img, 5, 10); cerca(got, quiero) {
		t.Errorf("StrokeWidth pinta %v, que YA es el color de producción: Fyne ha dejado de centrar el trazo\n"+
			"  y la regla 4 del paquete (Caja con dos rectángulos) se puede simplificar. Comprobadlo y cambiadlo.", got)
	}
}

// TestElAlfaSeComponeEnSRGB fija el espacio en el que Fyne mezcla, que es la mitad del
// problema que tuvo Gio: allí se componía en LINEAL y todos los bordes salían a media
// fuerza —(241,241,242) donde la web da (227,230,232)—.
//
// Fyne mezcla en sRGB, igual que el navegador. Aun así los bordes se guardan ya
// aplanados y opacos en `apps/nucleo/tokens`, porque lo que sí se pierde es el alfa al
// cuantizarlo a un byte: 0,12 son 30,6/255 y hay que elegir 30 o 31. Con el token
// aplanado el píxel es exactamente el de producción y no depende de ese redondeo.
func TestElAlfaSeComponeEnSRGB(t *testing.T) {
	a := 0.12
	tinta := color.NRGBA{R: 26, G: 53, B: 65, A: uint8(a*255 + 0.5)} // --sytue-ink-rgb
	r := fcanvas.NewRectangle(tinta)
	r.Resize(fyne.NewSize(20, 20))
	img := pinta(t, 40, 40, r)

	mezcla := func(c float64) uint8 { return uint8(255 + (c-255)*a + 0.5) }
	sRGB := color.NRGBA{R: mezcla(26), G: mezcla(53), B: mezcla(65), A: 255}
	lineal := color.NRGBA{R: 241, G: 241, B: 242, A: 255} // lo que daba Gio
	got := px(img, int(10*escala), int(10*escala))
	if !cerca(got, sRGB) {
		t.Errorf("tinta al 12 %% sobre blanco pinta %v; en sRGB (lo que hace el navegador) son %v", got, sRGB)
	}
	if cerca(got, lineal) {
		t.Errorf("tinta al 12 %% ha dado %v, que es la mezcla en espacio LINEAL: los bordes saldrán a media fuerza", got)
	}
}

// TestElConformadorNoRedondea es el guardián del `replace` de go-text/typesetting, que
// es lo que quitó el último techo de la paridad tipográfica.
//
// Hasta el 2026-09-21 el conformador escalaba la fuente a `ceil(tamaño)` antes de pedir
// los avances (`shaping/shaping.go`: `font.XScale = int32(input.Size.Ceil()) << …`), así
// que pedir 11,375 px daba EXACTAMENTE los avances de 12. Con `1 rem = 14 px` casi toda
// la escala de la suite es fraccionaria: `text-base` salía un +6,12 % más ancho,
// `text-sm` un +5,49 %, `text-3xl` un +1,59 %. El fork del go.mod pasa el tamaño en
// crudo y conforma al ppem exacto.
//
// Este test lo comprueba por PROPORCIONALIDAD y no comparando con un número grabado: el
// ancho a un tamaño fraccionario tiene que ser el del mismo texto a 64 px multiplicado
// por la razón de tamaños. Con `ceil` esa razón se rompe entre un +1,59 % y un +6,12 %;
// con el parche, lo medido en el piloto el 2026-09-21 se queda en **0,23 % en el peor
// caso** (`iiiillll` a 9,625 px; los rótulos reales, por debajo del 0,05 %), que es el
// redondeo de harfbuzz a 1/64 px por glifo. El margen de 0,5 % va justo en medio: el
// doble del ruido medido y un tercio del error más pequeño que quiere cazar.
//
// VISTO EN ROJO en las dos direcciones el 2026-09-21: con el `replace` quitado falla en
// las diez entradas de la escala, y apretando el margen a 0,1 % falla en `iiiillll`.
//
// Si se pone en rojo: o el `replace` se ha caído del go.mod, o el fork se rebasó sobre
// un upstream que volvió a redondear. Ver la política de rebase en el ADR 0048.
func TestElConformadorNoRedondea(t *testing.T) {
	// 64 es entero, así que el `ceil` no lo tocaba ni antes: sirve de patrón en las dos
	// versiones del conformador y hace del test una comparación contra la FUENTE, no
	// contra sí mismo.
	const referencia float32 = 64
	const margen = 0.005

	for _, texto := range []string{"Guardar cambios", "Cambiar de organización", "iiiillll"} {
		patron := Medir(texto, tema.Escalon{Tam: referencia}, Normal).Width
		for _, e := range []tema.Escalon{
			tema.Texto2XS, tema.TextoXS, tema.TextoSM, tema.TextoBase, tema.TextoRaiz,
			tema.TextoMD, tema.TextoLG, tema.TextoXL, tema.Texto2XL, tema.Texto3XL,
			{Tam: 9.625},
			{Tam: 11.375}, // guardianes fraccionarios aunque la escala actual sea entera
		} {
			quiero := patron * e.Tam / referencia
			got := Medir(texto, e, Normal).Width
			if desvio := got/quiero - 1; desvio > margen || desvio < -margen {
				t.Errorf("%q a %v px mide %v y la proporción desde 64 px pide %.4f (%+.3f %%):\n"+
					"  el conformador ha vuelto a cuantizar el ppem. ¿Sigue el `replace` de\n"+
					"  go-text/typesetting en apps/fyne/go.mod?",
					texto, e.Tam, got, quiero, desvio*100)
			}
		}
	}
}

// TestMedirDevuelveLaCajaDeLineaDeCSS es la calibración que más se cuela.
//
// El alto que contesta `canvas.Text` es la caja de línea de la FUENTE —1,2 em—: para
// `text-base` (12,25) son 14,70. Desde el parche del conformador ese 1,2 em ya sale del
// tamaño exacto y no del ppem redondeado (antes daba 15,59, que es 1,2 × 13), pero sigue
// sin ser la caja de línea de CSS.
//
// La que ocupa sitio en el layout de la web es la caja de línea de CSS (19,25 para el
// texto raíz), y es la que devuelve Medir. El glifo se centra dentro: ColocarTexto.
func TestMedirDevuelveLaCajaDeLineaDeCSS(t *testing.T) {
	const s = "Guardar cambios"
	nuestra := Medir(s, tema.TextoRaiz, Normal)
	if nuestra.Height != tema.TextoRaiz.Linea {
		t.Errorf("Medir da %v de alto y la caja de línea del texto raíz es %v", nuestra.Height, tema.TextoRaiz.Linea)
	}
	deFyne, _ := fyne.CurrentApp().Driver().RenderedTextSize(s, tema.TextoRaiz.Tam, fyne.TextStyle{}, tema.Regular)
	if deFyne.Height == nuestra.Height {
		t.Errorf("la caja de la fuente y la de línea coinciden en %v: revisad si sigue haciendo falta distinguirlas", nuestra.Height)
	}
}

// TestLaLetraEsFigtree comprueba que el tema de la suite está puesto: si no lo
// estuviera, todas las medidas de este paquete serían las de la fuente de serie de Fyne
// y los guardianes medirían otra cosa creyendo que miden la suite.
func TestLaLetraEsFigtree(t *testing.T) {
	nuestra := Medir("Guardar cambios", tema.TextoRaiz, Normal)
	// La misma cadena por la vía de Fyne, que ignora FontSource y usa el tema.
	delTema := fyne.MeasureText("Guardar cambios", tema.TextoRaiz.Tam, fyne.TextStyle{})
	if nuestra.Width != delTema.Width {
		t.Errorf("Medir da %v de ancho y el tema de la aplicación da %v: el tema de la suite no está puesto",
			nuestra.Width, delTema.Width)
	}
}

// TestLaTarjetaMideLoQueDiceMedir es el mismo guardián del botón aplicado al segundo
// widget: declarado == MinSize == pintado.
func TestLaTarjetaMideLoQueDiceMedir(t *testing.T) {
	cuerpo := NuevoBoton("Aceptar", nil)
	tj := NuevaTarjeta("Resumen", cuerpo)
	// Plana: la penumbra de la sombra pasa por el gris del borde suave y alargaría la
	// caja un píxel. La sombra no cuenta en la medida, así que mide lo mismo sin ella.
	tj.Plana = true
	declarada := MedidaTarjeta(tj.Titulo, tj.Cuerpo)
	if got := tj.MinSize(); got != declarada {
		t.Errorf("MinSize() = %v y MedidaTarjeta dice %v: hay dos cuentas donde tiene que haber una", got, declarada)
	}

	// `px-4` alrededor del cuerpo más el borde por cada lado, sin redondear.
	if quiero := cuerpo.MinSize().Width + 2*tema.Espacio(4) + 2*tema.Borde; declarada.Width != quiero {
		t.Errorf("la tarjeta mide %v de ancho y el cuerpo con su `px-4` y su borde son %v", declarada.Width, quiero)
	}

	// Se busca el BORDE y no el relleno: la superficie de la tarjeta es blanca, igual
	// que el lienzo de la captura, así que rastrear el relleno daría el lienzo entero.
	// El borde sí delimita la caja: en los lados rectos llega a los cuatro extremos.
	tj.Resize(declarada)
	img := pinta(t, declarada.Width+20, declarada.Height+20, tj)
	x0, x1, y0, y1 := caja(img, BordeSobre("border-subtle", "surface", tema.Claro))
	mismaCaja(t, "la tarjeta", declarada, x0, x1, y0, y1)
}

// TestUnaCajaConBordeSinMedirNoRevientaAlPintar: una caja con borde y radio a la que
// todavía no le ha llegado el layout (0 × 0) se pinta sin más. VISTO EN ROJO
// (2026-09-23, en el frontal): el relleno quedaba en −2 × −2 y `Capture` entraba en
// pánico en el rasterizador («index out of range [0] with length 0»), con los
// selectores de la cinta de Diapos recién rehechos al cambiar de diapositiva.
func TestUnaCajaConBordeSinMedirNoRevientaAlPintar(t *testing.T) {
	c := NuevaCaja(color.NRGBA{R: 255, A: 255}, color.NRGBA{A: 255}, 4)
	c.Redimensionar(fyne.NewSize(0, 0))
	lienzo := software.NewCanvas()
	lienzo.SetContent(container.NewWithoutLayout(c.Objetos()...))
	lienzo.Resize(fyne.NewSize(20, 20))
	lienzo.Capture()
}

// TestLaCajaNoAceptaLadosNegativos: un padre que resta más de lo que tiene le pide a la
// caja un lado negativo, y el rasterizador de las esquinas entraba en pánico (visto en
// Docus con −2). VISTO EN ROJO sin el tope: la pintura revienta.
func TestLaCajaNoAceptaLadosNegativos(t *testing.T) {
	c := NuevaCaja(tema.Color("surface", tema.Claro), tema.Color("border", tema.Claro), tema.RadioMD)
	c.Redimensionar(fyne.NewSize(-2, 30))
	for _, o := range c.Objetos() {
		if o.Size().Width < 0 || o.Size().Height < 0 {
			t.Fatalf("la caja se queda con %v: un lado negativo tumba el rasterizador", o.Size())
		}
	}
	pinta(t, 40, 40, c.Objetos()...)
}

// TestLaLineaBaseEsLaDeChromium compara dónde cae la tinta de una «H» con lo MEDIDO en
// Chromium (el frontal, `--font-render-hinting=none`, a ×2, re-medido el 2026-10-03 con raíz 16): «HHHH Recibidos»
// en seis escalones, cada uno con su caja de línea arriba en y = 10 + 40·i. La última
// fila con tinta de la H es la de encima de la línea base. Con el glifo centrado en la
// caja, como se hacía, caía de 2 a 3 filas de dispositivo más abajo en todos. VISTO EN
// ROJO con el centrado de antes.
func TestLaLineaBaseEsLaDeChromium(t *testing.T) {
	medido := []struct {
		e       tema.Escalon
		ultima  int // última fila con tinta en Chromium
		primera int // primera
	}{
		{tema.TextoXS, 43, 28},
		{tema.TextoSM, 125, 108},
		{tema.TextoBase, 209, 191},
		{tema.TextoMD, 293, 271},
		{tema.TextoLG, 377, 352},
		{tema.TextoXL, 463, 433},
	}
	objs := make([]fyne.CanvasObject, 0, len(medido))
	for i, m := range medido {
		tx := Texto("HHHH Recibidos", m.e, Normal, color.NRGBA{A: 255})
		ColocarTexto(tx, m.e, 10, float32(10+40*i))
		objs = append(objs, tx)
	}
	img := pinta(t, 400, 260, objs...)
	for i, m := range medido {
		y0, y1 := int((10+40*float32(i))*escala)-4, int((10+40*float32(i)+m.e.Linea)*escala)+4
		primera, ultima := -1, -1
		for y := y0; y < y1; y++ {
			for x := 20; x < 40; x++ { // la primera H, que empieza en x = 10
				if p := px(img, x, y); int(p.R)+int(p.G)+int(p.B) < 3*128 {
					if primera < 0 {
						primera = y
					}
					ultima = y
					break
				}
			}
		}
		if ultima != m.ultima || primera < m.primera-1 || primera > m.primera+1 {
			t.Errorf("escalón %v/%v: la H va de la fila %d a la %d y en Chromium de la %d a la %d",
				m.e.Tam, m.e.Linea, primera, ultima, m.primera, m.ultima)
		}
	}
}

// TestLaCajaSinBordeConservaElRadioDeCSS compara una esquina SM4 sin borde.
// Chromium a raíz de 16px pinta (247,250,253) en (1.25,.75): menos del 10%
// de cobertura azul. Descontar el borde inexistente vuelve azul ese píxel.
func TestLaCajaSinBordeConservaElRadioDeCSS(t *testing.T) {
	c := NuevaCaja(hex(0x1a6fbb), color.NRGBA{}, 4)
	img := pinta(t, 30, 30, nuevaCajaSuelta(c, fyne.NewSize(20, 20)))
	if got := px(img, 2, 1); got.R < 230 {
		t.Errorf("la esquina rounded-sm sin borde pinta %v; Chromium la deja casi blanca", got)
	}
}
