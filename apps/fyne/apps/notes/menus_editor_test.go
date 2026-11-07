// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Los menús que se abren al teclear en el editor de una nota: los casos de
// menus-editor.test.tsx de la web. «[[» ofrece las notas que enlazar, con la búsqueda de
// la herramienta de enlaces, y «@» la gente a la que mencionar.

// TestDobleCorcheteAbreComoEnLaWeb: los casos de `consultaDeEnlace` en comandos.test.ts.
//
// VISTO EN ROJO (2026-11-05, en el frontal): sin cambiar el espacio duro por uno normal,
// «[[Plan» seguido del espacio que deja el final de una línea editable no abría nada.
func TestDobleCorcheteAbreComoEnLaWeb(t *testing.T) {
	for antes, quiero := range map[string]string{
		"[[": "", "ver [[Presupuesto de oc": "Presupuesto de oc", "a[[b": "b", "[[Plan\u00a0": "Plan ",
	} {
		if q, ok := consultaDeEnlace(antes); !ok || q != quiero {
			t.Errorf("consultaDeEnlace(%q) = %q, %v; quiero %q", antes, q, ok, quiero)
		}
	}
	for _, antes := range []string{"[[x]] y sigue", "[[x] y", "[ [x", "[[" + strings.Repeat("a", 61)} {
		if q, ok := consultaDeEnlace(antes); ok {
			t.Errorf("consultaDeEnlace(%q) abre la lista con %q", antes, q)
		}
	}
}

// editorConFuentes abre una nota vacía de un cuaderno con dos notas más y con agenda. El
// último valor cuenta cuántas veces se ha pedido la gente. Es atómico: lo sube el
// servidor de prueba en su rutina y lo lee la prueba (con `-race`, el CI lo vio).
func editorConFuentes(t *testing.T, cifrado bool) (*Pantalla, *EditorNota, *atomic.Int32) {
	t.Helper()
	var pedidas atomic.Int32
	dir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pedidas.Add(1)
		_, _ = w.Write([]byte(`{"items":[{"user_id":"u1","email":"ana@acme.test","name":"Ana Pérez"},{"user_id":"u2","email":"luis@acme.test"}]}`))
	}))
	t.Cleanup(dir.Close)
	p, _ := pantallaConServicio(t, nil,
		api.Note{ID: "a", NotebookID: "c1", Kind: "text", Title: "Presupuesto de octubre"},
		api.Note{ID: "b", NotebookID: "c1", Kind: "text", Title: "Viaje a Lisboa"},
		api.Note{ID: "n", NotebookID: "c1", Kind: "text"})
	p.AppURL = func(r string) string { return "https://notes.sytue.test" + r }
	p.Directorio = api.NewDirectorio(dir.URL, nil, nil)
	p.activo.Cifrado = cifrado
	n, _ := p.nota("n")
	p.abrirEditor(n)
	if p.edicion == nil {
		t.Fatal("la nota no se abre")
	}
	return p, p.edicion.editor, &pedidas
}

// rotulos son las filas del menú abierto, o nil si no lo hay.
func rotulos(ed *EditorNota) []string {
	if ed.menu == nil {
		return nil
	}
	out := []string{}
	for _, s := range ed.sugerencias(ed.menu) {
		out = append(out, s.rotulo+s.detalle)
	}
	return out
}

// TestDobleCorcheteEnlazaUnaNota: «[[» en cualquier sitio de la línea ofrece las notas
// que casan, Escape la cierra y no vuelve en el mismo «[[», e Intro escribe lo MISMO que
// la herramienta de enlaces sin selección.
//
// VISTO EN ROJO (2026-11-05, en el frontal): sin `menuNota` en `disparos` la lista no
// salía; y sin el `Borrar` de `tomarDisparo`, «[[presu» se quedaba delante del enlace.
func TestDobleCorcheteEnlazaUnaNota(t *testing.T) {
	p, ed, _ := editorConFuentes(t, false)
	teclear(ed.zona, "ver [[presu")
	if got := rotulos(ed); len(got) != 1 || got[0] != "Presupuesto de octubre" || ed.menu.panel.rotulo != p.tr.T("editor.linkNote", nil) {
		t.Fatalf("«[[presu» ofrece %v", got)
	}
	ed.zona.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	teclear(ed.zona, "p")
	if ed.menu != nil {
		t.Fatal("tras Escape, el mismo «[[» vuelve a abrir la lista")
	}

	q, ed2, _ := editorConFuentes(t, false)
	teclear(ed2.zona, "ver [[presu")
	ed2.zona.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	// Lo mismo, con la herramienta: «ver » y el enlace elegido en ella.
	_, ed3, _ := editorConFuentes(t, false)
	teclear(ed3.zona, "ver ")
	o := q.opcionesLocales("presu")[0]
	ed3.aplicarEnlace(o.url, o.rotulo)
	if got, quiero := ed2.HTML(), ed3.HTML(); got != quiero || !strings.Contains(got, `href="https://notes.sytue.test/?notebook=c1&amp;note=a"`) {
		t.Errorf("«[[» e Intro escriben\n%s\ny la herramienta\n%s", got, quiero)
	}
	if ed2.menu != nil {
		t.Error("la lista sigue abierta tras elegir")
	}
}

// TestDobleCorcheteSinNadaLoDice: sin nada que case, la lista dice que no hay nada (tras
// buscar en los demás cuadernos); un corchete de cierre la cierra; y sin escribir nada
// tras «[[» salen las notas del cuaderno por su nombre.
//
// VISTO EN ROJO (2026-11-05, en el frontal): con `d.tipo != menuNota` quitado de
// mirarMenu, «[[zzz» no enseñaba la lista y no decía que no hubiera nada.
func TestDobleCorcheteSinNadaLoDice(t *testing.T) {
	_, ed, _ := editorConFuentes(t, false)
	teclear(ed.zona, "[[")
	if got := rotulos(ed); len(got) != 2 || got[0] != "Presupuesto de octubre" {
		t.Errorf("«[[» solo ofrece %v", got)
	}
	teclear(ed.zona, "zzz")
	if ed.menu == nil || len(rotulos(ed)) != 0 {
		t.Fatalf("«[[zzz» deja la lista %v", rotulos(ed))
	}
	esperar(t, func() bool { return !ed.enlaces.buscando })
	if !contieneTexto(ed.menu.panel, ed.tr.T("editor.linkNoResults", nil)) {
		t.Error("sin nada que case, la lista no lo dice")
	}
	teclear(ed.zona, "]] y sigo")
	if ed.menu != nil {
		t.Error("un corchete de cierre no cierra la lista")
	}
}

// TestArrobaMencionaAAlguien: «@» y una letra ofrecen a la gente por nombre o por correo
// (la agenda se pide UNA vez, a la primera arroba), e Intro escribe `@correo `; una
// arroba suelta, o dentro de una dirección, no menciona.
//
// VISTO EN ROJO (2026-11-05, en el frontal): sin `if m.q == ""` en `sugerencias`, una
// arroba suelta abría la lista con toda la organización.
func TestArrobaMencionaAAlguien(t *testing.T) {
	p, ed, pedidas := editorConFuentes(t, false)
	teclear(ed.zona, "Hablar con @")
	esperar(t, func() bool { return pedidas.Load() == 1 && len(p.agendaMenciones().Gente(nil, nil, nil)) == 2 })
	if ed.menu != nil {
		t.Errorf("una arroba suelta abre %v", rotulos(ed))
	}
	teclear(ed.zona, "pé")
	if got := rotulos(ed); len(got) != 1 || got[0] != "Ana Pérezana@acme.test" || ed.menu.panel.rotulo != p.tr.T("editor.mention", nil) {
		t.Fatalf("«@pé» ofrece %v", got)
	}
	ed.zona.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if got := ed.HTML(); got != "Hablar con @ana@acme.test " {
		t.Errorf("la mención escribe %q", got)
	}
	teclear(ed.zona, "y escribe a ana@acme")
	if ed.menu != nil {
		t.Error("una arroba dentro de una dirección menciona")
	}
	teclear(ed.zona, " @")
	if ed.menu != nil {
		t.Errorf("una arroba suelta, con la gente ya aquí, abre %v", rotulos(ed))
	}
	teclear(ed.zona, "lu")
	if got := rotulos(ed); len(got) != 1 || got[0] != "luis@acme.test" || pedidas.Load() != 1 {
		t.Errorf("la segunda mención ofrece %v (la agenda se ha pedido %d veces)", got, pedidas.Load())
	}
}

// TestSinGenteLaArrobaEsUnaArroba: en un cuaderno cifrado el servicio no lee a quién
// avisar, y la arroba no ofrece a nadie aunque la gente ya esté aquí; la barra sigue
// abriendo los bloques.
//
// VISTO EN ROJO (2026-11-05, en el frontal): sin `!p.activo.Cifrado` al darle la gente
// al editor, la lista salía en un cuaderno cifrado.
func TestSinGenteLaArrobaEsUnaArroba(t *testing.T) {
	p, ed, _ := editorConFuentes(t, true)
	p.agendaMenciones().Poner([]api.Miembro{{Email: "ana@acme.test", Nombre: "Ana"}})
	teclear(ed.zona, "Hablar con @an")
	if ed.menu != nil {
		t.Errorf("en un cuaderno cifrado la arroba ofrece %v", rotulos(ed))
	}
	teclear(ed.zona, " /tab")
	if got := rotulos(ed); len(got) != 1 || got[0] != ed.tr.T("editor.table", nil)+"/table" || ed.menu.panel.rotulo != p.tr.T("slash.title", nil) {
		t.Errorf("la barra ofrece %v", got)
	}
}

// contieneTexto dice si algún texto pintado dentro de `o` es `s`.
func contieneTexto(o fyne.CanvasObject, s string) bool {
	hay := false
	cadaObjeto(o, func(x fyne.CanvasObject) {
		if txt, ok := x.(*canvas.Text); ok && txt.Text == s {
			hay = true
		}
	})
	return hay
}
