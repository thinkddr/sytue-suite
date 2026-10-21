// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/office"
)

type sesionFija struct{}

func (sesionFija) AccessToken(context.Context) (string, error) { return "t", nil }
func (sesionFija) Invalidate(string)                           {}

// servicioHojas contesta la ficha y el estado de UNA hoja, con el papel que se le diga,
// y apunta lo que se guarda.
type servicioHojas struct {
	mu      sync.Mutex
	rol     string
	hoja    string
	lamport uint64
	dot     string // el reloj del registro; vacío, el de la web
	ops     []string
	creados []string // los cuerpos de las altas
	orden   []string // guardados y exportaciones, en el orden en que llegan
}

func (s *servicioHojas) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch r.Method + " " + r.URL.Path {
	case "GET /api/v1/documents/h1":
		_, _ = io.WriteString(w, `{"document":{"id":"h1","kind":"spreadsheet","title":"Cuentas"},"role":"`+s.rol+`"}`)
	case "GET /api/v1/documents/h1/state":
		valor, _ := json.Marshal(s.hoja)
		l, dot := itoa(s.lamport), s.dot
		if dot == "" {
			dot = `{"a":"web","c":2}`
		}
		_, _ = io.WriteString(w, `{"seq":3,"state":{"lamport":`+l+`,"registers":{"doc:sheet":{"value":`+
			string(valor)+`,"dot":`+dot+`,"lamport":`+l+`}}}}`)
	case "POST /api/v1/documents/h1/ops":
		cuerpo, _ := io.ReadAll(r.Body)
		s.ops = append(s.ops, string(cuerpo))
		var op struct {
			Ops []struct {
				Value   string          `json:"value"`
				Lamport uint64          `json:"lamport"`
				Dot     json.RawMessage `json:"dot"`
			} `json:"ops"`
		}
		_ = json.Unmarshal(cuerpo, &op)
		if len(op.Ops) > 0 {
			s.hoja, s.lamport, s.dot = op.Ops[0].Value, op.Ops[0].Lamport, string(op.Ops[0].Dot)
		}
		s.orden = append(s.orden, "guardar")
		_, _ = io.WriteString(w, `{"seq":4}`)
	case "GET /api/v1/documents":
		_, _ = io.WriteString(w, `{"documents":[]}`)
	case "POST /api/v1/documents/h1/exports":
		cuerpo, _ := io.ReadAll(r.Body)
		s.orden = append(s.orden, "export "+string(cuerpo))
		_, _ = io.WriteString(w, `{"export":{"id":"e1"}}`)
	case "GET /api/v1/documents/h1/exports/e1":
		_, _ = io.WriteString(w, "contenido exportado")
	case "POST /api/v1/documents":
		cuerpo, _ := io.ReadAll(r.Body)
		s.creados = append(s.creados, string(cuerpo))
		_, _ = io.WriteString(w, `{"document":{"id":"h2","kind":"spreadsheet","title":""},"role":"owner"}`)
	case "GET /api/v1/documents/h2":
		_, _ = io.WriteString(w, `{"document":{"id":"h2","kind":"spreadsheet","title":""},"role":"owner"}`)
	case "GET /api/v1/documents/h2/state":
		_, _ = io.WriteString(w, `{"seq":0,"state":{"lamport":0,"registers":{}}}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func itoa(n uint64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func appDePrueba(t *testing.T, s *servicioHojas) *App {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	a := NuevaApp(traductorPlay(t), "es", api.NewDocs(srv.URL, sesionFija{}, srv.Client()))
	a.lanzar = func(f func()) { f() }
	a.portada.Lanzar = a.lanzar
	return a
}

// TestCalcusAbreEditaYGuardaEnElServicio: la hoja del servicio llega al lienzo dentro
// del cromo, se edita, y lo que se guarda es el `doc:sheet` entero con el formato.
func TestCalcusAbreEditaYGuardaEnElServicio(t *testing.T) {
	s := &servicioHojas{
		rol: "editor", lamport: 4,
		hoja: `{"active":0,"sheets":[{"name":"Datos","cells":{"A1":"10"},"formats":{"A1":{"id":"currency"}}}]}`,
	}
	a := appDePrueba(t, s)
	a.AbrirID("h1", "")
	ab := a.abierto
	if ab.ctx.Estado != editor.Listo || ab.ctx.SoloLectura || ab.ctx.Titulo != "Cuentas" {
		t.Fatalf("la hoja se abre con estado %v, solo lectura %v y título %q", ab.ctx.Estado, ab.ctx.SoloLectura, ab.ctx.Titulo)
	}
	if got := ab.lz.Libro.Texto(0, 0); got != "10,00\u00a0€" {
		t.Errorf("A1 se ve %q", got)
	}
	ab.lz.Rejilla.Seleccionar(1, 0, false)
	ab.lz.Rejilla.Escribir("=A1*2")
	if ab.ctx.Guardado != editor.Sucio {
		t.Errorf("tras escribir, el chip dice %v y no «sin guardar»", ab.ctx.Guardado)
	}
	ab.auto.Parar() // el reloj no se espera en la prueba: se guarda a mano
	if err := a.guardar(ab); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	guardado := s.hoja
	s.mu.Unlock()
	if !strings.Contains(guardado, `"B1":"=A1*2"`) || !strings.Contains(guardado, `"currency"`) {
		t.Errorf("lo guardado no lleva la celda nueva y el formato: %s", guardado)
	}
	// Y un segundo guardado va encima del primero, no choca con él.
	ab.lz.Rejilla.Escribir("=A1*3")
	ab.auto.Parar() // y el reloj que ha armado no sobrevive a la prueba
	if err := a.guardar(ab); err != nil {
		t.Fatalf("el segundo guardado choca con el primero: %v", err)
	}
}

// TestVolverGuardaLoQueQuedaPorGuardar: lo escrito justo antes de volver a la portada
// sale al servicio, como en Docus y Diapos (y el `flush` de la web al irse de la página).
// Antes `Volver` desarmaba el reloj sin guardar.
func TestVolverGuardaLoQueQuedaPorGuardar(t *testing.T) {
	s := &servicioHojas{rol: "editor", lamport: 4, hoja: `{"active":0,"sheets":[{"name":"Datos","cells":{"A1":"10"}}]}`}
	a := appDePrueba(t, s)
	w := test.NewWindow(a.Objeto())
	defer func() { w.Close(); fyne.DoAndWait(func() {}) }()
	a.Arrancar()
	a.AbrirID("h1", "")
	ab := a.abierto
	ab.lz.Rejilla.Seleccionar(1, 0, false)
	ab.lz.Rejilla.Escribir("=A1*2")
	w.Canvas().OnTypedKey()(&fyne.KeyEvent{Name: mobile.KeyBack})
	s.mu.Lock()
	guardado := s.hoja
	s.mu.Unlock()
	if !strings.Contains(guardado, `"B1":"=A1*2"`) {
		t.Errorf("al volver a la portada lo último escrito no se ha guardado: %s", guardado)
	}
}

// TestCalcusAbreEnSoloLecturaLoQueNoPuedeEscribir: con papel de lector, o con algo que
// el lienzo no sabe reescribir, la hoja se ve y no se escribe ni se guarda.
func TestCalcusAbreEnSoloLecturaLoQueNoPuedeEscribir(t *testing.T) {
	for _, c := range []struct {
		nombre, rol, hoja string
	}{
		{"lector", "viewer", `{"active":0,"sheets":[{"name":"H","cells":{"A1":"x"}}]}`},
		{"con una clave desconocida", "editor", `{"active":0,"sheets":[{"name":"H","cells":{"A1":"x"},"futuro":[{"id":"g"}]}]}`},
	} {
		a := appDePrueba(t, &servicioHojas{rol: c.rol, hoja: c.hoja, lamport: 1})
		a.AbrirID("h1", "")
		ab := a.abierto
		if !ab.ctx.SoloLectura || !ab.lz.Rejilla.SoloLectura || ab.auto != nil {
			t.Errorf("%s: se abre para escribir", c.nombre)
		}
		if ab.lz.Libro.Crudo(0, 0) != "x" {
			t.Errorf("%s: no se ve lo que hay", c.nombre)
		}
	}
}

// TestLaCintaDeCalcusEsLaDeLaWeb: las pestañas de una hoja son Inicio, Datos, Fórmulas,
// Revisar y Vista —las de la taxonomía común con la web, `office.PestanasDe`—, y sus
// grupos, los de InicioSheets.tsx, Datos.tsx y FormulasTab en su orden.
func TestLaCintaDeCalcusEsLaDeLaWeb(t *testing.T) {
	lz := lienzoDePrueba()
	r := Registro(lz)
	ids := make([]string, 0, 4)
	for _, p := range r.Visibles(editor.Hoja) {
		ids = append(ids, p.ID)
	}
	if strings.Join(ids, ",") != "inicio,datos,formulas,revisar,vista" {
		t.Errorf("las pestañas de una hoja son %v", ids)
	}
	// Y son EXACTAMENTE las de la taxonomía común, en su orden: ni una menos, ni una más.
	comun := office.PestanasDe(office.Hoja)
	taxonomia := make([]string, 0, len(comun))
	for _, p := range comun {
		taxonomia = append(taxonomia, p.ID)
	}
	if strings.Join(ids, ",") != strings.Join(taxonomia, ",") {
		t.Errorf("la cinta de Calcus pinta %v y la taxonomía común dice %v", ids, taxonomia)
	}
	ctx := &editor.Contexto{Tipo: editor.Hoja, Lienzo: lz}
	grupos := func(cuerpo fyne.CanvasObject) []string {
		var out []string
		for _, o := range cuerpo.(*fyne.Container).Objects {
			if g, ok := o.(*editor.Grupo); ok {
				out = append(out, g.Rotulo)
			}
		}
		return out
	}
	vis := r.Visibles(editor.Hoja)
	if got := strings.Join(grupos(vis[0].Cuerpo(ctx)), ","); got != "group.font,group.alignment,group.number,group.styles,group.merge" {
		t.Errorf("los grupos de Inicio son %s", got)
	}
	quiero := "group.externalData,group.sortFilter,settings.table,names.group,group.tools,group.cells,slides.box.chart," +
		"extdata.step.review,rail.outline.title,group.window,group.protect"
	if got := strings.Join(grupos(vis[1].Cuerpo(ctx)), ","); got != quiero {
		t.Errorf("los grupos de Datos son %s", got)
	}
	// «Fórmulas»: el análisis de hipótesis (el asistente solo sale con servicio de IA).
	if got := strings.Join(grupos(vis[2].Cuerpo(ctx)), ","); got != "group.whatIf" {
		t.Errorf("los grupos de Fórmulas son %s", got)
	}
	// Y los botones mandan: la negrita de la cinta llega a la celda.
	lz.Libro.Escribir(0, 0, "x")
	for _, o := range vis[0].Cuerpo(ctx).(*fyne.Container).Objects {
		if g, ok := o.(*editor.Grupo); ok && g.Rotulo == "group.font" {
			for _, b := range g.Controles {
				if bc, ok := b.(*editor.BotonCinta); ok && bc.Rotulo == "format.bold" {
					bc.Tapped(nil)
				}
			}
		}
	}
	if b, _, _ := lz.Libro.Formato(0, 0); !b {
		t.Error("la negrita de la cinta no llega a la celda")
	}
}

// TestElEnlaceDeEditarAbreLaHoja: `sytue-sheets:/edit?id=…` y su ruta con `/docs`.
func TestElEnlaceDeEditarAbreLaHoja(t *testing.T) {
	for enlace, quiero := range map[string]string{
		"sytue-sheets:/edit?id=h1": "h1", "sytue-sheets:/calcus/edit?id=h2": "h2",
		"sytue-docs:/edit?id=h3": "", "sytue-sheets:/open-from-drive?file=x": "",
	} {
		u, _ := url.Parse(enlace)
		if got := IDDeEnlace(u); got != quiero {
			t.Errorf("IDDeEnlace(%s) = %q", enlace, got)
		}
	}
}

// TestCalcusEnteroSePinta pinta la app con una hoja abierta a ancho de escritorio y de
// móvil. Con `SYTUE_CAPTURA` deja los PNG para mirarlos al lado de la web.
func TestCalcusEnteroSePinta(t *testing.T) {
	s := &servicioHojas{rol: "editor", lamport: 1, hoja: `{"active":0,"sheets":[{"name":"Ventas","cells":{` +
		`"A1":"<b>Mes</b>","B1":"<b>Importe</b>","A2":"Enero","B2":"1200","A3":"Febrero","B3":"980",` +
		`"A4":"<b><i>Total</i></b>","B4":"=SUM(B2:B3)"},"formats":{"B2":{"id":"currency"},"B3":{"id":"currency"},"B4":{"id":"currency"}},` +
		`"styles":{"A1":{"fill":"#E2E8F0"},"B1":{"fill":"#E2E8F0"},"B4":{"border":{"t":true}}}},{"name":"Resumen","cells":{}}]}`}
	for _, tam := range []fyne.Size{fyne.NewSize(1280, 800), fyne.NewSize(360, 740)} {
		a := appDePrueba(t, s)
		a.AbrirID("h1", "")
		o := a.Objeto()
		o.Resize(tam)
		img := pinta(t, tam.Width, tam.Height, o)
		if destino := os.Getenv("SYTUE_CAPTURA"); destino != "" {
			t.Setenv("SYTUE_CAPTURA", strings.TrimSuffix(destino, ".png")+"-"+itoa(uint64(tam.Width))+".png")
			guarda(t, img)
			t.Setenv("SYTUE_CAPTURA", destino)
		}
		if got := a.abierto.lz.Libro.Texto(1, 3); got != "2180,00\u00a0€" && got != "2.180,00\u00a0€" {
			t.Errorf("el total se ve %q", got)
		}
	}
}

// TestElMenuArchivoDeCalcusCreaYAbre: «Nuevo hoja de cálculo» da de alta una hoja en
// blanco en el servicio y la abre para escribir; «Abrir un archivo» lleva al `cargar` de
// «Abrir con» lo que es de hoja —también el `.csv`, que la portada de la web importa—,
// al sistema lo de otro editor, y lo que no es de nadie se dice.
func TestElMenuArchivoDeCalcusCreaYAbre(t *testing.T) {
	s := &servicioHojas{rol: "editor", lamport: 1, hoja: `{"active":0,"sheets":[{"name":"H","cells":{"A1":"x"}}]}`}
	a := appDePrueba(t, s)
	var cargados, fuera []string
	a.PonerAbrir(func(listo func([]string)) {
		listo([]string{"/tmp/Ventas.csv", "/tmp/Cuentas.xlsx", "/tmp/Acta.docx", "/tmp/copia.zip"})
	}, func(_ context.Context, ap api.Apertura) error {
		cargados = append(cargados, ap.Local)
		return nil
	}, func(r string) error { fuera = append(fuera, r); return nil })
	a.abrirArchivo.Lanzar = a.lanzar
	if a.portada.AlAbrirArchivo == nil {
		t.Error("la portada no ofrece «Abrir un archivo»")
	}
	a.AbrirID("h1", "")
	ctx := a.abierto.ctx
	if ctx.Nuevo == nil || ctx.Abrir == nil {
		t.Fatal("el menú Archivo de la hoja no lleva «Nuevo» y «Abrir»")
	}
	ctx.Abrir()
	if strings.Join(cargados, ",") != "file:///tmp/Ventas.csv,file:///tmp/Cuentas.xlsx" ||
		strings.Join(fuera, ",") != "/tmp/Acta.docx" {
		t.Errorf("a Calcus llega %v y al sistema %v", cargados, fuera)
	}
	ctx.Nuevo()
	s.mu.Lock()
	creados := s.creados
	s.mu.Unlock()
	ab := a.abierto
	if len(creados) != 1 || !strings.Contains(creados[0], `"kind":"spreadsheet"`) {
		t.Fatalf("«Nuevo» no da de alta una hoja: %v", creados)
	}
	if ab.id != "h2" || ab.ctx.SoloLectura || ab.ctx.Estado != editor.Listo || len(ab.lz.Libro.Hojas()) != 1 ||
		ab.lz.Libro.Crudo(0, 0) != "" {
		t.Errorf("la hoja nueva se abre con id %q, solo lectura %v, %d hojas y A1 %q", ab.id, ab.ctx.SoloLectura,
			len(ab.lz.Libro.Hojas()), ab.lz.Libro.Crudo(0, 0))
	}
}

// TestDescargarUnaHojaGuardaAntesYDejaElFichero: «Descargar → Excel» guarda primero lo
// pendiente —el servicio exporta desde SU estado—, pide el formato y deja el fichero en
// la carpeta de descargas con el nombre del título; mientras, las descargas se apagan.
func TestDescargarUnaHojaGuardaAntesYDejaElFichero(t *testing.T) {
	s := &servicioHojas{rol: "viewer", lamport: 1, hoja: `{"active":0,"sheets":[{"name":"H","cells":{"A1":"1"}}]}`}
	a := appDePrueba(t, s)
	dir := t.TempDir()
	a.carpeta = func() (string, error) { return dir, nil }
	a.AbrirID("h1", "")
	ctx := a.abierto.ctx
	if ctx.Exportar == nil || ctx.GuardarVersion != nil || ctx.Versiones == nil {
		t.Fatal("un lector descarga y ve el historial, pero no guarda versiones")
	}
	// Con el cerrojo: la franja del lector sigue preguntando al servicio en segundo plano
	// (EdicionPedida lee `rol`), y escribirlo a pelo es una carrera con -race.
	s.mu.Lock()
	s.rol = "editor"
	s.mu.Unlock()
	a.AbrirID("h1", "")
	ab := a.abierto
	ab.lz.Rejilla.Escribir("2")
	ab.auto.Parar()
	ab.ctx.Exportar("ooxml")
	s.mu.Lock()
	orden := strings.Join(s.orden, "|")
	s.mu.Unlock()
	if orden != `guardar|export {"format":"ooxml"}` {
		t.Errorf("al servicio le llega %q: primero se guarda y luego se exporta", orden)
	}
	datos, err := os.ReadFile(filepath.Join(dir, "Cuentas.xlsx"))
	if err != nil || string(datos) != "contenido exportado" {
		t.Errorf("la descarga no está en su sitio: %v", err)
	}
	if ab.ctx.Exportando != "" || ab.ctx.Guardado != editor.Limpio || ab.ctx.GuardarVersion == nil {
		t.Errorf("tras descargar: exportando %q, chip %v", ab.ctx.Exportando, ab.ctx.Guardado)
	}
}
