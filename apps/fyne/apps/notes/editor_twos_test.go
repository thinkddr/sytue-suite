// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// LOS GUARDIANES DEL EDITOR AL ESTILO TWOS: el menú de «/», la burbuja de formato, buscar
// en la nota, las listas plegables, «Ir a» y la ficha de un enlace.

// Los mismos casos que client/comandos.test.ts.
func TestLaBarraAbreElMenuComoEnLaWeb(t *testing.T) {
	si := map[string]string{"/": "", "/tit": "tit", "Hola /h1": "h1", "Hola\u00a0/": ""}
	for antes, quiero := range si {
		if q, ok := consultaDeBarra(antes); !ok || q != quiero {
			t.Errorf("consultaDeBarra(%q) = %q, %v; quiero %q", antes, q, ok, quiero)
		}
	}
	for _, antes := range []string{"a/b", "https://sytue.eu/", "/titulo ", "/a/b", "/" + strings.Repeat("x", 21), "sin barra"} {
		if q, ok := consultaDeBarra(antes); ok {
			t.Errorf("consultaDeBarra(%q) abre el menú con %q", antes, q)
		}
	}
	tr := trInterno(t)
	rotulo := func(c comando) string { return tr.T(c.clave, nil) }
	ids := func(cs []comando) string {
		out := make([]string, 0, len(cs))
		for _, c := range cs {
			out = append(out, c.id)
		}
		return strings.Join(out, ",")
	}
	if got := ids(filtrarComandos("titulo", rotulo)); got != "h1,h2,h3" {
		t.Errorf("«titulo» deja %s", got)
	}
	if got := ids(filtrarComandos("todo", rotulo)); got != "tarea" {
		t.Errorf("«todo» deja %s", got)
	}
	if got := ids(filtrarComandos("---", rotulo)); got != "regla" {
		t.Errorf("«---» deja %s", got)
	}
	if len(filtrarComandos("", rotulo)) != len(comandos) || len(filtrarComandos("zzz", rotulo)) != 0 {
		t.Error("sin consulta salen todos y con una que no casa, ninguno")
	}
}

// TestElMenuDeBarraPoneElBloque: «/h2» e Intro convierten la línea en un título y borran
// lo escrito; Escape lo deja como texto; las flechas eligen otra fila.
//
// VISTO EN ROJO (2026-10-02, en el frontal): sin `teclaMenu` al principio de `tecla`,
// Intro parte el bloque y «/h2» se queda escrito.
func TestElMenuDeBarraPoneElBloque(t *testing.T) {
	ed := NuevoEditorNota(trInterno(t), Leer("<p>hola</p>"), "")
	z := ed.zona
	z.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	teclear(z, "/h2")
	if ed.menu == nil || ed.menu.q != "h2" {
		t.Fatalf("la barra no abre el menú: %+v", ed.menu)
	}
	z.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	teclear(z, "Plan")
	if got := ed.HTML(); got != "<p>hola</p><h2>Plan</h2>" {
		t.Errorf("«/h2» e Intro dejan %q", got)
	}
	if ed.menu != nil {
		t.Error("el menú sigue abierto tras elegir")
	}

	es := NuevoEditorNota(trInterno(t), Leer(""), "")
	teclear(es.zona, "/")
	es.zona.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	es.zona.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	if es.menu == nil || es.menu.idx != 2 {
		t.Fatalf("las flechas no mueven la fila activa: %+v", es.menu)
	}
	es.zona.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	teclear(es.zona, "x")
	if es.menu != nil || es.HTML() != "<p>/x</p>" && es.HTML() != "<div>/x</div>" && es.HTML() != "/x" {
		t.Errorf("Escape no deja la barra como texto: %q, menú %+v", es.HTML(), es.menu)
	}
}

// TestLaBurbujaSaleConLaSeleccion: con algo seleccionado y el cuerpo enfocado se ve la
// burbuja, y su negrita da formato a lo seleccionado.
func TestLaBurbujaSaleConLaSeleccion(t *testing.T) {
	ed := NuevoEditorNota(trInterno(t), Leer("<p>hola mundo</p>"), "")
	ed.zona.FocusGained()
	if ed.conBurbuja() {
		t.Fatal("sin selección no hay burbuja")
	}
	ed.ancla, ed.cur = Pos{0, 0}, Pos{0, 4}
	if !ed.conBurbuja() || len(ed.flotantes()) != 1 {
		t.Fatal("con selección no sale la burbuja")
	}
	fila := ed.burbuja.contenido.(*fyne.Container)
	fila.Objects[0].(*botonBarra).Tapped(&fyne.PointEvent{})
	if got := ed.HTML(); got != "<p><b>hola</b> mundo</p>" {
		t.Errorf("la negrita de la burbuja deja %q", got)
	}
}

// TestBuscarEnLaNota: Ctrl+F abre la barra, se encuentra sin acentos, Intro va a la
// siguiente dando la vuelta y Escape deja seleccionada la coincidencia.
//
// VISTO EN ROJO (2026-10-02, en el frontal): con `b.i = i` sin el módulo, la tercera
// pulsación se sale de la lista y la selección no se mueve.
func TestBuscarEnLaNota(t *testing.T) {
	ed := NuevoEditorNota(trInterno(t), Leer("<p>Reunión de equipo</p><p>otra reunion</p>"), "")
	ed.zona.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: fyne.KeyModifierShortcutDefault})
	if ed.buscador == nil {
		t.Fatal("Ctrl+F no abre la búsqueda")
	}
	ed.buscador.campo.PonerTexto("reunion")
	ed.buscarEnNota("reunion", 0)
	if got := ed.Seleccionado(); got != "Reunión" {
		t.Errorf("la primera coincidencia es %q", got)
	}
	if got := ed.buscador.cuenta.Text; got != "1 de 2" {
		t.Errorf("la cuenta dice %q", got)
	}
	enter := &fyne.KeyEvent{Name: fyne.KeyReturn}
	ed.buscador.campo.AlTeclear(enter)
	ed.buscador.campo.AlTeclear(enter)
	if got := ed.Seleccionado(); got != "Reunión" {
		t.Errorf("dar la vuelta deja %q", got)
	}
	ed.buscador.campo.AlTeclear(enter)
	ed.buscador.campo.AlTeclear(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if ed.buscador != nil || ed.Seleccionado() != "reunion" {
		t.Errorf("Escape deja la búsqueda %v y la selección %q", ed.buscador != nil, ed.Seleccionado())
	}
	ed.buscarEnNota("nada", 0)
	ed.abrirBuscador()
	ed.buscarEnNota("zzz", 0)
	if got := ed.buscador.cuenta.Text; got != ed.tr.T("find.none", nil) {
		t.Errorf("sin coincidencias la cuenta dice %q", got)
	}
}

// TestLaListaPlegadaSeEscondeAlLeer: `<li class="plegada">` es fiel, al leer esconde su
// sublista (y pone «…»), al escribir se ve entera, y plegar y desplegar vuelve al HTML
// de partida.
//
// VISTO EN ROJO (2026-10-02, en el frontal): sin `ocultos` en Maquetar, la sublista se
// sigue pintando al leer.
func TestLaListaPlegadaSeEscondeAlLeer(t *testing.T) {
	fuente := `<ul><li class="plegada">a<ul><li>b</li></ul></li><li>c</li></ul>`
	d := Leer(fuente)
	if !d.Fiel() || d.HTML() != fuente {
		t.Fatalf("una lista plegada no es fiel: %q", d.HTML())
	}
	pintados := func(d *Documento) map[int]bool {
		out := map[int]bool{}
		for _, l := range Maquetar(d, 400, tema.Claro).Lineas {
			out[l.Bloque] = true
		}
		return out
	}
	if l := pintados(d); l[1] || !l[0] || !l[2] {
		t.Errorf("al leer se pintan los bloques %v; la sublista debe esconderse", l)
	}
	if !strings.Contains(textoDeLineas(Maquetar(d, 400, tema.Claro), 0), "…") {
		t.Error("el plegado no lleva sus puntos")
	}
	if l := pintados(d.ParaEditar()); !l[1] {
		t.Error("al escribir la sublista tiene que verse")
	}
	c := d.copia()
	if !c.AlternarPlegado(0) || c.HTML() != `<ul><li>a<ul><li>b</li></ul></li><li>c</li></ul>` {
		t.Errorf("desplegar deja %q", c.HTML())
	}
	if c.AlternarPlegado(2) {
		t.Error("un elemento sin sublista no se pliega")
	}
}

func textoDeLineas(m *Maquetado, bloque int) string {
	var b strings.Builder
	for _, l := range m.Lineas {
		if l.Bloque == bloque {
			for _, tr := range l.Trozos {
				b.WriteString(tr.Texto)
			}
		}
	}
	return b.String()
}

// TestPlegarDesdeLaLecturaGuarda: pulsar la viñeta de una lista con sublista guarda la
// nota plegada.
func TestPlegarDesdeLaLecturaGuarda(t *testing.T) {
	p, falso := pantallaConServicio(t, map[string]string{
		"PUT /notes/n1": `{"id":"n1","notebook_id":"c1","kind":"text"}`,
	}, api.Note{ID: "n1", NotebookID: "c1", Kind: "text", Content: `<ul><li>a<ul><li>b</li></ul></li></ul>`})
	n, _ := p.nota("n1")
	p.plegarLista(n, 0)
	esperarRuta(t, falso, "PUT /notes/n1")
	if c := falso.ultima(t, "PUT /notes/n1"); c["content"] != `<ul><li class="plegada">a<ul><li>b</li></ul></li></ul>` {
		t.Errorf("plegar manda %v", c["content"])
	}
}

// TestIrAEncuentraCuadernosYNotas: sin acentos, los cuadernos primero, las notas del
// abierto después y las de la búsqueda sin repetir.
func TestIrAEncuentraCuadernosYNotas(t *testing.T) {
	cuadernos := []Cuaderno{{ID: "c1", Titulo: "Reuniones"}, {ID: "c2", Titulo: "Casa"}}
	notas := DeNotas([]api.Note{
		{ID: "n1", NotebookID: "c1", Title: "Reunión de lunes"},
		{ID: "n2", NotebookID: "c1", Title: "Compra"},
	})
	remotas := []api.NoteHit{{ID: "n1", NotebookID: "c1", Title: "Reunión de lunes"}, {ID: "n7", NotebookID: "c2", Notebook: "Casa", Title: "reunion vecinos"}}
	got := opcionesIrA("reunion", cuadernos, notas, remotas, "Sin título")
	ids := make([]string, 0, len(got))
	for _, o := range got {
		ids = append(ids, o.ID)
	}
	if strings.Join(ids, ",") != "c1,n1,n7" || !got[0].Cuaderno || got[1].Detalle != "Reuniones" {
		t.Errorf("«Ir a» da %+v", got)
	}
}

// TestLaFichaDeUnEnlaceEsDeLaSuite: un enlace a una nota del cuaderno abierto trae su
// título y su principio; uno a la web no tiene ficha.
func TestLaFichaDeUnEnlaceEsDeLaSuite(t *testing.T) {
	p, _ := pantallaConServicio(t, nil, api.Note{ID: "n9", NotebookID: "c1", Kind: "text", Title: "Lista", Content: "<p>leche y pan</p>"})
	p.AppURL = func(ruta string) string { return "https://notes.sytue.eu" + ruta }
	vp, listo := p.previaDe("https://notes.sytue.eu/?notebook=c1&note=n9", func() {})
	if !listo || !vp.Encontrada || vp.Titulo != "Lista" || vp.Texto != "leche y pan" {
		t.Errorf("la ficha es %+v (lista %v)", vp, listo)
	}
	if _, listo := p.previaDe("https://example.org/", func() {}); listo {
		t.Error("un enlace a la web no debe tener ficha")
	}
	if vp, listo := p.previaDe("https://notes.sytue.eu/?notebook=c1&note=otra", func() {}); !listo || vp.Encontrada {
		t.Errorf("una nota que no está debe decir que no se ve: %+v", vp)
	}
}

// esperarRuta deja pasar el guardado, que va en otra goroutine, hasta que llega la ruta.
func esperarRuta(t *testing.T, falso *servicioFalso, ruta string) {
	t.Helper()
	for i := 0; i < 200 && !slices.Contains(falso.rutas(), ruta); i++ {
		fyne.DoAndWait(func() {})
		time.Sleep(10 * time.Millisecond)
	}
}
