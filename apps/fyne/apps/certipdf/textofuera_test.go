// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"encoding/json"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// El texto fuera del PDF contra los VECTORES COMPARTIDOS con la web
// (`packages/office/test/vectores/certipdf-herramientas.json`, que recorre
// `pdf-herramientas.test.ts`): el mismo documento da el mismo texto en las dos.
const rutaTextoFuera = "../../../frontend/packages/office/test/vectores/certipdf-herramientas.json"

func TestElTextoFueraDelPDFConLosVectoresDeLaWeb(t *testing.T) {
	datos, err := os.ReadFile(rutaTextoFuera)
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Lineas []struct {
			Fragmentos []Fragmento
			Sale       []string
		}
		Texto []struct {
			Paginas [][]Fragmento
			Sale    string
		}
		Resumen []struct {
			Anotaciones []AnotacionLeida
			Sale        string
		}
	}
	if err := json.Unmarshal(datos, &v); err != nil {
		t.Fatal(err)
	}
	for _, c := range v.Lineas {
		if got := LineasDeTexto(c.Fragmentos); !slices.Equal(got, c.Sale) && (len(got) != 0 || len(c.Sale) != 0) {
			t.Errorf("renglones %q, quería %q", got, c.Sale)
		}
		// La voz los une con una pausa: lo de siempre, ahora sobre los mismos renglones.
		if got := TextoParaLeer(c.Fragmentos); got != strings.Join(c.Sale, ". ") {
			t.Errorf("para leer %q", got)
		}
	}
	for _, c := range v.Texto {
		if got := TextoDelPDF(c.Paginas); got != c.Sale {
			t.Errorf("texto %q, quería %q", got, c.Sale)
		}
	}
	rotulos := RotulosResumen{
		Pagina:  func(n int) string { return "Página " + strconv.Itoa(n) },
		Tipo:    func(k string) string { return k },
		Anonimo: "Anónimo",
	}
	for _, c := range v.Resumen {
		if got := ResumenDeComentarios(c.Anotaciones, rotulos); got != c.Sale {
			t.Errorf("resumen %q, quería %q", got, c.Sale)
		}
	}
}
