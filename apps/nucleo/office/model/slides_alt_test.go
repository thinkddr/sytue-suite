// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"encoding/json"
	"strings"
	"testing"
)

// diaposConAlt es una presentación con una imagen descrita (con un «&» y un salto que el
// texto alternativo aplana) y otra sin describir.
func diaposConAlt(t *testing.T) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"slides": []map[string]any{{
		"id": "s1",
		"boxes": []map[string]any{
			{
				"id": "a", "x": 10, "y": 10, "w": 30, "h": 30, "kind": "image", "html": "", "src": onePxPNG,
				"alt": "Gráfico de ventas\n  de 2026 & previsión",
			},
			{"id": "b", "x": 50, "y": 10, "w": 30, "h": 30, "kind": "image", "html": "", "src": onePxPNG},
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// imagenesImportadas son los textos alternativos de las imágenes de una presentación importada.
func imagenesImportadas(t *testing.T, nombre string, datos []byte) []string {
	t.Helper()
	res, err := Import(nombre, datos)
	if err != nil {
		t.Fatalf("importar %s: %v", nombre, err)
	}
	var m slidesModel
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatalf("modelo de %s: %v", nombre, err)
	}
	var alts []string
	for _, s := range m.Slides {
		for _, b := range s.Boxes {
			if b.Kind == "image" {
				alts = append(alts, b.Alt)
			}
		}
	}
	return alts
}

// TestElTextoAlternativoDeUnaImagenSaleEnElPptx: `descr` en el <p:cNvPr> de la imagen
// descrita (escapado y en una línea); la que no lo lleva sale sin él. Y vuelve al importar.
func TestElTextoAlternativoDeUnaImagenSaleEnElPptx(t *testing.T) {
	s := stateWithContent(t, "doc:slides", diaposConAlt(t))
	datos, _, _, err := Export(KindPresentation, "Alt", s, FormatOOXML)
	if err != nil {
		t.Fatal(err)
	}
	diapo := zipParts(t, datos)["ppt/slides/slide1.xml"]
	if !strings.Contains(diapo, `descr="Gráfico de ventas de 2026 &amp; previsión"`) {
		t.Errorf("la imagen descrita no lleva su descr:\n%s", diapo)
	}
	if strings.Count(diapo, "descr=") != 1 {
		t.Errorf("la imagen sin describir sale con descr:\n%s", diapo)
	}
	alts := imagenesImportadas(t, "alt.pptx", datos)
	if len(alts) != 2 || alts[0] != "Gráfico de ventas de 2026 & previsión" || alts[1] != "" {
		t.Errorf("al importar el .pptx: %q", alts)
	}
}

// TestElTextoAlternativoDeUnaImagenSaleEnElOdp: `svg:title` en el draw:frame de la
// imagen, como el de las imágenes del .odt; y de vuelta al importar, también de un
// `svg:desc` si no hay título.
func TestElTextoAlternativoDeUnaImagenSaleEnElOdp(t *testing.T) {
	s := stateWithContent(t, "doc:slides", diaposConAlt(t))
	datos, _, _, err := Export(KindPresentation, "Alt", s, FormatODF)
	if err != nil {
		t.Fatal(err)
	}
	partes := zipParts(t, datos)
	contenido := partes["content.xml"]
	if !strings.Contains(contenido, `<svg:title>Gráfico de ventas de 2026 &amp; previsión</svg:title></draw:frame>`) {
		t.Errorf("el marco de la imagen no lleva su svg:title:\n%s", contenido)
	}
	if strings.Count(contenido, "<svg:title>") != 1 {
		t.Errorf("la imagen sin describir sale con svg:title")
	}
	alts := imagenesImportadas(t, "alt.odp", datos)
	if len(alts) != 2 || alts[0] != "Gráfico de ventas de 2026 & previsión" || alts[1] != "" {
		t.Errorf("al importar el .odp: %q", alts)
	}
	// Un .odp de LibreOffice con la descripción solo en svg:desc.
	conDesc := strings.Replace(contenido, `<svg:title>Gráfico de ventas de 2026 &amp; previsión</svg:title>`,
		`<svg:desc>Solo la descripción</svg:desc>`, 1)
	partes["content.xml"] = conDesc
	entradas := map[string][]byte{}
	for k, v := range partes {
		entradas[k] = []byte(v)
	}
	if alts := imagenesImportadas(t, "desc.odp", zipDe(t, entradas)); len(alts) == 0 || alts[0] != "Solo la descripción" {
		t.Errorf("con svg:desc: %q", alts)
	}
}
