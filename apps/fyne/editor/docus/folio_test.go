// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"bytes"
	"encoding/base64"
	"image"
	imgpng "image/png"
	"os"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// enVentanaReal lo asigna ventanareal_test.go (bajo -tags ventanareal) para correr el
// recorrido de ventana real en el hilo principal antes de m.Run().
var enVentanaReal func(dir string) error

// TestMain arranca la aplicación de pruebas con NUESTRO tema: sin ella, medir texto
// resolvería la letra de serie y el folio no podría ni maquetarse.
func TestMain(m *testing.M) {
	if dir := os.Getenv("SYTUE_VENTANA_REAL"); dir != "" && enVentanaReal != nil {
		if err := enVentanaReal(dir); err != nil {
			os.Stderr.WriteString("VENTANA REAL: FALLO: " + err.Error() + "\n")
			os.Exit(1)
		}
		os.Stdout.WriteString("VENTANA REAL: bien\n")
		os.Exit(0)
	}
	a := test.NewApp()
	a.Settings().SetTheme(tema.Sytue{})
	tema.FormaDeLosGuardianes()
	código := m.Run()
	a.Quit()
	os.Exit(código)
}

// muestraReal es el documento de demostración del repositorio: el mismo que siembra
// `test/e2e/sembrar-demo.mjs` y el mismo que se midió en producción. Se LEE del fichero
// en vez de copiarlo: un fixture copiado deja de ser el documento real en cuanto alguien
// toca el original.
func muestraReal(t *testing.T) string {
	t.Helper()
	fuente := webref.Read(t, "test/e2e/sembrar-demo.mjs")
	i := strings.Index(fuente, `title: "Propuesta de servicio — Nordika",`)
	if i < 0 {
		t.Fatal("sembrar-demo.mjs ya no siembra «Propuesta de servicio — Nordika»: busca otro documento real")
	}
	j := strings.Index(fuente[i:], "\n  },")
	if j < 0 {
		t.Fatal("no se encuentra el final del documento de muestra en sembrar-demo.mjs")
	}
	var s strings.Builder
	for _, l := range strings.Split(fuente[i:i+j], "\n") {
		l = strings.TrimSpace(l)
		if !strings.HasPrefix(l, `"<`) {
			continue
		}
		l = strings.TrimSuffix(strings.TrimSuffix(l, ","), " +")
		s.WriteString(strings.Trim(l, `"`))
	}
	if !strings.HasPrefix(s.String(), "<h1>Propuesta de servicio") {
		t.Fatalf("el documento de muestra no se ha leído entero: %q", s.String())
	}
	return s.String()
}

// folioCon monta un folio editable con ese HTML, sin ventana y sin reloj.
func folioCon(t *testing.T, html string) *Folio {
	t.Helper()
	ctx := &editor.Contexto{Tipo: editor.Documento, Zoom: 100, Regla: true}
	f := NuevoFolio(ctx)
	ctx.Lienzo = f
	f.Programar = func(time.Duration, func()) {}
	if !f.Cargar(html) {
		t.Fatalf("el folio no entiende %q", html)
	}
	return f
}

// TestElDocumentoRealSeLeeEntero: la demo se entiende entera, sale SIN TOCAR byte a byte
// y cuenta las mismas palabras que la barra de estado de producción (56: cambiar cada
// etiqueta por un espacio deja la coma de «</strong>,» como palabra).
func TestElDocumentoRealSeLeeEntero(t *testing.T) {
	muestra := muestraReal(t)
	f := folioCon(t, muestra)
	if got := f.Serializar(); got != muestra {
		t.Errorf("sin tocar, el documento no vuelve igual:\n  entra: %s\n  sale:  %s", muestra, got)
	}
	if _, _, palabras := f.Estadisticas(); palabras != 56 {
		t.Errorf("palabras = %d y la barra de estado de producción dice 56", palabras)
	}
	if len(f.cajas) != 8 {
		t.Errorf("la demo tiene 8 bloques y se han maquetado %d", len(f.cajas))
	}
	// Y lo entendido es el documento, no una versión aguada: el <strong> del último
	// párrafo llega a la maqueta como negrita.
	negrita := false
	for _, c := range f.cajas {
		for _, tr := range c.b.Tramos {
			negrita = negrita || tr.Texto == "dos fines de semana" && tr.Estilo.Negrita()
		}
	}
	if !negrita {
		t.Error("«dos fines de semana» tiene que salir en negrita: su <strong> se ha perdido")
	}
}

// TestEscribirYDeshacer: lo tecleado entra donde está el cursor con el formato de al
// lado, Intro parte el bloque, Retroceso lo vuelve a unir y deshacer devuelve el
// documento EXACTO de antes.
func TestEscribirYDeshacer(t *testing.T) {
	const antes = "<p>Hola <b>mundo</b></p><p>dos</p>"
	f := folioCon(t, antes)
	f.mover(punto{0, 10}, false) // tras «mundo», dentro de la negrita
	for _, r := range "!!" {
		f.escribir(string(r))
	}
	if got := f.Serializar(); got != "<p>Hola <b>mundo!!</b></p><p>dos</p>" {
		t.Errorf("escribir: %s", got)
	}
	f.mover(punto{0, 4}, false)
	f.intro()
	if got := f.Serializar(); got != "<p>Hola</p><p>&nbsp;<b>mundo!!</b></p><p>dos</p>" || f.cur != (punto{1, 0}) {
		t.Errorf("Intro: %s, cursor %v", got, f.cur)
	}
	f.retroceso()
	if got := f.Serializar(); got != "<p>Hola&nbsp;<b>mundo!!</b></p><p>dos</p>" || f.cur != (punto{0, 4}) {
		t.Errorf("Retroceso al principio: %s, cursor %v", got, f.cur)
	}
	for len(f.hist.atras) > 0 {
		f.deshacer()
	}
	if got := f.Serializar(); got != antes {
		t.Errorf("deshacer todo no devuelve el documento de antes: %s", got)
	}
	f.rehacer()
	if got := f.Serializar(); got != "<p>Hola <b>mundo!!</b></p><p>dos</p>" {
		t.Errorf("rehacer: %s", got)
	}
}

// TestLaSeleccionCruzaBloques: una selección de dos bloques se borra uniéndolos, y la
// negrita se pone a los dos trozos a la vez.
func TestLaSeleccionCruzaBloques(t *testing.T) {
	f := folioCon(t, "<p>uno</p><h2>dos</h2><p>tres</p>")
	f.anc, f.cur = punto{0, 1}, punto{2, 2}
	f.Mandar(editor.Negrita, "")
	// El título ya es negrita (peso 600, como cuenta `queryCommandState`): no se envuelve.
	if got := f.Serializar(); got != "<p>u<b>no</b></p><h2>dos</h2><p><b>tr</b>es</p>" {
		t.Errorf("negrita en tres bloques: %s", got)
	}
	f.deshacer()
	f.anc, f.cur = punto{0, 1}, punto{2, 2}
	f.escribir("X")
	if got := f.Serializar(); got != "<p>uXes</p>" || f.cur != (punto{0, 2}) {
		t.Errorf("escribir sobre la selección: %s, cursor %v", got, f.cur)
	}
	f.TypedShortcutTodo()
	f.retroceso()
	if got := f.Serializar(); got != "<p><br></p>" {
		t.Errorf("borrar todo deja el párrafo vacío de la web: %s", got)
	}
}

// TypedShortcutTodo es Ctrl/Cmd+A, sin ventana.
func (f *Folio) TypedShortcutTodo() { f.pag.TypedShortcut(&fyne.ShortcutSelectAll{}) }

// TestLaCintaCambiaElDocumento: los comandos de la cinta escriben el HTML que escribe
// la web.
func TestLaCintaCambiaElDocumento(t *testing.T) {
	for _, c := range []struct {
		cmd    editor.Comando
		valor  string
		quiero string
	}{
		{editor.Cursiva, "", "<p><i>hola</i></p>"},
		{editor.Subrayado, "", "<p><u>hola</u></p>"},
		{editor.Tachado, "", "<p><strike>hola</strike></p>"},
		{editor.Superindice, "", "<p><sup>hola</sup></p>"},
		{editor.AlinearCen, "", `<p style="text-align: center;">hola</p>`},
		{editor.Justificar, "", `<p style="text-align: justify;">hola</p>`},
		{editor.Titulo, "h2", "<h2>hola</h2>"},
		{editor.Titulo, "blockquote", "<blockquote>hola</blockquote>"},
		{editor.ListaPuntos, "", "<ul><li>hola</li></ul>"},
		{editor.ListaNumero, "", "<ol><li>hola</li></ol>"},
		{editor.Interlineado, "2", `<p style="line-height: 2;">hola</p>`},
		{editor.MasSangria, "", `<p style="margin-left: 40px;">hola</p>`},
		{editor.ColorTexto, "#c0392b", `<p><span style="color: rgb(192, 57, 43);">hola</span></p>`},
		{editor.Resaltado, "#fde68a", `<p><span style="background-color: rgb(253, 230, 138);">hola</span></p>`},
		{editor.Fuente, "Times New Roman", `<p><span style="font-family: &quot;Times New Roman&quot;;">hola</span></p>`},
		{editor.Tamano, "5", `<p><span style="font-size: x-large;">hola</span></p>`},
	} {
		f := folioCon(t, "<p>hola</p>")
		f.anc, f.cur = punto{0, 0}, punto{0, 4}
		f.Mandar(c.cmd, c.valor)
		if got := f.Serializar(); got != c.quiero {
			t.Errorf("%s %q:\n  hay     %s\n  quería  %s", c.cmd, c.valor, got, c.quiero)
		}
		if !f.doc.Guardable() {
			t.Errorf("%s: lo que queda no se puede guardar", c.cmd)
		}
	}
	// Y la cinta lo refleja: el formato bajo el cursor (un título ES negrita: peso 600).
	f := folioCon(t, `<h2 style="text-align: right;"><b>a</b><span style="font-family: Georgia; font-size: x-large;">b</span></h2>`)
	f.mover(punto{0, 2}, false)
	fm := f.Formato()
	if fm.Titulo != "h2" || fm.Alineacion != editor.Derecha || fm.Fuente != "Georgia" || fm.Tamano != "5" || !fm.Negrita {
		t.Errorf("formato bajo el cursor: %+v", fm)
	}
}

// TestElCursorCaeDondeSePinta: la posición que da un clic sobre la raya del cursor es la
// del cursor, en todas las runas de un párrafo que se parte en varias líneas, alineado y
// sin alinear. Es lo que ata el pintado al teclado.
func TestElCursorCaeDondeSePinta(t *testing.T) {
	largo := strings.Repeat("Migración del correo y el almacenamiento de la organización. ", 6)
	for _, estilo := range []string{"", ` style="text-align: center;"`, ` style="text-align: justify;"`} {
		f := folioCon(t, "<h2>Alcance</h2><p"+estilo+">"+largo+"</p>")
		if n := len(f.cajas[1].lineas); n < 3 {
			t.Fatalf("el párrafo largo tendría que partirse en varias líneas y va en %d", n)
		}
		for _, l := range f.cajas[1].lineas {
			if fin := l.x0 + xEnLinea(l, l.hasta); fin > anchoContenido+medidor.ancho(" ", f.bloques[1].Base)+0.5 {
				t.Errorf("%s: una línea se sale de la página: %.2f > %.2f", estilo, fin, anchoContenido)
			}
		}
		for p := 0; p <= f.largo(1); p++ {
			x, y, alto, ok := f.caretEnMaqueta(punto{1, p})
			if !ok {
				t.Fatalf("sin cursor en %d", p)
			}
			if got := f.enMaqueta(x+0.01, y+alto/2); got != (punto{1, p}) {
				t.Errorf("%s: el cursor de %d se pinta donde un clic da %v", estilo, p, got)
			}
		}
	}
}

// TestLoQueNoSeEntiendeNoSeEscribe: un documento con algo que el modelo no representa
// —aquí, una tabla dentro de otra— se abre y se pinta, pero ni se escribe ni se guarda
// distinto de como llegó.
func TestLoQueNoSeEntiendeNoSeEscribe(t *testing.T) {
	const anidada = `<p>x</p><table><tbody><tr><td>a</td><td><table><tbody><tr><td>b</td></tr></tbody></table></td></tr></tbody></table>`
	ctx := &editor.Contexto{Tipo: editor.Documento, Zoom: 100}
	f := NuevoFolio(ctx)
	ctx.Lienzo = f
	if f.Cargar(anidada) {
		t.Fatal("Cargar ha dicho que entiende una tabla dentro de otra")
	}
	ctx.SoloLectura = true
	f.escribir("hola")
	f.Mandar(editor.Negrita, "")
	if got := f.Serializar(); got != anidada {
		t.Errorf("un documento en solo lectura ha cambiado: %s", got)
	}
	if ctx.MotivoSoloLectura == "" {
		t.Error("la franja no sabe por qué es de solo lectura")
	}
	// Se lee entera: la de dentro, en la celda de fuera.
	if n := len(f.cajas); n != 3 || f.cajas[2].b.Texto() != "b" || f.cajas[2].tabla == nil || f.cajas[2].izq <= f.cajas[1].izq {
		t.Errorf("la tabla no se maqueta entera: %d cajas", n)
	}
}

// TestLaTablaSeMaquetaComoLaWeb: `table-layout: fixed; width: 100%` reparte el ancho a
// partes iguales; cada celda tiene `padding: 4px 8px` y bordes de 1 px que se comparten;
// una fila mide lo que su celda más alta (24 px de letra + 8 + 1 = 33); la tabla lleva
// 8 px debajo. Un clic en una celda cae en ELLA, y la flecha abajo baja a la de debajo.
// Se vio en ROJO colocando las cajas sin el x de su celda: la «b» caía encima de la «a».
func TestLaTablaSeMaquetaComoLaWeb(t *testing.T) {
	f := folioCon(t, `<table><tbody><tr><td>a</td><td>b</td></tr><tr><td style="vertical-align: bottom;">c</td><td>d<br>e</td></tr></tbody></table><p>fuera</p>`)
	ancho := f.anchoTexto()
	col := ancho / 2
	type sitio struct{ x, y float32 }
	quiero := []sitio{
		{1 + 8, 1 + 4},
		{col + 1 + 8, 1 + 4},
		// La fila mide lo de «d<br>e», dos líneas, y la «c» va abajo, que es su alineación.
		{1 + 8, 33 + 1 + 4 + 24},
		{col + 1 + 8, 33 + 1 + 4},
	}
	for i, q := range quiero {
		if c := f.cajas[i]; abs(c.izq-q.x) > 0.01 || abs(c.y-q.y) > 0.01 {
			t.Errorf("celda %d en (%.2f, %.2f) y la web la pone en (%.2f, %.2f)", i, c.izq, c.y, q.x, q.y)
		}
	}
	tm := f.cajas[0].tabla
	if tm == nil || abs(tm.alto-(33+57+1)) > 0.01 || abs(f.cajas[4].y-(tm.alto+8)) > 0.01 {
		t.Errorf("la tabla mide %v y lo de debajo empieza en %.2f", tm, f.cajas[4].y)
	}
	if got := f.enMaqueta(col+20, 10); got.b != 1 {
		t.Errorf("un clic en la segunda celda cae en el bloque %d", got.b)
	}
	if got := f.enMaqueta(20, 40); got.b != 2 {
		t.Errorf("un clic en lo alto de una celda alineada abajo cae en el bloque %d", got.b)
	}
	f.mover(punto{0, 1}, false)
	f.vertical(true, false, 0)
	if f.cur.b != 2 {
		t.Errorf("la flecha abajo desde «a» va al bloque %d, y es la «c»", f.cur.b)
	}
}

// pngDe es una imagen de verdad de w × h, como data: URL.
func pngDe(t *testing.T, w, h int) string {
	t.Helper()
	var b bytes.Buffer
	if err := imgpng.Encode(&b, image.NewNRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

// TestUnaImagenMideLoQueDice: la de `insertImage`, sin tamaño, mide lo de su fichero y se
// apoya en la línea base; la gestionada mide sus data-* y va abajo de la línea
// (`vertical-align: bottom`), que crece para caberla; una más ancha que la línea encoge
// sin deformarse; y la de «arriba y abajo» va en su renglón con 14 px arriba y abajo. El
// cursor salta la imagen de una vez. Se vio en ROJO sin el `alPie`: la línea de la
// gestionada se quedaba en 24 px y la imagen salía por arriba.
func TestUnaImagenMideLoQueDice(t *testing.T) {
	src := pngDe(t, 30, 20)
	f := folioCon(t, `<p>a<img src="`+src+`">b</p><p>c<img src="`+src+`" data-sytue-image="" data-w="60" data-h="50">d</p>`+
		`<p><img src="`+pngDe(t, 2000, 1000)+`"></p><p>e<img src="`+src+`" data-wrap="topBottom" data-align="center" data-w="100" data-h="40">f</p>`)
	pz := func(c int) pieza {
		for _, l := range f.cajas[c].lineas {
			for _, p := range l.piezas {
				if p.img != nil {
					return p
				}
			}
		}
		t.Fatalf("el bloque %d no tiene imagen", c)
		return pieza{}
	}
	if p, l := pz(0), f.cajas[0].lineas[0]; p.ancho != 30 || p.alto != 20 || abs(p.arriba-(l.base-20)) > 0.01 {
		t.Errorf("la de insertImage: %+v en una línea de base %.2f", p, l.base)
	}
	if p, l := pz(1), f.cajas[1].lineas[0]; p.ancho != 60 || l.alto != 50 || p.arriba != 0 {
		t.Errorf("la gestionada: %+v en una línea de %.2f", p, l.alto)
	}
	if p, ancho := pz(2), f.anchoTexto(); abs(p.ancho-ancho) > 0.01 || abs(p.alto-ancho/2) > 0.01 {
		t.Errorf("la grande tendría que encoger al ancho, %.2f × %.2f, y mide %.2f × %.2f", ancho, ancho/2, p.ancho, p.alto)
	}
	c := f.cajas[3]
	if len(c.lineas) != 3 || c.lineas[1].alto != 14+40+14 || abs(c.lineas[1].x0-(f.anchoTexto()-100)/2) > 0.01 {
		t.Errorf("la de arriba y abajo: %d líneas, la suya de %.2f en x %.2f", len(c.lineas), c.lineas[1].alto, c.lineas[1].x0)
	}
	f.mover(punto{0, 1}, false)
	if q := f.paso(f.cur, 1); q != (punto{0, 2}) {
		t.Errorf("la flecha derecha no salta la imagen de una vez: %v", q)
	}
	x, _, _, _ := f.caretEnMaqueta(punto{0, 2})
	if x0, _, _, _ := f.caretEnMaqueta(punto{0, 1}); abs(x-x0-30) > 0.01 {
		t.Errorf("el cursor tras la imagen está %.2f px después y la imagen mide 30", x-x0)
	}
	f.escribir("!")
	if got := f.Serializar(); !strings.HasPrefix(got, `<p>a!<img src="`+src+`">b</p>`) {
		t.Errorf("escribir delante de la imagen: %.80s", got)
	}
}

// TestPalabraNoSeSaleDelBloque: el doble clic detrás de la última letra pide la palabra
// desde una posición más allá del final; tumbaba la app (barrido del 2026-09-25, macOS).
func TestPalabraNoSeSaleDelBloque(t *testing.T) {
	f := folioCon(t, "<p>Hola mundo</p>")
	fin := len([]rune("Hola mundo"))
	if a := f.palabra(punto{0, fin + 1}, -1); a.p != len([]rune("Hola ")) {
		t.Errorf("hacia atrás desde el final sale %d y es el principio de «mundo»", a.p)
	}
	if b := f.palabra(punto{0, fin + 1}, 1); b.p > fin {
		t.Errorf("hacia delante sale %d, fuera del bloque de %d", b.p, fin)
	}
}

// TestLasParadasPropiasDeUnDocx: con `--sytue-tabs` cada tabulador va a la siguiente parada
// propia, y lo que sigue se alinea con ella: a la izquierda, centrado o a la derecha (la fecha
// de un currículum, contra el margen). Pasada la última, vuelven las de `tab-size`; una parada
// con relleno pinta sus puntos.
func TestLasParadasPropiasDeUnDocx(t *testing.T) {
	f := folioCon(t, "<p style=\"white-space:pre-wrap;tab-size:50px;--sytue-tabs:120px left, 300px center, 500px right dot\">a\tb\tc\td\te</p>")
	xs := map[string]float32{}
	var puntos string
	for _, l := range f.cajas[0].lineas {
		for _, p := range l.piezas {
			if strings.HasPrefix(p.texto, ".") {
				puntos = p.texto
			} else if p.texto != "" {
				xs[p.texto] = p.x
			}
		}
	}
	ancho := func(s string) float32 { return medidor.ancho(s, f.doc.Bloques()[0].Base) }
	casi := func(a, b float32) bool { return a-b < 0.5 && b-a < 0.5 }
	if !casi(xs["b"], 120) || !casi(xs["c"], 300-ancho("c")/2) || !casi(xs["d"], 500-ancho("d")) || !casi(xs["e"], 550) {
		t.Errorf("las paradas caen en %v", xs)
	}
	if puntos == "" {
		t.Error("la parada con relleno no pinta sus puntos")
	}
}

// TestLosTabuladoresDeUnDocx: un párrafo importado de Word conserva sus tabuladores
// (`white-space: pre-wrap`) y cada uno avanza hasta la siguiente parada de `tab-size`, contada
// desde el borde del bloque: así se alinean las columnas de un currículum hecho con tabuladores.
// Escribir en el párrafo no los junta en un espacio.
func TestLosTabuladoresDeUnDocx(t *testing.T) {
	f := folioCon(t, "<p style=\"white-space:pre-wrap;tab-size:50px\">a\tb\t\tc</p>")
	xs := map[string]float32{}
	for _, l := range f.cajas[0].lineas {
		for _, p := range l.piezas {
			if p.texto != "" {
				xs[p.texto] = p.x
			}
		}
	}
	if xs["b"] != 50 || xs["c"] != 150 {
		t.Errorf("las paradas son cada 50 px y caen en %v", xs)
	}
	f.mover(punto{0, len([]rune("a\tb\t\tc"))}, false)
	f.escribir("d")
	if got := f.Serializar(); !strings.Contains(got, "a\tb\t\tcd") {
		t.Errorf("escribir juntó los tabuladores: %q", got)
	}
}
