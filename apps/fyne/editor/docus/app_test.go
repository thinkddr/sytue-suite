// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"context"
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/driver/software"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

type sesionFija struct{}

func (sesionFija) AccessToken(context.Context) (string, error) { return "t", nil }
func (sesionFija) Invalidate(string)                           {}

// servicioDocs contesta la ficha y el estado de UN documento, con el papel que se le
// diga, y apunta los renombrados.
type servicioDocs struct {
	mu        sync.Mutex
	rol       string
	html      string
	fallo     int
	renombres []string
	// autor es quien escribió el registro por última vez; cambiarlo es que otro guardó.
	autor      string
	ops        []string
	comentados []string
	// miembros es la respuesta de la presencia y remotas, las ops de los demás que da el
	// sondeo (sin ellas, 404: la coedición sigue sin enterarse de nadie).
	miembros string
	remotas  string
}

func (s *servicioDocs) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallo != 0 {
		w.WriteHeader(s.fallo)
		return
	}
	switch r.Method + " " + r.URL.Path {
	case "GET /api/v1/documents/d1":
		_, _ = io.WriteString(w, `{"document":{"id":"d1","kind":"document","title":"Propuesta"},"role":"`+s.rol+`"}`)
	case "GET /api/v1/documents/d1/state":
		valor, _ := json.Marshal(s.html)
		autor := s.autor
		if autor == "" {
			autor = "web"
		}
		_, _ = io.WriteString(w, `{"seq":3,"state":{"lamport":4,"registers":{"doc:html":{"value":`+
			string(valor)+`,"dot":{"a":"`+autor+`","c":2},"lamport":4}}}}`)
	case "GET /api/v1/documents/d1/comments":
		_, _ = io.WriteString(w, `{"comments":[`+
			`{"id":"r1","parent_id":"c1","author_sub":"ana-123456789","body":"De acuerdo","created_at":"2026-09-20T11:00:00Z"},`+
			`{"id":"c1","author_sub":"luis-12345678","body":"¿Revisamos\nel alcance?","anchor":{"text":"Alcance","mark":"m1"},"created_at":"2026-09-20T10:00:00Z"}]}`)
	case "POST /api/v1/documents/d1/comments":
		cuerpo, _ := io.ReadAll(r.Body)
		s.comentados = append(s.comentados, string(cuerpo))
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"comment":{"id":"c2","body":"x"}}`)
	case "POST /api/v1/documents/d1/exports":
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"export":{"id":"b1","format":"ooxml"}}`)
	case "GET /api/v1/documents/d1/exports/b1":
		_, _ = io.WriteString(w, "PK!")
	case "POST /api/v1/documents/d1/ops":
		cuerpo, _ := io.ReadAll(r.Body)
		s.ops = append(s.ops, string(cuerpo))
		_, _ = io.WriteString(w, `{"seq":4}`)
	case "PUT /api/v1/documents/d1/presence":
		if s.miembros == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, `{"members":`+s.miembros+`}`)
	case "GET /api/v1/documents/d1/ops":
		if s.remotas == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, `{"ops":`+s.remotas+`}`)
	case "PATCH /api/v1/documents/d1":
		cuerpo, _ := io.ReadAll(r.Body)
		s.renombres = append(s.renombres, string(cuerpo))
		w.WriteHeader(http.StatusNoContent)
	case "GET /api/v1/documents":
		_, _ = io.WriteString(w, `{"documents":[]}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func appDePrueba(t *testing.T, s *servicioDocs) *App {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	a := NuevaApp(traductorPlay(t), "es", api.NewDocs(srv.URL, sesionFija{}, srv.Client()))
	a.lanzar = func(f func()) { f() }
	a.EnVivo = false
	a.portada.Lanzar = a.lanzar
	// Las pausas no esperan al reloj: se quedan en `pendiente` y la prueba las dispara.
	a.programar = func(_ time.Duration, f func()) func() {
		pendiente = f
		return func() { pendiente = nil }
	}
	return a
}

// pendiente es lo último que la app dejó programado; disparar lo corre.
var pendiente func()

func disparar(t *testing.T) {
	t.Helper()
	f := pendiente
	if f == nil {
		t.Fatal("no hay nada programado")
	}
	pendiente = nil
	f()
}

// TestAbrirTraeElDocumentoDelServicio: la ficha y el registro `doc:html` llegan al
// editor —el título a la cabecera, el HTML al folio— y el editor pasa a «listo».
func TestAbrirTraeElDocumentoDelServicio(t *testing.T) {
	s := &servicioDocs{rol: "editor", html: "<h1>Propuesta</h1><p>Hola <strong>mundo</strong></p>"}
	a := appDePrueba(t, s)
	a.AbrirID("d1", "")
	ab := a.abierto
	if ab == nil || ab.ctx.Estado != editor.Listo {
		t.Fatalf("el editor no queda listo: %+v", ab)
	}
	if ab.ctx.Titulo != "Propuesta" {
		t.Errorf("el título es %q y el documento se llama «Propuesta»", ab.ctx.Titulo)
	}
	if d := ab.folio.Documento(); !d.Editable() || len(d.Bloques()) != 2 {
		t.Errorf("el folio no tiene el documento del servicio: editable %v, %d bloques", d.Editable(), len(d.Bloques()))
	}
	if ab.base.Lamport != 4 || ab.base.Dot.Actor != "web" {
		t.Errorf("la base del guardado no es el registro leído: %+v", ab.base)
	}
	// Con permiso de escritura y un documento que se entiende entero, se escribe.
	if ab.ctx.SoloLectura {
		t.Error("un editor con un documento que se entiende se ha abierto en solo lectura")
	}
}

// TestSoloQuienEscribePuedeRenombrar: un lector no tiene con qué cambiar el título.
func TestSoloQuienEscribePuedeRenombrar(t *testing.T) {
	for _, c := range []struct {
		rol   string
		puede bool
	}{{"owner", true}, {"editor", true}, {"commenter", false}, {"viewer", false}} {
		a := appDePrueba(t, &servicioDocs{rol: c.rol})
		a.AbrirID("d1", "")
		if got := a.abierto.ctx.AlRenombrar != nil; got != c.puede {
			t.Errorf("con el papel %q renombrar es %v y tendría que ser %v", c.rol, got, c.puede)
		}
	}
}

// TestElTituloSeMandaTrasLaPausa: `mandarTitulo` es lo que la pausa deja para el final,
// y pone el chip en «Guardado».
func TestElTituloSeMandaTrasLaPausa(t *testing.T) {
	s := &servicioDocs{rol: "owner"}
	a := appDePrueba(t, s)
	a.AbrirID("d1", "")
	a.mandarTitulo(a.abierto, "Propuesta final")
	s.mu.Lock()
	got := append([]string(nil), s.renombres...)
	s.mu.Unlock()
	if len(got) != 1 || got[0] != `{"title":"Propuesta final"}` {
		t.Errorf("el renombrado manda %v", got)
	}
	if a.abierto.ctx.Guardado != editor.Limpio {
		t.Errorf("tras renombrar el chip dice %v", a.abierto.ctx.Guardado)
	}
}

// TestUnFalloCedeLaPantallaEntera: como en la web, si el documento no se puede abrir el
// editor desaparece y queda el aviso con «Reintentar» y «Volver».
func TestUnFalloCedeLaPantallaEntera(t *testing.T) {
	a := appDePrueba(t, &servicioDocs{fallo: http.StatusInternalServerError})
	a.AbrirID("d1", "")
	if a.abierto.ctx.Estado != editor.Error {
		t.Errorf("con un 500 el editor queda en %v", a.abierto.ctx.Estado)
	}
	if a.raiz.Objects[0] == fyne.CanvasObject(a.abierto.marco) {
		t.Error("con el documento sin abrir sigue enseñándose el editor vacío")
	}
	a.Volver()
	if a.raiz.Objects[0] != a.portada.Objeto() {
		t.Error("«Volver» no lleva a la portada")
	}
}

// TestElEditorAbiertoSePinta pinta Docus con un documento del servicio. Con
// `SYTUE_CAPTURA` deja la imagen.
func TestElEditorAbiertoSePinta(t *testing.T) {
	a := appDePrueba(t, &servicioDocs{rol: "owner", html: muestraReal(t)})
	a.AbrirID("d1", "")
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetContent(a.Objeto())
	c.SetScale(1)
	c.Resize(fyne.NewSize(1280, 744))
	img := c.Capture()
	if ruta := os.Getenv("SYTUE_CAPTURA_EDITOR"); ruta != "" {
		f, err := os.Create(ruta)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}
}

// abrirParaEscribir abre d1 con ese HTML y deja el cursor al final del primer bloque.
func abrirParaEscribir(t *testing.T, s *servicioDocs) (*App, *Folio) {
	t.Helper()
	a := appDePrueba(t, s)
	a.AbrirID("d1", "")
	f := a.abierto.folio
	f.Programar = func(_ time.Duration, fn func()) { fn() }
	f.mover(punto{0, f.largo(0)}, false)
	return a, f
}

// TestEscribirGuardaTrasLaPausa: escribir pone el chip en «Cambios sin guardar» y, al
// cumplirse la pausa, el HTML sale por el camino de la web. El documento no tenía
// secuencia y no hay nadie más: el primer guardado la SIEMBRA, con sus ops por carácter
// y la marca `:lww` (autoritativo); el segundo ya solo lleva lo que ha cambiado, sin
// marca, y un reloj que sigue subiendo.
func TestEscribirGuardaTrasLaPausa(t *testing.T) {
	s := &servicioDocs{rol: "editor", html: "<p>Hola</p>"}
	a, f := abrirParaEscribir(t, s)
	f.escribir("!")
	if a.abierto.ctx.Guardado != editor.Sucio {
		t.Errorf("tras escribir el chip dice %v", a.abierto.ctx.Guardado)
	}
	disparar(t)
	f.escribir("?")
	disparar(t)
	s.mu.Lock()
	ops := append([]string(nil), s.ops...)
	s.mu.Unlock()
	if len(ops) != 2 || !strings.Contains(ops[0], `"key":"doc:html","value":"\u003cp\u003eHola!\u003c/p\u003e"`) ||
		!strings.Contains(ops[0], `"key":"doc:html:lww"`) || strings.Count(ops[0], `"kind":"insert"`) != len("<p>Hola!</p>") {
		t.Fatalf("la siembra: %v", ops)
	}
	if strings.Contains(ops[1], ":lww") || strings.Count(ops[1], `"kind":"insert"`) != 1 || !strings.Contains(ops[1], `"value":"?"`) {
		t.Errorf("el segundo guardado lleva solo el «?»: %s", ops[1])
	}
	if a.abierto.ctx.Guardado != editor.Limpio || a.abierto.base.Value != "<p>Hola!?</p>" {
		t.Errorf("tras guardar: chip %v, base %+v", a.abierto.ctx.Guardado, a.abierto.base)
	}
}

// TestUnConflictoNoPisa: con alguien más dentro, un documento sin secuencia no se puede
// sembrar (a dos se duplicaría), así que se guarda el registro con la comprobación de
// delante: si otro ha guardado entretanto, lo escrito NO sale y el chip lo dice.
func TestUnConflictoNoPisa(t *testing.T) {
	s := &servicioDocs{rol: "owner", html: "<p>Hola</p>", miembros: `[{"conn":"web-1","sub":"ana"}]`}
	a, f := abrirParaEscribir(t, s)
	if a.abierto.presentes != 1 {
		t.Fatalf("la presencia dice %d personas más", a.abierto.presentes)
	}
	s.mu.Lock()
	s.autor = "otra-pestaña"
	s.mu.Unlock()
	f.escribir("!")
	disparar(t)
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.ops) != 0 {
		t.Errorf("con conflicto se ha guardado encima: %v", s.ops)
	}
	if a.abierto.ctx.Guardado != editor.FalloAlGuardar {
		t.Errorf("con conflicto el chip dice %v", a.abierto.ctx.Guardado)
	}
}

// TestLoQueEscribeOtroSeFusiona: tras sembrar, una letra que otro mete en medio llega por
// el sondeo, se fusiona en el folio sin tocar lo propio y el registro se reescribe con la
// fusión, sin ops por carácter (la secuencia ya la tiene) ni marca. Se vio en ROJO sin
// `Recibir` en `recibir`: el folio seguía diciendo «Hola!».
func TestLoQueEscribeOtroSeFusiona(t *testing.T) {
	s := &servicioDocs{rol: "editor", html: "<p>Hola</p>"}
	a, f := abrirParaEscribir(t, s)
	f.escribir("!")
	disparar(t) // siembra: «<p>Hola!</p>» con los puntos (actor, 1…12)
	s.mu.Lock()
	s.remotas = `[{"seq":9,"actor":"web-1","op":{"kind":"insert","dot":{"a":"web-1","c":40},"after":{"a":"` + a.actor +
		`","c":4},"value":"X","lamport":30}}]`
	s.mu.Unlock()
	a.sondear(a.abierto)
	if got := f.Serializar(); got != "<p>HXola!</p>" {
		t.Fatalf("la fusión: %q", got)
	}
	if a.abierto.seq != 9 || a.abierto.lamport < 30 {
		t.Errorf("seq %d y reloj %d tras el sondeo", a.abierto.seq, a.abierto.lamport)
	}
	disparar(t)
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.ops) != 2 || !strings.Contains(s.ops[1], `HXola!`) || strings.Contains(s.ops[1], `"kind":"insert"`) || strings.Contains(s.ops[1], ":lww") {
		t.Errorf("tras fusionar se reescribe el registro, y nada más: %v", s.ops)
	}
}

// TestUnLectorNoEscribe: con papel de lector el folio se abre en solo lectura y teclear no
// cambia nada ni programa ningún guardado.
func TestUnLectorNoEscribe(t *testing.T) {
	s := &servicioDocs{rol: "viewer", html: "<p>Hola</p>"}
	a, f := abrirParaEscribir(t, s)
	pendiente = nil
	f.escribir("!")
	f.retroceso()
	f.Mandar(editor.Negrita, "")
	if got := f.Serializar(); got != "<p>Hola</p>" || !a.abierto.ctx.SoloLectura || pendiente != nil {
		t.Errorf("un lector ha escrito: %q, solo lectura %v", got, a.abierto.ctx.SoloLectura)
	}
}

// TestDescargarGuardaAntesYDejaElFichero: «Descargar como Word» guarda antes lo pendiente
// —el servicio exporta desde SU estado— y deja el fichero en la carpeta de descargas con
// el nombre del documento.
func TestDescargarGuardaAntesYDejaElFichero(t *testing.T) {
	s := &servicioDocs{rol: "editor", html: "<p>Hola</p>"}
	a, f := abrirParaEscribir(t, s)
	dir := t.TempDir()
	a.carpeta = func() (string, error) { return dir, nil }
	f.escribir("!")
	a.abierto.ctx.Exportar("ooxml")
	s.mu.Lock()
	guardados := len(s.ops)
	s.mu.Unlock()
	if guardados != 1 {
		t.Errorf("antes de exportar se tenía que guardar lo pendiente, y hay %d guardados", guardados)
	}
	datos, err := os.ReadFile(dir + "/Propuesta.docx")
	if err != nil || string(datos) != "PK!" {
		t.Fatalf("la descarga: %q, %v", datos, err)
	}
	if a.abierto.ctx.Exportando != "" {
		t.Error("al acabar, el botón sigue diciendo «Generando…»")
	}
}

// TestCtrlPImprimeElPDF: Ctrl/Cmd+P en el folio guarda antes lo pendiente, pide el PDF al
// servicio y se lo da al sistema para imprimirlo, sin dejar nada en las descargas. Se vio
// en ROJO sin `AtajoImprimir` en el atajo del folio: no llegaba nada al sistema.
func TestCtrlPImprimeElPDF(t *testing.T) {
	s := &servicioDocs{rol: "editor", html: "<p>Hola</p>"}
	a, f := abrirParaEscribir(t, s)
	dir := t.TempDir()
	a.carpeta = func() (string, error) { return dir, nil }
	var nombre, datos string
	antes := editor.ImprimirDatos
	editor.ImprimirDatos = func(n string, d []byte) error { nombre, datos = n, string(d); return nil }
	t.Cleanup(func() { editor.ImprimirDatos = antes })
	f.escribir("!")
	f.atajo(&desktop.CustomShortcut{KeyName: fyne.KeyP, Modifier: fyne.KeyModifierShortcutDefault})
	s.mu.Lock()
	guardados := len(s.ops)
	s.mu.Unlock()
	if guardados != 1 || nombre != "Propuesta.pdf" || datos != "PK!" {
		t.Errorf("imprimir: %d guardados, %q con %q", guardados, nombre, datos)
	}
	if hay, _ := os.ReadDir(dir); len(hay) != 0 || a.abierto.ctx.Exportando != "" {
		t.Errorf("imprimir no descarga nada ni se queda «Generando…»: %v, %q", hay, a.abierto.ctx.Exportando)
	}
}

// TestLosComentariosSonHilos: el panel pide los comentarios, pone la respuesta bajo su
// raíz, y «Comentar» manda el texto sin blancos de más.
func TestLosComentariosSonHilos(t *testing.T) {
	s := &servicioDocs{rol: "editor", html: "<p>Hola</p>"}
	a, _ := abrirParaEscribir(t, s)
	a.panelComentarios(a.abierto)
	c := a.abierto.comentarios
	raices, resp := hilos(c.lista)
	if len(raices) != 1 || raices[0].ID != "c1" || len(resp["c1"]) != 1 || raices[0].AnchorText() != "Alcance" {
		t.Fatalf("los hilos: %+v %+v", raices, resp)
	}
	if !c.enviar.Inerte {
		t.Error("con el campo vacío, «Comentar» tiene que estar apagado")
	}
	c.nuevo.PonerTexto("  Mira esto  ")
	c.pintarEnviar()
	c.enviar.Tapped(nil)
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.comentados) != 1 || s.comentados[0] != `{"body":"Mira esto"}` {
		t.Errorf("lo comentado: %v", s.comentados)
	}
}

// TestElComentarioSeAnclaALaSeleccion: «Nuevo comentario» con una selección envuelve lo
// seleccionado en el `<mark data-comment-id>` y crea el hilo con el ancla `{ mark, text }`
// de la web; si el servicio falla, el resaltado se quita y no queda huérfano; y resolver el
// hilo lo quita del folio. Se vio en ROJO sin el `QuitarAncla` del fallo.
func TestElComentarioSeAnclaALaSeleccion(t *testing.T) {
	s := &servicioDocs{rol: "editor", html: "<p>Hola mundo</p>"}
	a, f := abrirParaEscribir(t, s)
	f.mover(punto{0, 5}, false)
	f.mover(punto{0, 10}, true)
	a.comentarAnclado(a.abierto, "Revisar")
	s.mu.Lock()
	comentados := append([]string(nil), s.comentados...)
	s.mu.Unlock()
	html := f.Serializar()
	_, marca, _ := strings.Cut(html, `data-comment-id="`)
	marca, _, _ = strings.Cut(marca, `"`)
	if html != `<p>Hola <mark data-comment-id="`+marca+`">mundo</mark></p>` || len(marca) != 12 {
		t.Fatalf("el folio tras comentar: %s", html)
	}
	if len(comentados) != 1 || comentados[0] != `{"anchor":{"mark":"`+marca+`","text":"mundo"},"body":"Revisar"}` {
		t.Errorf("lo comentado: %v", comentados)
	}
	if a.abierto.ctx.Panel != editor.PanelComentarios {
		t.Error("tras comentar no se abre el panel de comentarios")
	}

	// Sin red, la marca se va.
	f.mover(punto{0, 0}, false)
	f.mover(punto{0, 4}, true)
	s.mu.Lock()
	s.fallo = http.StatusInternalServerError
	s.mu.Unlock()
	a.comentarAnclado(a.abierto, "Otro")
	if got := f.Serializar(); strings.Count(got, "<mark") != 1 {
		t.Errorf("con el servicio caído queda la marca: %s", got)
	}
	s.mu.Lock()
	s.fallo = 0
	s.mu.Unlock()

	// Resolver el hilo quita su resaltado.
	f.QuitarAncla(marca)
	if got := f.Serializar(); got != `<p>Hola mundo</p>` {
		t.Errorf("sin el ancla: %s", got)
	}
	f.ResaltarAncla("m-que-no-existe")
	if len(f.ancla) != 0 {
		t.Error("un ancla que no está no se resalta")
	}
}

// TestElCanalEnVivoMandaSobreElSondeo: con el canal abierto no se sondea (la presencia
// sí se renueva), y un lote que llega por él se fusiona igual que uno sondeado. Se vio en
// ROJO sin la rama `ops` de enVivo: la letra del otro no llegaba.
func TestElCanalEnVivoMandaSobreElSondeo(t *testing.T) {
	s := &servicioDocs{rol: "editor", html: "<p>Hola</p>", remotas: `[]`}
	a, f := abrirParaEscribir(t, s)
	f.escribir("!")
	disparar(t)
	ab := a.abierto
	a.enVivo(ab, api.DocMensaje{Type: "welcome", Seq: 3})
	if !ab.canalAbierto {
		t.Fatal("tras el welcome, el canal está abierto")
	}
	s.mu.Lock()
	s.remotas = "" // si ahora se sondeara, sería un 404 y no llegaría nada
	s.mu.Unlock()
	ab.rondas = 1 // la próxima no toca presencia
	a.sondear(ab)
	var op api.DocOpSecuenciada
	if err := json.Unmarshal([]byte(`{"seq":9,"actor":"web-1","op":{"kind":"insert","dot":{"a":"web-1","c":40},"after":{"a":"`+
		a.actor+`","c":4},"value":"X","lamport":30}}`), &op); err != nil {
		t.Fatal(err)
	}
	a.enVivo(ab, api.DocMensaje{Type: "ops", Ops: []api.DocOpSecuenciada{op}})
	if got := f.Serializar(); got != "<p>HXola!</p>" || ab.seqVista.Load() != 9 {
		t.Errorf("por el canal: %q, seq vista %d", got, ab.seqVista.Load())
	}
}

// TestElAsistenteEscribeConFormato: sin selección, la acción va sobre el documento entero
// con la directiva de la web; la respuesta en Markdown se inserta con títulos, lista y
// negrita de verdad, y con selección se pone en su lugar. Se vio en ROJO insertando el
// texto tal cual: salían los «##» y los «**» en el folio.
func TestElAsistenteEscribeConFormato(t *testing.T) {
	s := &servicioDocs{rol: "editor", html: "<p>Hola</p><p>mundo</p>"}
	a := appDePrueba(t, s)
	var pedida api.PeticionIA
	respuesta := "## Título\n- uno\n- dos\nTexto **fuerte**"
	a.Asistir = func(_ context.Context, q api.PeticionIA) (string, error) { pedida = q; return respuesta, nil }
	a.AbrirID("d1", "")
	f := a.abierto.folio
	f.Programar = func(_ time.Duration, fn func()) { fn() }
	if !a.abierto.ctx.IA {
		t.Fatal("con Sytue AI, la cinta ofrece «Asistente»")
	}
	p := a.nuevoAsistente(a.abierto)
	f.mover(punto{1, 5}, false)
	p.pedir("resumir", "")
	if pedida.Contexto != "Hola\nmundo" || pedida.Instruccion != directivasIA["resumir"] || pedida.Tipo != "document" || p.resultado == "" {
		t.Fatalf("la petición: %+v, resultado %q", pedida, p.resultado)
	}
	p.aplicar(false)
	if got := f.Serializar(); got != "<p>Hola</p><p>mundo</p><h2>Título</h2><ul><li>uno</li><li>dos</li></ul><p>Texto <strong>fuerte</strong></p>" {
		t.Errorf("insertado con formato: %s", got)
	}
	respuesta = "Adiós"
	f.mover(punto{0, 0}, false)
	f.mover(punto{0, 4}, true)
	p.pedir("corregir", "")
	if pedida.Contexto != "Hola" || !p.conSeleccion {
		t.Fatalf("con selección, el contexto es ella: %+v", pedida)
	}
	p.aplicar(true)
	if got := f.Serializar(); !strings.HasPrefix(got, "<p>Adiós</p><p>mundo</p>") {
		t.Errorf("reemplazar la selección: %s", got)
	}
	if q := PeticionIA("traducir", "", "x"); q.Instruccion != "Traduce el texto a inglés. Devuelve solo la traducción." {
		t.Errorf("traducir sin idioma: %q", q.Instruccion)
	}
}

// TestLaVentanaLlevaElTituloDelDocumento: abierto, la ventana dice cómo se llama el
// documento; de vuelta en la portada, vacío (la ventana pone el nombre de la app). Antes
// decía «Edit — Sytue Docus» siempre, también en la portada (visto en Windows).
func TestLaVentanaLlevaElTituloDelDocumento(t *testing.T) {
	a := appDePrueba(t, &servicioDocs{rol: "editor", html: "<h1>Propuesta</h1>"})
	var titulos []string
	a.AlTitulo = func(s string) { titulos = append(titulos, s) }
	a.AbrirID("d1", "")
	a.Volver()
	if len(titulos) < 2 || titulos[len(titulos)-2] != "Propuesta" || titulos[len(titulos)-1] != "" {
		t.Fatalf("títulos de la ventana %q; quiero …, «Propuesta», «» (portada)", titulos)
	}
}

// TestCoedicionConEmojiCompuestoEnSesionReal (FYE-05): en una sesión de coedición real
// sobre Docus, los emojis compuestos (tono de piel, secuencia ZWJ, bandera regional y
// letra con acento combinante) viajan como una op por runa, convergen sin partir ni
// entremezclar sus puntos de código, y un insert malicioso de varios caracteres se
// descarta sin desincronizar el folio.
func TestCoedicionConEmojiCompuestoEnSesionReal(t *testing.T) {
	s := &servicioDocs{rol: "editor", html: "<p>Acta</p>", remotas: `[]`}
	a, f := abrirParaEscribir(t, s)
	f.escribir(" 👍🏽")
	disparar(t) // siembra «<p>Acta 👍🏽</p>» (10 runas hasta «🏽»: puntos 1..10 de a.actor)
	ab := a.abierto
	a.enVivo(ab, api.DocMensaje{Type: "welcome", Seq: 4})

	// Un coautor legítimo («web-1») inserta detrás de «👍🏽» (punto 10 de a.actor) la
	// secuencia compuesta « 👩🏽‍🔬 🇪🇸 é» (runa a runa, encadenadas), mientras un actor
	// malicioso («eva») intenta colar un insert de varios caracteres («TRAMPA»).
	remoto := " 👩🏽‍🔬 🇪🇸 e\u0301"
	lote := make([]api.DocOpSecuenciada, 0, len(remoto)+1)
	prevA, prevC := a.actor, uint64(10)
	seq := int64(10)
	var c uint64 = 50
	for _, r := range remoto {
		seq++
		c++
		crudo, err := json.Marshal(map[string]any{
			"kind":    "insert",
			"dot":     map[string]any{"a": "web-1", "c": c},
			"after":   map[string]any{"a": prevA, "c": prevC},
			"value":   string(r),
			"lamport": 40 + c,
		})
		if err != nil {
			t.Fatal(err)
		}
		lote = append(lote, api.DocOpSecuenciada{Seq: seq, Actor: "web-1", Op: crudo})
		prevA, prevC = "web-1", c
	}
	// Op inválida de varios caracteres (FYE-05): debe ser descartada por OpDeSecuencia.
	seq++
	lote = append(lote, api.DocOpSecuenciada{
		Seq:   seq,
		Actor: "eva",
		Op:    json.RawMessage(`{"kind":"insert","dot":{"a":"eva","c":999},"after":{"a":"` + a.actor + `","c":8},"value":"TRAMPA","lamport":999}`),
	})

	a.enVivo(ab, api.DocMensaje{Type: "ops", Ops: lote})
	esperado := "<p>Acta 👍🏽 👩🏽‍🔬 🇪🇸 e\u0301</p>"
	if got := f.Serializar(); got != esperado {
		t.Fatalf("fusión con emoji compuesto: %q != %q", got, esperado)
	}
	disparar(t) // reescribe el registro con el resultado fusionado
	if ab.base.Value != esperado {
		t.Fatalf("registro tras fusionar: %q != %q", ab.base.Value, esperado)
	}
}
