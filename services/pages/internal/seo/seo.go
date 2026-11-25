// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package seo revisa el borrador de una web con los ojos de un buscador antes de
// publicarlo: lo que hace que una página no aparezca, aparezca mal recortada o se
// confunda con otra.
//
// Existe porque el editor deja escribir el título y la descripción de cada página,
// pero nada decía que la mitad estaban vacíos, que tres páginas se llamaban
// «Inicio» o que las fotos no tenían texto alternativo —que además es lo que oye
// quien navega con lector de pantalla—. Son AVISOS, no un bloqueo: publicar con
// ellos es legítimo, y la verja de publicar (ir.Validar) sigue siendo la que manda.
//
// Vive en Go y no en la pantalla porque es lógica sobre el IR, que es de este
// servicio: el mismo `ir.Sitio` que se publica, no una copia en TypeScript que
// diverja (CONVENTIONS §13.4).
package seo

import (
	"sort"
	"strings"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

// Límites a partir de los cuales los buscadores recortan lo que enseñan. No son
// una norma escrita: son lo que cabe en el resultado de búsqueda de escritorio.
const (
	MaxTitulo      = 60
	MaxDescripcion = 160
)

// Códigos estables de aviso (la pantalla los traduce).
const (
	TituloFalta       = "title_missing"
	TituloLargo       = "title_long"
	TituloDuplicado   = "title_duplicate"
	DescripcionFalta  = "description_missing"
	DescripcionLarga  = "description_long"
	ImagenSinAlt      = "image_alt_missing"
	PaginaNoindex     = "page_noindex"
	SitioNoindex      = "site_noindex"
	ImagenCompartirNo = "share_image_missing"
)

// Gravedad de un aviso: `warning` empeora cómo se ve en un buscador; `info` es una
// decisión que puede ser intencionada (ocultar una página) y conviene tener presente.
const (
	GravedadAviso = "warning"
	GravedadInfo  = "info"
)

// Aviso es un problema concreto, situado en una página y un idioma si aplica.
type Aviso struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	PageID   string `json:"page_id,omitempty"`
	// Page es cómo se reconoce la página: su título en el idioma por defecto, o su
	// ruta si no lo tiene.
	Page  string `json:"page,omitempty"`
	Lang  string `json:"lang,omitempty"`
	Count int    `json:"count,omitempty"`
}

// Revisar devuelve los avisos del sitio, ordenados por página (en el orden del
// sitio) y, dentro de cada una, por código. Un sitio sin avisos devuelve un slice
// vacío, no nil: la pantalla dice «todo bien», no «no sé».
func Revisar(s *ir.Sitio) []Aviso {
	out := []Aviso{}
	if s == nil {
		return out
	}
	idiomas := append([]string{s.Languages.Default}, s.Languages.Others...)

	if s.SEO.Noindex {
		out = append(out, Aviso{Code: SitioNoindex, Severity: GravedadInfo})
	}
	if s.SEO.OGImage == nil {
		out = append(out, Aviso{Code: ImagenCompartirNo, Severity: GravedadInfo})
	}
	duplicada := titulosDuplicados(s.Pages, idiomas)
	for i := range s.Pages {
		out = append(out, avisosDePagina(&s.Pages[i], i, idiomas, duplicada)...)
	}
	return out
}

// titulosDuplicados marca, por (página, idioma), los títulos que comparten dos o más
// páginas en ese idioma, sin acentos ni mayúsculas: dos páginas con el mismo título
// compiten entre sí en el buscador y no se distinguen en la pestaña.
func titulosDuplicados(paginas []ir.Pagina, idiomas []string) map[[2]string]bool {
	vistos := map[[2]string][]string{} // (idioma, título plegado) → ids de página
	for i := range paginas {
		for _, l := range idiomas {
			if t := strings.TrimSpace(paginas[i].Title[l]); t != "" {
				k := [2]string{l, buscar.Plegar(t)}
				vistos[k] = append(vistos[k], paginas[i].ID)
			}
		}
	}
	duplicada := map[[2]string]bool{}
	for k, ids := range vistos {
		if len(ids) < 2 {
			continue
		}
		for _, id := range ids {
			duplicada[[2]string{id, k[0]}] = true
		}
	}
	return duplicada
}

// avisosDePagina son los de UNA página, ordenados por código y por idioma.
func avisosDePagina(p *ir.Pagina, i int, idiomas []string, duplicada map[[2]string]bool) []Aviso {
	def := idiomas[0]
	nombre := nombrePagina(p, i, def)
	aviso := func(code, sev, lang string, n int) Aviso {
		return Aviso{Code: code, Severity: sev, PageID: p.ID, Page: nombre, Lang: lang, Count: n}
	}
	var out []Aviso
	if p.Noindex {
		out = append(out, aviso(PaginaNoindex, GravedadInfo, "", 0))
	}
	for _, l := range idiomas {
		if c, n := longitud(p.Title[l], MaxTitulo, TituloFalta, TituloLargo); c != "" {
			out = append(out, aviso(c, GravedadAviso, l, n))
		}
		if duplicada[[2]string{p.ID, l}] {
			out = append(out, aviso(TituloDuplicado, GravedadAviso, l, 0))
		}
		if c, n := longitud(p.Description[l], MaxDescripcion, DescripcionFalta, DescripcionLarga); c != "" {
			out = append(out, aviso(c, GravedadAviso, l, n))
		}
	}
	if n := imagenesSinAlt(p.Sections, def); n > 0 {
		out = append(out, aviso(ImagenSinAlt, GravedadAviso, def, n))
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Code != out[b].Code {
			return out[a].Code < out[b].Code
		}
		return out[a].Lang < out[b].Lang
	})
	return out
}

// longitud devuelve el código del problema de un texto (vacío o más largo que max)
// y, si es largo, cuántos caracteres tiene. Código vacío: está bien.
func longitud(texto string, maxRunas int, falta, largo string) (string, int) {
	t := strings.TrimSpace(texto)
	n := utf8.RuneCountInString(t)
	switch {
	case n == 0:
		return falta, 0
	case n > maxRunas:
		return largo, n
	}
	return "", 0
}

// nombrePagina es cómo la reconoce quien edita: su título en el idioma por defecto
// o, si no lo tiene, su ruta («/» la portada).
func nombrePagina(p *ir.Pagina, i int, def string) string {
	if t := strings.TrimSpace(p.Title[def]); t != "" {
		return t
	}
	if i == 0 {
		return "/"
	}
	if s := strings.TrimSpace(p.Slug[def]); s != "" {
		return "/" + s
	}
	return p.ID
}

// imagenesSinAlt cuenta las imágenes de las secciones de una página que no son
// decorativas y no tienen texto alternativo en el idioma por defecto. Una imagen
// marcada como decorativa no necesita alt a propósito (el lector la salta).
func imagenesSinAlt(secciones []ir.Nodo, def string) int {
	n := 0
	var visitar func(x *ir.Nodo)
	visitar = func(x *ir.Nodo) {
		if x.Hidden {
			return
		}
		if x.Type == "image" && !x.Decorative && strings.TrimSpace(x.Alt[def]) == "" {
			n++
		}
		for i := range x.Children {
			visitar(&x.Children[i])
		}
	}
	for i := range secciones {
		visitar(&secciones[i])
	}
	return n
}
