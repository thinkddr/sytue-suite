// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import "testing"

const (
	fuentesWeb = `<script type="application/json" data-sytue-sources="">[{"id":"src-1","type":"libro","author":"García, Ana","title":"El libro",` +
		`"year":"2020","publisher":"Planeta"}]</script>`
	citaWeb    = `<span data-citation="src-1" contenteditable="false">(García, 2020)</span>`
	entradaWeb = `<p data-bib-entry="" style="margin:.35em 0;padding-left:2em;text-indent:-2em">García, Ana (2020). <i>El libro</i>. Planeta.</p>`
)

// TestLaCitaYLaEntradaSonLasDeLaWeb: `citationText` y `apaEntry`, con sus reservas
// (título si no hay autor, «Anónimo», «s.f.») y la cursiva del título o de la revista.
func TestLaCitaYLaEntradaSonLasDeLaWeb(t *testing.T) {
	for _, c := range []struct {
		f           Fuente
		cita, entra string
	}{
		{Fuente{Tipo: "libro", Autor: "García, Ana", Titulo: "El libro", Anio: "2020", Editorial: "Planeta"}, "(García, 2020)", "García, Ana (2020). <i>El libro</i>. Planeta."},
		{Fuente{Tipo: "articulo", Autor: "Pérez", Titulo: "Un estudio", Anio: "2019", Editorial: "Revista X"}, "(Pérez, 2019)", "Pérez (2019). Un estudio. <i>Revista X</i>."},
		{Fuente{Tipo: "web", Titulo: "Sitio <b>", Editorial: "Blog", URL: "https://x.eu"}, "(Sitio <b>, s.f.)", "(s.f.). <i>Sitio &lt;b&gt;</i>. Blog. https://x.eu"},
		{Fuente{Tipo: "libro"}, "(Anónimo, s.f.)", "(s.f.)."},
	} {
		if got := TextoCita(c.f); got != c.cita {
			t.Errorf("TextoCita(%+v) = %q, quiero %q", c.f, got, c.cita)
		}
		if got := entradaAPA(c.f); got != c.entra {
			t.Errorf("entradaAPA(%+v) = %q, quiero %q", c.f, got, c.entra)
		}
	}
	if got := SiguienteFuente([]Fuente{{ID: "src-3"}, {ID: "otra"}, {ID: "src-10"}}); got != "src-11" {
		t.Errorf("la siguiente fuente es %s", got)
	}
}

// TestLaBibliografiaDeLaWeb: lo que dejan las citas y la bibliografía se entiende y vuelve
// igual —las fuentes van en su `<script>`, la cita es UNA runa y la entrada lleva su sangría
// francesa—; las fuentes se escriben como `JSON.stringify` con «<» escapado; y generar pone
// al día las citas y lista solo las citadas, antes de las notas al pie. Se vio en ROJO sin
// `SetEscapeHTML(false)`: «&» salía como `\u0026`, y la web lo deja tal cual.
func TestLaBibliografiaDeLaWeb(t *testing.T) {
	web := `<p>Según ` + citaWeb + `.</p><section data-bibliography=""><p><b>Bibliografía</b></p>` + entradaWeb + `</section>` + fuentesWeb
	d := Parse(web)
	if !d.Editable() || d.HTML() != web {
		t.Fatalf("la web no vuelve igual o no se edita (%v): %s", d.NoEntendido(), d.HTML())
	}
	bs := d.Bloques()
	if bs[0].Texto() != "Según "+string(ObjetoRuna)+"." || bs[0].Tramos[1].Rotulo != "(García, 2020)" {
		t.Errorf("la cita: %q, %+v", bs[0].Texto(), bs[0].Tramos[1])
	}
	if e := bs[len(bs)-1].Base; e.MargenIzq != 32 || e.Sangria1 != -32 {
		t.Errorf("la entrada lleva la sangría francesa de 2 em: %+v", e)
	}
	if fs := d.Fuentes(); len(fs) != 1 || fs[0].Autor != "García, Ana" || fs[0].Tipo != "libro" {
		t.Errorf("las fuentes: %+v", fs)
	}

	d = Parse(`<p>a</p><section data-footnotes=""><hr><ol><li id="fn-1" value="1">n</li></ol></section>`)
	d.PonerFuentes([]Fuente{
		{ID: "src-1", Tipo: "libro", Autor: "Zapata & Cía", Titulo: "</script>", Anio: "2021"},
		{ID: "src-2", Tipo: "web", Autor: "Álvarez", Titulo: "Web", URL: "https://a.eu"},
	})
	abrir(t, d, 0).InsertarCita(1, d.Fuentes()[0])
	if n := d.GenerarBibliografia("Bibliografía", "No hay fuentes citadas."); n != 1 {
		t.Errorf("la bibliografía lista %d fuentes y solo hay una citada", n)
	}
	despues(t, "generar", d, `<p>a<span data-citation="src-1" contenteditable="false">(Zapata &amp; Cía, 2021)</span></p>`+
		`<section data-bibliography=""><p><b>Bibliografía</b></p><p data-bib-entry="" style="margin:.35em 0;padding-left:2em;text-indent:-2em">`+
		`Zapata &amp; Cía (2021). <i>&lt;/script&gt;</i>.</p></section>`+
		`<section data-footnotes=""><hr><ol><li id="fn-1" value="1">n</li></ol></section>`+
		`<script type="application/json" data-sytue-sources="">[{"id":"src-1","type":"libro","author":"Zapata & Cía","title":"\u003c/script>",`+
		`"year":"2021","publisher":""},{"id":"src-2","type":"web","author":"Álvarez","title":"Web","year":"","publisher":"","url":"https://a.eu"}]</script>`)
	d.PonerFuentes(nil)
	if d.Fuentes() != nil || len(d.Bloques()) == 0 {
		t.Error("sin fuentes, el `<script>` se va")
	}
}
