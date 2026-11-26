// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package seo

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

func pagina(id, titulo, desc string, secciones ...ir.Nodo) ir.Pagina {
	return ir.Pagina{
		ID: id, Slug: ir.Textos{"es": id},
		Title: ir.Textos{"es": titulo}, Description: ir.Textos{"es": desc},
		Sections: secciones,
	}
}

func imagen(alt string, decorativa bool) ir.Nodo {
	n := ir.Nodo{ID: "img", Type: "image", Decorative: decorativa}
	if alt != "" {
		n.Alt = ir.Textos{"es": alt}
	}
	return n
}

func codigos(avisos []Aviso, pagina string) []string {
	var out []string
	for _, a := range avisos {
		if a.PageID == pagina {
			out = append(out, a.Code)
		}
	}
	return out
}

func TestRevisarEncuentraLoQueVeUnBuscador(t *testing.T) {
	desc := "Pastelería artesana en Valladolid: tartas por encargo y desayunos."
	s := &ir.Sitio{
		Languages: ir.IdiomasSitio{Default: "es"},
		Pages: []ir.Pagina{
			pagina("inicio", "Dulce Norte", desc,
				ir.Nodo{ID: "s1", Type: "section", Children: []ir.Nodo{
					imagen("", false),               // sin alt: cuenta
					imagen("", true),                // decorativa: no cuenta
					imagen("Tarta de queso", false), // con alt: no cuenta
				}}),
			pagina("carta", "Dulce Norte", ""),                            // título repetido y sin descripción
			pagina("contacto", "", strings.Repeat("x", MaxDescripcion+1)), // sin título y descripción larga
		},
	}
	s.Pages[2].Noindex = true
	avisos := Revisar(s)

	if got := codigos(avisos, "inicio"); strings.Join(got, ",") != "image_alt_missing,title_duplicate" {
		t.Errorf("inicio: %v", got)
	}
	if got := codigos(avisos, "carta"); strings.Join(got, ",") != "description_missing,title_duplicate" {
		t.Errorf("carta: %v", got)
	}
	if got := codigos(avisos, "contacto"); strings.Join(got, ",") != "description_long,page_noindex,title_missing" {
		t.Errorf("contacto: %v", got)
	}
	for _, a := range avisos {
		if a.PageID == "inicio" && a.Code == ImagenSinAlt && a.Count != 1 {
			t.Errorf("imágenes sin alt = %d, quiero 1 (la decorativa y la descrita no cuentan)", a.Count)
		}
	}
	// Sin imagen para compartir, el aviso es del sitio, no de una página.
	if avisos[0].Code != ImagenCompartirNo || avisos[0].PageID != "" {
		t.Errorf("primer aviso = %+v, quiero el del sitio sin imagen para compartir", avisos[0])
	}
}

// Un título repetido en un idioma no lo es en otro: la comparación es por idioma,
// y sin acentos ni mayúsculas, que es como los compara quien lee los resultados.
func TestTitulosDuplicadosPorIdiomaYSinAcentos(t *testing.T) {
	s := &ir.Sitio{
		Languages: ir.IdiomasSitio{Default: "es", Others: []string{"en"}},
		Pages: []ir.Pagina{
			{ID: "a", Title: ir.Textos{"es": "Menú", "en": "Menu"}, Description: ir.Textos{"es": "d", "en": "d"}},
			{ID: "b", Title: ir.Textos{"es": "MENU", "en": "Drinks"}, Description: ir.Textos{"es": "d", "en": "d"}},
		},
		SEO: ir.SeoSitio{OGImage: &ir.ImagenRef{}},
	}
	var dup []string
	for _, a := range Revisar(s) {
		if a.Code == TituloDuplicado {
			dup = append(dup, a.PageID+":"+a.Lang)
		}
	}
	if strings.Join(dup, ",") != "a:es,b:es" {
		t.Fatalf("duplicados = %v, quiero solo los de español", dup)
	}
}

func TestSitioSinAvisosDevuelveListaVacia(t *testing.T) {
	s := &ir.Sitio{
		Languages: ir.IdiomasSitio{Default: "es"},
		Pages:     []ir.Pagina{pagina("inicio", "Inicio", "Una descripción suficiente.")},
		SEO:       ir.SeoSitio{OGImage: &ir.ImagenRef{}},
	}
	if a := Revisar(s); a == nil || len(a) != 0 {
		t.Fatalf("quiero lista vacía (no nil), got %#v", a)
	}
}
