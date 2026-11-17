// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Los casos de tiemporeal.test.ts, uno a uno: ante cada cambio del flujo se pide de
// nuevo lo justo, y la presencia nunca cuenta la propia ventana.

var ctxPlan = contextoPlan{tabla: "t1", propia: func(op string) bool { return op == "op-mia" }}

func cambio(accion, tabla, registro string) api.BasesChange {
	return api.BasesChange{Action: accion, TableID: tabla, RecordID: registro}
}

func TestPlanUnRegistroDeLaTablaSeVuelveAPedirYBorradoSeQuita(t *testing.T) {
	for _, c := range []struct {
		cambio api.BasesChange
		quiero plan
	}{
		{cambio("record.update", "t1", "r1"), plan{tipo: planRegistro, id: "r1"}},
		{cambio("record.link", "t1", "r1"), plan{tipo: planRegistro, id: "r1"}},
		{cambio("record.create", "t1", "r2"), plan{tipo: planRegistro, id: "r2", nuevo: true}},
		{cambio("record.delete", "t1", "r1"), plan{tipo: planQuitar, id: "r1"}},
	} {
		if got := planDe(c.cambio, ctxPlan); got != c.quiero {
			t.Errorf("planDe(%+v) = %+v, quiero %+v", c.cambio, got, c.quiero)
		}
	}
}

func TestPlanElEcoDeUnaOperacionPropiaNoSeVuelveAPedir(t *testing.T) {
	mia, ajena := cambio("record.update", "t1", "r1"), cambio("record.update", "t1", "r1")
	mia.OpID, ajena.OpID = "op-mia", "op-ajena"
	if got := planDe(mia, ctxPlan).tipo; got != planNada {
		t.Errorf("el eco propio = %v", got)
	}
	if got := planDe(ajena, ctxPlan).tipo; got != planRegistro {
		t.Errorf("la operación ajena = %v", got)
	}
}

func TestPlanLoDeOtraTablaNoTocaYElEsquemaRecargaLaEstructura(t *testing.T) {
	for _, c := range []struct {
		cambio api.BasesChange
		quiero tipoPlan
	}{
		{cambio("record.update", "t2", "r9"), planNada},
		{cambio("comment.create", "t2", "r9"), planNada},
		{cambio("view.create", "t2", ""), planNada},
		{cambio("view.create", "t1", ""), planEstructura},
		{cambio("access", "t2", ""), planEstructura},
		{cambio("base.update", "t2", ""), planEstructura},
		{cambio("field.create", "t2", ""), planEstructura},
		{cambio("field.delete", "t2", ""), planEstructura},
		{cambio("table.rename", "t2", ""), planEstructura},
		{cambio("table.delete", "t2", ""), planEstructura},
		{cambio("algo.desconocido", "t1", "r1"), planNada},
	} {
		if got := planDe(c.cambio, ctxPlan).tipo; got != c.quiero {
			t.Errorf("planDe(%+v) = %v, quiero %v", c.cambio, got, c.quiero)
		}
	}
	if got := planDe(cambio("comment.delete", "t1", "r1"), ctxPlan); got != (plan{tipo: planComentarios, id: "r1"}) {
		t.Errorf("un comentario de la tabla = %+v", got)
	}
}

func TestLaPresenciaPorCeldaAgrupaYDejaFueraLaPropiaVentana(t *testing.T) {
	pr := func(conn, sub, tabla, registro, campo string) api.BasesPresence {
		return api.BasesPresence{Conn: conn, Sub: sub, Name: strings.ToUpper(sub), TableID: tabla, RecordID: registro, FieldID: campo}
	}
	lista := []api.BasesPresence{
		pr("yo", "ana", "t1", "r1", "f1"),
		pr("c2", "bea", "t1", "r1", "f1"),
		pr("c3", "bea", "t1", "r1", "f1"),
		pr("c4", "cleo", "t1", "r1", "f1"),
		pr("c5", "dani", "t2", "r1", "f1"),
		pr("c6", "eva", "t1", "", ""),
	}
	if got := presentesPorCelda(lista, "t1", "yo"); !reflect.DeepEqual(got, map[string][]string{"r1:f1": {"BEA", "CLEO"}}) {
		t.Errorf("presentesPorCelda = %v", got)
	}
	if got := personasPresentes(lista, "yo"); !reflect.DeepEqual(got, []string{"BEA", "CLEO", "DANI", "EVA"}) {
		t.Errorf("personasPresentes = %v", got)
	}
	// Sin nombre, el principio del `sub`.
	if got := personasPresentes([]api.BasesPresence{{Conn: "c9", Sub: "0123456789abcdef"}}, "yo"); !reflect.DeepEqual(got, []string{"01234567"}) {
		t.Errorf("sin nombre = %v", got)
	}
}

// flujoFalso es el flujo de Bases de pega encima del servicio `falso`: cada conexión a
// `/bases/b1/events` escribe lo que se le manda por `eventos`, y los latidos se apuntan.
type flujoFalso struct {
	*falso
	eventos chan string
	// cerrado suelta los flujos abiertos al acabar la prueba: sin él, una prueba que
	// falla con el flujo abierto se queda colgada en el cierre del servidor.
	cerrado chan struct{}
	mu      sync.Mutex
	// conexiones son las aperturas del flujo (el `Last-Event-ID` de cada una) y abiertas
	// las que siguen vivas.
	conexiones []string
	abiertas   int
	latidos    []string
}

func (s *flujoFalso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch strings.TrimPrefix(r.URL.Path, "/api/v1") {
	case "/bases/b1/events":
		s.mu.Lock()
		s.conexiones = append(s.conexiones, r.Header.Get("Last-Event-ID"))
		s.abiertas++
		s.mu.Unlock()
		defer func() { s.mu.Lock(); s.abiertas--; s.mu.Unlock() }()
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-s.cerrado:
				return
			case e, sigue := <-s.eventos:
				if !sigue || e == "" { // "" corta esta conexión
					return
				}
				_, _ = io.WriteString(w, e+"\n\n")
				w.(http.Flusher).Flush()
			}
		}
	case "/bases/b1/presence":
		cuerpo, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.latidos = append(s.latidos, string(cuerpo))
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		s.falso.ServeHTTP(w, r)
	}
}

func (s *flujoFalso) leer() (conexiones []string, abiertas int, latidos []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.conexiones...), s.abiertas, append([]string(nil), s.latidos...)
}

// abrirEnVivo es `abrir` con el flujo de pega y los relojes del tiempo real acortados.
func abrirEnVivo(t *testing.T, f *falso) (*Pantalla, fyne.Window, *flujoFalso) {
	t.Helper()
	s := &flujoFalso{falso: f, eventos: make(chan string), cerrado: make(chan struct{})}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(s.cerrado) }) // antes que srv.Close
	tr, err := Traductor("es")
	if err != nil {
		t.Fatal(err)
	}
	w := test.NewWindow(nil)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	p := NuevaPantalla(tr, api.NewBases(srv.URL, nil, nil), w)
	p.fondo = func(g func()) { g() }
	p.cadaLatido, p.esperaRecarga = 20*time.Millisecond, 5*time.Millisecond
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	p.Abrir("b1")
	t.Cleanup(func() { fyne.DoAndWait(func() { p.dejarDeSeguir(p.abierta) }) })
	return p, w, s
}

// esperar repite `cond` en la rutina de la interfaz (que así atiende lo que le llega
// del flujo) hasta que se cumple, o falla.
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
			t.Fatalf("no ha pasado: %s", que)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// borrarDelServicio quita un registro del servicio de pega, para que la recarga que
// sigue al `hello` (que puede llegar tarde con la máquina cargada) no lo devuelva.
func borrarDelServicio(f *falso, id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.registros["t1"] = slices.DeleteFunc(f.registros["t1"], func(r m) bool { return r["id"] == id })
}

// mandar escribe un evento en el flujo abierto.
func (s *flujoFalso) mandar(t *testing.T, evento string) {
	t.Helper()
	select {
	case s.eventos <- evento:
	case <-time.After(5 * time.Second):
		t.Fatalf("nadie escucha el flujo para mandar %q", evento)
	}
}

// TestUnCambioAjenoLlegaSinPerderElFocoNiElEditor: con un servidor SSE de pega, lo que
// otra persona cambia aparece solo en la rejilla, y ni el foco ni el editor abierto en
// otra celda se mueven; ni siquiera cuando desaparece una fila por encima.
func TestUnCambioAjenoLlegaSinPerderElFocoNiElEditor(t *testing.T) {
	f := nuevoFalso(3)
	p, w, s := abrirEnVivo(t, f)
	esperar(t, "la rejilla con sus tres registros", func() bool {
		return p.abierta.rejilla != nil && len(p.abierta.rejilla.Registros) == 3
	})
	s.mandar(t, `event: hello`+"\n"+`data: {"v":1,"cursor":"c0","role":"owner"}`)

	// Se edita «Nombre» del segundo registro, sin confirmar.
	g := p.abierta.rejilla
	fyne.DoAndWait(func() {
		w.Canvas().Focus(g)
		g.enfocar(celda{1, 0})
		g.TypedRune('X')
	})
	editor := g.editor
	if editor == nil {
		t.Fatal("no se ha abierto el editor")
	}

	// Otra persona cambia el primero.
	f.mu.Lock()
	f.registros["t1"][0]["data"].(m)["nom"] = "Cambiado por Bea"
	f.mu.Unlock()
	s.mandar(t, `id: c1`+"\n"+`event: change`+"\n"+`data: {"action":"record.update","table_id":"t1","record_id":"r001","field_id":"nom","op_id":"op-de-bea"}`)
	esperar(t, "el cambio de Bea en la rejilla", func() bool {
		return g.Registros[0].Data["nom"] == "Cambiado por Bea"
	})
	fyne.DoAndWait(func() {
		if !g.editando || g.editor != editor || editor.Texto() != "X" {
			t.Errorf("el editor no ha sobrevivido: editando=%v, texto=%q", g.editando, editor.Texto())
		}
		if r, _, _ := g.actual(); r.ID != "r002" || g.foco.col != 0 {
			t.Errorf("el foco se ha ido a %v (%s)", g.foco, r.ID)
		}
		if w.Canvas().Focused() != fyne.Focusable(editor) {
			t.Errorf("el foco de la ventana ya no está en el editor: %T", w.Canvas().Focused())
		}
	})

	// Y borra el primero: el foco sigue en el mismo registro, una fila más arriba.
	borrarDelServicio(f, "r001")
	s.mandar(t, `id: c2`+"\n"+`event: change`+"\n"+`data: {"action":"record.delete","table_id":"t1","record_id":"r001"}`)
	esperar(t, "el registro borrado fuera de la rejilla", func() bool { return len(g.Registros) == 2 })
	fyne.DoAndWait(func() {
		if r, _, _ := g.actual(); r.ID != "r002" || g.foco.fila != 0 || !g.editando || g.editor != editor {
			t.Errorf("tras el borrado el foco está en %v (%s), editando=%v", g.foco, r.ID, g.editando)
		}
	})
	// Ninguno de los dos ha recargado la tabla entera: se ha pedido solo el registro.
	if n := len(f.peticiones("GET /tables/t1/records")); n > 2 { // la carga y la de tras el `hello`
		t.Errorf("se ha recargado la tabla %d veces", n)
	}
	if !f.vio("GET /records/r001") {
		t.Error("no se ha vuelto a pedir el registro cambiado")
	}
}

// TestSiBorranElRegistroQueSeEditaSeCierraElEditor: lo escrito era de una celda que ya
// no está; confirmarlo lo guardaría en el registro que ha ocupado su sitio.
func TestSiBorranElRegistroQueSeEditaSeCierraElEditor(t *testing.T) {
	f := nuevoFalso(3)
	p, w, s := abrirEnVivo(t, f)
	esperar(t, "la rejilla con sus tres registros", func() bool {
		return p.abierta.rejilla != nil && len(p.abierta.rejilla.Registros) == 3
	})
	s.mandar(t, `event: hello`+"\n"+`data: {}`)
	g := p.abierta.rejilla
	fyne.DoAndWait(func() {
		w.Canvas().Focus(g)
		g.enfocar(celda{1, 0})
		g.TypedRune('X')
	})
	if !g.editando {
		t.Fatal("no se ha abierto el editor")
	}
	borrarDelServicio(f, "r002")
	s.mandar(t, `id: c1`+"\n"+`event: change`+"\n"+`data: {"action":"record.delete","table_id":"t1","record_id":"r002"}`)
	esperar(t, "el registro borrado fuera de la rejilla", func() bool { return len(g.Registros) == 2 })
	fyne.DoAndWait(func() {
		if g.editando || g.editor != nil {
			t.Error("el editor sigue abierto sobre otra celda")
		}
		if w.Canvas().Focused() != fyne.Focusable(g) {
			t.Errorf("el foco no ha vuelto a la rejilla: %T", w.Canvas().Focused())
		}
	})
	if n := f.peticiones("PUT /records"); len(n) > 0 {
		t.Fatalf("se ha escrito en otro registro: %q", n)
	}
}

// TestLaPresenciaSeVeEnLaCabeceraYEnLaCelda: quien está en la base sale en la cabecera,
// y la celda en la que está, marcada; la propia ventana, en ningún sitio.
func TestLaPresenciaSeVeEnLaCabeceraYEnLaCelda(t *testing.T) {
	f := nuevoFalso(3)
	p, w, s := abrirEnVivo(t, f)
	esperar(t, "la rejilla", func() bool { return p.abierta.rejilla != nil && len(p.abierta.rejilla.Registros) == 3 })
	s.mandar(t, `event: hello`+"\n"+`data: {}`)
	s.mandar(t, fmt.Sprintf(`event: presence`+"\n"+`data: [{"conn":%q,"sub":"u1","name":"Yo"},{"conn":"otra-ventana","sub":"u2","name":"Bea Ruiz","table_id":"t1","record_id":"r002","field_id":"eda"}]`, p.conn))
	esperar(t, "Bea en la celda", func() bool {
		return reflect.DeepEqual(p.abierta.rejilla.Presentes, map[string][]string{"r002:eda": {"Bea Ruiz"}})
	})
	var avatares []string
	fyne.DoAndWait(func() {
		recorrer(w.Canvas().Content(), func(o fyne.CanvasObject) {
			if a, ok := o.(*ui.Avatar); ok {
				avatares = append(avatares, a.Nombre)
			}
		})
	})
	if !reflect.DeepEqual(avatares, []string{"Bea Ruiz"}) {
		t.Fatalf("avatares en la cabecera = %q", avatares)
	}
	// La marca va en su celda: recuadro y etiqueta con el nombre.
	fyne.DoAndWait(func() {
		r := test.WidgetRenderer(p.abierta.rejilla).(*rejillaRenderer)
		m, ok := r.otros[celda{1, 1}]
		if !ok || len(r.otros) != 1 || m.texto.Text != "Bea Ruiz" || m.recuadro.Size().Width == 0 {
			t.Errorf("marcas de presencia = %v", r.otros)
		}
	})
}

// TestElLatidoDiceLaCeldaYSeParaAlCerrarLaBase: el latido lleva la ventana, la tabla y
// la celda con el foco, y al volver a la lista se deja de latir y se cierra el flujo.
func TestElLatidoDiceLaCeldaYSeParaAlCerrarLaBase(t *testing.T) {
	f := nuevoFalso(3)
	p, _, s := abrirEnVivo(t, f)
	esperar(t, "la rejilla", func() bool { return p.abierta.rejilla != nil && len(p.abierta.rejilla.Registros) == 3 })
	s.mandar(t, `event: hello`+"\n"+`data: {}`)
	// Abrir la base no es estar en su primera celda: hasta que el foco llega a la
	// rejilla, el latido solo dice la tabla.
	esperar(t, "un latido sin celda", func() bool {
		_, _, latidos := s.leer()
		return len(latidos) > 0
	})
	if _, _, latidos := s.leer(); latidos[0] != fmt.Sprintf(`{"conn":%q,"table_id":"t1"}`, p.conn) {
		t.Fatalf("primer latido = %s", latidos[0])
	}
	fyne.DoAndWait(func() { p.abierta.rejilla.enfocar(celda{2, 1}) })
	quiero := fmt.Sprintf(`{"conn":%q,"table_id":"t1","record_id":"r003","field_id":"eda"}`, p.conn)
	esperar(t, "un latido con la celda", func() bool {
		_, _, latidos := s.leer()
		return len(latidos) > 0 && latidos[len(latidos)-1] == quiero
	})

	fyne.DoAndWait(p.volverALista)
	esperar(t, "el flujo cerrado", func() bool { _, abiertas, _ := s.leer(); return abiertas == 0 })
	_, _, antes := s.leer()
	time.Sleep(10 * p.cadaLatido)
	if _, _, despues := s.leer(); len(despues) != len(antes) {
		t.Fatalf("se sigue latiendo con la base cerrada: %d latidos más", len(despues)-len(antes))
	}
}

// TestSinConexionYAccesoPerdido: si el flujo se corta se dice, y al volver se reanuda
// desde el último cursor; si el servicio dice que ya no hay acceso, se dice y no se
// reintenta.
func TestSinConexionYAccesoPerdido(t *testing.T) {
	f := nuevoFalso(1)
	p, w, s := abrirEnVivo(t, f)
	esperar(t, "la rejilla", func() bool { return p.abierta.rejilla != nil })
	s.mandar(t, `event: hello`+"\n"+`data: {}`)
	s.mandar(t, `id: c7`)
	s.mandar(t, "") // se corta
	sinConexion := "Se ha perdido la conexión. Los cambios de otras personas aparecerán cuando vuelva."
	esperar(t, "el aviso de sin conexión", func() bool { return strings.Contains(textos(w), sinConexion) })

	s.mandar(t, `event: hello`+"\n"+`data: {}`) // reabre solo
	esperar(t, "el aviso fuera al volver", func() bool { return !strings.Contains(textos(w), sinConexion) })
	if conexiones, _, _ := s.leer(); !reflect.DeepEqual(conexiones, []string{"", "c7"}) {
		t.Fatalf("Last-Event-ID de cada apertura = %q", conexiones)
	}

	s.mandar(t, `event: bye`+"\n"+`data: {"reason":"access"}`)
	esperar(t, "el aviso de acceso perdido", func() bool { return strings.Contains(textos(w), "Ya no tienes acceso a esta base.") })
	time.Sleep(1500 * time.Millisecond) // más que la primera espera de reconexión
	if conexiones, abiertas, _ := s.leer(); len(conexiones) != 2 || abiertas != 0 {
		t.Fatalf("sin acceso se ha vuelto a abrir el flujo: %q (%d abiertas)", conexiones, abiertas)
	}
}
