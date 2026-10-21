// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package plantillas

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

func marcaDeBloques() *domain.TemaComun {
	t := domain.MarcaOrg{Version: 4, Name: "Acme", Bg: "#FFF7ED", Ink: "#1C1917", Accent: "#B45309", HeadingFont: "Georgia"}.Tema()
	return &t
}

// Cada bloque, en cada idioma, sale con la forma que su tipo necesita, sin huecos, y la
// marca lo viste sin cambiarle el contenido.
func TestCadaBloqueSaleValidoEnTodosLosIdiomas(t *testing.T) {
	if err := cargar(); err != nil {
		t.Fatal(err)
	}
	vistos := 0
	for _, kind := range []domain.Kind{domain.KindDocument, domain.KindSpreadsheet, domain.KindPresentation} {
		for idioma := range textos {
			lista, err := ListarBloques(kind, idioma)
			if err != nil || len(lista) < 3 {
				t.Fatalf("%s/%s: %d bloques, %v", kind, idioma, len(lista), err)
			}
			for _, b := range lista {
				vistos++
				info, neutro, err := GenerarBloque(b.Key, idioma, nil)
				if err != nil || info != b || info.Name == "" || info.Description == "" || strings.Contains(neutro, "{{") {
					t.Fatalf("%s/%s: %v %+v %s", idioma, b.Key, err, info, neutro)
				}
				_, conMarca, err := GenerarBloque(b.Key, idioma, marcaDeBloques())
				if err != nil {
					t.Fatal(err)
				}
				comprobarBloque(t, idioma+"/"+b.Key, kind, neutro, conMarca)
			}
		}
	}
	if vistos != len(catalogoBloques)*len(textos) {
		t.Errorf("se vieron %d bloques, quería %d", vistos, len(catalogoBloques)*len(textos))
	}
	if _, _, err := GenerarBloque("no-existe", "es", nil); !errors.Is(err, ErrNoExiste) {
		t.Errorf("clave desconocida: %v", err)
	}
}

func comprobarBloque(t *testing.T, donde string, kind domain.Kind, neutro, conMarca string) {
	t.Helper()
	switch kind {
	case domain.KindDocument:
		// HTML canónico del editor (el importador lo devuelve tal cual) y sin estilos.
		res, err := domain.Import("bloque.html", []byte(neutro))
		if err != nil || res.Content != neutro {
			t.Errorf("%s: no es HTML canónico (%v)\n quiere %s\n sale   %s", donde, err, neutro, res.Content)
		}
		if strings.Contains(neutro, "style=") {
			t.Errorf("%s: el bloque neutro lleva estilos: %s", donde, neutro)
		}
	case domain.KindPresentation:
		var s struct {
			ID    string `json:"id"`
			Boxes []struct {
				ID   string `json:"id"`
				HTML string `json:"html"`
			} `json:"boxes"`
		}
		if err := json.Unmarshal([]byte(neutro), &s); err != nil || s.ID == "" || len(s.Boxes) < 2 {
			t.Fatalf("%s: diapositiva inválida: %v", donde, err)
		}
		if conMarca != neutro {
			t.Errorf("%s: una diapositiva hereda el tema de su presentación; no se toca", donde)
		}
	case domain.KindSpreadsheet:
		type bloque struct {
			Cells  map[string]string         `json:"cells"`
			Styles map[string]map[string]any `json:"styles"`
			Charts []struct {
				Range  string   `json:"range"`
				Colors []string `json:"colors"`
			} `json:"charts"`
		}
		var n, m bloque
		if err := json.Unmarshal([]byte(neutro), &n); err != nil || n.Cells["A1"] == "" {
			t.Fatalf("%s: bloque de hoja inválido (debe empezar en A1): %v", donde, err)
		}
		if err := json.Unmarshal([]byte(conMarca), &m); err != nil {
			t.Fatal(err)
		}
		if len(n.Styles) != 0 {
			t.Errorf("%s: el bloque neutro lleva estilos de tema: %v", donde, n.Styles)
		}
		pintadas := 0
		for _, e := range m.Styles {
			if e["fill"] == "#B45309" && e["tema"] == "marca" {
				pintadas++
			}
		}
		if pintadas == 0 {
			t.Errorf("%s: la marca no vistió ninguna celda: %s", donde, conMarca)
		}
		for i, g := range m.Charts {
			if len(g.Colors) != 1 || g.Colors[0] != "#B45309" || len(n.Charts[i].Colors) != 0 {
				t.Errorf("%s: el gráfico no lleva el color de la marca: %v", donde, g.Colors)
			}
		}
		for ref := range n.Cells {
			if n.Cells[ref] != m.Cells[ref] {
				t.Errorf("%s: la marca cambió la celda %s", donde, ref)
			}
		}
	}
}

// Un documento con la marca: los títulos del bloque llevan su color y su tipografía.
func TestBloqueDeDocumentoConMarca(t *testing.T) {
	_, portada, err := GenerarBloque("b_portada", "es", marcaDeBloques())
	if err != nil || !strings.Contains(portada, `<h1 style="color:#B45309;font-family:Georgia" data-tema="marca:color,font-family">`) {
		t.Errorf("portada sin marca: %v %s", err, portada)
	}
	_, firmas, err := GenerarBloque("b_firmas", "es", nil)
	if err != nil || !strings.Contains(firmas, "<td><strong>Firma</strong></td>") {
		t.Errorf("tabla de firmas: %v %s", err, firmas)
	}
}
