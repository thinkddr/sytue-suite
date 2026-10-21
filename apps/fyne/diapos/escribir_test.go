// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// enfocado es lo que tiene el teclado en la ventana: al escribir, la página del folio de
// la caja.
func enfocado(t *testing.T, w fyne.Window) fyne.Focusable {
	t.Helper()
	f := w.Canvas().Focused()
	if f == nil {
		t.Fatal("nada tiene el foco")
	}
	return f
}

// TestSeEscribeEnLaCaja: doble toque sobre una caja de texto la abre con el cursor al
// final; lo escrito va a ESE sitio y la negrita que ya había sobrevive; Escape termina,
// guarda y devuelve el teclado al escenario.
//
// VISTO EN ROJO (2026-09-24, en el frontal): sin `AlFinal` el cursor se quedaba al
// principio y el HTML salía «<h1>&nbsp;y másTítulo</h1>…».
func TestSeEscribeEnLaCaja(t *testing.T) {
	e, ctx, w := montar(t, `{"slides":[{"id":"s","boxes":[`+
		`{"id":"t","x":10,"y":10,"w":80,"h":40,"html":"<h1>Título</h1><p>Uno <b>fuerte</b></p>"}]}]}`)
	marcoDe(t, e, "t").DoubleTapped(nil)
	if e.Escribiendo() != "t" || e.escenario.Oculta != "t" || e.Folio() == nil {
		t.Fatal("el doble toque no abre la caja para escribir")
	}
	test.Type(enfocado(t, w), " y más")
	enfocado(t, w).TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if e.Escribiendo() != "" {
		t.Fatal("Escape termina")
	}
	if h := e.m.caja(0, "t").HTML; h != "<h1>Título</h1><p>Uno <b>fuerte y más</b></p>" {
		t.Errorf("HTML = %q", h)
	}
	if !strings.Contains(e.Serializar(), "fuerte y más") || ctx.Guardado != editor.Sucio {
		t.Error("lo escrito se guarda: el documento queda sin guardar y el modelo lo lleva")
	}
	if w.Canvas().Focused() != e.capa {
		t.Error("Escape devuelve el teclado al escenario")
	}
}

// TestLaCintaFormateaLaCaja: con la caja abierta, «Fuente» y «Párrafo» se encienden y
// actúan sobre lo elegido, escribiendo el HTML que escribe el `exec` de la web —etiqueta
// para las marcas, `<span style>` para el color y el tamaño, `style` en el bloque para
// la alineación—, y la cinta enseña lo que hay bajo el cursor.
//
// Sin caja abierta, «Fuente» y «Párrafo» se ven encendidos (así los pinta la web) y
// pulsarlos no toca nada.
//
// VISTO EN ROJO (2026-09-24, en el frontal): con el `Mandar` de antes (que no hacía
// nada) el HTML volvía intacto, la negrita no salía pulsada y la lista no se hacía.
func TestLaCintaFormateaLaCaja(t *testing.T) {
	e, ctx, w := montar(t, `{"slides":[{"id":"s","boxes":[`+
		`{"id":"t","x":10,"y":10,"w":80,"h":40,"html":"<p>hola mundo</p>"}]}]}`)
	negrita := func() *editor.BotonCinta {
		for _, b := range buscar[*editor.BotonCinta](w.Canvas().Content()) {
			if b.Icono == iconos.EdNegrita {
				return b
			}
		}
		t.Fatal("no está el botón de negrita")
		return nil
	}
	// Como en la web: sin caja abierta el grupo se ve encendido y pulsar no hace nada.
	if negrita().Inerte {
		t.Error("sin caja abierta, «Fuente» se ve encendido, como en la web")
	}
	test.Tap(negrita())
	if h := e.m.caja(0, "t").HTML; h != "<p>hola mundo</p>" {
		t.Errorf("sin caja abierta, la negrita ha tocado el HTML: %q", h)
	}
	e.EmpezarEscritura("t")
	ctx.Refrescar()
	if negrita().Inerte {
		t.Fatal("con la caja abierta, «Fuente» se enciende")
	}
	enfocado(t, w).(fyne.Shortcutable).TypedShortcut(&fyne.ShortcutSelectAll{})
	test.Tap(negrita())
	ctx.Mandar(editor.ColorTexto, "#DC2626")
	ctx.Mandar(editor.Titulo, "h2")
	ctx.Mandar(editor.AlinearCen, "")
	if !e.Formato().Negrita || e.Formato().Titulo != "h2" || e.Formato().Alineacion != editor.Centro {
		t.Errorf("la cinta enseñaría %+v", e.Formato())
	}
	if !negrita().Activo {
		t.Error("la negrita sale pulsada")
	}
	e.TerminarEscritura()
	h := e.m.caja(0, "t").HTML
	if h != `<h2 style="text-align: center;"><b><span style="color: rgb(220, 38, 38);">hola mundo</span></b></h2>` {
		t.Errorf("HTML = %q", h)
	}
	// Y las listas: un párrafo pasa a elemento de lista, que la web pinta sangrado.
	e.EmpezarEscritura("t")
	ctx.Mandar(editor.ListaPuntos, "")
	e.TerminarEscritura()
	if h := e.m.caja(0, "t").HTML; !strings.HasPrefix(h, "<ul><li") {
		t.Errorf("la lista deja %q", h)
	}
}

// TestLosBloquesDeTextoSonLosDeLaHoja fija los tamaños, pesos y márgenes de `BOX_CSS`
// sobre el modelo de texto enriquecido: un titular sin margen, un párrafo con su 0,3 em,
// la lista con el suyo al final, el `text-align` y el `line-height` que deja la cinta, y
// ni viñeta ni número (el reset de Tailwind).
//
// VISTO EN ROJO (2026-09-24, en el frontal): con la hoja del folio en vez de la de la
// caja, el h1 salía a 32 px con 11,2 de margen, el cuerpo a 16 y la lista con su «•».
func TestLosBloquesDeTextoSonLosDeLaHoja(t *testing.T) {
	bs := rico.ParseCon(`<h1 style="text-align: center;">T&iacute;tulo</h1><p>uno<p style="line-height: 1.5">dos`+
		`<ul><li>a</li><li>b</li></ul><h3>tres</h3>`, hojaCaja(tinta, "", "")).Bloques()
	m := tamCuerpo * margenBloque
	quiero := []struct {
		texto        string
		tam          float32
		peso         int
		abajo        float32
		al           rico.Alineacion
		interlineado float32
	}{
		{"Título", 44, 700, 0, rico.Centro, interlineado},
		{"uno", 20, 400, m, rico.Izquierda, interlineado},
		{"dos", 20, 400, m, rico.Izquierda, 1.5},
		{"a", 20, 400, 0, rico.Izquierda, interlineado},
		{"b", 20, 400, m, rico.Izquierda, interlineado},
		{"tres", 24, 600, 0, rico.Izquierda, interlineado},
	}
	if len(bs) != len(quiero) {
		t.Fatalf("salen %d bloques y son %d", len(bs), len(quiero))
	}
	for i, q := range quiero {
		b := bs[i]
		if b.Texto() != q.texto || b.Base.Tamano != q.tam || b.Base.Peso != q.peso || b.Abajo != q.abajo || b.Arriba != 0 ||
			b.Base.Alineacion != q.al || b.Base.Interlineado != q.interlineado || b.Marca != "" || b.Base.Color != tinta {
			t.Errorf("bloque %d: %q a %v/%v, margen %v, alineación %v, interlineado %v, marca %q\n  quiero %+v",
				i, b.Texto(), b.Base.Tamano, b.Base.Peso, b.Abajo, b.Base.Alineacion, b.Base.Interlineado, b.Marca, q)
		}
	}
	if bs[3].Sangria != 1 {
		t.Errorf("un elemento de lista va sangrado un nivel: %d", bs[3].Sangria)
	}
}

// pintaCaja pinta una diapositiva de una sola caja de texto a 960 × 540 (escala 1).
func pintaCaja(t *testing.T, html string, y, h float64) image.Image {
	t.Helper()
	html8, _ := json.Marshal(html)
	m, err := Leer([]byte(fmt.Sprintf(`{"slides":[{"id":"s","boxes":[{"id":"b","x":10,"y":%v,"w":80,"h":%v,"html":%s}]}]}`,
		y, h, html8)))
	if err != nil {
		t.Fatal(err)
	}
	l := NuevoLienzo(m, 0)
	l.Resize(fyne.NewSize(AnchoEscenario, 540))
	return pinta(t, AnchoEscenario, 540, l)
}

// TestElFormatoEnLineaSePinta: el color, el resaltado y la negrita de un TROZO salen en
// la diapositiva, cada uno en su sitio, y no el párrafo entero con el peso de su etiqueta.
//
// VISTO EN ROJO (2026-09-24, en el frontal): con el lector de antes (`htmltext.Text`
// por bloque) no había ni un píxel rojo ni amarillo.
func TestElFormatoEnLineaSePinta(t *testing.T) {
	rojo := color.NRGBA{R: 0xDC, G: 0x26, B: 0x26, A: 255}
	amarillo := color.NRGBA{R: 0xFE, G: 0xF0, B: 0x8A, A: 255}
	img := pintaCaja(t, `<p>uno <span style="color: rgb(220, 38, 38);">rojo</span> `+
		`<span style="background-color: rgb(254, 240, 138);">resalte</span> <b>fuerte</b></p>`, 10, 30)
	guarda(t, "formato-en-linea.png", img)
	rx0, rx1, ry0, ry1 := caja(img, rojo)
	ax0, ax1, ay0, ay1 := caja(img, amarillo)
	if rx1 < 0 || ax1 < 0 {
		t.Fatalf("ni rojo (%d) ni resalte (%d) en la diapositiva", rx1, ax1)
	}
	// El rojo va detrás de «uno » y el resalte detrás del rojo, en la misma línea.
	if float32(rx0) <= (10*9.6+sangriaCaja)*escala || ax0 <= rx1 {
		t.Errorf("el rojo empieza en %d y el resalte en %d: no van en su orden", rx0, ax0)
	}
	// El resalte cubre la caja de la LETRA (1,2 em = 24 px) y no la línea (25).
	if alto := float32(ay1-ay0+1) / escala; alto < 23 || alto > 24.5 {
		t.Errorf("el resalte mide %v px de alto y la letra de 20 pide 24", alto)
	}
	if ry0 < ay0 || ry1 > ay1 {
		t.Errorf("el texto rojo (%d–%d) se sale de la banda de su línea (%d–%d)", ry0, ry1, ay0, ay1)
	}
}

// TestLoQueNoCabeSeRecorta es el `overflow-hidden` de TEXT_BOX_CLASS: debajo de la caja
// no hay tinta, y la línea que no cabe entera se corta a mitad (antes desaparecía si no
// cabía la mitad de su caja).
//
// VISTO EN ROJO (2026-09-24, en el frontal): sin `recortada`, las líneas de más seguían
// pintándose por debajo de la caja (tinta hasta la fila 454; la caja acaba en la 345,6).
func TestLoQueNoCabeSeRecorta(t *testing.T) {
	// Una caja de 64,8 px de alto (12 %) con cinco líneas de 25: caben dos y 14,8 px de
	// la tercera, que asoma cortada por debajo de sus letras.
	img := pintaCaja(t, `<p>uno<br>dos<br>tres<br>cuatro<br>cinco</p>`, 20, 12)
	guarda(t, "recorte.png", img)
	arriba := float32(20) / 100 * 540
	fondo := (arriba + float32(12)/100*540) * escala
	_, _, _, y1 := caja(img, tinta)
	if y1 < 0 {
		t.Fatal("no se ha pintado nada")
	}
	if float32(y1) >= fondo {
		t.Errorf("hay tinta en la fila %d y la caja acaba en la %v", y1, fondo)
	}
	// Y la tercera línea asoma: hay tinta por debajo del final de la segunda.
	if finSegunda := (arriba + 2*25) * escala; float32(y1) <= finSegunda {
		t.Errorf("la tercera línea no asoma: la tinta acaba en %d y la segunda en %v", y1, finSegunda)
	}
}

// TestEscribirSePintaIgualQueLeer: el folio de la caja que se escribe pinta los MISMOS
// píxeles que el lienzo cuando no se escribe —misma hoja, misma maqueta, mismo sitio—.
// Si no, el texto saltaría al hacer doble clic y al pulsar Escape.
//
// VISTO EN ROJO (2026-09-24, en el frontal): con el relleno de la caja sin escalar en
// el folio (`Relleno` sin ×k), 1406 píxeles cambiaban al empezar a escribir.
func TestEscribirSePintaIgualQueLeer(t *testing.T) {
	e, _, w := montar(t, `{"slides":[{"id":"s","boxes":[`+
		`{"id":"t","x":10,"y":10,"w":80,"h":60,"html":"<h1>Título</h1><p>Uno <b>fuerte</b> <i>y cursiva</i></p>"}]}]}`)
	recorte := func() image.Image {
		img := w.Canvas().Capture()
		m := marcoDe(t, e, "t")
		abs := fyne.CurrentApp().Driver().AbsolutePositionForObject(m)
		// Por dentro del anillo de foco de la caja en edición, que el lienzo no lleva.
		r := image.Rect(int(abs.X)+4, int(abs.Y)+4, int(abs.X+m.Size().Width)-4, int(abs.Y+m.Size().Height)-4)
		return img.(interface {
			SubImage(image.Rectangle) image.Image
		}).SubImage(r)
	}
	antes := recorte()
	e.EmpezarEscritura("t")
	e.capa.enfocar() // sin el cursor, que el lienzo no pinta
	durante := recorte()
	guarda(t, "leer.png", antes)
	guarda(t, "escribir.png", durante)
	b := antes.Bounds()
	distintos := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if !cerca(px(antes, x, y), px(durante, x, y)) {
				distintos++
			}
		}
	}
	if distintos > 0 {
		t.Errorf("%d píxeles cambian al empezar a escribir", distintos)
	}
}

// TestSeInsertaEnElCursorDeLaCaja: con la caja abierta, «Insertar» escribe en el cursor
// lo mismo que el `exec` de la web —la tabla de `tableHtml`, el `<hr>`, el salto, el
// enlace y el símbolo—, y en la diapositiva la regla es de la tinta de la caja y la tabla
// va sin rayas, que es lo que pinta el reset de Tailwind fuera de `.sytue-page`.
//
// VISTO EN ROJO (2026-09-26, en el frontal): con la regla del folio (`TintaLinea`) no
// había ni un píxel de la tinta de la diapositiva en la fila de la regla.
func TestSeInsertaEnElCursorDeLaCaja(t *testing.T) {
	e, ctx, _ := montar(t, `{"slides":[{"id":"s","boxes":[`+
		`{"id":"t","x":10,"y":10,"w":80,"h":60,"html":"<p>uno</p>"}]}]}`)
	e.EmpezarEscritura("t")
	ctx.Mandar(editor.InsertarTexto, "→")
	ctx.Mandar(editor.InsertarEnlace, "https://sytue.eu\nSytue")
	ctx.Mandar(editor.InsertarRegla, "")
	ctx.Mandar(editor.SaltoPagina, "")
	ctx.Mandar(editor.InsertarTabla, "2x2")
	e.TerminarEscritura()
	h := e.m.caja(0, "t").HTML
	for _, quiero := range []string{
		"uno→", `<a href="https://sytue.eu">Sytue</a>`, "<hr>", "data-page-break",
		"<table><tbody><tr><td><br></td><td><br></td></tr>",
	} {
		if !strings.Contains(h, quiero) {
			t.Errorf("falta %q en %q", quiero, h)
		}
	}
	// La regla, pintada: 1 px de la tinta de la caja, de lado a lado de la caja.
	img := pintaCaja(t, `<p>arriba</p><hr><p>abajo</p>`, 10, 30)
	guarda(t, "regla.png", img)
	fila := int((float32(10)/100*540 + 25 + 6) * escala) // bajo «arriba» (25 + 0,3 em)
	n := 0
	for y := fila - 2; y <= fila+2; y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			if cerca(px(img, x, y), tinta) {
				n++
			}
		}
	}
	if pide := 1200; n < pide { // el 80 % de los ~1520 píxeles de ancho de la caja a ×2
		t.Errorf("en la fila de la regla hay %d píxeles de la tinta y la regla pide %d", n, pide)
	}
}

// TestLaLetraDeArenaEsLaDelEquipo: en el tema «arena» las cajas van en Georgia (el
// `--font-sans` que pone la web en el escenario), con la que tenga el equipo —la misma
// que pide la hoja de cálculo—; y sin tema, en Figtree.
//
// VISTO EN ROJO (2026-09-26, en el frontal): sin la familia en `hojaCaja`, la caja de
// «arena» salía en Figtree.
func TestLaLetraDeArenaEsLaDelEquipo(t *testing.T) {
	georgia := tema.CaraDelSistema("Georgia", false, false)
	if georgia == nil {
		t.Fatal("el frontal tiene fontconfig: algo tiene que dar por Georgia")
	}
	letra := func(tm string) fyne.Resource {
		m, err := Leer([]byte(`{"theme":"` + tm + `","slides":[{"id":"s","boxes":[{"id":"b","x":0,"y":0,"w":50,"h":20,"html":"<p>hola</p>"}]}]}`))
		if err != nil {
			t.Fatal(err)
		}
		l := NuevoLienzo(m, 0)
		l.ConPasos = true
		l.Resize(fyne.NewSize(960, 540))
		_ = test.WidgetRenderer(l)
		_, tx, _ := textoDe(t, l.r.porCaja["b"])
		return tx.FontSource
	}
	if got := letra("arena"); got != georgia {
		t.Errorf("«arena» pinta con %v y quiero la Georgia del equipo (%v)", got.Name(), georgia.Name())
	}
	if got := letra(""); got != tema.Regular {
		t.Errorf("sin tema pinta con %v y quiero Figtree", got.Name())
	}
}
