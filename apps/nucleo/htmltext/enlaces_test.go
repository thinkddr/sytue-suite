// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package htmltext

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// TestEnlacesVectoresComunes recorre los vectores que también recorre enlaces.test.ts:
// el mismo correo tiene que enlazar lo mismo en la web y en Fyne.
func TestEnlacesVectoresComunes(t *testing.T) {
	crudo, err := os.ReadFile("testdata/enlaces.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Casos []struct {
			Nombre string  `json:"nombre"`
			Texto  string  `json:"texto"`
			Tramos []Tramo `json:"tramos"`
		} `json:"casos"`
	}
	if err := json.Unmarshal(crudo, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Casos) < 10 {
		t.Fatalf("solo %d vectores", len(v.Casos))
	}
	for _, c := range v.Casos {
		got := Enlaces(c.Texto)
		if len(got) == 0 && len(c.Tramos) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, c.Tramos) {
			t.Errorf("%s: Enlaces(%q)\n  = %+v\nquiero %+v", c.Nombre, c.Texto, got, c.Tramos)
		}
		var junto strings.Builder
		for _, tr := range got {
			junto.WriteString(tr.Texto)
		}
		if junto.String() != c.Texto {
			t.Errorf("%s: los tramos no rehacen el texto: %q", c.Nombre, junto.String())
		}
	}
}

// Un correo que solo trae HTML no puede perder adónde lleva «Restablece tu contraseña»:
// sin el destino el botón no lleva a ningún sitio, y con él se ve el dominio REAL.
func TestTextoConEnlacesConservaElDestino(t *testing.T) {
	for html, quiero := range map[string]string{
		`<p><a href="https://id.malo.example/reset?a=1&amp;b=2">Restablece tu contraseña</a></p>`: "Restablece tu contraseña <https://id.malo.example/reset?a=1&b=2>",
		`<a href='https://sytue.eu/'>https://sytue.eu</a>`:                                        "https://sytue.eu",
		`<a href="https://sytue.eu">www.sytue.eu</a>`:                                             "www.sytue.eu",
		`Escribe a <a href="mailto:ana@sytue.eu?subject=Hola">Ana</a>.`:                           "Escribe a Ana <ana@sytue.eu>.",
		`<a href="mailto:ana@sytue.eu">ana@sytue.eu</a>`:                                          "ana@sytue.eu",
		`<a href="javascript:alert(1)">Pulsa</a>`:                                                 "Pulsa",
		`<a href="https://seguimiento.example/px"><img src="x.png"></a>Texto`:                     "Texto",
		`<a name="arriba">Arriba</a>`:                                                             "Arriba",
		`<a href=https://sin-comillas.example>Ir</a>`:                                             "Ir <https://sin-comillas.example>",
		`<A HREF="HTTPS://MAYUS.EXAMPLE"><b>Ver</b> más</A>`:                                      "Ver más <HTTPS://MAYUS.EXAMPLE>",
		"<p>a\uE000b\uE001c</p>":                                                                  "abc",
		`<abbr href="https://x.example">OMS</abbr>`:                                               "OMS",
	} {
		if got := TextoConEnlaces(html); got != quiero {
			t.Errorf("TextoConEnlaces(%q) = %q, quiero %q", html, got, quiero)
		}
	}
	// Y lo que sale se enlaza con el destino real, no con el texto del remitente.
	tr := Enlaces(TextoConEnlaces(`<a href="https://id.malo.example/r">https://banco.example</a>`))
	if len(tr) < 3 || tr[2].Destino != "https://id.malo.example/r" {
		t.Errorf("el destino real no sale enlazado: %+v", tr)
	}
	// Text no cambia: lo usan las notas y los widgets, que no enseñan destinos.
	if got := Text(`<a href="https://sytue.eu">Sytue</a>`); got != "Sytue" {
		t.Errorf("Text = %q", got)
	}
}
