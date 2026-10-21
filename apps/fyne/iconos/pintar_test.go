// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package iconos

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

func TestMain(m *testing.M) {
	test.NewApp().Settings().SetTheme(tema.Sytue{})
	tema.FormaDeLosGuardianes()
	m.Run()
}

// escala, igual que en `ui`: a ×2 los medios píxeles de la suite salen exactos, así
// que una diferencia ahí es un fallo nuestro y no del redondeo.
const escala float32 = 2

// pinta captura un dibujo sobre BLANCO a `escala`, en píxeles de DISPOSITIVO.
func pinta(t *testing.T, lado float32, d *Dibujo) image.Image {
	t.Helper()
	fondo := fcanvas.NewRectangle(color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	fondo.Resize(fyne.NewSize(lado, lado))
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetContent(container.NewWithoutLayout(fondo, d.Objeto()))
	c.SetScale(escala)
	c.Resize(fyne.NewSize(lado, lado))
	d.Colocar(0, 0, lado)
	return c.Capture()
}

func px(img image.Image, x, y int) color.NRGBA {
	r, g, b, a := img.At(x, y).RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

func cerca(a, b color.NRGBA) bool {
	d := func(x, y uint8) bool { return int(x)-int(y) > 1 || int(y)-int(x) > 1 }
	return !d(a.R, b.R) && !d(a.G, b.G) && !d(a.B, b.B)
}

// cobertura suma la tinta de una columna: 1 por píxel totalmente entintado y la
// fracción que toque en los de antialiasing. Es lo que hay que medir de un trazo, no
// «cuántas filas tienen algo»: el navegador también reparte cobertura en los bordes.
//
// Se normaliza por la distancia entre el fondo y la TINTA, no por 255: con el token
// `text` —(26,53,65) sobre blanco— un píxel lleno solo recorre el 89,8 % del canal, y
// dar eso por bueno hacía parecer que faltaba un 10 % de trazo.
func cobertura(img image.Image, x, y0, y1 int, fondo, tinta color.NRGBA) float64 {
	rango := float64(int(fondo.R) - int(tinta.R))
	var s float64
	for y := y0; y < y1; y++ {
		s += float64(int(fondo.R)-int(px(img, x, y).R)) / rango
	}
	return s
}

// TestElTrazoMideLoDeLaWeb es el guardián que decide que este paquete no use
// `canvas.Image` con un recurso SVG.
//
// El vector NO sale de este código: `Glifo.tsx` pinta un trazo de 1,5 px de CSS a
// cualquier tamaño, así que a ×2 tiene que dejar TRES píxeles de dispositivo de
// cobertura por barra, con el icono a 16, a 24 o a 32. Ni la mitad (que es lo que da
// `oksvg` si no se le escala el grosor), ni un grosor que crezca con el icono (que es
// lo que hacía el `stroke-width="2"` fijo: 1,3 px a 16 y 2,7 a 32), ni la cobertura
// emborronada en más filas (que es lo que sale cuando Fyne rasteriza a píxeles de CSS
// y amplía el mapa de bits).
//
// VISTO EN ROJO (2026-10-04, en el frontal): con `grosorTrazo` devolviendo el 2 fijo de
// antes, las tres barras dejan 7,98 px a 16, 12,00 a 24 y 15,94 a 32, contra los 9 que
// tocan en los tres. Deshecho.
func TestElTrazoMideLoDeLaWeb(t *testing.T) {
	blanco := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	for _, lado := range []float32{16, 24, 32} {
		// `menu` son tres horizontales limpias en y = 6,5 / 12 / 17,5 de la rejilla.
		d := Trazo(Menu, lado, "text", tema.Claro)
		img := pinta(t, lado, d)

		enRejilla := lado * escala / 24 // píxeles de dispositivo por unidad de rejilla
		centro := int(12 * enRejilla)
		got := cobertura(img, centro, 0, int(lado*escala), blanco, tema.Color("text", tema.Claro))
		// Tres trazos de 1,5 px de CSS cada uno.
		quiero := float64(3 * grosorPx * escala)
		if got < quiero-0.35 || got > quiero+0.35 {
			t.Errorf("a %g px las tres barras de `menu` dejan %.2f px de cobertura y 1,5 px de CSS a ×%g"+
				" son %.2f.\n  Si sale la mitad, `oksvg` ha dejado de recibir el grosor escalado (ver la"+
				" nota de pintar.go); si crece con el tamaño, el grosor ya no se compensa.",
				lado, got, escala, quiero)
		}
	}

	// Y que la cobertura esté CONCENTRADA, no repartida: a 24 la barra de en medio va
	// de la unidad 11,25 a la 12,75, así que las filas de fuera tienen que estar limpias.
	const lado, enRejilla = 24, 24 * escala / 24
	img := pinta(t, lado, Trazo(Menu, lado, "text", tema.Claro))
	for _, y := range []int{int(10.5 * enRejilla), int(13.5 * enRejilla)} {
		if p := px(img, int(12*enRejilla), y); !cerca(p, blanco) {
			t.Errorf("la fila %d, fuera de la barra, pinta %v: el trazo está emborronado"+
				" (Fyne está rasterizando a píxeles de CSS y ampliando)", y, p)
		}
	}
}

// TestElTrazoSePintaConElColorDelToken comprueba lo segundo que un icono tiene que
// acertar: que el color es EL del tema y no uno parecido.
func TestElTrazoSePintaConElColorDelToken(t *testing.T) {
	for _, v := range []tema.Variante{tema.Claro, tema.Oscuro} {
		// `close` es una X de trazo entero; su centro está totalmente entintado.
		d := Trazo(Cerrar, 24, "text", v)
		img := pinta(t, 24, d)
		quiero := tema.Color("text", v)
		if got := px(img, 24, 24); !cerca(got, quiero) {
			t.Errorf("el aspa pinta %v en el centro y el token `text` es %v", got, quiero)
		}
	}
}

// TestTenirNoReconstruyeYCambiaElColor cubre el camino que usa un renderer al pasar el
// puntero por encima: cambiar el color sin rehacer el objeto.
func TestTenirNoReconstruyeYCambiaElColor(t *testing.T) {
	d := Trazo(Cerrar, 24, "text", tema.Claro)
	if got, quiero := px(pinta(t, 24, d), 24, 24), tema.Color("text", tema.Claro); !cerca(got, quiero) {
		t.Fatalf("de salida pinta %v y `text` es %v", got, quiero)
	}
	d.Tenir("danger", tema.Claro)
	if got, quiero := px(pinta(t, 24, d), 24, 24), tema.Color("danger", tema.Claro); !cerca(got, quiero) {
		t.Errorf("tras teñir pinta %v y `danger` es %v", got, quiero)
	}
}

// TestLosRematesYUnionesSonRedondos es la comprobación de que NO hace falta reescribir
// ningún trazo a mano: `oksvg` respeta `stroke-linecap` y `stroke-linejoin`.
//
// Se mide donde se nota: la punta de una barra de `menu` (que con remate recto acaba
// una unidad antes) y el vértice de abajo de la V del `check` (que con unión en pico
// se alarga).
func TestLosRematesYUnionesSonRedondos(t *testing.T) {
	blanco := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	const lado, enRejilla = 24, 24 * escala / 24

	// Remate: `menu` va de x = 3,5 a 20,5 en la rejilla; con remate redondo el trazo
	// se alarga media anchura (0,75 a 24 px), o sea hasta 21,25, y con remate recto se
	// queda en 20,5. La columna que EMPIEZA en 20,5 cae entera en el remate, y con
	// remate recto estaría vacía.
	img := pinta(t, lado, Trazo(Menu, lado, "text", tema.Claro))
	if got := cobertura(img, int(20.5*enRejilla), 0, int(lado*escala), blanco, tema.Color("text", tema.Claro)); got < 1 {
		t.Errorf("en la columna que empieza en x = 20,5 de la rejilla, donde acaba la barra, hay %.2f px"+
			" de tinta: el remate NO es redondo y habría que dibujar los cabos a mano", got)
	}

	// Unión: el mismo `check`, una vez con la unión redonda de la web y otra en pico.
	// Si salen IGUALES es que `oksvg` no lee `stroke-linejoin`, y entonces las esquinas
	// de los ochenta y tres iconos no son las del navegador. Se comparan los dos mapas
	// de bits en vez de buscar un píxel concreto porque la diferencia entre redondo y
	// pico en este vértice (88,8°) es de medio píxel, y medio píxel no se busca a mano.
	negro := color.NRGBA{A: 255}
	redonda := svgDeTrazo(catalogo[Tic].d, pintura{tinta: negro, lado: 24})
	enPico := bytes.Replace(redonda,
		[]byte(`stroke-linejoin="round"`), []byte(`stroke-linejoin="miter"`), 1)
	if bytes.Equal(redonda, enPico) {
		t.Fatal("no se ha podido cambiar la unión: `svgDeTrazo` ya no la declara así")
	}
	if a, b := rasteriza(redonda, 96, 96), rasteriza(enPico, 96, 96); bytes.Equal(a.Pix, b.Pix) {
		t.Error("el `check` con unión redonda y con unión en pico salen idénticos:" +
			" `oksvg` no está leyendo `stroke-linejoin` y las esquinas habría que dibujarlas a mano")
	}
}

// TestLaMarcaEsLaTeselaDeLaWeb comprueba la otra familia: el color del producto, la
// esquina redonda y el glifo blanco dentro.
func TestLaMarcaEsLaTeselaDeLaWeb(t *testing.T) {
	const lado = MarcaLG // 40, la tesela del lanzador
	blanco := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	img := pinta(t, lado, Marca(Mail, lado, tema.Claro))

	// El color de la tesela es el token del producto, el mismo `#dc3450` de
	// `productColor.mail`.
	quiero := tema.Color("product-mail", tema.Claro)
	if got := px(img, int(20*escala), int(30*escala)); !cerca(got, quiero) {
		t.Errorf("la tesela de Mail pinta %v y el token `product-mail` es %v", got, quiero)
	}

	// La esquina REDONDA. El badge va de 0 a 40 con radio 9 en su propio viewBox, así
	// que la esquina (1,1) queda fuera del arco y tiene que seguir siendo el fondo.
	// Es el guardián de la primera regla de `normaliza`: sin ella `oksvg` deja `ry` en
	// cero y pinta la tesela cuadrada.
	if got := px(img, 2, 2); !cerca(got, blanco) {
		t.Errorf("la esquina de la tesela pinta %v y tendría que estar fuera del arco (fondo %v):"+
			" `rx` sin `ry` ha vuelto a perderse", got, blanco)
	}

	// Y el GLIFO, que vive dentro de un `<g transform="… scale(0.9910) …">`. Es el
	// guardián de la segunda regla: con el `scale` de un solo argumento `oksvg` aborta
	// el fichero entero y la marca sale sin glifo.
	var hayBlanco bool
	for y := int(12 * escala); y < int(28*escala) && !hayBlanco; y++ {
		for x := int(8 * escala); x < int(32*escala); x++ {
			if cerca(px(img, x, y), blanco) {
				hayBlanco = true
				break
			}
		}
	}
	if !hayBlanco {
		t.Error("la marca de Mail no tiene ni un píxel blanco dentro: el sobre no se ha pintado" +
			" (el `scale(k)` de un solo argumento ha vuelto a tumbar el SVG)")
	}
}

// TestLasDosCorreccionesDeOksvgHacenFalta invierte a mano lo que `normaliza` arregla y
// comprueba que SIN ellas el dibujo sale mal. Un guardián que no se ha visto en rojo
// no guarda nada; este trae su propio rojo dentro.
func TestLasDosCorreccionesDeOksvgHacenFalta(t *testing.T) {
	crudo, err := marcasFS.ReadFile("marcas/mail.svg")
	if err != nil {
		t.Fatal(err)
	}
	// Con `normaliza`, la esquina está fuera de la tesela y hay glifo blanco.
	bien := rasteriza(crudo, 80, 80)
	// Sin ella, lo mismo pero saltándose las dos reglas.
	mal := rasterizaCrudo(crudo, 80, 80)

	if bien.NRGBAAt(3, 3).A != 0 {
		t.Errorf("con `normaliza` la esquina de la tesela está entintada (%v): la esquina redonda no sale",
			bien.NRGBAAt(3, 3))
	}
	if mal.NRGBAAt(3, 3).A == 0 {
		t.Error("sin la regla de `ry`, la esquina sigue saliendo redonda: `oksvg` ya aplica el valor" +
			" por defecto de la especificación y `rectSinRy` sobra")
	}
	cuenta := func(img *image.NRGBA) int {
		var n int
		for y := 24; y < 56; y++ {
			for x := 16; x < 64; x++ {
				if c := img.NRGBAAt(x, y); c.A > 200 && c.R > 240 && c.G > 240 && c.B > 240 {
					n++
				}
			}
		}
		return n
	}
	if cuenta(bien) < 100 {
		t.Errorf("con `normaliza` solo hay %d píxeles de glifo: el sobre no se está pintando", cuenta(bien))
	}
	if cuenta(mal) > 0 {
		t.Errorf("sin la regla de `scale`, el glifo sale igual (%d px): `oksvg` ya entiende `scale(k)`"+
			" y `escalaDeUno` sobra", cuenta(mal))
	}
}

// TestElDibujoSeCacheaPorTamanoYColor: el generador se llama en cada repintado, y
// reinterpretar el SVG cuesta entre 0,15 y 0,7 ms. Con doscientas filas eso se nota.
func TestElDibujoSeCacheaPorTamanoYColor(t *testing.T) {
	d := Trazo(Buscar, 16, "text", tema.Claro)
	a := d.generar(32, 32)
	if b := d.generar(32, 32); a != b {
		t.Error("dos llamadas iguales han devuelto dos mapas de bits: no se está cacheando")
	}
	if b := d.generar(48, 48); a == b {
		t.Error("a otro tamaño ha devuelto el mismo mapa de bits: la caché ignora el tamaño")
	}
	d.Tenir("danger", tema.Claro)
	if b := d.generar(32, 32); a == b {
		t.Error("tras teñir ha devuelto el mismo mapa de bits: la caché ignora el color")
	}
}

// TestLasFigurasConAgujerosLosConservan: el edificio del ERP es un `path` con
// `fill-rule="evenodd"` cuyas ventanas son agujeros. `oksvg` no lee esa regla y lo
// rellenaba ENTERO: once de las diecisiete marcas salían con el glifo macizo. Dentro
// de la primera ventana tiene que verse la tesela, no el blanco del edificio; y sin la
// corrección se ve el blanco, que es el rojo que trae el guardián.
func TestLasFigurasConAgujerosLosConservan(t *testing.T) {
	erp, err := marcasFS.ReadFile("marcas/erp.svg")
	if err != nil {
		t.Fatal(err)
	}
	blanco := func(c color.NRGBA) bool { return c.R > 240 && c.G > 240 && c.B > 240 }
	// La primera ventana va de (12, 15) a (17, 19) en el viewBox de 40; a 80 px, su
	// centro cae en (29, 32).
	if c := rasteriza(erp, 80, 80).NRGBAAt(29, 32); blanco(c) {
		t.Errorf("la ventana del ERP sale blanca (%v): el agujero se ha rellenado", c)
	}
	// Sin `pintarParImpar` (pero con `normaliza`, que sin ella el glifo ni se pinta) la
	// ventana sale blanca: es el rojo que trae el guardián.
	if c := rasterizaCon(normaliza(erp), 80, 80, nil).NRGBAAt(29, 32); !blanco(c) {
		t.Errorf("sin la corrección la ventana ya sale hueca (%v): `rasterx` pinta par-impar y `pintarParImpar` sobra", c)
	}
	// El edificio sigue ahí: entre dos ventanas, blanco.
	if c := rasteriza(erp, 80, 80).NRGBAAt(40, 32); !blanco(c) {
		t.Errorf("entre las ventanas no queda edificio (%v)", c)
	}
	if got := parImpar([]byte(`<svg><rect/><g fill-rule="evenodd"><path/><path fill-rule="nonzero"/><path fill="none"/></g><defs><path/></defs><path style="fill-rule: evenodd"/></svg>`)); fmt.Sprint(got) != "[1 4]" {
		t.Errorf("parImpar = %v y son la 1 (heredada del g) y la 4 (por style); la 3 no rellena", got)
	}
}

// TestCadaArcoLlevaSuLetra: el círculo de dos medios arcos tras una sola `a` es el de
// la mitad de la suite —la cabeza de Contacts, el aro de ID, la lupa, el círculo de
// información— y `oksvg` pintaba el segundo medio con los datos del primero. Con la
// letra repetida sale el círculo entero: el punto de su borde izquierdo está pintado.
// Sin ella (`rasterizaCon` con el SVG crudo menos el `scale`), no: es el rojo que trae.
func TestCadaArcoLlevaSuLetra(t *testing.T) {
	// Tres arcos con su letra, las banderas pegadas separadas y el resto intacto (los
	// radios llevan la holgura, que no se compara al decimal).
	got := arcosSueltos("M20 9.2a5 5 0 1 1 0 10.8 5 5 0 0 1 0-10.8Zm1 2a2 2 0 012 2")
	if strings.Count(got, "a") != 3 || !strings.Contains(got, " 0 1 1 0 10.8a") ||
		!strings.HasSuffix(got, " 0 0 1 2 2") || !strings.HasPrefix(got, "M20 9.2a") || !strings.Contains(got, "Zm1 2a") {
		t.Errorf("arcosSueltos = %q", got)
	}
	contacts, err := marcasFS.ReadFile("marcas/contacts.svg")
	if err != nil {
		t.Fatal(err)
	}
	blanco := func(c color.NRGBA) bool { return c.R > 240 && c.G > 240 && c.B > 240 }
	// La cabeza es un círculo de radio 5,4 centrado en (20, 14,6) del viewBox de 40; a
	// 80 px, su borde izquierdo está en x ≈ 29 y su centro en y ≈ 29.
	if c := rasteriza(contacts, 80, 80).NRGBAAt(31, 29); !blanco(c) {
		t.Errorf("el lado izquierdo de la cabeza de Contacts no está pintado (%v)", c)
	}
	crudo := escalaDeUno.ReplaceAll(contacts, []byte("scale($1 $1)"))
	if c := rasterizaCon(crudo, 80, 80, nil).NRGBAAt(31, 29); blanco(c) {
		t.Errorf("sin la corrección la cabeza ya sale entera (%v): `oksvg` repite bien los arcos y `arcosSueltos` sobra", c)
	}
	// El aro de ID es un semicírculo exacto con el final relativo: sin la holgura de los
	// radios no pinta nada y del aro queda solo el agujero.
	id, err := marcasFS.ReadFile("marcas/id.svg")
	if err != nil {
		t.Fatal(err)
	}
	// El aro (radio 6,4 en (15,2, 25) del viewBox, agujero de 2,6) pasa, en su lado
	// izquierdo, por x ≈ 10,5; a 80 px y con su `scale(0.9692)`, por (22, 47).
	if c := rasteriza(id, 80, 80).NRGBAAt(22, 47); !blanco(c) {
		t.Errorf("el aro de la llave de ID no está pintado (%v)", c)
	}
}

// TestUnaMarcaApagadaSaleEnGrisYAl40 es el `opacity-40 grayscale` de la tesela de un
// producto que la cuenta no puede usar (AppGrid.tsx, el lanzador del Hub). VISTO EN
// ROJO sin `apagar`: la tesela del ERP sigue en su ámbar y opaca.
func TestUnaMarcaApagadaSaleEnGrisYAl40(t *testing.T) {
	d := Marca(ERP, MarcaLG, tema.Claro)
	viva := d.generar(40, 40).(*image.NRGBA).NRGBAAt(8, 8)
	d.Apagar(true)
	gris := d.generar(40, 40).(*image.NRGBA).NRGBAAt(8, 8)
	quiero := uint8(0.2126*float64(viva.R) + 0.7152*float64(viva.G) + 0.0722*float64(viva.B) + 0.5)
	if gris.R != quiero || gris.G != quiero || gris.B != quiero {
		t.Errorf("apagada, la tesela (%v) es %v y en gris tendría que ser %d", viva, gris, quiero)
	}
	if gris.A != 102 {
		t.Errorf("apagada, el alfa es %d y el 40 %% de 255 son 102", gris.A)
	}
	d.Apagar(false)
	if otra := d.generar(40, 40).(*image.NRGBA).NRGBAAt(8, 8); otra != viva {
		t.Errorf("encendida otra vez sale %v y era %v", otra, viva)
	}
}

// TestDeSVGPintaElSVGEnteroASuTamano: un rectángulo de 100×50 del viewBox con trazo de 2
// pintado a 50×25 CSS y ×2 ocupa los 100×50 píxeles de dispositivo, con el relleno
// dentro y un trazo de dos píxeles (el grosor se escala con el viewBox, como en los
// iconos). VISTO EN ROJO (2026-09-23, en el frontal): sin el `SetMinSize`, el mínimo
// es 1×1 y una fila no le deja sitio.
func TestDeSVGPintaElSVGEnteroASuTamano(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 50" width="100" height="50">` +
		`<rect x="1" y="1" width="98" height="48" fill="#dc2626" stroke="#0a0a0a" stroke-width="2"/></svg>`)
	d := DeSVG(svg, fyne.NewSize(50, 25))
	if d.Objeto().MinSize() != fyne.NewSize(50, 25) {
		t.Fatalf("el mínimo es %v y tenía que ser el tamaño pedido", d.Objeto().MinSize())
	}
	fondo := fcanvas.NewRectangle(color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	fondo.Resize(fyne.NewSize(50, 25))
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetContent(container.NewWithoutLayout(fondo, d.Objeto()))
	c.SetScale(escala)
	c.Resize(fyne.NewSize(50, 25))
	img := c.Capture()
	if p := px(img, 50, 25); !cerca(p, color.NRGBA{R: 0xdc, G: 0x26, B: 0x26, A: 255}) {
		t.Errorf("el centro pinta %v y es el relleno rojo", p)
	}
	if p := px(img, 50, 0); !cerca(p, color.NRGBA{R: 0x0a, G: 0x0a, B: 0x0a, A: 255}) {
		t.Errorf("la fila de arriba pinta %v y es el trazo", p)
	}
	if p := px(img, 99, 25); !cerca(p, color.NRGBA{R: 0x0a, G: 0x0a, B: 0x0a, A: 255}) {
		t.Errorf("la última columna pinta %v: el SVG no llega al tamaño pedido", p)
	}
}

// TestGirarEsElRotateDeCSS: `rotate()` gira en el sentido del reloj alrededor del
// centro. A 90° la rotación bilineal es exacta: el píxel de arriba a la izquierda pasa
// arriba a la derecha y no se reparte. Y a 0° el dibujo sale el mismo mapa de siempre.
func TestGirarEsElRotateDeCSS(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	img.SetNRGBA(2, 0, color.NRGBA{R: 255, A: 255})
	g := rotar(img, 90)
	if got := g.NRGBAAt(7, 2); got != (color.NRGBA{R: 255, A: 255}) {
		t.Errorf("a 90° el píxel (2,0) tiene que ir a (7,2) y ahí hay %v", got)
	}
	n := 0
	for i := 3; i < len(g.Pix); i += 4 {
		if g.Pix[i] > 0 {
			n++
		}
	}
	if n != 1 {
		t.Errorf("a 90° hay %d píxeles con tinta y tiene que haber uno", n)
	}
	d := TrazoConTinta(Rotar, 16, color.NRGBA{A: 255})
	derecho := d.generar(32, 32)
	d.Girar(90)
	if d.generar(32, 32) == derecho {
		t.Error("girado, el dibujo devuelve el mismo mapa que derecho")
	}
	d.Girar(0)
	if d.generar(32, 32) != derecho {
		t.Error("a 0° el dibujo tiene que volver a su mapa de siempre, sin rotarlo")
	}
}

// TestRasterizarSVGPintaUnSVGAjeno: un SVG que no es un icono (un rectángulo y un trazo
// de 2 en un viewBox de 40×20 pintado a 80×40) sale con el trazo ESCALADO, 4 píxeles, y
// con el relleno donde toca. Es el guardián de la entrada pública.
func TestRasterizarSVGPintaUnSVGAjeno(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 20" width="40" height="20">` +
		`<rect x="0" y="0" width="10" height="20" fill="#2563EB"/>` +
		`<path d="M20 10L40 10" fill="none" stroke="#DC2626" stroke-width="2"/></svg>`)
	img := RasterizarSVG(svg, 80, 40)
	if got := img.NRGBAAt(5, 20); got != (color.NRGBA{R: 0x25, G: 0x63, B: 0xEB, A: 0xff}) {
		t.Errorf("el rectángulo pinta %v", got)
	}
	grueso := 0
	for y := range 40 {
		if c := img.NRGBAAt(60, y); c.R > 0xC0 && c.A > 0xC0 && c.B < 0x60 {
			grueso++
		}
	}
	if grueso != 4 {
		t.Errorf("el trazo de 2 unidades a escala ×2 ocupa %d filas y deberían ser 4", grueso)
	}
}
