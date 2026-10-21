// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"encoding/json"
	"image/color"
	"image/png"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// motorDeAnotar finge el motor para lo que se hace sobre la hoja: una A4, su dibujo sin
// pintar, el SVG de su texto, y apunta lo que le llega a anotar, censurar y crear campos.
// Drive no está: guardar el resultado falla, pero lo que se mandó al motor ya se ve.
type motorDeAnotar struct {
	mu    sync.Mutex
	datos map[string]string // ruta → el JSON de la operación
	svgs  int
}

func (m *motorDeAnotar) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseMultipartForm(1 << 20)
	m.mu.Lock()
	defer m.mu.Unlock()
	switch r.URL.Path {
	case "/api/v1/pdf/content":
		_, _ = io.WriteString(w, `{"paginas":[{"numero":1,"ancho":595,"alto":842,"caja":[0,0,595,842]}],"total":1}`)
	case "/api/v1/pdf/render":
		if r.FormValue("formato") == "png" {
			_, _ = io.WriteString(w, `{"paginas":{},"total":1}`)
			return
		}
		m.svgs++
		_, _ = io.WriteString(w, svgDePrueba)
	case "/api/v1/pdf/text":
		_, _ = io.WriteString(w, `{"paginas":[[{"texto":"cliente","x":80,"y":800,"tamano":12},{"texto":"Estimado","x":10,"y":800,"tamano":12}]]}`)
	case "/api/v1/pdf/annotate":
		m.datos[r.URL.Path] = r.FormValue("anotaciones")
		_, _ = io.WriteString(w, "%PDF-anotado")
	case "/api/v1/pdf/protect":
		m.datos[r.URL.Path] = r.FormValue("areas")
		_, _ = io.WriteString(w, "%PDF-censurado")
	case "/api/v1/pdf/form":
		m.datos[r.URL.Path] = r.FormValue("campos")
		_, _ = io.WriteString(w, "%PDF-formulario")
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (m *motorDeAnotar) llego(ruta string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.datos[ruta]
}

// svgDePrueba son dos líneas de texto como las escribe el motor: la matriz base de la
// página (A4, sin rotar) ya aplicada, `scale(1,-1)` y el `textLength` del avance.
const svgDePrueba = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 595 842"><defs><text transform="matrix(1 0 0 -1 0 0)">no cuenta</text></defs>
<text transform="matrix(1 0 0 -1 100 100) scale(1,-1)" font-family="F1" font-size="10" textLength="100">Hola mundo</text>
<text transform="matrix(1 0 0 -1 100 120) scale(1,-1)" font-family="F1" font-size="10" textLength="50">adi&#243;s</text>
</svg>`

// abrirParaAnotar monta la pantalla con el documento del motor fingido abierto.
func abrirParaAnotar(t *testing.T) (*Pantalla, *motorDeAnotar, chan struct{}, func()) {
	t.Helper()
	m := &motorDeAnotar{datos: map[string]string{}}
	srv := httptest.NewServer(m)
	doc := &Documento{pdf: api.NewPDF(srv.URL, srv.URL, nil, nil), ruta: "/PDF/a.pdf", fichero: api.File{Name: "a.pdf"}, datos: []byte("%PDF-")}
	p := NuevaPantalla(traductorES(t))
	p.Autor = "Ada Lovelace"
	w := ventanaSinRelleno(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	listo := make(chan struct{}, 16)
	p.trasRed = func() { listo <- struct{}{} }
	p.Abrir(doc)
	esperarRed(t, listo) // el contenido
	esperarRed(t, listo) // el primer lote
	return p, m, listo, func() { w.Close(); srv.Close() }
}

func elegir(p *Pantalla, h Herramienta) {
	p.barra.Herramienta = h
	p.Recomponer()
}

func anotacionesDe(t *testing.T, j string) []AnotacionNueva {
	t.Helper()
	var a []AnotacionNueva
	if err := json.Unmarshal([]byte(j), &a); err != nil {
		t.Fatalf("lo que llegó a anotar no es la lista de la web: %v (%q)", err, j)
	}
	return a
}

func casi(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// TestElLapizMandaSuTrazoEnCoordenadasDelPDF es la cadena del lápiz: con la herramienta
// la hoja lleva la tapa que recoge el puntero; el arrastre empieza donde se pulsó, y al
// soltar va a `op: anotar` UNA anotación `tinta` con los puntos pasados al PDF (origen
// abajo), el ámbar de la cinta, el grosor y quien la firma.
func TestElLapizMandaSuTrazoEnCoordenadasDelPDF(t *testing.T) {
	p, m, listo, cerrar := abrirParaAnotar(t)
	defer cerrar()
	elegir(p, Lapiz)
	h := p.hojas[0]
	if !contieneObjeto(test.WidgetRenderer(h).Objects(), h.r.tapa) {
		t.Fatal("con el lápiz la hoja no lleva la tapa: arrastrar desplazaría el documento")
	}
	z := h.zoom
	h.Arrastrar(fyne.NewPos(100, 120), fyne.NewDelta(10, 10))
	h.Arrastrar(fyne.NewPos(150, 160), fyne.NewDelta(50, 40))
	if !h.r.previa.Visible() || h.r.previa.Size().Width <= 0 {
		t.Error("mientras se arrastra no se enseña el trazo")
	}
	h.Soltar()
	esperarRed(t, listo)

	a := anotacionesDe(t, m.llego("/api/v1/pdf/annotate"))
	if len(a) != 1 || a[0].Tipo != "tinta" || len(a[0].Trazos) != 1 {
		t.Fatalf("llegó %+v; tenía que ser un trazo de tinta", a)
	}
	quiero := [][2]float64{{90 / z, 842 - 110/z}, {100 / z, 842 - 120/z}, {150 / z, 842 - 160/z}}
	for i, q := range quiero {
		if got := a[0].Trazos[0][i]; !casi(float64(float32(got[0])), float64(float32(q[0]))) || !casi(float64(float32(got[1])), float64(float32(q[1]))) {
			t.Errorf("punto %d = %v; en el PDF es %v", i, got, q)
		}
	}
	if a[0].Color != ColorARGB("#f9b923") || a[0].Grosor != GrosorPorDefecto || a[0].Autor != "Ada Lovelace" || a[0].Opacidad != 1 {
		t.Errorf("color %v, grosor %v, autor %q, opacidad %v", a[0].Color, a[0].Grosor, a[0].Autor, a[0].Opacidad)
	}
	if h.r.previa.Visible() {
		t.Error("al soltar la previa se queda puesta")
	}
}

// TestLaMedidaSeRotulaEnMilimetros: la línea de medir lleva sus dos puntos y la medida
// escrita, en mm con un decimal y punto (el `toFixed(1)` de la web); un clic sin arrastre
// no es una línea y no manda nada.
func TestLaMedidaSeRotulaEnMilimetros(t *testing.T) {
	p, m, listo, cerrar := abrirParaAnotar(t)
	defer cerrar()
	elegir(p, Medida)
	h := p.hojas[0]
	h.Arrastrar(fyne.NewPos(10, 10), fyne.NewDelta(0.5, 0))
	h.Soltar()
	if m.llego("/api/v1/pdf/annotate") != "" {
		t.Fatal("un clic sin arrastre ha mandado una línea de longitud cero")
	}
	z := h.zoom
	// 72 puntos son 25,4 mm: se arrastran 72 puntos de la página en horizontal.
	h.Arrastrar(fyne.NewPos(float32(10+72*z), 10), fyne.NewDelta(float32(72*z), 0))
	h.Soltar()
	esperarRed(t, listo)
	a := anotacionesDe(t, m.llego("/api/v1/pdf/annotate"))
	if len(a) != 1 || a[0].Tipo != "medida" || len(a[0].Puntos) != 2 || a[0].Etiqueta != "25.4 mm" {
		t.Fatalf("llegó %+v; tenía que ser una medida de «25.4 mm»", a)
	}
}

// TestCensurarVaEnDosPasos: la zona arrastrada NO va al motor: se marca sobre la hoja y
// sale la franja ámbar con el recuento. Solo «Censurar», tras confirmar, la manda.
func TestCensurarVaEnDosPasos(t *testing.T) {
	p, m, listo, cerrar := abrirParaAnotar(t)
	defer cerrar()
	elegir(p, Censura)
	h := p.hojas[0]
	h.Arrastrar(fyne.NewPos(120, 140), fyne.NewDelta(60, 40))
	h.Soltar()
	if m.llego("/api/v1/pdf/protect") != "" {
		t.Fatal("marcar una zona ya ha censurado: tienen que ser dos pasos")
	}
	if len(p.zonas) != 1 || len(h.marcas) != 1 || h.marcas[0].Clase != MarcaCensura {
		t.Fatalf("la zona no se ve sobre la hoja: %d zonas, marcas %+v", len(p.zonas), h.marcas)
	}
	var aplicar *Accion
	todos(p.raiz, func(o fyne.CanvasObject) {
		if a, ok := o.(*Accion); ok && a.Texto == "Censurar y guardar una copia" {
			aplicar = a
		}
	})
	if aplicar == nil {
		t.Fatal("no sale la franja con «Censurar y guardar una copia»")
	}
	// «Censurar» pregunta antes (lo borrado no vuelve): se confirma en el diálogo.
	aplicar.AlPulsar()
	if m.llego("/api/v1/pdf/protect") != "" {
		t.Fatal("se ha censurado sin confirmar")
	}
	var confirmar *ui.Boton
	c := fyne.CurrentApp().Driver().CanvasForObject(p.raiz)
	todos(c.Overlays().Top(), func(o fyne.CanvasObject) {
		if b, ok := o.(*ui.Boton); ok && b.Texto == "Censurar y guardar una copia" {
			confirmar = b
		}
	})
	if confirmar == nil || confirmar.Variante != ui.Peligro {
		t.Fatal("no sale la confirmación con el botón de peligro")
	}
	confirmar.AlPulsar()
	esperarRed(t, listo)
	var areas []map[string]float64
	if err := json.Unmarshal([]byte(m.llego("/api/v1/pdf/protect")), &areas); err != nil || len(areas) != 1 {
		t.Fatalf("no llegó la zona a `redactar`: %v %v", err, areas)
	}
	z := h.zoom
	if a := areas[0]; !casi(a["x1"], 60/z) || !casi(a["y2"], 842-100/z) || a["pagina"] != 0 {
		t.Errorf("la zona llegó como %v", a)
	}
	if len(p.zonas) != 1 {
		t.Error("tras un fallo al guardar se han tirado las marcas")
	}
}

// TestLosTramosSonLosDelSVG: los `<text>` de fuera de `<defs>`, con su matriz compuesta
// con `scale(1,-1)`, su tamaño, su largo y el texto con las entidades resueltas.
func TestLosTramosSonLosDelSVG(t *testing.T) {
	tr := TramosDelSVG(svgDePrueba)
	if len(tr) != 2 {
		t.Fatalf("hay %d tramos y son 2 (el de <defs> no cuenta)", len(tr))
	}
	if string(tr[1].Runas) != "adiós" || tr[0].Tam != 10 || tr[0].Largo != 100 {
		t.Errorf("tramo 0 = %+v, tramo 1 = %q", tr[0], string(tr[1].Runas))
	}
	if tr[0].M != [6]float64{1, 0, 0, 1, 100, 100} {
		t.Errorf("la matriz compuesta es %v y es la identidad trasladada a (100, 100)", tr[0].M)
	}
	// «Hola mundo» entero: diez caracteres en cien puntos, de la altura de la letra.
	c := tr[0].caja(0, 10)
	if !casi(c.X, 100) || !casi(c.Ancho, 100) || !casi(c.Y, 100-9.05) || !casi(c.Alto, 11.17) {
		t.Errorf("la caja del tramo es %+v", c)
	}
}

// TestLaSeleccionVaDeUnHuecoAOtro: de media primera línea a media segunda son DOS
// trozos —el final de una y el principio de la otra—, en el orden del documento, y da
// igual hacia dónde se arrastre.
func TestLaSeleccionVaDeUnHuecoAOtro(t *testing.T) {
	tr := TramosDelSVG(svgDePrueba)
	ida := Seleccion(tr, [2]float64{150, 97}, [2]float64{120, 117})
	vuelta := Seleccion(tr, [2]float64{120, 117}, [2]float64{150, 97})
	if len(ida) != 2 || len(vuelta) != 2 || ida[0] != vuelta[0] || ida[1] != vuelta[1] {
		t.Fatalf("ida %+v, vuelta %+v", ida, vuelta)
	}
	// «mundo» (del hueco 5 al 10) y «ad» (del 0 al 2 de cinco en cincuenta puntos).
	if !casi(ida[0].X, 150) || !casi(ida[0].Ancho, 50) || !casi(ida[1].X, 100) || !casi(ida[1].Ancho, 20) {
		t.Errorf("los trozos son %+v", ida)
	}
	if len(Seleccion(tr, [2]float64{150, 97}, [2]float64{150, 97})) != 0 {
		t.Error("sin moverse ya hay algo seleccionado")
	}
}

// TestResaltarMandaLosCuadrosDeLoSeleccionado es la cadena de resaltar: la primera vez
// se pide el SVG de la página, al arrastrar se ve lo seleccionado y al soltar va UNA
// anotación `resaltado` al 40 % con un cuadro por trozo, en coordenadas del PDF.
func TestResaltarMandaLosCuadrosDeLoSeleccionado(t *testing.T) {
	p, m, listo, cerrar := abrirParaAnotar(t)
	defer cerrar()
	elegir(p, Resaltar)
	esperarRed(t, listo) // el SVG de la página, que se pide al elegir la herramienta
	h := p.hojas[0]
	z := float32(h.zoom)
	h.Arrastrar(fyne.NewPos(150*z, 97*z), fyne.NewDelta(0, 0))
	h.Arrastrar(fyne.NewPos(120*z, 117*z), fyne.NewDelta(-30*z, 20*z))
	if len(h.seleccion) != 2 || len(h.r.sel) != 2 {
		t.Fatalf("mientras se arrastra no se ve lo seleccionado: %+v", h.seleccion)
	}
	h.Soltar()
	esperarRed(t, listo)
	if len(h.seleccion) != 0 {
		t.Error("al soltar la selección se queda pintada")
	}
	a := anotacionesDe(t, m.llego("/api/v1/pdf/annotate"))
	if len(a) != 1 || a[0].Tipo != "resaltado" || a[0].Opacidad != 0.4 || len(a[0].Cuadros) != 2 {
		t.Fatalf("llegó %+v", a)
	}
	// «mundo»: x de 150 a 200 y, en el PDF, de 842-(100+2,12) a 842-(100-9,05).
	if c := a[0].Cuadros[0]; !casi(c[0], 150) || !casi(c[2], 200) || !casi(c[1], 842-102.12) || !casi(c[3], 842-90.95) {
		t.Errorf("el primer cuadro es %v", c)
	}
	if m.svgs != 1 {
		t.Errorf("se pidió el SVG %d veces; una por página y documento", m.svgs)
	}
}

// TestConLaManoSeSeleccionaYSeCopia: con la mano, en el escritorio, la hoja lleva la tapa
// DEBAJO de los enlaces; arrastrar selecciona el texto (el SVG se pide al primer arrastre,
// una vez), la selección se queda al soltar, Ctrl+C la copia con un salto entre las dos
// líneas, y un clic la quita.
func TestConLaManoSeSeleccionaYSeCopia(t *testing.T) {
	p, m, listo, cerrar := abrirParaAnotar(t)
	defer cerrar()
	h := p.hojas[0]
	if !contieneObjeto(test.WidgetRenderer(h).Objects(), h.r.tapa) {
		t.Fatal("con la mano la hoja no lleva la tapa: no se puede seleccionar el texto")
	}
	z := float32(h.zoom)
	fyne.DoAndWait(func() { h.Arrastrar(fyne.NewPos(150*z, 97*z), fyne.NewDelta(1, 0)) })
	esperarRed(t, listo) // el SVG de la página, que pide el primer arrastre
	fyne.DoAndWait(func() {
		h.Arrastrar(fyne.NewPos(150*z, 97*z), fyne.NewDelta(0, 0))
		h.Arrastrar(fyne.NewPos(120*z, 117*z), fyne.NewDelta(-30*z, 20*z))
		h.Soltar()
	})
	if len(h.seleccion) != 2 {
		t.Fatalf("al soltar la selección de la mano no se queda: %+v", h.seleccion)
	}
	portapapeles := fyne.CurrentApp().Clipboard()
	portapapeles.SetContent("")
	fyne.DoAndWait(p.copiarSeleccion)
	if got := portapapeles.Content(); got != "mundo\nad" {
		t.Errorf("Ctrl+C copia %q; lo seleccionado es «mundo» y «ad», en dos líneas", got)
	}
	fyne.DoAndWait(func() { h.r.tapa.Tapped(&fyne.PointEvent{Position: fyne.NewPos(300*z, 300*z)}) })
	if len(h.seleccion) != 0 || p.copiable != "" {
		t.Error("un clic no quita la selección")
	}
	if m.svgs != 1 {
		t.Errorf("se pidió el SVG %d veces; una por página y documento", m.svgs)
	}
	if a := m.llego("/api/v1/pdf/annotate"); a != "" {
		t.Errorf("seleccionar con la mano ha anotado %s", a)
	}
}

// TestElDobleClicTomaLaPalabraYElTripleLaLinea: sobre «Hola mundo», un doble clic en la
// «u» selecciona «mundo»; en el espacio, el espacio; el triple, «Hola mundo» entera y
// no la línea de debajo. Y con la mano, el doble clic deja «mundo» para copiar.
func TestElDobleClicTomaLaPalabraYElTripleLaLinea(t *testing.T) {
	tr := TramosDelSVG(svgDePrueba)
	texto := func(clics int, x, y float64) string {
		d, h, ok := PalabraEn(tr, x, y, clics > 2)
		if !ok {
			return "(nada)"
		}
		return TextoSeleccionado(tr, d, h)
	}
	// «Hola mundo»: diez caracteres en cien puntos desde x=100; la «u» es el séptimo.
	if got := texto(2, 166, 97); got != "mundo" {
		t.Errorf("doble clic en la «u» selecciona %q", got)
	}
	if got := texto(2, 145, 97); got != " " {
		t.Errorf("doble clic en el espacio selecciona %q", got)
	}
	if got := texto(3, 166, 97); got != "Hola mundo" {
		t.Errorf("triple clic selecciona %q", got)
	}

	p, _, listo, cerrar := abrirParaAnotar(t)
	defer cerrar()
	h := p.hojas[0]
	z := float32(h.zoom)
	en := &fyne.PointEvent{Position: fyne.NewPos(166*z, 97*z)}
	fyne.DoAndWait(func() { h.r.tapa.DoubleTapped(en) })
	esperarRed(t, listo) // el primero pide el texto de la página
	fyne.DoAndWait(func() { h.r.tapa.DoubleTapped(en) })
	if p.copiable != "mundo" || len(h.seleccion) != 1 {
		t.Errorf("con la mano el doble clic deja %q para copiar (%d trozos)", p.copiable, len(h.seleccion))
	}
	fyne.DoAndWait(func() { h.r.tapa.Tapped(en) })
	if p.copiable != "Hola mundo" {
		t.Errorf("el clic justo tras el doble es el triple y deja %q", p.copiable)
	}
}

// TestLoCopiadoSeJuntaComoEnUnVisor: en la misma línea base, pegado no lleva nada y
// separado lleva un espacio; en otra línea, un salto.
func TestLoCopiadoSeJuntaComoEnUnVisor(t *testing.T) {
	en := func(x, y, largo float64, s string) TramoTexto {
		return TramoTexto{M: [6]float64{1, 0, 0, 1, x, y}, Tam: 10, Largo: largo, Runas: []rune(s)}
	}
	tr := []TramoTexto{en(100, 100, 40, "Hola"), en(140, 100, 10, "!"), en(160, 100, 50, "mundo"), en(100, 115, 30, "otra")}
	if got := TextoSeleccionado(tr, [2]float64{100, 97}, [2]float64{122.5, 112}); got != "Hola! mundo\notr" {
		t.Errorf("se copia %q", got)
	}
}

// TestLaAnotacionPuestaSeVeDiscontinua: el contorno de una anotación que ya está en el
// documento es `stroke-dasharray="3 2"`: a lo largo del borde de arriba hay tramos con
// tinta y huecos sin ella, que `canvas.Rectangle` no sabía hacer.
func TestLaAnotacionPuestaSeVeDiscontinua(t *testing.T) {
	pag := a4()
	o := rectanguloDeMarca(Marca{Rect: Rect{100, 700, 200, 740}, Clase: MarcaAnotacion}, pag, 4, 0)
	r, ok := o.(*canvas.Raster)
	if !ok {
		t.Fatalf("la marca es %T", o)
	}
	// A zoom 4: 100 × 40 puntos más medio grosor por lado son 402,4 × 162,4 píxeles.
	pintado := r.Generator(402, 162)
	con, sin := 0, 0
	y := 1 // el borde de arriba, a medio grosor (0,3 × 4 = 1,2 px) del canto
	for x := 10; x < 390; x++ {
		if _, _, _, a := pintado.At(x, y).RGBA(); a > 0 {
			con++
		} else {
			sin++
		}
	}
	if con == 0 || sin == 0 {
		t.Errorf("el borde de arriba tiene %d píxeles con tinta y %d sin: no es discontinuo", con, sin)
	}
	// 3 de tinta por cada 2 de hueco, con remate recto: el 60 %.
	if f := float64(con) / float64(con+sin); f < 0.55 || f > 0.65 {
		t.Errorf("el %0.f %% del borde tiene tinta; con «3 2» es el 60 %%", f*100)
	}
}

// TestLaElipseNoEsUnCirculo: la previa de la elipse llena su caja —más ancha que alta—,
// que `canvas.Circle` no hace (siempre un círculo del lado corto).
func TestLaElipseNoEsUnCirculo(t *testing.T) {
	g := Gesto{Desde: [2]float64{0, 0}, Hasta: [2]float64{100, 40}}
	img := pintarPrevia(g, Elipse, color.NRGBA{R: 255, A: 255}, 2, 104, 44)
	tinta := func(x, y int) bool { _, _, _, a := img.At(x, y).RGBA(); return a > 0 }
	// Los extremos de la elipse (0 y 100 en x, a media altura) llevan tinta; el centro no.
	if !tinta(2, 22) || !tinta(101, 22) || tinta(52, 22) {
		t.Error("la elipse no llega a los lados de su caja o está rellena")
	}
}

func contieneObjeto(objs []fyne.CanvasObject, o fyne.CanvasObject) bool {
	for _, x := range objs {
		if x == o {
			return true
		}
	}
	return false
}

// TestLasFranjasDeLoMarcadoSonLasDeLaWeb usa Chromium remoto 2026-10-04,
// PdfApp real con gestos de campo y censura, CSS1Compat, raíz16 y Figtree cargada.
// Campos y censura miden 42; el select26 centra los botones24 y el peligro26.
func TestLasFranjasDeLoMarcadoSonLasDeLaWeb(t *testing.T) {
	p, _, _, cerrar := abrirParaAnotar(t)
	defer cerrar()
	p.zonas = []ZonaCensura{{Pagina: 0, Rect: Rect{100, 700, 150, 740}}}
	p.camposNuevos = []CampoMarcado{{Pagina: 0, Rect: Rect{100, 600, 220, 625}, Tipo: "texto"}}
	p.refrescarMarcas()
	p.Recomponer()
	fyne.DoAndWait(func() {})

	var franjas []*franja
	acciones := map[string][]*Accion{}
	var tipo *ui.Selector
	todos(p.raiz, func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *franja:
			franjas = append(franjas, x)
		case *Accion:
			acciones[x.Texto] = append(acciones[x.Texto], x)
		case *ui.Selector:
			tipo = x
		}
	})
	if len(franjas) != 2 || tipo == nil || len(acciones["Quitar las marcas"]) != 2 {
		t.Fatalf("hay %d franjas, selector %v y %d «Quitar»", len(franjas), tipo != nil, len(acciones["Quitar las marcas"]))
	}
	cerca(t, "la franja de campos y", absoluta(franjas[0]).Y, 112)
	cerca(t, "su alto", franjas[0].Size().Height, 42)
	cerca(t, "la de censura y", absoluta(franjas[1]).Y, 154)
	cerca(t, "su alto", franjas[1].Size().Height, 42)

	casiTexto(t, "el selector x", absoluta(tipo).X, 916.98438)
	cerca(t, "el selector y", absoluta(tipo).Y, 120)
	cerca(t, "el selector ancho", tipo.Size().Width, 105)
	cerca(t, "el selector alto", tipo.Size().Height, 26)
	for _, b := range []struct {
		a              *Accion
		x, y, ancho, h float32
	}{
		{acciones["Quitar las marcas"][0], 1029.98438, 121, 108.40625, 24},
		{acciones["Crear los campos"][0], 1146.39063, 121, 117.60938, 24},
		{acciones["Quitar las marcas"][1], 963.5625, 163, 108.40625, 24},
		{acciones["Censurar y guardar una copia"][0], 1079.96875, 162, 184.03125, 26},
	} {
		casiTexto(t, b.a.Texto+" x", absoluta(b.a).X, b.x)
		cerca(t, b.a.Texto+" y", absoluta(b.a).Y, b.y)
		casiTexto(t, b.a.Texto+" ancho", b.a.Size().Width, b.ancho)
		cerca(t, b.a.Texto+" alto", b.a.Size().Height, b.h)
	}
}

// TestCapturaDeLasHerramientas deja en el directorio SYTUE_CAPTURA_HERRAMIENTAS la pantalla
// con lo marcado y sin aplicar (las dos franjas) y, encima de la hoja, un rectángulo, un
// trazo de lápiz y una selección de texto a medio arrastrar: lo que un recuento de
// píxeles no dice si se ve bien.
func TestCapturaDeLasHerramientas(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURA_HERRAMIENTAS")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURA_HERRAMIENTAS: esto es para sacar la lámina, no un guardián")
	}
	p, _, listo, cerrar := abrirParaAnotar(t)
	defer cerrar()
	c := fyne.CurrentApp().Driver().CanvasForObject(p.raiz)
	guardar := func(nombre string) {
		fyne.DoAndWait(func() {})
		f, err := os.Create(filepath.Join(dir, nombre))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, c.Capture()); err != nil {
			t.Fatal(err)
		}
	}
	p.zonas = []ZonaCensura{{Pagina: 0, Rect: Rect{80, 700, 200, 740}}}
	p.camposNuevos = []CampoMarcado{{Pagina: 0, Rect: Rect{80, 600, 300, 625}, Tipo: "texto"}}
	p.Contenido.Anotaciones = []AnotacionLeida{{Pagina: 0, Tipo: "Square", Rect: Rect{320, 690, 520, 760}}}
	p.refrescarMarcas()
	elegir(p, Rectangulo)
	h := p.hojas[0]
	h.Arrastrar(fyne.NewPos(120, 360), fyne.NewDelta(0, 0))
	h.Arrastrar(fyne.NewPos(330, 450), fyne.NewDelta(210, 90))
	guardar("rectangulo.png")
	h.Soltar()

	elegir(p, Lapiz)
	h.Arrastrar(fyne.NewPos(120, 520), fyne.NewDelta(0, 0))
	for i := 1; i <= 40; i++ {
		x := float32(120 + i*6)
		h.Arrastrar(fyne.NewPos(x, float32(520+30*math.Sin(float64(i)/5))), fyne.NewDelta(6, 0))
	}
	guardar("lapiz.png")

	elegir(p, Resaltar)
	esperarRed(t, listo)
	z := float32(h.zoom)
	h.Arrastrar(fyne.NewPos(150*z, 97*z), fyne.NewDelta(0, 0))
	h.Arrastrar(fyne.NewPos(120*z, 117*z), fyne.NewDelta(-30*z, 20*z))
	guardar("seleccion.png")
}
