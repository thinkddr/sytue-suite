// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import "testing"

// LOS GUARDIANES DE LAS ÓRDENES DEL EDITOR. Cada uno parte de un HTML como el que guarda
// la web y comprueba el HTML que sale, que es lo que el servicio va a sanear y la web va a
// pintar: el editor de aquí y el de la web tienen que escribir lo mismo.

func editar(t *testing.T, html string) *Documento {
	t.Helper()
	d := Leer(html)
	if !d.Fiel() {
		t.Fatalf("el documento de partida no es fiel: %q", html)
	}
	return d.ParaEditar()
}

func quiero(t *testing.T, d *Documento, html string) {
	t.Helper()
	if got := d.ParaGuardar(); got != html {
		t.Errorf("sale\n  %s\ny tenía que salir\n  %s", got, html)
	}
	if !Leer(d.ParaGuardar()).Fiel() {
		t.Errorf("lo que sale no vuelve entero: %s", d.ParaGuardar())
	}
}

// TestEscribirEnUnaNotaVacia: el primer texto va suelto (como en un `contenteditable`
// nuevo), Intro abre un `<div>` y la línea vacía lleva su `<br>`.
func TestEscribirEnUnaNotaVacia(t *testing.T) {
	d := Leer("")
	p := d.Insertar(Pos{}, "hola", Tramo{})
	quiero(t, d, "hola")
	p = d.Partir(p)
	quiero(t, d, "hola<div><br/></div>")
	d.Insertar(p, "adiós", estiloEn(d.Bloques[p.B], p.O))
	quiero(t, d, "hola<div>adiós</div>")
}

// TestPartirUnParrafoYUnTitulo: un `<p>` se parte en dos `<p>`; al final de un título
// sigue un `<div>`, y en medio el título se parte en dos títulos.
func TestPartirUnParrafoYUnTitulo(t *testing.T) {
	d := editar(t, "<p>ab</p>")
	d.Partir(Pos{0, 1})
	quiero(t, d, "<p>a</p><p>b</p>")

	h := editar(t, "<h1>Título</h1>")
	h.Partir(Pos{0, 6})
	quiero(t, h, "<h1>Título</h1><div><br/></div>")
	h2 := editar(t, "<h1>Título</h1>")
	h2.Partir(Pos{0, 3})
	quiero(t, h2, "<h1>Tít</h1><h1>ulo</h1>")
}

// TestLaTareaSeEscribeComoEnLaWeb: Intro en una tarea con texto da otra tarea sin marcar
// con lo que había detrás; en una vacía, sale a un párrafo; y el `&nbsp;` de una tarea
// recién puesta se pisa al escribir.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con lo escrito FUERA del `<span>`, la tarea
// sale sin él. Deshecho.
func TestLaTareaSeEscribeComoEnLaWeb(t *testing.T) {
	d := editar(t, `<div class="task-row"><input type="checkbox" checked/> <span>pan y leche</span></div>`)
	p := d.Partir(Pos{0, 1 + 3})
	quiero(t, d, `<div class="task-row"><input type="checkbox" checked/> <span>pan</span></div>`+
		`<div class="task-row"><input type="checkbox"/> <span> y leche</span></div>`)
	if p != (Pos{1, 1}) {
		t.Errorf("el cursor queda en %v y va al principio de la tarea nueva", p)
	}
	vacia := editar(t, `<div class="task-row"><input type="checkbox"/> <span>&nbsp;</span></div>`)
	q := vacia.Insertar(Pos{0, 0}, "x", estiloEn(vacia.Bloques[0], 1))
	quiero(t, vacia, `<div class="task-row"><input type="checkbox"/> <span>x</span></div>`)
	vacia.Borrar(Pos{0, 1}, q)
	vacia.Partir(Pos{0, 1})
	quiero(t, vacia, "<p><br/></p>")
}

// TestNegritaSeAlternaSobreLaSeleccion: se pone con `<b>` y, si toda la selección ya lo
// estaba (con `<b>` o con `<strong>`), se quita.
//
// VISTO EN ROJO (2026-09-23, en el frontal): poniendo siempre la marca, no se quita.
func TestNegritaSeAlternaSobreLaSeleccion(t *testing.T) {
	d := editar(t, "<p>Nueva calle</p>")
	d.AlternarMarca(Pos{0, 0}, Pos{0, 5}, "b")
	quiero(t, d, "<p><b>Nueva</b> calle</p>")
	d.AlternarMarca(Pos{0, 0}, Pos{0, 5}, "b")
	quiero(t, d, "<p>Nueva calle</p>")
	s := editar(t, "<p><strong>ya</strong></p>")
	s.AlternarMarca(Pos{0, 0}, Pos{0, 2}, "b")
	quiero(t, s, "<p>ya</p>")
}

// TestBorrarUneLosBloques: Retroceso al principio de un párrafo lo junta con el anterior,
// y una selección entre bloques deja el primero con el final del último.
func TestBorrarUneLosBloques(t *testing.T) {
	d := editar(t, "<p>uno</p><p>dos</p>")
	if p := d.Retroceso(Pos{1, 0}); p != (Pos{0, 3}) {
		t.Errorf("el cursor queda en %v", p)
	}
	quiero(t, d, "<p>unodos</p>")
	e := editar(t, "<p>uno</p><h2>dos</h2><p>tres</p>")
	e.Borrar(Pos{0, 1}, Pos{2, 2})
	quiero(t, e, "<p>ues</p>")
	l := editar(t, "<ul><li>a</li></ul>")
	l.Retroceso(Pos{0, 0})
	quiero(t, l, "<div>a</div>")
}

// TestSangrarAnidaLaLista: subir de nivel un elemento lo mete DENTRO del anterior, y la
// nota sigue siendo fiel al volver a leerla.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin abrir la lista honda dentro del `<li>`,
// la lista sangrada sale plana. Deshecho.
func TestSangrarAnidaLaLista(t *testing.T) {
	d := editar(t, "<ul><li>a</li><li>b</li><li>c</li></ul>")
	d.Sangrar(Pos{1, 0}, Pos{1, 0}, 1)
	quiero(t, d, "<ul><li>a<ul><li>b</li></ul></li><li>c</li></ul>")
	d.Sangrar(Pos{1, 0}, Pos{1, 0}, -1)
	quiero(t, d, "<ul><li>a</li><li>b</li><li>c</li></ul>")
}

// TestLosAtajosDeMarkdown son los de `client/markdown.ts`.
func TestLosAtajosDeMarkdown(t *testing.T) {
	casos := []struct {
		linea  string
		intro  bool
		orden  string
		valor  string
		borrar int
	}{
		{"-", false, "vinetas", "", 1},
		{"1.", false, "numeros", "", 2},
		{"###", false, "bloque", "h3", 3},
		{">", false, "bloque", "blockquote", 1},
		{"```", false, "bloque", "pre", 3},
		{"[]", false, "tarea", "", 2},
		{"- [ ]", false, "tarea", "", 5},
		{"---", true, "regla", "", 3},
	}
	for _, c := range casos {
		orden, valor, borrar, ok := atajoMarkdown(c.linea, c.intro)
		if !ok || orden != c.orden || valor != c.valor || borrar != c.borrar {
			t.Errorf("«%s»: %q %q %d %v", c.linea, orden, valor, borrar, ok)
		}
	}
	for _, no := range []string{"a", "######", "--", "1.2"} {
		if _, _, _, ok := atajoMarkdown(no, false); ok {
			t.Errorf("«%s» dispara un atajo", no)
		}
	}
}

// TestFormatoDeBloqueYListas: `formatBlock` y las listas sobre la selección.
func TestFormatoDeBloqueYListas(t *testing.T) {
	d := editar(t, "<p>uno</p><p>dos</p>")
	d.Formatear(Pos{0, 0}, Pos{0, 0}, "h2")
	quiero(t, d, "<h2>uno</h2><p>dos</p>")
	d.AlternarLista(Pos{0, 0}, Pos{1, 1}, Numerada)
	quiero(t, d, "<ol><li>uno</li><li>dos</li></ol>")
	d.AlternarLista(Pos{0, 0}, Pos{1, 1}, Numerada)
	quiero(t, d, "<div>uno</div><div>dos</div>")
	d.Alinear(Pos{0, 0}, Pos{0, 0}, "center")
	quiero(t, d, `<div style="text-align:center">uno</div><div>dos</div>`)
}

// TestEnlazarYColor: el enlace nace con el `target`/`rel` que el servicio pone, y el
// color va en un `<span style>`.
func TestEnlazarYColor(t *testing.T) {
	d := editar(t, "<p>ver web</p>")
	d.Enlazar(Pos{0, 4}, Pos{0, 7}, "https://sytue.eu")
	quiero(t, d, `<p>ver <a href="https://sytue.eu" target="_blank" rel="noopener noreferrer">web</a></p>`)
	d.Pintar(Pos{0, 0}, Pos{0, 3}, "#b91c1c")
	quiero(t, d, `<p><span style="color:#b91c1c">ver</span> <a href="https://sytue.eu" target="_blank" rel="noopener noreferrer">web</a></p>`)
	d.QuitarFormato(Pos{0, 0}, Pos{0, 7})
	quiero(t, d, `<p>ver <a href="https://sytue.eu" target="_blank" rel="noopener noreferrer">web</a></p>`)
}

// TestPegarUnDocumentoComoElNavegador: unas palabras con formato entran en la línea del
// cursor; varios bloques parten la línea, el primero se une a lo de delante y lo de
// detrás sigue al último; en una nota vacía, lo pegado es la nota; y en una tarea, lo
// pegado va en su `<span>`.
func TestPegarUnDocumentoComoElNavegador(t *testing.T) {
	d := editar(t, "<p>Hola equipo</p>")
	p := d.InsertarDocumento(Pos{0, 5}, Leer("<b>querido</b>, "+"<i>mi</i>"))
	quiero(t, d, "<p>Hola <b>querido</b>, <i>mi</i>equipo</p>")
	if p != (Pos{0, 16}) {
		t.Errorf("cursor en %v tras pegar en línea", p)
	}

	d = editar(t, "<p>Antes|después</p>")
	p = d.InsertarDocumento(Pos{0, 6}, Leer("<p>uno</p><ul><li>dos</li><li>tres</li></ul>"))
	quiero(t, d, "<p>Antes|uno</p><ul><li>dos</li><li>tresdespués</li></ul>")
	if p != (Pos{2, 4}) {
		t.Errorf("cursor en %v tras pegar bloques; tenía que quedar tras «tres»", p)
	}

	d = Leer("")
	p = d.InsertarDocumento(Pos{}, Leer("<h2>Título</h2><p>texto</p>"))
	quiero(t, d, "<h2>Título</h2><p>texto</p>")
	if p != (Pos{1, 5}) {
		t.Errorf("cursor en %v en una nota vacía", p)
	}

	d = editar(t, `<div class="task-row"><input type="checkbox"/> <span>comprar</span></div>`)
	d.InsertarDocumento(Pos{0, 8}, Leer("<b>pan</b>"))
	quiero(t, d, `<div class="task-row"><input type="checkbox"/> <span>comprar</span><b><span>pan</span></b></div>`)
}
