// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"math"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// TestElHTMLDeLaCeldaEsElDeLaWeb son los casos de `parseCell`/`renderCellHtml`,
// `escapeHtml` y `stripHtml`: el envoltorio va en el orden alineación → negrita →
// cursiva, y leer y volver a escribir es la identidad.
func TestElHTMLDeLaCeldaEsElDeLaWeb(t *testing.T) {
	for _, html := range []string{
		"x",
		"<b>x</b>",
		"<i>x</i>",
		"<b><i>x</i></b>",
		`<div style="text-align: center;">x</div>`,
		`<div style="text-align: right;"><b><i>a &amp; b</i></b></div>`,
	} {
		if got := leerCelda(html).html(); got != html {
			t.Errorf("%q leído y vuelto a escribir da %q", html, got)
		}
	}
	// `parseCell` es tolerante con lo que llega de fuera: mayúsculas y sin espacio.
	if p := leerCelda(`<DIV style="text-align:right">x</DIV>`); p.alinear != "right" || p.dentro != "x" {
		t.Errorf("la alineación sin espacio y en mayúsculas se lee como %+v", p)
	}
	if got := escaparHTML(`a<b>&"c"`); got != "a&lt;b&gt;&amp;&quot;c&quot;" {
		t.Errorf("escapeHtml da %q", got)
	}
	if got := textoPlano("<b>a &amp;lt; b&nbsp;&quot;c&quot;</b>"); got != `a &lt; b "c"` {
		t.Errorf("stripHtml da %q: `&amp;lt;` tiene que quedarse en `&lt;`, no bajar a `<`", got)
	}
	if got := conTexto("<b>viejo</b>", "a<b"); got != "<b>a&lt;b</b>" {
		t.Errorf("escribir sobre una celda en negrita da %q", got)
	}
	if got := conTexto("<b>viejo</b>", "  "); got != "" {
		t.Errorf("vaciar una celda con formato da %q y la web la borra", got)
	}
	for html, quiero := range map[string]bool{
		"<b>x</b>": true, "a &amp; b": true, "a&nbsp;b": true, "=A1&amp;&quot;x&quot;": true,
		"<u>x</u>": false, "x<br>y": false, "caf&eacute;": false, "&#39;": false, "<b><u>x</u></b>": false,
	} {
		if got := celdaEntendida(html); got != quiero {
			t.Errorf("celdaEntendida(%q) = %v", html, got)
		}
	}
}

// TestDeshacerSonFotos: cada cambio es un paso, deshacer y rehacer van y vuelven, y
// escribir tras deshacer tira el futuro. Es `sheets/historial.ts` caso a caso.
func TestDeshacerSonFotos(t *testing.T) {
	l := NuevoLibro()
	l.Escribir(0, 0, "1")
	l.Escribir(0, 0, "2")
	l.Escribir(0, 0, "2") // no cambia nada: no gasta un paso
	l.Vaciar(0, 0, 3, 3)
	if !l.Deshacer() || l.Crudo(0, 0) != "2" {
		t.Fatalf("deshacer el vaciado deja %q", l.Crudo(0, 0))
	}
	if !l.Deshacer() || l.Crudo(0, 0) != "1" {
		t.Fatalf("deshacer «2» deja %q: escribir lo mismo ha gastado un paso", l.Crudo(0, 0))
	}
	if !l.Rehacer() || l.Crudo(0, 0) != "2" {
		t.Fatalf("rehacer deja %q", l.Crudo(0, 0))
	}
	l.Escribir(1, 0, "nuevo")
	if l.PuedeRehacer() {
		t.Error("escribir tras deshacer ha dejado un futuro que ya no existe")
	}
	// El motor vuelve con la foto: una fórmula ve el valor restaurado.
	l.Escribir(2, 0, "=A1*10")
	l.Escribir(0, 0, "5")
	l.Deshacer()
	if got := l.Texto(2, 0); got != "20" {
		t.Errorf("tras deshacer, `=A1*10` da %q y A1 vuelve a ser 2", got)
	}
	// El tope.
	for i := range 2 * TopeHistorial {
		l.Escribir(3, 3, formula.FormatNumber(float64(i)))
	}
	n := 0
	for l.Deshacer() {
		n++
	}
	if n != TopeHistorial {
		t.Errorf("se han podido deshacer %d pasos y el tope es %d", n, TopeHistorial)
	}
}

// TestLasHojasSeAnadenDuplicanMuevenYBorran: las cinco operaciones de la barra de
// hojas, con los nombres que pone la web y el motor al día: una fórmula que apunta a
// una hoja que todavía no existe tiene que ver lo que se escriba en ella al crearla.
func TestLasHojasSeAnadenDuplicanMuevenYBorran(t *testing.T) {
	l := NuevoLibroCon([]string{"Datos"}, 0)
	l.Escribir(0, 0, "=Otra!A1+1")
	l.Anadir("Otra")
	if l.Activa() != 1 || l.Hoja() != "Otra" {
		t.Fatalf("la hoja nueva no es la activa: %d %q", l.Activa(), l.Hoja())
	}
	l.Escribir(0, 0, "41")
	l.Activar(0)
	if got := l.Texto(0, 0); got != "42" {
		t.Errorf("tras crear «Otra», `=Otra!A1+1` da %q", got)
	}

	l.Duplicar()
	l.Activar(0)
	l.Duplicar()
	if got := l.Hojas(); !slices.Equal(got, []string{"Datos", "Datos (3)", "Datos (2)", "Otra"}) {
		// Duplicar pone la copia JUSTO DETRÁS de la activa, y la copia pasa a serlo.
		t.Errorf("tras duplicar dos veces las hojas son %v", got)
	}
	if l.Crudo(0, 0) != "=Otra!A1+1" {
		t.Errorf("la hoja duplicada no trae las celdas: A1 es %q", l.Crudo(0, 0))
	}

	l.Renombrar(1, "  Resumen  ")
	l.Renombrar(1, "")
	if l.Hojas()[1] != "Resumen" {
		t.Errorf("renombrar recorta y no acepta el vacío: %v", l.Hojas())
	}

	l.Activar(3) // «Otra»
	l.Mover(3, 0)
	if l.Hojas()[0] != "Otra" || l.Hoja() != "Otra" {
		t.Errorf("mover la hoja activa al principio deja %v con la activa en %q", l.Hojas(), l.Hoja())
	}

	for len(l.Hojas()) > 1 {
		l.Borrar()
	}
	l.Borrar()
	if len(l.Hojas()) != 1 {
		t.Error("se ha podido borrar la última hoja")
	}
	for l.Deshacer() {
	}
	if got := l.Hojas(); !slices.Equal(got, []string{"Datos"}) {
		t.Errorf("deshaciendo todo se vuelve a %v y no al libro del principio", got)
	}
}

// TestElResumenEsElDeLaWeb son los casos de `selectionStats` en
// `packages/office/test/sheets-format.test.ts`, uno a uno.
func TestElResumenEsElDeLaWeb(t *testing.T) {
	for _, c := range []struct {
		vals        []formula.Value
		n           int
		suma, media float64
		ok          bool
	}{
		{[]formula.Value{1.0, 2.0, 3.0, "texto", ""}, 4, 6, 2, true},
		{[]formula.Value{10.0, formula.ErrDiv0, true, ""}, 3, 10, 10, true},
		{[]formula.Value{"a", "b"}, 0, 0, 0, false},
		{nil, 0, 0, 0, false},
		{[]formula.Value{1.0, 1.0, 0.0}, 3, 2, 0.666667, true},
		{[]formula.Value{1e12, 1e12}, 2, 2e12, 1e12, true},
	} {
		n, suma, media, ok := resumenDe(c.vals)
		if n != c.n || suma != c.suma || math.Abs(media-c.media) > 1e-12 || ok != c.ok {
			t.Errorf("resumen de %v = (%d, %v, %v, %v) y la web da (%d, %v, %v, %v)",
				c.vals, n, suma, media, ok, c.n, c.suma, c.media, c.ok)
		}
	}
}

// TestPegarDesplazaLasFormulas: lo copiado dentro de Calcus se pega con las
// referencias relativas movidas; lo que llega de fuera es texto; cortar mueve; y todo
// en un solo paso de deshacer.
func TestPegarDesplazaLasFormulas(t *testing.T) {
	c := lienzoDePrueba()
	r := c.Rejilla
	c.Libro.Escribir(0, 0, "1")
	c.Libro.Escribir(0, 1, "2")
	c.Libro.Escribir(1, 0, "=A1+$A$1")
	r.Seleccionar(1, 0, false)
	r.Copiar(nil, false)
	if r.copiado.tsv != "2" {
		t.Errorf("al portapapeles va lo que se ve (2) y ha ido %q", r.copiado.tsv)
	}
	r.Seleccionar(1, 1, false)
	r.Pegar(r.copiado.tsv, false)
	if got := c.Libro.Crudo(1, 1); got != "=A2+$A$1" {
		t.Errorf("`=A1+$A$1` pegada una fila más abajo es %q", got)
	}
	if got := c.Libro.Texto(1, 1); got != "3" {
		t.Errorf("y vale %q", got)
	}
	r.Seleccionar(2, 1, false)
	r.Pegar(r.copiado.tsv, true)
	if got := c.Libro.Crudo(2, 1); got != "2" {
		t.Errorf("pegar solo valores deja %q", got)
	}

	// De fuera: TSV con dos filas y el salto final que añaden los portapapeles.
	r.Seleccionar(3, 3, false)
	r.Pegar("a\tb<c\nd\te\n", false)
	if c.Libro.Crudo(4, 3) != "b<c" || c.Libro.HTML(4, 3) != "b&lt;c" || c.Libro.Crudo(4, 4) != "e" {
		t.Errorf("el TSV de fuera se ha pegado mal: %q %q", c.Libro.HTML(4, 3), c.Libro.Crudo(4, 4))
	}
	if c1, f1, c2, f2 := r.Rango(); c1 != 3 || f1 != 3 || c2 != 4 || f2 != 4 {
		t.Errorf("lo pegado queda seleccionado: %d,%d–%d,%d", c1, f1, c2, f2)
	}
	c.Deshacer(false)
	if c.Libro.Crudo(3, 3) != "" || c.Libro.Crudo(4, 4) != "" {
		t.Error("deshacer no ha quitado el bloque pegado de una vez")
	}

	// Cortar es mover.
	r.Seleccionar(0, 0, false)
	r.Copiar(nil, true)
	r.Seleccionar(5, 5, false)
	r.Pegar(r.copiado.tsv, false)
	if c.Libro.Crudo(0, 0) != "" || c.Libro.Crudo(5, 5) != "1" {
		t.Errorf("cortar y pegar deja el origen %q y el destino %q", c.Libro.Crudo(0, 0), c.Libro.Crudo(5, 5))
	}
}
