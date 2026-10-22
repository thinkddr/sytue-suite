// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import "testing"

const (
	origenWeb = `<script type="application/json" data-sytue-mailmerge="">{"fields":["Nombre","Email"],` +
		`"records":[{"Nombre":"Ana","Email":"ana@x.eu"},{"Nombre":"Luis","Email":"luis@x.eu"}]}</script>`
	campoWeb = `<span data-mergefield="Nombre" contenteditable="false">«Nombre»</span>`
)

// TestParsearOrigenEsElDeLaWeb: `detectDelimiter`, `splitRows` y `parseMergeSource`: TSV si
// hay tabulador, si no el delimitador dominante fuera de comillas; comillas dobles
// entrecomillan, `""` escapa una; las cabeceras vacías se numeran, y las filas en blanco
// se descartan.
func TestParsearOrigenEsElDeLaWeb(t *testing.T) {
	o := ParsearOrigen("Nombre;Email\nAna;ana@x.eu\n\nLuis;luis@x.eu\n")
	if len(o.Campos) != 2 || len(o.Registros) != 2 || o.Registros[0]["Nombre"] != "Ana" {
		t.Fatalf("csv con punto y coma: %+v", o)
	}
	o = ParsearOrigen("Nombre\tEmail\nAna\tana@x.eu")
	if len(o.Campos) != 2 || o.Registros[0]["Email"] != "ana@x.eu" {
		t.Fatalf("tsv: %+v", o)
	}
	o = ParsearOrigen(`Nombre,"Cita ""famosa"""` + "\n" + `Ana,"dice ""hola"", adiós"`)
	if o.Campos[1] != `Cita "famosa"` || o.Registros[0][o.Campos[1]] != `dice "hola", adiós` {
		t.Fatalf("comillas: %+v", o)
	}
	o = ParsearOrigen("Nombre,\nAna,x")
	if o.Campos[1] != "Campo 2" {
		t.Fatalf("cabecera vacía: %+v", o)
	}
	o = ParsearOrigen("Nombre,Email\nAna")
	if o.Registros[0]["Email"] != "" {
		t.Fatalf("celda que falta: %+v", o)
	}
	if o := ParsearOrigen("  \n"); len(o.Campos) != 0 || o.Campos == nil || o.Registros == nil {
		t.Fatalf("vacío: %+v", o)
	}
}

// TestElOrigenYLosCamposDeLaWeb: lo que dejan `writeMergeSource` e `insertMergeField` se
// entiende y vuelve igual; el campo es UNA runa; y combinar sustituye cada marcador por su
// valor, escapado, con un salto de página entre registros.
func TestElOrigenYLosCamposDeLaWeb(t *testing.T) {
	web := `<p>Hola ` + campoWeb + `.</p>` + origenWeb
	d := Parse(web)
	if !d.Editable() || d.HTML() != web {
		t.Fatalf("la web no vuelve igual o no se edita (%v): %s", d.NoEntendido(), d.HTML())
	}
	if b := d.Bloques()[0]; b.Texto() != "Hola "+string(ObjetoRuna)+"." || b.Tramos[1].Campo != "Nombre" {
		t.Errorf("el campo: %q, %+v", b.Texto(), b.Tramos[1])
	}
	if o := d.OrigenCombinacion(); o == nil || len(o.Registros) != 2 || o.Registros[0]["Nombre"] != "Ana" {
		t.Fatalf("el origen leído: %+v", o)
	}

	d = Parse(`<p>a</p>`)
	abrir(t, d, 0).InsertarCampoCombinacion(1, "Nombre")
	despues(t, "insertar", d, `<p>a<span data-mergefield="Nombre" contenteditable="false">«Nombre»</span></p>`)
	d.PonerOrigen(&Origen{Campos: []string{"Nombre"}, Registros: []map[string]string{{"Nombre": `1 < 2 & "z"`}, {"Nombre": "y"}}})
	// El marcador entero se sustituye por el valor: el <span> no sobrevive a la
	// combinación, como `substituteMergeFields` de la web.
	got := d.Combinado(*d.OrigenCombinacion())
	quiero := `<p>a1 &lt; 2 &amp; &quot;z&quot;</p>` + saltoCombinacion + `<p>ay</p>`
	if got != quiero {
		t.Errorf("combinado:\n  hay     %s\n  quería  %s", got, quiero)
	}
	d.PonerOrigen(nil)
	if d.OrigenCombinacion() != nil || len(d.Bloques()) == 0 {
		t.Error("quitar el origen deja el `<script>` fuera")
	}
}
