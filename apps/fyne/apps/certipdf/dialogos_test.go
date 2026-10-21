// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"context"
	"image"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Los casos de `tramos.test.ts`, uno a uno.
func TestLosTramosDelIndiceSonLosDeLaWeb(t *testing.T) {
	m := func(titulo string, pagina, nivel int) Marcador {
		return Marcador{Titulo: titulo, Pagina: pagina, Nivel: nivel}
	}
	for _, c := range []struct {
		indice []Marcador
		total  int
		quiero []tramo
	}{
		{[]Marcador{m("Uno", 0, 0), m("Dos", 4, 0), m("Tres", 9, 0)}, 20, []tramo{{"Uno", "1-4"}, {"Dos", "5-9"}, {"Tres", "10-20"}}},
		{[]Marcador{m("Uno", 0, 0), m("Uno.a", 1, 1), m("sin destino", -1, 0), m("Dos", 5, 0)}, 8, []tramo{{"Uno", "1-5"}, {"Dos", "6-8"}}},
		{[]Marcador{m("A", 0, 0), m("B", 2, 0), m("C", 3, 0)}, 4, []tramo{{"A", "1-2"}, {"B", "3"}, {"C", "4"}}},
		{nil, 10, []tramo{}},
	} {
		if got := tramosDelIndice(c.indice, c.total); !reflect.DeepEqual(got, c.quiero) {
			t.Errorf("tramosDelIndice(%v) = %v; la web, %v", c.indice, got, c.quiero)
		}
	}
}

// dialogoDePrueba abre un diálogo de la cinta sobre la pantalla a 1280×800 con el
// documento de la medida, y devuelve lo que hay en su capa.
func dialogoDePrueba(t *testing.T, d Dialogo) (*Pantalla, fyne.Window, []fyne.CanvasObject) {
	t.Helper()
	return dialogoA(t, d, fyne.NewSize(1280, 800))
}

// dialogoA es dialogoDePrueba en una ventana de otro tamaño (el móvil, 360×740).
func dialogoA(t *testing.T, d Dialogo, tam fyne.Size) (*Pantalla, fyne.Window, []fyne.CanvasObject) {
	t.Helper()
	p := NuevaPantalla(traductorES(t))
	w := ventanaSinRelleno(p.Objeto())
	w.Resize(tam)
	p.PonerContenido(documentoDePrueba())
	w.Resize(fyne.NewSize(tam.Width, tam.Height+1))
	w.Resize(tam)
	p.doc = &Documento{}
	p.abrirDialogo(d)
	capa := w.Canvas().Overlays().Top()
	if capa == nil || p.dialogo == nil {
		t.Fatal("el diálogo no se abre")
	}
	var objs []fyne.CanvasObject
	todos(capa, func(o fyne.CanvasObject) { objs = append(objs, o) })
	return p, w, objs
}

func accionesDe(objs []fyne.CanvasObject) map[string][]*Accion {
	r := map[string][]*Accion{}
	for _, o := range objs {
		if a, ok := o.(*Accion); ok {
			r[a.Texto] = append(r[a.Texto], a)
		}
	}
	return r
}

// botonesDe son los botones de la suite por su rótulo: los del pie del diálogo.
func botonesDe(objs []fyne.CanvasObject) map[string]*ui.Boton {
	r := map[string]*ui.Boton{}
	for _, o := range objs {
		if b, ok := o.(*ui.Boton); ok {
			r[b.Texto] = b
		}
	}
	return r
}

func primero[T any](objs []fyne.CanvasObject) (T, bool) {
	for _, o := range objs {
		if x, ok := o.(T); ok {
			return x, true
		}
	}
	var cero T
	return cero, false
}

// TestOrganizarSeColocaComoEnLaWeb: Paneles.tsx real en Chromium, CSS1Compat,
// Figtree cargada y raíz de 16 px. Modal md de 560, cuerpo de 510; operaciones
// grid-cols-4 gap-2 de 121,5×30; campo de 510×32 y capítulos de 26 px de alto.
// «Cancelar» y «Aplicar», los de la suite en el pie del diálogo: re-medido el
// 2026-10-17 con el arnés del barrido (`medir-certipdf.mjs`), y con ellos todo el
// panel sube 5,5 px.
func TestOrganizarSeColocaComoEnLaWeb(t *testing.T) {
	_, w, objs := dialogoDePrueba(t, DialogoOrganizar)
	defer w.Close()
	acc := accionesDe(objs)
	for i, op := range []string{"Extraer", "Eliminar", "Girar", "Combinar"} {
		a := acc[op]
		if len(a) != 1 {
			t.Fatalf("no está la operación «%s»", op)
		}
		// Modal md: 560 px; border + px-6 dejan 510. Grid de cuatro y gap-2.
		casiTexto(t, op+" x", absoluta(a[0]).X, []float32{385, 514.5, 644, 773.5}[i])
		cerca(t, op+" y", absoluta(a[0]).Y, 324)
		cerca(t, op+" ancho", a[0].Size().Width, 121.5)
		cerca(t, op+" alto", a[0].Size().Height, 30)
	}
	if acc["Extraer"][0].Fondo != "selected" || acc["Eliminar"][0].Fondo != "" {
		t.Error("la operación elegida al abrir es «Extraer», y solo ella lleva el fondo")
	}
	// `bg-selected` es translúcido: pintado dentro del borde, se aplana sobre la
	// superficie; si no, se ve el rectángulo del borde a través y sale gris oscuro.
	caja := test.WidgetRenderer(acc["Extraer"][0]).(*accionRenderer).caja.Objetos()
	if got, quiero := caja[1].(*canvas.Rectangle).FillColor, tokens.SobreSRGB(tema.Color("selected", tema.Claro), tema.Color("surface", tema.Claro)); got != quiero {
		t.Errorf("la opción elegida se pinta %v; `bg-selected` sobre la superficie es %v", got, quiero)
	}
	e, ok := primero[*ui.Entrada](objs)
	if !ok {
		t.Fatal("no está el campo de páginas")
	}
	cerca(t, "el campo x", absoluta(e).X, 385)
	cerca(t, "el campo y", absoluta(e).Y, 386)
	cerca(t, "el campo ancho", e.Size().Width, 510)
	cerca(t, "el campo alto", e.Size().Height, 32)
	var tramos []*Accion
	for _, o := range objs {
		if a, ok := o.(*Accion); ok && a.Texto == "" {
			tramos = append(tramos, a)
		}
	}
	if len(tramos) != 2 {
		t.Fatalf("hay %d capítulos; la web enseña dos", len(tramos))
	}
	cerca(t, "el primer capítulo y", absoluta(tramos[0]).Y, 446)
	casiTexto(t, "el primer capítulo ancho", tramos[0].Size().Width, 143.07813)
	casiTexto(t, "el segundo capítulo x", absoluta(tramos[1]).X, 532.07813)
	cerca(t, "el capítulo alto", tramos[0].Size().Height, 26)
	bot := botonesDe(objs)
	for _, b := range []struct {
		texto    string
		x, ancho float32
	}{{"Cancelar", 721.609, 88.406}, {"Aplicar", 818.016, 76.984}} {
		a := bot[b.texto]
		if a == nil {
			t.Fatalf("no está «%s» en el pie", b.texto)
		}
		casiTexto(t, b.texto+" x", absoluta(a).X, b.x)
		cerca(t, b.texto+" y", absoluta(a).Y, 521)
		casiTexto(t, b.texto+" ancho", a.Size().Width, b.ancho)
		cerca(t, b.texto+" alto", a.Size().Height, 36)
	}
}

// TestOrganizarMandaLoDeLaWeb: extraer lleva las páginas escritas; girar, además, los
// grados; combinar, las rutas en su orden y ninguna página. Sin lo `required`, nada.
func TestOrganizarMandaLoDeLaWeb(t *testing.T) {
	p, w, _ := dialogoDePrueba(t, DialogoOrganizar)
	defer w.Close()
	o := &dialogoOrganizar{a: p.dialogo, op: "extraer", grados: 90}
	o.vista(516)
	if _, ok := o.cambio(); ok {
		t.Error("sin páginas no se manda nada: el campo es `required`")
	}
	o.paginas.PonerTexto("1-2")
	c, _ := o.cambio()
	if !maps.Equal(c.Campos, map[string]string{"op": "extraer", "paginas": "1-2"}) || c.Sufijo != "extraído" || c.Op != "paginas" {
		t.Errorf("extraer manda %+v", c)
	}
	o.op, o.grados = "rotar", 270
	if c, _ = o.cambio(); c.Campos["grados"] != "270" || c.Sufijo != "girado" {
		t.Errorf("girar manda %+v", c)
	}
	o.op = "combinar"
	o.vista(516)
	o.otros.PonerTexto("/PDF/anexo.pdf\n\n  /Fotos/recibo.jpg ")
	if c, _ = o.cambio(); !slices.Equal(c.Rutas, []string{"/PDF/anexo.pdf", "/Fotos/recibo.jpg"}) || c.Campos["paginas"] != "" {
		t.Errorf("combinar manda %+v", c)
	}
}

// TestCapasSeColocaComoEnLaWeb: Paneles.tsx real en Chromium con raíz de 16 px y
// Figtree cargada. El select nativo de text-sm y line-height normal mide 31 px;
// el texto y los botones conservan sus contratos de required y operación.
func TestCapasSeColocaComoEnLaWeb(t *testing.T) {
	p, w, objs := dialogoDePrueba(t, DialogoCapas)
	defer w.Close()
	s, ok := primero[*ui.Selector](objs)
	if !ok {
		t.Fatal("no está el selector")
	}
	cerca(t, "el selector y", absoluta(s).Y, 332.5)
	cerca(t, "el selector alto", s.Size().Height, 31)
	cerca(t, "el selector ancho", s.Size().Width, 510)
	e, _ := primero[*ui.Entrada](objs)
	cerca(t, "el texto y", absoluta(e).Y, 395.5)
	cerca(t, "«Aplicar» y", absoluta(botonesDe(objs)["Aplicar"]).Y, 504.5)

	k := &dialogoCapas{a: p.dialogo, op: "marca-agua"}
	k.vista(516)
	if _, ok := k.cambio(); ok {
		t.Error("una marca de agua sin texto no se aplica")
	}
	k.op = "numerar"
	if c, ok := k.cambio(); !ok || c.Campos["op"] != "numerar" || c.Sufijo != "sellado" {
		t.Errorf("numerar sin prefijo manda %+v, %v", c, ok)
	}
}

// TestProtegerSeColocaComoEnLaWeb: Paneles.tsx real en Chromium, CSS1Compat,
// Figtree cargada y raíz de 16 px. El cuerpo se desplaza; los seis permisos de
// flex-wrap gap-2 ocupan dos filas, y las propiedades text-xs miden 26 px de alto.
func TestProtegerSeColocaComoEnLaWeb(t *testing.T) {
	_, w, objs := dialogoDePrueba(t, DialogoProteger)
	defer w.Close()
	var entradas []*ui.Entrada
	var casillas []*casillaNativa
	for _, o := range objs {
		switch x := o.(type) {
		case *ui.Entrada:
			entradas = append(entradas, x)
		case *casillaNativa:
			casillas = append(casillas, x)
		}
	}
	if len(entradas) != 6 || len(casillas) != 6 {
		t.Fatalf("hay %d campos y %d casillas; la web pone seis y seis", len(entradas), len(casillas))
	}
	// El DOM medido conserva el cuerpo completo, incluso debajo de su zona de scroll.
	// Re-medido el 2026-10-17 con «Cancelar»/«Aplicar» en el pie (medir-certipdf.mjs,
	// contra el mismo arnés en main): lo que iba debajo de esa fila sube 46.
	for i, y := range []float32{148.40625, 232.40625, 571.40625, 621.40625, 671.40625, 721.40625} {
		cerca(t, "el campo y", absoluta(entradas[i]).Y, y)
		cerca(t, "el campo ancho", entradas[i].Size().Width, 510)
	}
	for i, c := range []struct{ x, y float32 }{{385, 296.40625}, {456.45313, 296.40625}, {550.25, 296.40625}, {608.85938, 296.40625}, {672.32813, 296.40625}, {385, 320.40625}} {
		casiTexto(t, "la casilla x", absoluta(casillas[i]).X, c.x)
		cerca(t, "la casilla y", absoluta(casillas[i]).Y, c.y)
	}
	acc := accionesDe(objs)
	cerca(t, "«Aplicar» y", absoluta(botonesDe(objs)["Aplicar"]).Y, 688.59375)
	cerca(t, "«Quitar los metadatos» y", absoluta(acc["Quitar los metadatos"][0]).Y, 489.40625)
	casiTexto(t, "«Quitar los metadatos» ancho", acc["Quitar los metadatos"][0].Size().Width, 130.25)
	cerca(t, "«Guardar las propiedades» y", absoluta(acc["Guardar las propiedades"][0]).Y, 791.40625)
	if entradas[0].Texto() != "" || entradas[1].Texto() != "" || entradas[2].Texto() != "Contrato" {
		t.Error("las contraseñas empiezan vacías y el título es el del documento")
	}
}

// TestProtegerMandaLoDeLaWeb: los permisos en el orden de la lista —el que se vuelve a
// marcar va al final—, las dos contraseñas; y las propiedades, las cuatro tal cual.
func TestProtegerMandaLoDeLaWeb(t *testing.T) {
	p, w, _ := dialogoDePrueba(t, DialogoProteger)
	defer w.Close()
	d := nuevoDialogoProteger(p.dialogo)
	d.vista(516)
	d.alternar("copiar", false)
	d.alternar("copiar", true)
	d.usuario.PonerTexto("abrir")
	c := d.cambioProteger()
	if !maps.Equal(c.Campos, map[string]string{"usuario": "abrir", "dueno": "", "permisos": "imprimir,modificar,anotar,rellenar,ensamblar,copiar"}) || c.Sufijo != "protegido" {
		t.Errorf("proteger manda %+v", c)
	}
	d.metadatos["autor"].PonerTexto("")
	m := d.cambioMetadatos()
	if datos, _ := m.Datos.(map[string]string); m.Op != "metadatos" || datos["autor"] != "" || datos["titulo"] != "Contrato" || len(datos) != 4 {
		t.Errorf("metadatos manda %+v", m)
	}
}

// TestCompararSeColocaComoEnLaWeb: Modal lg de 720 en Chromium con Figtree cargada y
// raíz de 16 px. El campo flex-1 mide 584,20313×32 y Comparar se estira a su alto.
// Con el resultado del mismo fixture, la ficha de página 2 se recentra y las líneas
// quitada y añadida ocupan cajas de 16 px. La medición de 2026-10-04 incluye
// la tira Texto/Visual: 24 + pb-2(8) + border1 y gap-3(12) añaden45 de alto;
// el recentrado desplaza los controles22,5 respecto al diálogo sin pestañas.
func TestCompararSeColocaComoEnLaWeb(t *testing.T) {
	p, w, objs := dialogoDePrueba(t, DialogoComparar)
	defer w.Close()
	e, ok := primero[*ui.Entrada](objs)
	if !ok {
		t.Fatal("no está el campo de la ruta")
	}
	// Modal lg de 720, border y px-6: empieza en 280 + 1 + 24.
	cerca(t, "el campo x", absoluta(e).X, 305)
	cerca(t, "el campo y", absoluta(e).Y, 455)
	casiTexto(t, "el campo ancho", e.Size().Width, 584.20313)
	b := accionesDe(objs)["Comparar"]
	if len(b) != 1 {
		t.Fatal("no está «Comparar»")
	}
	casiTexto(t, "«Comparar» x", absoluta(b[0]).X, 897.20313)
	cerca(t, "«Comparar» alto", b[0].Size().Height, 32)

	c := p.dialogo
	var comp *dialogoComparar
	c.pintar = func(ancho float32) fyne.CanvasObject {
		if comp == nil {
			comp = &dialogoComparar{a: c, resultado: &Comparacion{Diferencias: []DiferenciaPagina{{
				Pagina: 2, Quitado: []string{"Precio: 1.200 € al año"}, Anadido: []string{"Precio: 1.300 € al año, pagaderos por adelantado en doce mensualidades iguales"},
			}}}}
		}
		return comp.vista(ancho)
	}
	c.rehacer()
	w.Resize(fyne.NewSize(1280, 801))
	w.Resize(fyne.NewSize(1280, 800))
	var parrafos []*ui.Parrafo
	var titulo *ui.Rotulo
	todos(w.Canvas().Overlays().Top(), func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.Parrafo:
			if x.Texto[0] == '+' || strings.HasPrefix(x.Texto, "−") {
				parrafos = append(parrafos, x)
			}
		case *ui.Rotulo:
			if x.Texto == "Página 2" {
				titulo = x
			}
		}
	})
	if titulo == nil || len(parrafos) != 2 {
		t.Fatalf("la ficha no está: título %v, %d líneas", titulo != nil, len(parrafos))
	}
	cerca(t, "el título x", absoluta(titulo).X, 314)
	cerca(t, "el título y", absoluta(titulo).Y, 469)
	cerca(t, "lo quitado y", absoluta(parrafos[0]).Y, 485)
	cerca(t, "lo añadido y", absoluta(parrafos[1]).Y, 501)
}

// motorDeComparar finge Drive y el motor: «/PDF/otro.pdf» existe y `/pdf/compare`
// apunta cuántos ficheros le llegan y en qué orden.
type motorDeComparar struct{ ficheros []string }

func (m *motorDeComparar) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method + " " + r.URL.Path {
	case "GET /api/v1/folders/root":
		_, _ = io.WriteString(w, `{"id":"raiz"}`)
	case "GET /api/v1/folders/raiz/children":
		_, _ = io.WriteString(w, `{"folders":[{"id":"cpdf","name":"PDF"}],"files":[]}`)
	case "GET /api/v1/folders/cpdf/children":
		_, _ = io.WriteString(w, `{"folders":[],"files":[{"id":"o1","folder_id":"cpdf","name":"otro.pdf"}]}`)
	case "GET /api/v1/files/o1/content":
		_, _ = io.WriteString(w, "%PDF-otro")
	case "POST /api/v1/pdf/compare":
		_ = r.ParseMultipartForm(1 << 20)
		for _, ch := range r.MultipartForm.File["file"] {
			m.ficheros = append(m.ficheros, ch.Filename)
		}
		_, _ = io.WriteString(w, `{"diferencias":[{"pagina":1,"anadido":["b"],"quitado":["a"],"solo_en_uno":false}],"iguales":false}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// TestCompararMandaLosDosDocumentos: el actual primero y el otro, traído de Drive por su
// ruta, detrás; y la respuesta llega entera.
func TestCompararMandaLosDosDocumentos(t *testing.T) {
	m := &motorDeComparar{}
	srv := httptest.NewServer(m)
	defer srv.Close()
	doc := &Documento{pdf: api.NewPDF(srv.URL, srv.URL, nil, nil), ruta: "/PDF/a.pdf", fichero: api.File{Name: "a.pdf"}, datos: []byte("%PDF-a")}
	r, err := doc.Comparar(context.Background(), "/PDF/otro.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.ficheros, []string{"a.pdf", "otro.pdf"}) || len(r.Diferencias) != 1 || r.Diferencias[0].Quitado[0] != "a" {
		t.Errorf("al motor le llegan %v y vuelve %+v", m.ficheros, r)
	}
	if _, err := doc.Comparar(context.Background(), "/PDF/no-esta.pdf"); err == nil {
		t.Error("una ruta que no existe tiene que dar error")
	}
}

// herramientasDePrueba abre Herramientas con el reconocimiento disponible (una página
// sin texto; español, inglés y `osd`), las tres páginas dibujadas y el informe dado.
func herramientasDePrueba(t *testing.T, inf *InformeArchivo) (*dialogoHerramientas, fyne.Window) {
	t.Helper()
	p, w, _ := dialogoDePrueba(t, DialogoHerramientas)
	d := nuevoDialogoHerramientas(p.dialogo)
	d.ocr = &EstadoOCR{PaginasSinTexto: []int{3}, Total: 3, Disponible: true, Idiomas: []string{"spa", "eng", "osd"}}
	d.informe = inf
	for n := 1; n <= 3; n++ {
		p.ponerImagen(n, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	}
	p.dialogo.pintar = d.vista
	p.dialogo.rehacer()
	w.Resize(fyne.NewSize(1280, 801))
	w.Resize(fyne.NewSize(1280, 800))
	return d, w
}

// TestHerramientasSeColocaComoEnLaWeb: Paneles.tsx real en Chromium, CSS1Compat,
// Figtree cargada y raíz16. Modal lg con cabecera de 61 (sin el `py-5` de antes del
// 2026-10-15: todo sube 20 en el tope) y desplazamiento; cuerpo x305 de 670. Selectores
// de 31 de alto en 154,40625/525,40625/819,40625; márgenes 328,40625, páginas 604,40625
// e idioma 1057,40625. Las tres páginas están dibujadas, OCR disponible
// y el informe cumple PDF/A: se incluyen todas las secciones de ese estado.
func TestHerramientasSeColocaComoEnLaWeb(t *testing.T) {
	_, w := herramientasDePrueba(t, &InformeArchivo{CumplePDFA: true})
	defer w.Close()
	var objs []fyne.CanvasObject
	todos(w.Canvas().Overlays().Top(), func(o fyne.CanvasObject) { objs = append(objs, o) })
	var selectores []*ui.Selector
	var entradas []*ui.Entrada
	for _, o := range objs {
		switch x := o.(type) {
		case *ui.Selector:
			selectores = append(selectores, x)
		case *ui.Entrada:
			entradas = append(entradas, x)
		}
	}
	if len(selectores) != 3 || len(entradas) != 3 {
		t.Fatalf("hay %d selectores y %d campos; la web pone tres y tres", len(selectores), len(entradas))
	}
	for i, y := range []float32{154.40625, 525.40625, 819.40625} {
		cerca(t, "el selector y", absoluta(selectores[i]).Y, y)
	}
	casiTexto(t, "el selector de formato ancho", selectores[2].Size().Width, 70)
	for i, y := range []float32{328.40625, 604.40625, 1057.40625} {
		cerca(t, "el campo y", absoluta(entradas[i]).Y, y)
	}
	casiTexto(t, "el idioma ancho", entradas[2].Size().Width, 555.28125)
	acc := accionesDe(objs)
	for _, b := range []struct {
		texto       string
		x, y, ancho float32
	}{
		{"Comprimir", 305, 217.40625, 81.46875},
		{"Recortar", 305, 388.40625, 72.26563},
		{"Reconocer texto", 305, 664.40625, 113.95313},
		{"Exportar imágenes", 305, 858.40625, 126.10938},
		{"Comprobar", 305, 975.40625, 87.14063},
		{"Declarar PDF/A", 868.28125, 1055.40625, 106.71875},
	} {
		a := acc[b.texto]
		if len(a) != 1 {
			t.Errorf("no está «%s»", b.texto)
			continue
		}
		casiTexto(t, b.texto+" x", absoluta(a[0]).X, b.x)
		cerca(t, b.texto+" y", absoluta(a[0]).Y, b.y)
		casiTexto(t, b.texto+" ancho", a[0].Size().Width, b.ancho)
	}
}

// TestHerramientasMandaLoDeLaWeb: los idiomas que se ofrecen, las páginas del OCR solo
// si se escriben, y los nombres de las imágenes con sus ceros.
func TestHerramientasMandaLoDeLaWeb(t *testing.T) {
	if got := opcionesDeIdioma([]string{"spa", "eng", "osd"}); !slices.Equal(got, []string{"spa+eng", "spa", "eng"}) {
		t.Errorf("opcionesDeIdioma = %v", got)
	}
	if nombreDeIdioma("spa+eng") != "Español + English" {
		t.Error("el par se nombra en su lengua")
	}
	d, w := herramientasDePrueba(t, nil)
	defer w.Close()
	if c := d.cambioOCR(); !maps.Equal(c.Campos, map[string]string{"idiomas": "spa+eng"}) || c.Op != "reconocer-texto" {
		t.Errorf("sin páginas, OCR manda %+v", c)
	}
	d.paginasOCR.PonerTexto(" 26-50 ")
	if c := d.cambioOCR(); c.Campos["paginas"] != "26-50" {
		t.Errorf("con páginas, OCR manda %+v", c)
	}
	if got := nombreDePagina("contrato", 7, 120, "jpeg"); got != "contrato-007.jpeg" {
		t.Errorf("nombreDePagina = %q", got)
	}
	if baseDeNombre("/PDF/Contrato.PDF") != "Contrato" {
		t.Error("baseDeNombre no quita la extensión")
	}
}

// TestFirmarSeColocaComoEnLaWeb: Paneles.tsx real en Chromium, CSS1Compat,
// Figtree cargada y raíz de 16 px. Pestañas de 24 px; canvas de 510×158,625 por
// width=480/height=148 con borde; la página ocupa w-16 (64 px). Aplicar sigue
// desactivado mientras no haya firma.
func TestFirmarSeColocaComoEnLaWeb(t *testing.T) {
	_, w, objs := dialogoDePrueba(t, DialogoFirmar)
	defer w.Close()
	acc := accionesDe(objs)
	for _, b := range []struct {
		texto    string
		x, ancho float32
	}{{"A mano", 385, 61.34375}, {"Desde imagen", 450.34375, 96.78125}, {"Con certificado", 551.125, 104.15625}, {"Con mi aplicación oficial", 659.28125, 151.65625}} {
		a := acc[b.texto]
		if len(a) != 1 {
			t.Fatalf("no está la pestaña «%s»", b.texto)
		}
		casiTexto(t, b.texto+" x", absoluta(a[0]).X, b.x)
		cerca(t, b.texto+" y", absoluta(a[0]).Y, 223.1875) // con el pie, el panel sube 5,5
		cerca(t, b.texto+" alto", a[0].Size().Height, 24)
	}
	l, ok := primero[*lienzoFirma](objs)
	if !ok {
		t.Fatal("no está el lienzo")
	}
	cerca(t, "el lienzo y", absoluta(l).Y, 292.1875)
	cerca(t, "el lienzo ancho", l.Size().Width, 510)
	cerca(t, "el lienzo alto", l.Size().Height, 158.625)
	cerca(t, "«Borrar» y", absoluta(acc["Borrar y volver a empezar"][0]).Y, 458.8125)
	var pagina *ui.Entrada
	var casillas []*casillaNativa
	for _, o := range objs {
		switch x := o.(type) {
		case *ui.Entrada:
			if x.Texto() == "1" {
				pagina = x
			}
		case *casillaNativa:
			casillas = append(casillas, x)
		}
	}
	if pagina == nil || len(casillas) != 2 {
		t.Fatalf("la fila del recuadro no está: página %v, %d casillas", pagina != nil, len(casillas))
	}
	casiTexto(t, "la página x", absoluta(pagina).X, 823.21875)
	cerca(t, "la página y", absoluta(pagina).Y, 486.8125)
	cerca(t, "la página ancho", pagina.Size().Width, 64)
	casiTexto(t, "la segunda casilla x", absoluta(casillas[1]).X, 573.01563)
	cerca(t, "la primera casilla y", absoluta(casillas[0]).Y, 491.8125)
	ap := botonesDe(objs)["Aplicar"]
	cerca(t, "«Aplicar» y", absoluta(ap).Y, 593.8125)
	if !ap.Inerte {
		t.Error("sin firma, «Aplicar» tiene que estar apagado")
	}
}

// TestFirmarMandaLoDeLaWeb: a mano, los trazos llevados a la página con el eje vertical
// al derecho; sin recuadro, el rectángulo a cero; con un .p12, el fichero y su clave.
func TestFirmarMandaLoDeLaWeb(t *testing.T) {
	p, w, _ := dialogoDePrueba(t, DialogoFirmar)
	defer w.Close()
	d := nuevoDialogoFirmar(p.dialogo)
	if d.listo() {
		t.Error("sin trazos no hay nada que firmar")
	}
	d.lienzo.trazos = [][][2]float64{{{0, 0}, {480, 148}}}
	if !d.listo() {
		t.Error("con un trazo de dos puntos se puede firmar")
	}
	c := d.cambio()
	datos, _ := c.Datos.(map[string]any)
	trazos, _ := datos["trazos"].([][][2]float64)
	if c.Op != "firmar" || len(trazos) != 1 || trazos[0][0] != [2]float64{56, 130} || trazos[0][1] != [2]float64{296, 56} {
		t.Errorf("a mano se manda %+v", c)
	}
	if datos["rect"] != rectFirma || datos["pagina"] != 0 || datos["grosor"] != 1.6 {
		t.Errorf("los datos son %+v", datos)
	}
	d.visible = false
	if datos, _ = d.cambio().Datos.(map[string]any); datos["rect"] != [4]float64{} {
		t.Errorf("sin recuadro el rectángulo es %v", datos["rect"])
	}
	d.modo, d.visible = "certificado", true
	d.pkcs12 = ficheroElegido{nombre: "yo.p12", datos: []byte("P12")}
	d.claveP12.PonerTexto("secreta")
	c = d.cambio()
	if c.Op != "firmar-certificado" || len(c.Partes) != 1 || c.Partes[0].Campo != "pkcs12" || c.Campos["clave_p12"] != "secreta" {
		t.Errorf("con certificado se manda %+v", c)
	}
	if datos, _ = c.Datos.(map[string]any); datos["Rect"] != rectFirma || datos["Bloquear"] != false {
		t.Errorf("los datos del certificado son %+v", datos)
	}
}

// TestLosBotonesDelDialogoVanSobreSuPanel: en oscuro el panel del Modal
// (`surface-raised`) no es la superficie de los paneles (`surface`). Una opción
// con borde y sin fondo, como «Eliminar» en Organizar, deja ver el panel; pintada con la
// superficie salía más oscura que en la web.
func TestLosBotonesDelDialogoVanSobreSuPanel(t *testing.T) {
	tema.Elegir("oscuro")
	defer tema.Elegir("")
	_, w, objs := dialogoDePrueba(t, DialogoOrganizar)
	defer w.Close()
	e := accionesDe(objs)["Eliminar"]
	if len(e) != 1 {
		t.Fatal("no está «Eliminar»")
	}
	caja := test.WidgetRenderer(e[0]).(*accionRenderer).caja.Objetos()
	if got, quiero := caja[1].(*canvas.Rectangle).FillColor, tema.Color("surface-raised", tema.Oscuro); got != quiero {
		t.Errorf("«Eliminar» se pinta %v; el panel del diálogo es %v", got, quiero)
	}
}

// TestFirmarAlEstrechoEsElDeLaWeb: Paneles.tsx real en Chromium a 360×740, con
// Figtree cargada y raíz de 16 px. Desde el 2026-10-15 el diálogo del teléfono es la
// hoja: 360 de ancho, sin filetes a los lados y cuerpo de 312, apoyada abajo. Las
// pestañas flex envuelven en dos o tres líneas (56 px) y las casillas en tres (48 px); el
// canvas conserva width=480/height=148, incluido su borde: 312×97,578.
func TestFirmarAlEstrechoEsElDeLaWeb(t *testing.T) {
	_, w, objs := dialogoA(t, DialogoFirmar, fyne.NewSize(360, 740))
	defer w.Close()
	acc := accionesDe(objs)
	for _, b := range []struct {
		texto    string
		x, ancho float32
	}{{"A mano", 24, 50.15625}, {"Desde imagen", 78.15625, 68.25}, {"Con certificado", 150.40625, 78.85938}, {"Con mi aplicación oficial", 233.26563, 102.73438}} {
		a := acc[b.texto]
		if len(a) != 1 {
			t.Fatalf("no está la pestaña «%s»", b.texto)
		}
		casiTexto(t, b.texto+" x", absoluta(a[0]).X, b.x)
		casiTexto(t, b.texto+" ancho", a[0].Size().Width, b.ancho)
		cerca(t, b.texto+" y", absoluta(a[0]).Y, 280.42188) // con el pie, la hoja sube 19
		cerca(t, b.texto+" alto", a[0].Size().Height, 56)
	}
	l, ok := primero[*lienzoFirma](objs)
	if !ok {
		t.Fatal("no está el lienzo")
	}
	cerca(t, "el lienzo y", absoluta(l).Y, 397.42188)
	// A 360 la hoja va a todo el ancho y sin filetes a los lados: el px-6 deja 312.
	cerca(t, "el lienzo ancho", l.Size().Width, 312)
	casiTexto(t, "el lienzo alto", l.Size().Height, 97.57813)
	var casillas []*casillaNativa
	var pagina *ui.Entrada
	for _, o := range objs {
		switch x := o.(type) {
		case *casillaNativa:
			casillas = append(casillas, x)
		case *ui.Entrada:
			if x.Texto() == "1" {
				pagina = x
			}
		}
	}
	if len(casillas) != 2 || pagina == nil {
		t.Fatalf("la fila del recuadro no está: %d casillas, página %v", len(casillas), pagina != nil)
	}
	for i, c := range []struct{ x, ancho float32 }{{24, 89.1875}, {129.1875, 84.96875}} {
		casiTexto(t, "la casilla x", absoluta(casillas[i]).X, c.x)
		casiTexto(t, "la casilla ancho", casillas[i].Size().Width, c.ancho)
		cerca(t, "la casilla y", absoluta(casillas[i]).Y, 531)
		cerca(t, "la casilla alto", casillas[i].Size().Height, 48)
	}
	casiTexto(t, "la página x", absoluta(pagina).X, 272)
	cerca(t, "la página y", absoluta(pagina).Y, 542)
}

// TestLosDialogosAlEstrechoSonLosDeLaWeb: Paneles.tsx real en Chromium a 360×740,
// CSS1Compat, Figtree cargada y raíz de 16 px, medido otra vez el 2026-10-15 con la hoja
// del teléfono (360 de ancho, cuerpo de 312, apoyada abajo): Organizar usa dos columnas
// de 152; Proteger envuelve los permisos en dos filas; Comparar conserva la altura de 32
// y Herramientas se mide desde el borde del Modal.
func TestLosDialogosAlEstrechoSonLosDeLaWeb(t *testing.T) {
	movil := fyne.NewSize(360, 740)
	t.Run("organizar", func(t *testing.T) {
		_, w, objs := dialogoA(t, DialogoOrganizar, movil)
		defer w.Close()
		acc := accionesDe(objs)
		for _, b := range []struct {
			texto string
			x, y  float32
		}{{"Extraer", 24, 425}, {"Eliminar", 184, 425}, {"Girar", 24, 463}, {"Combinar", 184, 463}} {
			a := acc[b.texto][0]
			cerca(t, b.texto+" x", absoluta(a).X, b.x)
			cerca(t, b.texto+" y", absoluta(a).Y, b.y)
			cerca(t, b.texto+" ancho", a.Size().Width, 152)
		}
		e, _ := primero[*ui.Entrada](objs)
		cerca(t, "el campo y", absoluta(e).Y, 525)
		cerca(t, "el campo ancho", e.Size().Width, 312)
		// El pie de la hoja: «Cancelar | Aplicar» a partes iguales, 152×44.
		ap := botonesDe(objs)["Aplicar"]
		casiTexto(t, "«Aplicar» x", absoluta(ap).X, 184)
		cerca(t, "«Aplicar» y", absoluta(ap).Y, 676)
		cerca(t, "«Aplicar» ancho", ap.Size().Width, 152)
	})
	t.Run("proteger", func(t *testing.T) {
		_, w, objs := dialogoA(t, DialogoProteger, movil)
		defer w.Close()
		var casillas []*casillaNativa
		for _, o := range objs {
			if c, ok := o.(*casillaNativa); ok {
				casillas = append(casillas, c)
			}
		}
		if len(casillas) != 6 {
			t.Fatalf("hay %d permisos", len(casillas))
		}
		for i, c := range []struct{ x, y float32 }{{24, 298}, {95.45313, 298}, {189.25, 298}, {247.85938, 298}, {24, 322}, {157.20313, 322}} {
			casiTexto(t, "el permiso x", absoluta(casillas[i]).X, c.x)
			cerca(t, "el permiso y", absoluta(casillas[i]).Y, c.y)
		}
	})
	t.Run("comparar", func(t *testing.T) {
		_, w, objs := dialogoA(t, DialogoComparar, movil)
		defer w.Close()
		e, _ := primero[*ui.Entrada](objs)
		cerca(t, "el campo x", absoluta(e).X, 24)
		cerca(t, "el campo y", absoluta(e).Y, 688)
		casiTexto(t, "el campo ancho", e.Size().Width, 226.20313)
		casiTexto(t, "«Comparar» x", absoluta(accionesDe(objs)["Comparar"][0]).X, 258.20313)
	})
	t.Run("herramientas", func(t *testing.T) {
		_, w, objs := dialogoA(t, DialogoHerramientas, movil)
		defer w.Close()
		// Sin páginas dibujadas ni reconocimiento, el diálogo es más corto que el de la
		// web y no llega al tope: se mide desde su borde, con el mismo CSS del formulario.
		s, _ := primero[*ui.Selector](objs)
		d, _ := primero[*ui.Dialogo](objs)
		cerca(t, "el primer select x", absoluta(s).X, 24)
		cerca(t, "el primer select desde el borde", absoluta(s).Y-absoluta(d).Y, 100)
		cerca(t, "el primer select ancho", s.Size().Width, 312)
	})
}

// TestIntroEnviaElDialogo: Intro en un campo de una línea es el `onSubmit` del `<form>`
// de la web. El de páginas de Organizar lo lleva puesto; en un cuerpo cualquiera llega a
// los campos anidados y deja en paz el que ya atiende Intro.
func TestIntroEnviaElDialogo(t *testing.T) {
	_, w, objs := dialogoDePrueba(t, DialogoOrganizar)
	defer w.Close()
	if e, ok := primero[*ui.Entrada](objs); !ok || e.AlTeclear == nil {
		t.Fatal("el campo de páginas de Organizar no envía con Intro")
	}

	enviado := 0
	suelto, propio := ui.NuevaEntrada(""), ui.NuevaEntrada("")
	propio.AlTeclear = func(*fyne.KeyEvent) bool { return true }
	conIntro(container.NewVBox(container.NewHBox(suelto), propio), func() { enviado++ })
	intro := &fyne.KeyEvent{Name: fyne.KeyReturn}
	if !suelto.AlTeclear(intro) || enviado != 1 {
		t.Errorf("Intro en un campo anidado envía %d veces", enviado)
	}
	if suelto.AlTeclear(&fyne.KeyEvent{Name: fyne.KeyA}) || enviado != 1 {
		t.Error("otra tecla también envía")
	}
	propio.AlTeclear(intro)
	if enviado != 1 {
		t.Error("se ha pisado el Intro de un campo que ya lo atendía")
	}
}

// Herramientas real, Paneles.tsx, Chromium/Figtree y rem16 (2026-10-04): el
// rótulo de márgenes ocupa32px a278 y16px a670. No impone su ancho natural290px
// a los selectores del resto de secciones. Oráculo: herramientas-360/1280.json. Desde el
// 2026-10-15 el teléfono lleva la hoja a todo el ancho: a 312 el rótulo cabe en una
// línea (medido otra vez en Chromium).
func TestElRotuloDeMargenesSeReparteSinEnsancharHerramientas(t *testing.T) {
	for _, c := range []struct {
		ventana     fyne.Size
		ancho, alto float32
	}{
		{fyne.NewSize(360, 740), 312, 16}, {fyne.NewSize(1280, 800), 670, 16},
	} {
		p, w, objs := dialogoA(t, DialogoHerramientas, c.ventana)
		var rotulo *ui.Parrafo
		for _, o := range objs {
			if x, ok := o.(*ui.Parrafo); ok && x.Texto == p.t("pdf.tools.crop.margins") {
				rotulo = x
				break
			}
		}
		if rotulo == nil {
			w.Close()
			t.Fatal("falta el párrafo del rótulo de márgenes")
		}
		cerca(t, "ancho del rótulo de márgenes", rotulo.Size().Width, c.ancho)
		cerca(t, "alto del rótulo de márgenes", rotulo.Size().Height, c.alto)
		s, ok := primero[*ui.Selector](objs)
		if !ok {
			w.Close()
			t.Fatal("falta el selector de compresión")
		}
		cerca(t, "selector de otra sección", s.Size().Width, c.ancho)
		w.Close()
	}
}
