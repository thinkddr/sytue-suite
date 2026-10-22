// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"context"
	"encoding/base64"
	"image"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// motorVisual finge Drive y el motor: «/PDF/otro.pdf» existe y `/pdf/compare` apunta qué
// campos le llegan, y contesta como el motor de verdad (la forma de `domain`).
type motorVisual struct {
	mu     sync.Mutex
	campos []map[string]string
}

func (m *motorVisual) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
		campos := map[string]string{"ficheros": strings.Join(nombresDe(r), ",")}
		for k, v := range r.MultipartForm.Value {
			campos[k] = v[0]
		}
		m.mu.Lock()
		m.campos = append(m.campos, campos)
		m.mu.Unlock()
		if campos["pagina"] != "" {
			// El PNG del resaltado va en base64 dentro del JSON, como en `[]byte` de Go.
			_, _ = io.WriteString(w, `{"modo":"visual","pagina":{"pagina":2,"ancho":595,"alto":842,"escala":1.5,
			  "porcentaje":0.0123,"regiones":[{"x":50,"y":120,"ancho":120,"alto":14}],"imagen":"`+
				base64.StdEncoding.EncodeToString([]byte("png"))+`"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"modo":"visual","paginas_a":2,"paginas_b":3,"comparadas":3,"truncado":false,"iguales":false,
		  "paginas":[{"pagina":2,"ancho":595,"alto":842,"escala":1.5,"porcentaje":0.0123,"regiones":[{"x":50,"y":120,"ancho":120,"alto":14}]},
		  {"pagina":3,"ancho":595,"alto":842,"escala":0,"porcentaje":0,"regiones":[],"solo_en":"b"}]}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func nombresDe(r *http.Request) []string {
	out := make([]string, 0, len(r.MultipartForm.File["file"]))
	for _, ch := range r.MultipartForm.File["file"] {
		out = append(out, ch.Filename)
	}
	return out
}

// TestCompararVisualMandaElModoLaToleranciaYLaPagina: el documento actual primero y el
// otro detrás, `modo=visual`, la tolerancia de la sensibilidad y, para una página, `pagina`.
func TestCompararVisualMandaElModoLaToleranciaYLaPagina(t *testing.T) {
	m := &motorVisual{}
	srv := httptest.NewServer(m)
	defer srv.Close()
	doc := &Documento{pdf: api.NewPDF(srv.URL, srv.URL, nil, nil), ruta: "/PDF/a.pdf", fichero: api.File{Name: "a.pdf"}, datos: []byte("%PDF-a")}
	ctx := context.Background()
	otro, err := doc.TraerOtro(ctx, "/PDF/otro.pdf")
	if err != nil {
		t.Fatal(err)
	}
	r, err := doc.CompararVisual(ctx, otro, 24)
	if err != nil {
		t.Fatal(err)
	}
	if r.PaginasA != 2 || r.PaginasB != 3 || len(r.Paginas) != 2 || r.Paginas[1].SoloEn != "b" || r.Paginas[0].Regiones[0].Ancho != 120 {
		t.Errorf("la respuesta no llega entera: %+v", r)
	}
	p, err := doc.CompararVisualPagina(ctx, otro, 2, 96)
	if err != nil {
		t.Fatal(err)
	}
	if string(p.Imagen) != "png" || p.Pagina != 2 || len(p.Regiones) != 1 {
		t.Errorf("la página no llega entera: %+v", p)
	}
	if len(m.campos) < 2 {
		t.Fatalf("el motor recibió %d peticiones", len(m.campos))
	}
	if c := m.campos[0]; c["modo"] != "visual" || c["tolerancia"] != "24" || c["pagina"] != "" || c["ficheros"] != "a.pdf,otro.pdf" {
		t.Errorf("el resumen manda %v", c)
	}
	if c := m.campos[1]; c["modo"] != "visual" || c["tolerancia"] != "96" || c["pagina"] != "2" {
		t.Errorf("la página manda %v", c)
	}
}

func TestParadasVanEnOrdenYUnaPaginaSinCajasCuentaUna(t *testing.T) {
	got := paradasDe([]DiferenciaVisual{
		{Pagina: 1, Regiones: []RegionVisual{{}, {}}},
		{Pagina: 2, SoloEn: "b"},
		{Pagina: 4, Regiones: []RegionVisual{{}}},
	})
	quiero := []parada{{1, 0}, {1, 1}, {2, -1}, {4, 0}}
	if !slices.Equal(got, quiero) {
		t.Errorf("paradas %v, quería %v", got, quiero)
	}
	if len(paradasDe(nil)) != 0 {
		t.Error("sin páginas no hay paradas")
	}
	for _, c := range []struct{ actual, total, paso, quiero int }{
		{0, 3, 1, 1}, {2, 3, 1, 0}, {0, 3, -1, 2}, {0, 0, 1, 0}, {0, 1, -1, 0},
	} {
		if got := moverParada(c.actual, c.total, c.paso); got != c.quiero {
			t.Errorf("moverParada(%d,%d,%d) = %d, quería %d", c.actual, c.total, c.paso, got, c.quiero)
		}
	}
}

func TestElPorcentajeNoDiceCeroSiAlgoDifiere(t *testing.T) {
	for _, c := range []struct{ v, quiero float64 }{
		{0, 0}, {0.00001, 0.001}, {0.0123, 0.012}, {0.5, 0.5}, {2.345, 2.35}, {12.345, 12.3},
	} {
		if got := porcentajeRedondeado(c.v); math.Abs(got-c.quiero) > 1e-9 {
			t.Errorf("porcentajeRedondeado(%g) = %g, quería %g", c.v, got, c.quiero)
		}
	}
}

func TestCadaAvisoVisualTieneSuTexto(t *testing.T) {
	p := NuevaPantalla(traductorES(t))
	vistos := map[string]bool{}
	for _, d := range []DiferenciaVisual{{SoloEn: "a"}, {SoloEn: "b"}, {Omitida: true, Motivo: "demasiado_grande"}, {Omitida: true, Motivo: "ilegible"}} {
		k := claveAvisoVisual(d)
		if k == "" || vistos[k] {
			t.Fatalf("%+v: clave %q repetida o vacía", d, k)
		}
		vistos[k] = true
		if p.t(k) == k {
			t.Errorf("la clave %q no tiene texto", k)
		}
	}
	if claveAvisoVisual(DiferenciaVisual{Regiones: []RegionVisual{{}}}) != "" {
		t.Error("una página con cajas no lleva aviso")
	}
	for _, s := range sensibilidades {
		if p.t(s.clave) == s.clave {
			t.Errorf("la sensibilidad %q no tiene texto", s.clave)
		}
	}
}

// visualDePrueba abre «Comparar» en la pestaña Visual con el mismo caso que se midió en
// Chromium: tres de cuatro páginas con diferencias (la 1 con dos cajas, la 3 de otro
// tamaño y la 4 solo en el segundo documento) y la página 1 ya dibujada.
func visualDePrueba(t *testing.T) (*dialogoComparar, fyne.Window) {
	t.Helper()
	return visualDePruebaA(t, fyne.NewSize(1280, 800))
}

func visualDePruebaA(t *testing.T, tam fyne.Size) (*dialogoComparar, fyne.Window) {
	t.Helper()
	p, w, _ := dialogoA(t, DialogoComparar, tam)
	c := p.dialogo
	dif := DiferenciaVisual{
		Pagina: 1, Ancho: 595, Alto: 842, Escala: 1.5, Porcentaje: 0.0123,
		Regiones: []RegionVisual{{X: 50, Y: 120, Ancho: 120, Alto: 14}, {X: 290, Y: 130, Ancho: 80, Alto: 14}},
	}
	comp := &dialogoComparar{a: c, modo: "visual", v: &estadoVisual{
		sens: sensibilidadPorDefecto,
		res: &ComparacionVisual{PaginasA: 3, PaginasB: 4, Comparadas: 4, Paginas: []DiferenciaVisual{
			dif,
			{Pagina: 3, Ancho: 595, Alto: 842, Escala: 1.5, Porcentaje: 2.5, TamanosDistintos: true, Regiones: []RegionVisual{{X: 10, Y: 10, Ancho: 100, Alto: 20}}},
			{Pagina: 4, Ancho: 595, Alto: 842, SoloEn: "b"},
		}},
		paginas: map[int]*paginaVisual{1: {dif: dif, img: image.NewRGBA(image.Rect(0, 0, 892, 1263))}},
	}}
	c.pintar = comp.vista
	c.rehacer()
	w.Resize(fyne.NewSize(tam.Width, tam.Height+1))
	w.Resize(tam)
	return comp, w
}

func visualObjetos(w fyne.Window) []fyne.CanvasObject {
	var objs []fyne.CanvasObject
	todos(w.Canvas().Overlays().Top(), func(o fyne.CanvasObject) { objs = append(objs, o) })
	return objs
}

// TestCompararVisualSeColocaComoEnLaWeb: medido en Chromium (2026-10-04), CSS1Compat, raíz de
// 16 px y Figtree cargada, con el mismo
// caso. Las posiciones son RELATIVAS al párrafo de ayuda —el diálogo se desplaza— y se
// aceptan a un píxel: la web reparte los sobrantes fraccionarios a su manera.
func TestCompararVisualSeColocaComoEnLaWeb(t *testing.T) {
	_, w := visualDePrueba(t)
	defer w.Close()
	objs := visualObjetos(w)
	var ayuda *ui.Parrafo
	var parrafos []*ui.Parrafo
	for _, o := range objs {
		if x, ok := o.(*ui.Parrafo); ok {
			parrafos = append(parrafos, x)
			if strings.HasPrefix(x.Texto, "Compara cada página") {
				ayuda = x
			}
		}
	}
	if ayuda == nil {
		t.Fatal("no está el párrafo de ayuda")
	}
	y0 := absoluta(ayuda).Y
	cerca1 := func(que string, got, quiero float32) {
		t.Helper()
		if math.Abs(float64(got-quiero)) > 1 {
			t.Errorf("%s: %g; la web, %g", que, got, quiero)
		}
	}
	e, _ := primero[*ui.Entrada](objs)
	sel, _ := primero[*ui.Selector](objs)
	cerca1("el campo x", absoluta(e).X, 305)
	cerca1("el campo y", absoluta(e).Y-y0, 44)
	// El campo flex-1 recibe lo que dejan Sensibilidad, el selector y el botón.
	// Se conserva el margen de dos píxeles de la medición tipográfica original.
	if got := e.Size().Width; math.Abs(float64(got-424.78125)) > 2 {
		t.Errorf("el campo ancho: %g; la web, 424,78125", got)
	}
	cerca1("el selector x", absoluta(sel).X, 808.20313)
	cerca1("el selector y", absoluta(sel).Y-y0, 44.5)
	cerca1("el selector ancho", sel.Size().Width, 81)
	cmp := accionesDe(objs)["Comparar"]
	if len(cmp) != 1 {
		t.Fatalf("hay %d botones «Comparar»", len(cmp))
	}
	cerca1("«Comparar» x", absoluta(cmp[0]).X, 897.20313)
	cerca1("«Comparar» y", absoluta(cmp[0]).Y-y0, 46)
	cerca1("«Comparar» alto", cmp[0].Size().Height, 28)

	for _, p := range parrafos {
		switch {
		case strings.HasPrefix(p.Texto, "3 de 4 páginas"):
			cerca1("el resumen y", absoluta(p).Y-y0, 88)
		case strings.HasPrefix(p.Texto, "La página 4"):
			// El aviso ámbar tiene p-2: la medición DOM empieza antes del relleno.
			cerca1("el aviso y", absoluta(p).Y-y0-8, 116)
		}
	}
	var cab *ui.Parrafo
	for _, o := range objs {
		if r, ok := o.(*ui.Parrafo); ok && strings.HasPrefix(r.Texto, "Página 1 ·") {
			cab = r
		}
	}
	if cab == nil {
		t.Fatal("no está la cabecera de la página")
	}
	if cab.Texto != "Página 1 · 0,012 % distinto · Diferencia 1 de 2" {
		t.Errorf("la cabecera dice %q", cab.Texto)
	}
	cerca1("la cabecera y", absoluta(cab).Y-y0, 165)
	ant, sig := accionesDe(objs)["Anterior"], accionesDe(objs)["Siguiente"]
	if len(ant) != 1 || len(sig) != 1 {
		t.Fatal("faltan Anterior y Siguiente")
	}
	cerca1("Anterior x", absoluta(ant[0]).X, 830.28125)
	cerca1("Anterior y", absoluta(ant[0]).Y-y0, 160)
	cerca1("Siguiente x", absoluta(sig[0]).X, 902.01563)

	v, ok := primero[*vistaDiferencias](objs)
	if !ok {
		t.Fatal("no está la imagen")
	}
	cerca1("la imagen x", absoluta(v).X, 305)
	cerca1("la imagen y", absoluta(v).Y-y0, 194)
	cerca1("la imagen ancho", v.Size().Width, 670)
	cerca1("la imagen alto", v.Size().Height, 948.125)
	// Las cajas usan el interior de 668×946,125, el borde de un píxel y los
	// porcentajes de cajaEnPorcentaje redondeados a tres decimales.
	cajas := vistaCajas(v)
	if len(cajas) != 2 {
		t.Fatalf("hay %d cajas", len(cajas))
	}
	cerca1("caja 1 x", absoluta(cajas[0]).X-305, 57.125)
	cerca1("caja 1 y", absoluta(cajas[0]).Y-absoluta(v).Y, 135.82813)
	cerca1("caja 1 ancho", cajas[0].Size().Width, 134.71875)
	cerca1("caja 2 x", absoluta(cajas[1]).X-305, 326.5625)
}

// El mismo CompararVisual.tsx medido a 360 y 390 × 740: campo en una fila,
// sensibilidad y Comparar en la siguiente; los botones de navegación bajan debajo
// del título sin invadirlo. Se compara el DOM, no la propia cuenta del layout.
//
// Desde el 2026-10-15 el diálogo del teléfono es la hoja a todo el ancho: el cuerpo pasa
// de 278/308 a 312/342 y todo empieza 17 px más a la izquierda (sin el `p-4` ni el
// filete). La imagen y sus cajas salen de las mismas reglas del DOM medido: mide el
// ancho del cuerpo con la proporción de la página (842/595) y las regiones van dentro de
// su borde de 1 px, a escala del interior.
func TestCompararVisualEnvuelveLaNavegacionEnElMovil(t *testing.T) {
	for _, c := range []struct {
		ancho, cuerpo, imagenAlto, cajaX, cajaY, cajaAncho, cajaAlto float32
	}{
		{360, 312, 441.51261, 27.05042, 63.63795, 62.52101, 7.30781},
		{390, 342, 483.96639, 29.57143, 69.68877, 68.57143, 8.01372},
	} {
		t.Run(strconv.Itoa(int(c.ancho)), func(t *testing.T) {
			_, w := visualDePruebaA(t, fyne.NewSize(c.ancho, 740))
			defer w.Close()
			objs := visualObjetos(w)
			var ayuda, titulo *ui.Parrafo
			for _, o := range objs {
				if p, ok := o.(*ui.Parrafo); ok {
					if strings.HasPrefix(p.Texto, "Compara cada página") {
						ayuda = p
					}
					if strings.HasPrefix(p.Texto, "Página 1 ·") {
						titulo = p
					}
				}
			}
			if ayuda == nil || titulo == nil {
				t.Fatal("faltan ayuda o cabecera visual")
			}
			y0 := absoluta(ayuda).Y
			cerca1 := func(que string, got, quiero float32) {
				t.Helper()
				if math.Abs(float64(got-quiero)) > 1 {
					t.Errorf("%s: %g; DOM, %g", que, got, quiero)
				}
			}
			e, _ := primero[*ui.Entrada](objs)
			sel, _ := primero[*ui.Selector](objs)
			acc := accionesDe(objs)
			cerca1("campo x", absoluta(e).X, 24)
			cerca1("campo y", absoluta(e).Y-y0, 76)
			cerca1("campo ancho", e.Size().Width, c.cuerpo)
			cerca1("selector x", absoluta(sel).X, 94.42188)
			cerca1("selector y", absoluta(sel).Y-y0, 116)
			cerca1("Comparar x", absoluta(acc["Comparar"][0]).X, 183.42188)
			cerca1("Comparar y", absoluta(acc["Comparar"][0]).Y-y0, 117.5)
			cerca1("título y", absoluta(titulo).Y-y0, 231)
			cerca1("título alto", titulo.Size().Height, 16)
			cerca1("Anterior x", absoluta(acc["Anterior"][0]).X, 24)
			cerca1("Anterior y", absoluta(acc["Anterior"][0]).Y-y0, 255)
			cerca1("Siguiente x", absoluta(acc["Siguiente"][0]).X, 95.73438)
			if absoluta(acc["Anterior"][0]).Y < absoluta(titulo).Y+titulo.Size().Height+8 {
				t.Error("los botones invaden el título en lugar de bajar con gap-2")
			}
			v, ok := primero[*vistaDiferencias](objs)
			if !ok {
				t.Fatal("no está la imagen visual")
			}
			cerca1("imagen y", absoluta(v).Y-y0, 289)
			cerca1("imagen ancho", v.Size().Width, c.cuerpo)
			cerca1("imagen alto", v.Size().Height, c.imagenAlto)
			cajas := vistaCajas(v)
			if len(cajas) != 2 {
				t.Fatalf("hay %d cajas", len(cajas))
			}
			cerca1("caja x", absoluta(cajas[0]).X-absoluta(v).X, c.cajaX)
			cerca1("caja y", absoluta(cajas[0]).Y-absoluta(v).Y, c.cajaY)
			cerca1("caja ancho", cajas[0].Size().Width, c.cajaAncho)
			cerca1("caja alto", cajas[0].Size().Height, c.cajaAlto)
		})
	}
}

// La imagen absoluta y sus regiones empiezan dentro de `border`, y rounded-sm
// conserva el radio de cuatro píxeles fijado por el canon, independiente del rem.
func TestLaImagenVisualYLasRegionesRespetanSuBorde(t *testing.T) {
	d, w := visualDePrueba(t)
	defer w.Close()
	v, _ := primero[*vistaDiferencias](visualObjetos(w))
	r := test.WidgetRenderer(v).(*vistaDiferenciasRenderer)
	if r.papel.Position() != fyne.NewPos(1, 1) {
		t.Errorf("la imagen empieza en %v", r.papel.Position())
	}
	if math.Abs(float64(r.papel.Size().Width-668)) > 1 || math.Abs(float64(r.papel.Size().Height-946.125)) > 1 {
		t.Errorf("la imagen interior mide %v; DOM, 668×946,125", r.papel.Size())
	}
	for _, c := range r.cajas {
		if c.CornerRadius != 4 {
			t.Errorf("rounded-sm de la región mide %g", c.CornerRadius)
		}
	}
	var muestras int
	for _, o := range visualObjetos(w) {
		if c, ok := o.(*canvas.Rectangle); ok && c.MinSize() == fyne.NewSize(10, 10) {
			muestras++
			if c.CornerRadius != 4 || (c.FillColor != tema.Color("danger-bg", tema.Claro) && c.FillColor != tema.Color("success-bg", tema.Claro)) {
				t.Errorf("la leyenda no sigue bg-{danger,success}-bg y rounded-sm: %+v", c)
			}
		}
	}
	if muestras != 2 {
		t.Errorf("hay %d muestras de color en la leyenda", muestras)
	}
	// Puntos que el DOM sitúa justo antes y dentro de la segunda caja. El borde
	// desplaza la zona interactiva; no se compara con una posición calculada por Go.
	v.Tapped(&fyne.PointEvent{Position: fyne.NewPos(327, 146.7)})
	if d.v.parada != 0 {
		t.Error("un clic antes del borde superior elige la segunda región")
	}
	v.Tapped(&fyne.PointEvent{Position: fyne.NewPos(327, 148)})
	if d.v.parada != 1 {
		t.Error("un clic dentro de la segunda región no la elige")
	}
}

func vistaCajas(v *vistaDiferencias) []fyne.CanvasObject {
	cajas := test.WidgetRenderer(v).(*vistaDiferenciasRenderer).cajas
	out := make([]fyne.CanvasObject, 0, len(cajas))
	for _, c := range cajas {
		out = append(out, c)
	}
	return out
}

// TestPulsarUnaCajaVaALaRegion: el clic sobre una región la elige, y navegar con
// Siguiente da la vuelta.
func TestPulsarUnaCajaVaALaRegion(t *testing.T) {
	d, w := visualDePrueba(t)
	defer w.Close()
	objs := visualObjetos(w)
	v, _ := primero[*vistaDiferencias](objs)
	s := v.Size()
	// El centro de la segunda caja (x 290..370, y 130..144 de 595×842).
	v.Tapped(&fyne.PointEvent{Position: fyne.NewPos(float32(330.0/595)*s.Width, float32(137.0/842)*s.Height)})
	if d.v.parada != 1 {
		t.Errorf("tras pulsar la segunda caja, la parada es %d", d.v.parada)
	}
	// Fuera de toda caja no cambia nada.
	v, _ = primero[*vistaDiferencias](visualObjetos(w))
	v.Tapped(&fyne.PointEvent{Position: fyne.NewPos(1, 1)})
	if d.v.parada != 1 {
		t.Errorf("pulsar en blanco movió la parada a %d", d.v.parada)
	}
	paradas := paradasDe(d.v.res.Paginas)
	for i := 0; i < len(paradas); i++ {
		d.v.parada = moverParada(d.v.parada, len(paradas), 1)
	}
	if d.v.parada != 1 {
		t.Errorf("dar la vuelta entera no vuelve al sitio: %d", d.v.parada)
	}
}

// TestCambiarDePestanaNoBorraLaRuta: el campo es el mismo en Texto y en Visual.
func TestCambiarDePestanaNoBorraLaRuta(t *testing.T) {
	p, w, _ := dialogoDePrueba(t, DialogoComparar)
	defer w.Close()
	c := p.dialogo
	comp := &dialogoComparar{a: c}
	c.pintar = comp.vista
	c.rehacer()
	comp.otro.PonerTexto("/PDF/otro.pdf")
	comp.modo = "visual"
	c.rehacer()
	if got := comp.otro.Texto(); got != "/PDF/otro.pdf" {
		t.Errorf("al pasar a Visual el campo quedó en %q", got)
	}
	e, _ := primero[*ui.Entrada](visualObjetos(w))
	if e == nil || e.Texto() != "/PDF/otro.pdf" {
		t.Error("el campo que se ve no es el de la ruta")
	}
}
