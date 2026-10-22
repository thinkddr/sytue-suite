// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// LOS GUARDIANES DE LAS ESCRITURAS. Hablan con un servicio falso por HTTP —el mismo
// cliente de `apps/nucleo/api` que usa la app— y comprueban dos cosas: qué viaja por la
// red, y qué queda en la pantalla. Un test que solo mirase la pantalla pasaría con un
// cliente que no manda nada.
//
// `fyne.DoAndWait` y `fyne.Do` son síncronos con el driver de pruebas, así que cada
// escritura ha terminado cuando la función vuelve.

type peticion struct {
	metodo, ruta string
	cuerpo       map[string]any
}

// servicioFalso responde lo que se le pida por ruta y apunta cada petición.
type servicioFalso struct {
	mu        sync.Mutex
	vistas    []peticion
	respuesta map[string]string
}

func (s *servicioFalso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	crudo, _ := io.ReadAll(r.Body)
	var c map[string]any
	_ = json.Unmarshal(crudo, &c)
	clave := r.Method + " " + strings.TrimPrefix(r.URL.Path, "/api/v1")
	s.mu.Lock()
	s.vistas = append(s.vistas, peticion{r.Method, strings.TrimPrefix(r.URL.Path, "/api/v1"), c})
	resp, ok := s.respuesta[clave]
	s.mu.Unlock()
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	_, _ = io.WriteString(w, resp)
}

func (s *servicioFalso) ultima(t *testing.T, clave string) map[string]any {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.vistas) - 1; i >= 0; i-- {
		if s.vistas[i].metodo+" "+s.vistas[i].ruta == clave {
			return s.vistas[i].cuerpo
		}
	}
	t.Fatalf("no ha llegado %s; llegaron %v", clave, s.vistas)
	return nil
}

func (s *servicioFalso) rutas() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.vistas))
	for _, v := range s.vistas {
		out = append(out, v.metodo+" "+v.ruta)
	}
	return out
}

func trInterno(t *testing.T) *i18n.Translator {
	t.Helper()
	tr, err := i18n.New("es", i18n.Common(), catalogos.Notes())
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

// pantallaConServicio monta la pantalla contra el servicio falso con un cuaderno
// abierto y sus notas.
func pantallaConServicio(t *testing.T, respuestas map[string]string, notas ...api.Note) (*Pantalla, *servicioFalso) {
	t.Helper()
	falso := &servicioFalso{respuesta: respuestas}
	srv := httptest.NewServer(falso)
	t.Cleanup(srv.Close)
	p := NuevaPantalla(trInterno(t), "es", api.NewNotes(srv.URL, nil, nil), nil, nil)
	c1 := DeCuaderno(api.Notebook{ID: "c1", Title: "Mudanza", NoteCount: len(notas)})
	c2 := DeCuaderno(api.Notebook{ID: "c2", Title: "Trabajo", NoteCount: 5})
	p.cuadernos, p.activo = []Cuaderno{c1, c2}, c1
	p.notas, p.lienzo.Fase = DeNotas(notas), Exito
	return p, falso
}

func ids(ns []Nota) string {
	out := make([]string, 0, len(ns))
	for _, n := range ns {
		out = append(out, n.ID)
	}
	return strings.Join(out, ",")
}

// TestCrearNotaNaceEnSuSitioYSeDeshace: la nota nueva va a la columna siguiente y, en el
// lienzo libre, escalonada —los números son los de `crearNota` en la web—, queda
// seleccionada, y deshacer la manda a la papelera con el mismo ID.
func TestCrearNotaNaceEnSuSitioYSeDeshace(t *testing.T) {
	p, falso := pantallaConServicio(t, map[string]string{
		"POST /notebooks/c1/notes": `{"id":"n9","notebook_id":"c1","kind":"text"}`,
	}, api.Note{ID: "a", NotebookID: "c1"}, api.Note{ID: "b", NotebookID: "c1"})

	if err := p.crear(api.NoteInput{Kind: "text"}); err != nil {
		t.Fatal(err)
	}
	cuerpo := falso.ultima(t, "POST /notebooks/c1/notes")
	// Dos notas ya: columna 2 % 3 = 2 y escalón 24 + 2·28 = 80.
	if cuerpo["col"] != float64(2) || cuerpo["x"] != float64(80) || cuerpo["y"] != float64(80) {
		t.Errorf("la nota nueva viaja con %v", cuerpo)
	}
	if ids(p.notas) != "a,b,n9" || p.lienzo.Seleccionada != "n9" {
		t.Fatalf("después de crear: notas %s, seleccionada %q", ids(p.notas), p.lienzo.Seleccionada)
	}
	if got, quiero := p.hist.SiguienteDeshacer(), p.tr.T("history.create", nil); got != quiero {
		t.Fatalf("lo siguiente que deshacer es %q, y tenía que ser %q", got, quiero)
	}

	p.deshacer()
	if ids(p.notas) != "a,b" {
		t.Errorf("deshacer crear deja %s", ids(p.notas))
	}
	p.rehacer()
	if ids(p.notas) != "a,b,n9" {
		t.Errorf("rehacer crear deja %s", ids(p.notas))
	}
	quiero := []string{"POST /notebooks/c1/notes", "DELETE /notes/n9", "POST /notes/n9/restore"}
	if got := falso.rutas(); strings.Join(got, "|") != strings.Join(quiero, "|") {
		t.Errorf("peticiones %v, esperaba %v", got, quiero)
	}
}

// TestFijarNoPierdeLoQueLaVistaNoPinta: el PUT del servicio SUSTITUYE, así que fijar
// una nota tiene que devolver también lo que esta vista no usa —datos, enlaces, ancho,
// visibilidad—. La casilla de una tarea mandaba solo tipo, título, contenido y
// `checked`, y marcarla le borraba todo lo demás.
//
// VISTO EN ROJO (2026-09-22, en el frontal): armando el cuerpo con los campos de la
// vista, como hacía `marcar`, el PUT llega sin `data`, sin `links` y con `width` a
// cero. Deshecho.
func TestFijarNoPierdeLoQueLaVistaNoPinta(t *testing.T) {
	grupo := "g1"
	original := api.Note{
		ID: "n1", NotebookID: "c1", Kind: "todo", Title: "Llamar", Content: "<p>al banco</p>",
		Data: json.RawMessage(`{"icon":"pin","due":"2026-10-01"}`), Width: 320, Visibility: "organization",
		GroupID: &grupo, Links: []api.NoteLink{{TargetKind: "drive_file", TargetID: "f9", Label: "ticket"}},
	}
	crudo, _ := json.Marshal(original)
	respuesta := strings.Replace(string(crudo), `"pinned":false`, `"pinned":true`, 1)
	p, falso := pantallaConServicio(t, map[string]string{"PUT /notes/n1": respuesta}, original)

	if err := p.editar("n1", Parche{Fijada: ptr(true)}, true); err != nil {
		t.Fatal(err)
	}
	cuerpo := falso.ultima(t, "PUT /notes/n1")
	if cuerpo["pinned"] != true {
		t.Errorf("fijar no manda pinned: %v", cuerpo)
	}
	for campo, quiero := range map[string]string{
		"data":       `{"due":"2026-10-01","icon":"pin"}`,
		"links":      `[{"label":"ticket","target_id":"f9","target_kind":"drive_file"}]`,
		"width":      `320`,
		"visibility": `"organization"`,
		"group_id":   `"g1"`,
	} {
		got, _ := json.Marshal(cuerpo[campo])
		if string(got) != quiero {
			t.Errorf("al fijar, %q viaja como %s y era %s", campo, got, quiero)
		}
	}
	if cuerpo["content"] != "<p>al banco</p>" || cuerpo["title"] != "Llamar" || cuerpo["kind"] != "todo" {
		t.Errorf("al fijar, el texto de la nota viaja como %v / %v / %v", cuerpo["kind"], cuerpo["title"], cuerpo["content"])
	}
	if n, _ := p.nota("n1"); !n.Fijada {
		t.Error("la pantalla no enseña la nota fijada")
	}
	// Y deshacer la desfija.
	p.deshacer()
	if cuerpo := falso.ultima(t, "PUT /notes/n1"); cuerpo["pinned"] != false {
		t.Errorf("deshacer fijar manda %v", cuerpo["pinned"])
	}
}

// TestUnaEscrituraQueFallaVuelveAtras: la actualización optimista se deshace si el
// servicio dice que no, y la acción no entra en el historial.
func TestUnaEscrituraQueFallaVuelveAtras(t *testing.T) {
	falso := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"conflict"}`, http.StatusConflict)
	})
	srv := httptest.NewServer(falso)
	defer srv.Close()
	p := NuevaPantalla(trInterno(t), "es", api.NewNotes(srv.URL, nil, nil), nil, nil)
	p.activo = Cuaderno{ID: "c1"}
	p.notas, p.lienzo.Fase = DeNotas([]api.Note{{ID: "a", NotebookID: "c1"}}), Exito

	if err := p.borrar("a", true); err == nil {
		t.Fatal("un 409 tiene que llegar como error")
	}
	if ids(p.notas) != "a" {
		t.Errorf("la nota que no se pudo borrar ha desaparecido: %s", ids(p.notas))
	}
	if p.hist.SiguienteDeshacer() != "" {
		t.Error("una escritura que falló ha entrado en el historial")
	}
}

// TestMoverACuadernoSeLlevaLaFamilia: la nota viaja con todo lo agrupado bajo ella, a
// cualquier nivel, y deshacer la devuelve.
func TestMoverACuadernoSeLlevaLaFamilia(t *testing.T) {
	a, b := "a", "b"
	p, falso := pantallaConServicio(t, nil,
		api.Note{ID: "a", NotebookID: "c1"},
		api.Note{ID: "b", NotebookID: "c1", GroupID: &a},
		api.Note{ID: "c", NotebookID: "c1", GroupID: &b},
		api.Note{ID: "d", NotebookID: "c1"})

	if err := p.moverACuaderno("a", p.cuadernos[1], true); err != nil {
		t.Fatal(err)
	}
	if c := falso.ultima(t, "POST /notes/a/move"); c["notebook_id"] != "c2" || len(c) != 1 {
		t.Errorf("mover de cuaderno manda %v", c)
	}
	if ids(p.notas) != "d" {
		t.Errorf("tras mover quedan %s y tenía que quedar d", ids(p.notas))
	}
	if p.cuadernos[1].Notas != 8 {
		t.Errorf("el destino dice %d notas y tiene 5 + 3", p.cuadernos[1].Notas)
	}
	p.deshacer()
	if c := falso.ultima(t, "POST /notes/a/move"); c["notebook_id"] != "c1" {
		t.Errorf("deshacer mover manda %v", c)
	}
	if got := ids(p.notas); got != "d,a,b,c" {
		t.Errorf("tras deshacer quedan %s", got)
	}
}

// TestLosAjustesConservanElCifrado: guardar los ajustes de un cuaderno cifrado parte
// del cuaderno ENTERO, así que su cifrado y su sala siguen ahí.
func TestLosAjustesConservanElCifrado(t *testing.T) {
	padre := "p1"
	orig := DeCuaderno(api.Notebook{
		ID: "c1", ParentID: &padre, Title: "Diario", Encrypted: true, RoomID: "sala",
		Crypto: json.RawMessage(`{"v":1}`), Visibility: "private",
	})
	f := formAjustes{color: "#0e7490", disposicion: "list", visibilidad: "private", cifrado: true}
	in := entradaAjustes(&orig, "", "  Diario íntimo ", "📓", f)
	if !in.Encrypted || string(in.Crypto) != `{"v":1}` || in.RoomID != "sala" || in.ParentID == nil || *in.ParentID != "p1" {
		t.Errorf("los ajustes pierden lo que no se edita: %+v", in)
	}
	if in.Title != "Diario íntimo" || in.Indexable == nil || *in.Indexable {
		t.Errorf("título %q e índice %v: un cifrado nunca se indexa", in.Title, in.Indexable)
	}
	// Quitar el cifrado quita también sus parámetros, que es lo que la web manda.
	f.cifrado = false
	if in := entradaAjustes(&orig, "", "Diario", "", f); in.Encrypted || in.Crypto != nil {
		t.Errorf("sin cifrado viajan %v y %s", in.Encrypted, in.Crypto)
	}
	// Uno nuevo dentro de otro lleva a su padre.
	if in := entradaAjustes(nil, "p9", "Hijo", "", f); in.ParentID == nil || *in.ParentID != "p9" {
		t.Errorf("el cuaderno nuevo no cuelga de su padre: %+v", in)
	}
	// Y la casilla del índice vuelve a su valor al cambiar de visibilidad.
	if defectoIndexable("private", false) || !defectoIndexable("personal", false) || defectoIndexable("public", true) {
		t.Error("el índice por defecto no es el de la web")
	}
}

// TestElMenuDeLaNotaEsElDeLaWeb: las entradas y su orden son los de NoteCard.tsx, con
// la que aquí falta («Compartir en Rooms») dicha en `menuNota`.
func TestElMenuDeLaNotaEsElDeLaWeb(t *testing.T) {
	p, _ := pantallaConServicio(t, nil, api.Note{ID: "a", NotebookID: "c1"})
	n, _ := p.nota("a")
	claves := func(m *ui.Menu) []string {
		var out []string
		for _, e := range m.Elementos {
			if it, ok := e.(*ui.ElementoMenu); ok {
				out = append(out, it.Texto)
			} else {
				out = append(out, "---")
			}
		}
		return out
	}
	tr := p.tr.T
	quiero := []string{
		tr("note.setIcon", nil),
		tr("tags.apply", nil), tr("reminder.set", nil), tr("note.pin", nil), tr("note.rename", nil), tr("note.duplicate", nil),
		tr("tree.newInside", nil), tr("note.moveTo", nil), tr("note.export", nil), tr("backlinks.title", nil),
		tr("note.attach", nil), "---", tr("note.delete", nil),
	}
	if got := claves(p.menuNota(n, false)); strings.Join(got, "|") != strings.Join(quiero, "|") {
		t.Errorf("el menú es\n%v\ny en la web\n%v", got, quiero)
	}
	// El del árbol es el mismo con «Renombrar» en su clave y sin adjuntar (NoteTree.tsx).
	quiero[4] = tr("tree.rename", nil)
	quiero = slices.Delete(quiero, 10, 11)
	if got := claves(p.menuNota(n, true)); strings.Join(got, "|") != strings.Join(quiero, "|") {
		t.Errorf("el menú del árbol es\n%v\ny en la web\n%v", got, quiero)
	}
	// En un cuaderno cifrado y cerrado no se mueve ni se exporta, y lo que escribe
	// texto no se puede pulsar.
	p.activo.Cifrado, p.bloqueado = true, true
	m := p.menuNota(n, false)
	for _, e := range m.Elementos {
		it, ok := e.(*ui.ElementoMenu)
		if !ok {
			continue
		}
		if it.Texto == tr("note.moveTo", nil) || it.Texto == tr("note.export", nil) {
			t.Errorf("un cuaderno cifrado ofrece %q", it.Texto)
		}
		if it.Texto == tr("note.duplicate", nil) && !it.Inerte {
			t.Error("duplicar en un cuaderno cifrado cerrado se puede pulsar")
		}
	}
}

// TestLasClavesQueSeArmanExisten: las claves que se componen en tiempo de ejecución
// (`export.` + formato, `visibility.` + clave + `.hint`…) no las ve el guardián central
// de catálogos, que busca literales. Una que no exista sale ESCRITA en pantalla.
func TestLasClavesQueSeArmanExisten(t *testing.T) {
	tr := trInterno(t)
	claves := make([]string, 0, len(FormatosExport)+2*len(Disposiciones)+2*len(Visibilidades)+len(Ordenes))
	for _, f := range FormatosExport {
		claves = append(claves, "export."+f.ID)
	}
	for _, d := range Disposiciones {
		claves = append(claves, d.D.ClaveI18n(), d.D.ClaveI18n()+".hint")
	}
	for _, v := range Visibilidades {
		claves = append(claves, "visibility."+v.Clave, "visibility."+v.Clave+".hint")
	}
	for _, o := range Ordenes {
		claves = append(claves, o.ClaveI18n())
	}
	for _, id := range IconosNota {
		claves = append(claves, RotuloIcono(id))
	}
	for _, r := range append(slices.Clone(Repeticiones), "custom") {
		claves = append(claves, "reminder.repeat."+r)
	}
	for _, k := range claves {
		if tr.T(k, nil) == k {
			t.Errorf("la clave %q no está en el catálogo", k)
		}
	}
}

// TestElHistorialEsElDeLaWeb: deshacer y rehacer se cruzan, lo nuevo borra lo que se
// podía rehacer, una inversa que falla vuelve a su sitio, y los guardados con la misma
// marca se deshacen de una vez.
func TestElHistorialEsElDeLaWeb(t *testing.T) {
	var h Historial
	var hecho []string
	paso := func(n string) Accion {
		return Accion{
			Etiqueta: n,
			Deshacer: func() error { hecho = append(hecho, "-"+n); return nil },
			Rehacer:  func() error { hecho = append(hecho, "+"+n); return nil },
		}
	}
	h.Registrar(paso("a"), "")
	h.Registrar(paso("b"), "")
	if e, _ := h.Deshacer(); e != "b" || h.SiguienteRehacer() != "b" || h.SiguienteDeshacer() != "a" {
		t.Fatalf("deshacer b: %q, rehacer %q, deshacer %q", e, h.SiguienteRehacer(), h.SiguienteDeshacer())
	}
	h.Registrar(paso("c"), "")
	if h.SiguienteRehacer() != "" {
		t.Error("una acción nueva tiene que borrar lo que se podía rehacer")
	}

	// La inversa que falla se queda como lo siguiente que deshacer.
	h.Registrar(Accion{Etiqueta: "roto", Deshacer: func() error { return errors.New("red") }}, "")
	if _, err := h.Deshacer(); err == nil || h.SiguienteDeshacer() != "roto" {
		t.Errorf("tras fallar, lo siguiente es %q (err %v)", h.SiguienteDeshacer(), err)
	}

	// Tres guardados de la misma edición son UN paso: deshace hasta el primero y
	// rehace hasta el último.
	var g Historial
	var estado string
	guardar := func(de, a string) Accion {
		return Accion{
			Etiqueta: "editar",
			Deshacer: func() error { estado = de; return nil },
			Rehacer:  func() error { estado = a; return nil },
		}
	}
	g.Registrar(guardar("", "h"), "s1")
	g.Registrar(guardar("h", "ho"), "s1")
	g.Registrar(guardar("ho", "hola"), "s1")
	_, _ = g.Deshacer()
	if estado != "" || g.SiguienteDeshacer() != "" {
		t.Errorf("deshacer una edición fusionada deja %q", estado)
	}
	_, _ = g.Rehacer()
	if estado != "hola" {
		t.Errorf("rehacer una edición fusionada deja %q", estado)
	}

	// Y el tope: cincuenta pasos.
	var l Historial
	for range 60 {
		l.Registrar(paso("x"), "")
	}
	if len(l.pasado) != limiteHistorial {
		t.Errorf("el historial guarda %d pasos y la web %d", len(l.pasado), limiteHistorial)
	}
}

// Duplicar NO hereda el client_id del original, haya almacén o no: el servicio
// deduplicaría por él, devolvería el original y su «Deshacer» lo borraría.
func TestDuplicarLlevaClientIDNuevo(t *testing.T) {
	p, falso := pantallaConServicio(t, map[string]string{
		"POST /notebooks/c1/notes": `{"id":"n9","notebook_id":"c1","kind":"text"}`,
	}, api.Note{ID: "a", NotebookID: "c1", Kind: "text", Data: json.RawMessage(`{"client_id":"orig"}`)})
	p.Almacen = nil
	if err := p.duplicar(p.notas[0]); err != nil {
		t.Fatal(err)
	}
	id, _ := falso.ultima(t, "POST /notebooks/c1/notes")["data"].(map[string]any)["client_id"].(string)
	if id == "" || id == "orig" {
		t.Fatalf("la copia sale con client_id %q", id)
	}
}
