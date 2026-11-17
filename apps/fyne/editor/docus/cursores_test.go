// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"context"
	"encoding/json"
	"image/color"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/coedicion"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// TestElTextoVisibleEsElDeLaWeb: el texto sobre el que se miden los cursores es el
// `textContent` del HTML que se guarda (lo que mide la web y la secuencia), y cada offset
// va y vuelve a la misma posición. Si no, el cursor de la web caería en otra letra aquí.
func TestElTextoVisibleEsElDeLaWeb(t *testing.T) {
	for _, html := range []string{
		"<p>Hola</p><p>mundo</p>",
		"<h1>Tí👍tulo</h1><p>uno<br>dos</p><ul><li>a</li><li>b</li></ul><p></p><p>fin &amp; más</p>",
		"<p>a <strong>negrita</strong> y <em>cursiva</em></p><blockquote><p>cita</p></blockquote>",
	} {
		f := NuevoFolio(nil)
		f.Cargar(html)
		texto := f.textoVisible()
		if quiero := coedicion.LongitudVisible(f.Serializar()); len(texto) != quiero {
			t.Errorf("%s: %d unidades de texto, y la web cuenta %d", html, len(texto), quiero)
		}
		for off := 0; off <= len(texto); off++ {
			if off > 0 && texto[off-1] >= 0xD800 && texto[off-1] < 0xDC00 {
				continue // a mitad de un emoji: el navegador tampoco deja el cursor ahí
			}
			q, ok := f.puntoDeOffset(off)
			if !ok || f.offsetDe(q) != off {
				t.Errorf("%s: el offset %d va a %+v (%v) y vuelve como %d", html, off, q, ok, f.offsetDe(q))
			}
		}
		if _, ok := f.puntoDeOffset(len(texto) + 1); ok {
			t.Errorf("%s: un offset más allá del texto se coloca", html)
		}
	}
}

// TestUnaFronteraSeQuedaEnSuBloque: el final de un párrafo y el principio del siguiente
// son el mismo offset; la web se queda con el de arriba, y el cursor propio, con el bloque
// en el que estaba.
func TestUnaFronteraSeQuedaEnSuBloque(t *testing.T) {
	f := NuevoFolio(nil)
	f.Cargar("<p>abc</p><p></p><p>def</p>")
	if q, _ := f.puntoDeOffset(3); q != (punto{0, 3}) {
		t.Errorf("como la web, el de arriba: %+v", q)
	}
	for cerca, quiero := range map[int]punto{0: {0, 3}, 1: {1, 0}, 2: {2, 0}} {
		if q, _ := f.puntoDeOffsetCerca(3, cerca); q != quiero {
			t.Errorf("cerca del bloque %d: %+v, quiero %+v", cerca, q, quiero)
		}
	}
}

// TestLaEtiquetaSeDesvanece: entera hasta el 55 % de la animación y luego hasta cero, como
// `sytue-peer-label-fade`.
func TestLaEtiquetaSeDesvanece(t *testing.T) {
	for tt, quiero := range map[float32]float32{0: 1, 0.55: 1, 0.775: 0.5, 1: 0} {
		if got := opacidadEtiqueta(tt); got < quiero-0.001 || got > quiero+0.001 {
			t.Errorf("en %v: %v, quiero %v", tt, got, quiero)
		}
	}
}

// objetosConColor son los rectángulos pintados en el color (sin mirar el alfa) de ese token.
func objetosConColor(objs []fyne.CanvasObject, token string) []*canvas.Rectangle {
	c := tema.Color(token, tema.Claro)
	var out []*canvas.Rectangle
	for _, o := range objs {
		if r, ok := o.(*canvas.Rectangle); ok {
			if n, ok := r.FillColor.(color.NRGBA); ok && n.R == c.R && n.G == c.G && n.B == c.B && n.A > 0 {
				out = append(out, r)
			}
		}
	}
	return out
}

// TestElCursorDeOtroSePintaConSuColor: el par se pinta con un caret de 2 px y su
// selección translúcida en el color de su persona, y la etiqueta con su nombre encima; el
// que no cae en el texto no se pinta, y quitarlo lo borra.
func TestElCursorDeOtroSePintaConSuColor(t *testing.T) {
	f := NuevoFolio(nil)
	f.Cargar("<p>Hola mundo</p>")
	f.PonerPares([]ParCursor{
		{ID: "ana", Nombre: "Ana", Color: "avatar-3", Anchor: 5, Focus: 10, Clave: "d5:10"},
		{ID: "luis", Nombre: "Luis", Color: "avatar-5", Anchor: 99, Focus: 99, Clave: "d99:99"},
	})
	if len(f.pares) != 1 || f.pares[0].anc != (punto{0, 5}) || f.pares[0].cur != (punto{0, 10}) {
		t.Fatalf("los pares situados: %+v", f.pares)
	}
	objs := pintar(f, fyne.NewPos(0, 0), tema.Claro)
	rects := objetosConColor(objs, "avatar-3")
	var caret, sel, etiqueta bool
	for _, r := range rects {
		switch alfa := r.FillColor.(color.NRGBA).A; {
		case r.Size().Width == anchoCaretPar && alfa == 0xff:
			caret = true
		case alfa < 0xff && r.Size().Width > 20:
			sel = true // «mundo», al 18 %
		}
	}
	for _, o := range objs {
		if tx, ok := o.(*canvas.Text); ok && tx.Text == "Ana" {
			etiqueta = true
		}
	}
	if !caret || !sel || !etiqueta {
		t.Errorf("caret %v, selección %v, etiqueta %v en %d objetos de su color", caret, sel, etiqueta, len(rects))
	}
	if len(objetosConColor(objs, "avatar-5")) != 0 {
		t.Error("se pinta un cursor que no cae en el texto")
	}
	f.PonerPares(nil)
	if len(objetosConColor(pintar(f, fyne.NewPos(0, 0), tema.Claro), "avatar-3")) != 0 {
		t.Error("el cursor quitado se sigue pintando")
	}
}

// TestElCursorPropioSigueASusLetrasAlFusionar: lo que otro escribe delante en la misma
// línea corre el cursor propio con su texto. Se vio en ROJO con el Fusionar de antes: el
// cursor se quedaba en la runa 5, entre la «a» y la «!».
func TestElCursorPropioSigueASusLetrasAlFusionar(t *testing.T) {
	s := &servicioDocs{rol: "editor", html: "<p>Hola</p>", remotas: `[]`}
	a, f := abrirParaEscribir(t, s)
	f.escribir("!")
	disparar(t) // siembra «<p>Hola!</p>»
	if f.cur != (punto{0, 5}) {
		t.Fatalf("el cursor tras escribir: %+v", f.cur)
	}
	var op api.DocOpSecuenciada
	if err := json.Unmarshal([]byte(`{"seq":9,"actor":"web-1","op":{"kind":"insert","dot":{"a":"web-1","c":40},"after":{"a":"`+
		a.actor+`","c":4},"value":"X","lamport":30}}`), &op); err != nil {
		t.Fatal(err)
	}
	a.recibir(a.abierto, []api.DocOpSecuenciada{op})
	if got := f.Serializar(); got != "<p>HXola!</p>" || f.cur != (punto{0, 6}) || f.anc != f.cur {
		t.Errorf("tras fusionar %q el cursor está en %+v (ancla %+v); quiero tras la «!»", got, f.cur, f.anc)
	}
}

// TestElCursorDeOtroSeRecolocaAlEscribirDelante: un cursor ajeno sin puntos se corre con
// lo que se escribe aquí delante de él, como `anclarPos` en la web.
func TestElCursorDeOtroSeRecolocaAlEscribirDelante(t *testing.T) {
	a, f := abrirParaEscribir(t, &servicioDocs{rol: "editor", html: "<p>Hola mundo</p>"})
	ab := a.abierto
	ana := &api.DocPresencia{Sub: "ana", Name: "Ana", Color: "avatar-3", Cursor: &api.Cursor{Kind: "doc", Anchor: 5, Focus: 5}}
	a.presencia(ab, api.DocMensaje{Type: "presence", Cursor: ana})
	f.mover(punto{0, 0}, false)
	f.escribir("¡")
	if len(f.pares) != 1 || f.pares[0].cur != (punto{0, 6}) {
		t.Fatalf("el cursor de Ana tras escribir delante: %+v", f.pares)
	}
	// Una lista de presentes sin Ana la quita.
	a.presencia(ab, api.DocMensaje{Type: "presence", Presence: []api.DocPresencia{{Sub: "yo"}}})
	if len(f.pares) != 0 || len(ab.pares) != 0 {
		t.Errorf("Ana se fue y su cursor sigue: %+v", f.pares)
	}
}

// TestSinCanalNoSeMandaElCursor: con solo el sondeo (sin canal en vivo) mover el cursor
// no manda nada ni falla, como `sendCursor` sin WS.
func TestSinCanalNoSeMandaElCursor(t *testing.T) {
	a, f := abrirParaEscribir(t, &servicioDocs{rol: "editor", html: "<p>Hola mundo</p>"})
	lanzados := 0
	a.lanzar = func(fn func()) { lanzados++; fn() }
	pendiente = nil
	f.mover(punto{0, 3}, false)
	f.mover(punto{0, 7}, true)
	if lanzados != 0 || pendiente != nil || a.abierto.cursorEnEspera {
		t.Errorf("sin canal salió algo: %d envíos, programado %v", lanzados, pendiente != nil)
	}
}

// sesionDe es una sesión con su propio token: la sala falsa lo toma por el `sub`.
type sesionDe string

func (s sesionDe) AccessToken(context.Context) (string, error) { return string(s), nil }
func (sesionDe) Invalidate(string)                             {}

// salaFalsa es el hub del servicio (collab/hub.go) para UN documento: saluda, difunde la
// lista de presentes al entrar y al salir alguien y reparte el cursor de cada uno a los
// demás, con su nombre.
type salaFalsa struct {
	mu        sync.Mutex
	conns     map[*websocket.Conn]string
	recibidos []string
}

func (s *salaFalsa) difundir(m string, salvo *websocket.Conn) {
	s.mu.Lock()
	destinos := make([]*websocket.Conn, 0, len(s.conns))
	for c := range s.conns {
		if c != salvo {
			destinos = append(destinos, c)
		}
	}
	s.mu.Unlock()
	for _, c := range destinos {
		ctx, cancelar := context.WithTimeout(context.Background(), time.Second)
		_ = c.Write(ctx, websocket.MessageText, []byte(m))
		cancelar()
	}
}

func (s *salaFalsa) lista() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	subs := make([]string, 0, len(s.conns))
	for _, sub := range s.conns {
		subs = append(subs, `{"sub":"`+sub+`"}`)
	}
	return `{"type":"presence","presence":[` + strings.Join(subs, ",") + `]}`
}

func (s *salaFalsa) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sub := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = c.CloseNow() }()
	var hola map[string]any
	if wsjson.Read(r.Context(), c, &hola) != nil {
		return
	}
	if c.Write(r.Context(), websocket.MessageText, []byte(`{"type":"welcome","version":1,"role":"editor","seq":3}`)) != nil {
		return
	}
	s.mu.Lock()
	s.conns[c] = sub
	s.mu.Unlock()
	s.difundir(s.lista(), nil)
	defer func() {
		s.mu.Lock()
		delete(s.conns, c)
		s.mu.Unlock()
		s.difundir(s.lista(), nil)
	}()
	for {
		var m struct {
			Type   string          `json:"type"`
			Cursor json.RawMessage `json:"cursor"`
		}
		if wsjson.Read(r.Context(), c, &m) != nil {
			return
		}
		if m.Type != "cursor" {
			continue
		}
		s.mu.Lock()
		s.recibidos = append(s.recibidos, string(m.Cursor))
		s.mu.Unlock()
		nombre := strings.ToUpper(sub[:1]) + sub[1:]
		s.difundir(`{"type":"presence","cursor":{"sub":"`+sub+`","name":"`+nombre+`","cursor":`+string(m.Cursor)+`}}`, c)
	}
}

// esperar repite la comprobación en el hilo de la interfaz (lo que llega del canal se
// encola y corre ahí) hasta que se cumple o pasan cinco segundos.
func esperar(t *testing.T, que string, cond func() bool) {
	t.Helper()
	limite := time.Now().Add(5 * time.Second)
	for {
		var ok bool
		fyne.DoAndWait(func() { ok = cond() })
		if ok {
			return
		}
		if time.Now().After(limite) {
			t.Fatalf("no llega: %s", que)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestDosAppsSeVenElCursor: dos Docus contra el mismo servicio falso con canal en vivo.
// Lo que Luis selecciona sale en el ritmo de la web (el primero ya; los siguientes de la
// pausa, juntos en el último) y Ana lo ve pintado en el color de Luis; cuando Luis sale,
// su caret desaparece del folio de Ana.
func TestDosAppsSeVenElCursor(t *testing.T) {
	docs := &servicioDocs{rol: "editor", html: "<p>Hola mundo</p>"}
	sala := &salaFalsa{conns: map[*websocket.Conn]string{}}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/documents/d1/ws", sala)
	mux.Handle("/", docs)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	var programados []func()
	nueva := func(sub string) *App {
		a := NuevaApp(traductorPlay(t), "es", api.NewDocs(srv.URL, sesionDe(sub), srv.Client()))
		a.lanzar = func(f func()) { f() }
		a.portada.Lanzar = a.lanzar
		a.programar = func(_ time.Duration, f func()) func() {
			programados = append(programados, f)
			return func() {}
		}
		a.AbrirID("d1", "")
		a.abierto.folio.Programar = func(_ time.Duration, fn func()) { fn() }
		t.Cleanup(func() {
			if ab := a.abierto; ab != nil && ab.cerrarCanal != nil {
				ab.cerrarCanal()
			}
		})
		return a
	}
	ana, luis := nueva("ana"), nueva("luis")
	esperar(t, "los dos canales abiertos", func() bool {
		return ana.abierto.canal != nil && ana.abierto.canalAbierto && luis.abierto.canal != nil && luis.abierto.canalAbierto
	})

	fl := luis.abierto.folio
	fl.mover(punto{0, 5}, false) // sale ya
	programados = nil
	fl.mover(punto{0, 7}, true) // dentro de la pausa: se aplaza…
	fl.mover(punto{0, 10}, true)
	if len(programados) != 1 {
		t.Fatalf("dentro de la pausa se programa UN envío, y hay %d", len(programados))
	}
	fyne.DoAndWait(programados[0]) // …y sale con la última posición
	esperar(t, "el cursor de Luis en el folio de Ana", func() bool {
		p := ana.abierto.folio.pares
		return len(p) == 1 && p[0].anc == (punto{0, 5}) && p[0].cur == (punto{0, 10})
	})
	sala.mu.Lock()
	recibidos := append([]string(nil), sala.recibidos...)
	sala.mu.Unlock()
	if len(recibidos) != 2 || !strings.Contains(recibidos[0], `"focus":5`) || !strings.Contains(recibidos[1], `"anchor":5`) ||
		!strings.Contains(recibidos[1], `"focus":10`) || !strings.Contains(recibidos[1], `"kind":"doc"`) {
		t.Errorf("lo que mandó Luis: %v", recibidos)
	}
	var par ParCursor
	fyne.DoAndWait(func() { par = ana.abierto.folio.pares[0].ParCursor })
	if par.ID != "luis" || par.Nombre != "Luis" || par.Color != tokens.Persona("luis") {
		t.Errorf("el par que ve Ana: %+v", par)
	}

	luis.Volver()
	esperar(t, "que el caret de Luis se vaya al salir", func() bool { return len(ana.abierto.folio.pares) == 0 })
}

// TestAlRecargarSeQuitanLosCursoresAjenos: volver a leer el documento (restaurar una
// versión, un cambio de rol) cierra el canal y abre otro; los cursores de los demás eran
// del texto de antes y se quitan, y lo que llegue tarde del canal viejo no toca el nuevo.
func TestAlRecargarSeQuitanLosCursoresAjenos(t *testing.T) {
	a, f := abrirParaEscribir(t, &servicioDocs{rol: "editor", html: "<p>Hola mundo</p>"})
	ab := a.abierto
	ab.cerrarCanal, ab.canalAbierto, ab.canal = func() {}, true, &api.CanalDoc{}
	ana := &api.DocPresencia{Sub: "ana", Name: "Ana", Color: "avatar-3", Cursor: &api.Cursor{Kind: "doc", Anchor: 5, Focus: 5}}
	a.presencia(ab, api.DocMensaje{Type: "presence", Cursor: ana})
	if len(f.pares) != 1 {
		t.Fatalf("el cursor de Ana no se pinta: %+v", f.pares)
	}
	gen := ab.genCanal
	a.recargarPorRol(ab)
	if len(f.pares) != 0 || len(ab.pares) != 0 || ab.canal != nil || ab.canalAbierto {
		t.Errorf("tras recargar quedan el cursor de Ana %+v o el canal viejo %v", f.pares, ab.canal != nil)
	}
	if ab.genCanal == gen {
		t.Error("el canal viejo sigue contando como el de ahora")
	}
}

// TestUnColaboradorSinNombreLlevaLaReserva: si la presencia de alguien no trae su nombre,
// su cursor sale con «Colaborador» (`header.peerUnnamed`, como `peerName` de la web) y no
// sin etiqueta.
func TestUnColaboradorSinNombreLlevaLaReserva(t *testing.T) {
	a, f := abrirParaEscribir(t, &servicioDocs{rol: "editor", html: "<p>Hola mundo</p>"})
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		t.Fatal(err)
	}
	a.T = tr.T
	ab := a.abierto
	ab.cerrarCanal, ab.canalAbierto, ab.canal = func() {}, true, &api.CanalDoc{}
	sinNombre := &api.DocPresencia{Sub: "x", Name: "  ", Color: "avatar-3", Cursor: &api.Cursor{Kind: "doc", Anchor: 2, Focus: 2}}
	a.presencia(ab, api.DocMensaje{Type: "presence", Cursor: sinNombre})
	if len(f.pares) != 1 || f.pares[0].Nombre != "Otra persona" {
		t.Fatalf("el cursor sin nombre: %+v", f.pares)
	}
}
