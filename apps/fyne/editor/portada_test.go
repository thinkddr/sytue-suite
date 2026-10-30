// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestLasPiezasDeLaPortadaMidenLoQueLaHoja ata las medidas de la portada a las clases de
// `DocList.tsx` con la raíz de 16. NO son medidas de producción: la sonda del piloto no
// se ha pasado sobre esta pantalla (ver el informe). Cada número lleva su cuenta.
func TestLasPiezasDeLaPortadaMidenLoQueLaHoja(t *testing.T) {
	casos := []struct {
		que         string
		got, quiero float32
	}{
		// 1 + h-24 (96) + p-3·2 (24) + 22 (texto raíz) + gap-0.5 (2) + 18 (text-sm) + 1.
		{"la tesela de documento", MedidaTesela(), 164},
		// 1 + p-4·2 (32) + h-11 (44) + 1: la tesela de color manda sobre las dos líneas (35).
		{"el acceso «Crear nuevo»", MedidaAccesoNuevo(), 78},
		{"el filtro de destacados", MedidaFiltro("Destacados").Height, 32},
		{"el botón de la tesela", LadoBotonTesela, 28},
		{"el margen de la portada", MargenPortada, 24},
		{"el lema, como mucho", AnchoLema, 672},
		{"el buscador", AnchoBuscador, 256},
	}
	for _, c := range casos {
		if c.got != c.quiero {
			t.Errorf("%s mide %v y la hoja da %v", c.que, c.got, c.quiero)
		}
	}
}

// TestLasColumnasSonLasDeLaCuentaDeLaWeb: GAP_PX y gap-4 son ambos 16 con
// raíz de 16. Los umbrales de columna proceden de DocList, no del layout nativo.
func TestLasColumnasSonLasDeLaCuentaDeLaWeb(t *testing.T) {
	for _, c := range []struct {
		ancho float32
		cols  int
	}{
		{1232, 6}, // 1280 de ventana menos el px-6 a ambos lados
		{1155, 5},
		{180, 1},
		{375, 1},
		{376, 2},
		{0, 1},
	} {
		if got := ColumnasPortada(c.ancho); got != c.cols {
			t.Errorf("a %v de ancho salen %d columnas y la web pinta %d", c.ancho, got, c.cols)
		}
	}
}

// TestLaTeselaPintaSuBandaYSuBorde pinta una tesela a ×2 y mira los píxeles: el borde,
// la banda `bg-subtle`, su filete inferior y el cuerpo en `surface`.
func TestLaTeselaPintaSuBandaYSuBorde(t *testing.T) {
	d := api.Document{ID: "d1", Kind: api.DocText, Title: "Propuesta", UpdatedAt: time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)}
	s := NuevaTeselaDoc(d, nil, "es", AccionesTesela{})
	s.Resize(fyne.NewSize(200, MedidaTesela()))
	img := pinta(t, 200, MedidaTesela(), s)
	v := tema.Claro
	casos := []struct {
		que   string
		x, y  float32
		token string
	}{
		{"el borde de arriba", 100, 0.25, "border"},
		{"la banda", 100, 20, "bg-subtle"},
		{"el filete de la banda", 100, 96.25, "border"},
		{"el cuerpo", 190, 130, "surface"},
		{"el borde de abajo", 100, MedidaTesela() - 0.25, "border"},
	}
	for _, c := range casos {
		got := px(img, int(c.x*escala), int(c.y*escala))
		if quiero := tema.Color(c.token, v); !cerca(got, quiero) {
			t.Errorf("%s: pinta %v y la hoja pide %s = %v", c.que, got, c.token, quiero)
		}
	}
	// El glifo del tipo va en el color del PRODUCTO: tiene que haber tinta de
	// `product-docs` en la banda.
	azul := tema.Color("product-docs", v)
	hay := false
	for y := 20 * int(escala); y < 70*int(escala) && !hay; y++ {
		for x := 80 * int(escala); x < 120*int(escala); x++ {
			if cerca(px(img, x, y), azul) {
				hay = true
				break
			}
		}
	}
	if !hay {
		t.Error("la banda no lleva el glifo en el color del producto")
	}
}

// sesionFija entrega siempre el mismo token.
type sesionFija struct{}

func (sesionFija) AccessToken(context.Context) (string, error) { return "t", nil }
func (sesionFija) Invalidate(string)                           {}

// servicio es un services/docs de mentira que cuenta lo que le piden.
type servicio struct {
	mu       sync.Mutex
	pedido   []string
	lista    string
	estado   int
	estrella int
	// papelera es lo que contesta GET /trash, y restaurar/purgar/vaciar con esos
	// estados (0 = bien).
	papelera                     string
	restaurar, purgar, vaciarSig int
}

func (s *servicio) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pedido = append(s.pedido, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/documents":
		if s.estado != 0 {
			w.WriteHeader(s.estado)
			return
		}
		_, _ = io.WriteString(w, s.lista)
	case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/star"):
		if s.estrella != 0 {
			w.WriteHeader(s.estrella)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/trash":
		_, _ = io.WriteString(w, s.papelera)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/trash/"):
		if s.restaurar != 0 {
			w.WriteHeader(s.restaurar)
			_, _ = io.WriteString(w, `{"error":{"code":"file_name_taken","message":"x"}}`)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/trash/"):
		if s.purgar != 0 {
			w.WriteHeader(s.purgar)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/trash":
		// La primera página deja algo detrás; la segunda termina.
		s.vaciarSig++
		if s.vaciarSig == 1 {
			_, _ = io.WriteString(w, `{"purged":50,"failed":1,"next":"2026-09-28T10:00:00Z|x"}`)
			return
		}
		_, _ = io.WriteString(w, `{"purged":3,"failed":0,"next":""}`)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/documents":
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"document":{"id":"nuevo","kind":"document","title":""},"role":"owner"}`)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *servicio) ultimo() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pedido) == 0 {
		return ""
	}
	return s.pedido[len(s.pedido)-1]
}

// portadaDePrueba monta una portada contra el servicio de mentira, con las peticiones en
// el mismo hilo para que la prueba no tenga que esperar a nadie.
func portadaDePrueba(t *testing.T, s *servicio) *Portada {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	p := NuevaPortada(Documento, nil, "es", api.NewDocs(srv.URL, sesionFija{}, srv.Client()))
	p.Lanzar = func(f func()) { f() }
	return p
}

const dosDocs = `{"documents":[` +
	`{"id":"a","kind":"document","title":"Acta","starred":false,"updated_at":"2026-09-21T10:00:00Z"},` +
	`{"id":"b","kind":"document","title":"Borrador","starred":true,"updated_at":"2026-09-20T10:00:00Z"}` +
	`],"next_cursor":"c2"}`

// TestLaPortadaPideLoQuePideLaWeb: el tipo del producto, la página de 100 y, con el
// filtro, `starred=1` y la consulta. Es la misma petición que hace `DocList.tsx`.
func TestLaPortadaPideLoQuePideLaWeb(t *testing.T) {
	s := &servicio{lista: dosDocs}
	p := portadaDePrueba(t, s)
	p.Cargar()
	if got := s.ultimo(); got != "GET /api/v1/documents?kind=document&limit=100" {
		t.Errorf("la primera carga pide %q", got)
	}
	if p.Estado != Listo || len(p.Docs) != 2 || p.Cursor != "c2" {
		t.Fatalf("tras cargar: estado %v, %d documentos, cursor %q", p.Estado, len(p.Docs), p.Cursor)
	}
	p.Consulta = "  acta "
	p.aplicarConsulta()
	p.AlternarDestacados()
	if got := s.ultimo(); got != "GET /api/v1/documents?kind=document&limit=100&q=acta&starred=1" {
		t.Errorf("con filtro pide %q", got)
	}
	p.Mas()
	if got := s.ultimo(); got != "GET /api/v1/documents?cursor=c2&kind=document&limit=100&q=acta&starred=1" {
		t.Errorf("la página siguiente pide %q", got)
	}
	if len(p.Docs) != 2 {
		t.Errorf("la página siguiente repite los mismos documentos y no se deduplican: %d", len(p.Docs))
	}
}

// TestCadaFalloTieneSuEstado: el 401 es la sesión, el 403 el permiso, el resto un error,
// y una lista vacía es «vacío», no un error.
func TestCadaFalloTieneSuEstado(t *testing.T) {
	for _, c := range []struct {
		estado int
		lista  string
		quiero Estado
	}{
		{http.StatusUnauthorized, "", SinSesion},
		{http.StatusForbidden, "", SinPermiso},
		{http.StatusInternalServerError, "", Error},
		{0, `{"documents":[]}`, Vacio},
	} {
		s := &servicio{estado: c.estado, lista: c.lista}
		p := portadaDePrueba(t, s)
		p.Cargar()
		if p.Estado != c.quiero {
			t.Errorf("con HTTP %d la portada queda en %v y tendría que ser %v", c.estado, p.Estado, c.quiero)
		}
		// Pintar el estado no entra en pánico: un token inventado revienta AL PINTAR.
		p.Objeto().Resize(fyne.NewSize(900, 700))
		pinta(t, 900, 700, p.Objeto())
	}
}

// TestLaEstrellaVuelveSiElServicioDiceQueNo: destacar es optimista y REVERSIBLE.
func TestLaEstrellaVuelveSiElServicioDiceQueNo(t *testing.T) {
	s := &servicio{lista: dosDocs}
	p := portadaDePrueba(t, s)
	p.Cargar()
	p.Destacar(p.Docs[0])
	if !p.Docs[0].Starred {
		t.Fatal("con el servicio de acuerdo, la estrella se queda puesta")
	}
	s.mu.Lock()
	s.estrella = http.StatusInternalServerError
	s.mu.Unlock()
	p.Destacar(p.Docs[1])
	if !p.Docs[1].Starred {
		t.Error("el servicio dijo que no y la estrella se ha quedado quitada: la tesela miente")
	}
}

// TestConElFiltroQuitarLaEstrellaSacaLaTesela: es lo que se está mirando.
func TestConElFiltroQuitarLaEstrellaSacaLaTesela(t *testing.T) {
	s := &servicio{lista: dosDocs}
	p := portadaDePrueba(t, s)
	p.Destacados = true
	p.Cargar()
	p.Destacar(p.Docs[1])
	for _, d := range p.Docs {
		if d.ID == "b" {
			t.Error("con «Destacados» puesto, el documento al que se le quita la estrella sigue en la lista")
		}
	}
}

// TestCrearAbreElDocumentoNuevo: la portada crea del tipo del producto y lo abre.
func TestCrearAbreElDocumentoNuevo(t *testing.T) {
	s := &servicio{lista: dosDocs}
	p := portadaDePrueba(t, s)
	var abierto api.Document
	p.AlAbrir = func(d api.Document) { abierto = d }
	p.Crear()
	if got := s.ultimo(); got != "POST /api/v1/documents?" {
		t.Errorf("crear pide %q", got)
	}
	if abierto.ID != "nuevo" {
		t.Errorf("tras crear se abre %q", abierto.ID)
	}
	if p.Creando {
		t.Error("la portada se queda en «Creando…»")
	}
}

// TestLaPortadaSePinta pinta la portada con documentos a 1280 de ancho y guarda la
// captura con `SYTUE_CAPTURA`: es como se MIRA la pantalla.
func TestLaPortadaSePinta(t *testing.T) {
	s := &servicio{lista: dosDocs}
	p := portadaDePrueba(t, s)
	p.Cargar()
	p.Objeto().Resize(fyne.NewSize(1280, 744))
	img := pinta(t, 1280, 744, p.Objeto())
	// El fondo de la página es `bg`, no `surface`: la lista va sobre el papel de la
	// suite.
	if got, quiero := px(img, 5, 5), tema.Color("bg", tema.Claro); !cerca(got, quiero) {
		t.Errorf("el fondo de la portada es %v y la hoja pide bg = %v", got, quiero)
	}
}

// TestA360RecientesNoQuedaTapado: a 360 el buscador de «Recientes» (`w-64 max-w-[60%]`)
// encoge como el hijo flex que es, y «Destacados» se queda a la derecha del encabezado
// y de su insignia. Antes medía el 60 % entero y «Destacados» caía encima de
// «Recientes» (visto en una ventana real el 2026-09-27). VISTO EN ROJO con el 60 % fijo.
func TestA360RecientesNoQuedaTapado(t *testing.T) {
	for _, loc := range []i18n.Locale{"es", "en"} {
		t.Run(string(loc), func(t *testing.T) {
			s := &servicio{lista: dosDocs}
			tr, err := i18n.New(loc, i18n.Common(), catalogos.Docs())
			if err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(s)
			defer srv.Close()
			p := NuevaPortada(Documento, tr.T, loc, api.NewDocs(srv.URL, sesionFija{}, srv.Client()))
			p.Lanzar = func(f func()) { f() }
			p.Cargar()
			ventana := test.NewWindow(p.Objeto())
			defer ventana.Close()
			ventana.SetPadded(false)
			ventana.Resize(fyne.NewSize(360, 740))
			w := p.vista
			fin := w.insignia.Position().X + w.insignia.Size().Width
			if x := w.filtro.Position().X; x < fin {
				t.Errorf("%s empieza en %g y la insignia termina en %g", w.filtro.Rotulo, x, fin)
			}
			if b := w.buscador; b.Position().X+b.Size().Width > 360 {
				t.Errorf("el buscador se sale: termina en %g", b.Position().X+b.Size().Width)
			}
			// DocList ES real a360: px-6=24, filtro x141.32813 y buscador
			// x260.14063/ancho75.85938. Conserva el margen de2 px de fuente.
			q := map[i18n.Locale][3]float32{
				"es": {141.32813, 260.14063, 75.85938},
				"en": {121.375, 214.82813, 121.17188},
			}[loc]
			if abs32(w.filtro.Position().X-q[0]) > 2 || abs32(w.buscador.Position().X-q[1]) > 2 || abs32(w.buscador.Size().Width-q[2]) > 2 {
				t.Errorf("portada %s: filtro %v, buscador %v/%v; difieren del DOM", loc, w.filtro.Position(), w.buscador.Position(), w.buscador.Size())
			}
		})
	}
}

func TestLaPortadaConservaElBuscadorAlGirar(t *testing.T) {
	p := portadaDePrueba(t, &servicio{lista: dosDocs})
	p.Cargar()
	w := test.NewTempWindow(t, p.Objeto())
	w.SetPadded(false)
	p.vista.buscador.AlCambiar(nil) // este test gira el campo; no programa peticiones
	p.vista.buscador.PonerTexto("acta")
	for _, ancho := range []float32{320, 568, 1024, 320} {
		w.Resize(fyne.NewSize(ancho, 700))
		buscador, filtro := p.vista.buscador, p.vista.filtro
		if buscador.Size().Width < min(AnchoBuscador/2, ancho-2*MargenPortada) || buscador.Texto() != "acta" {
			t.Fatalf("a %g: el buscador perdió su área o consulta: %v/%q", ancho, buscador.Size(), buscador.Texto())
		}
		if ancho == 320 && buscador.Position().Y < filtro.Position().Y+filtro.Size().Height {
			t.Fatal("en el teléfono el buscador no baja a su propia fila")
		}
	}
}
