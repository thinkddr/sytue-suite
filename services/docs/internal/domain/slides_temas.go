// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Temas propios de una presentación: los colores (fondo y tinta) y la fuente que fija
// quien la hace, guardados en el modelo (`themes`, y `theme` con su id) y no en un
// catálogo. Es lo que el editor guarda en `SlideCustomTheme` (slides/temas.ts); aquí se
// sanean igual, porque el modelo de un fichero ajeno es entrada no confiable.

// customTheme es un tema propio.
type customTheme struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Bg   string `json:"bg"`
	Ink  string `json:"ink"`
	Font string `json:"font,omitempty"`
}

const (
	maxTemasPropios = 12
	maxNombreTema   = 60
	// idTemaImportado es el id del tema que se deduce al importar una presentación ajena.
	idTemaImportado = "custom-importado"
)

var (
	reIDTema     = regexp.MustCompile(`^custom-[a-z0-9-]{1,24}$`)
	reFuenteTema = regexp.MustCompile(`^[\p{L}\p{N} .\-]{1,40}$`)
)

// fuenteTema es la familia tipográfica de un tema si es una de verdad (letras, cifras,
// espacios, puntos y guiones): va a un atributo de estilo y a un nombre de fuente de
// OOXML/ODF, y no se deja pasar nada más.
func fuenteTema(raw string) string {
	f := strings.TrimSpace(raw)
	if !reFuenteTema.MatchString(f) {
		return ""
	}
	return f
}

// resolved es el tema ya resuelto: colores sin «#» en mayúsculas; ok=false si el tema no
// vale (id, fondo o tinta mal formados).
func (c customTheme) resolved() (slideTheme, bool) {
	bg, ink := normalizeColor(c.Bg), normalizeColor(c.Ink)
	if !reIDTema.MatchString(c.ID) || bg == "" || ink == "" {
		return slideTheme{}, false
	}
	name := strings.TrimSpace(c.Name)
	if utf8.RuneCountInString(name) > maxNombreTema {
		name = string([]rune(name)[:maxNombreTema])
	}
	return slideTheme{ID: c.ID, Bg: bg, Ink: ink, Font: fuenteTema(c.Font), Name: name}, true
}

// esPapel dice si un fondo, una tinta y una fuente son los de la presentación sin tema: el
// folio blanco con letra oscura y la fuente por defecto (la que el exportador pone cuando
// el tema no fija la suya). No hace falta un tema propio para esa.
func esPapel(bg, ink, font string) bool {
	return strings.EqualFold(bg, temaPapel.Bg) && luminancia(ink) < 60 && (font == "" || font == pptxFontDefault)
}

// temaAjeno fija el tema de una presentación importada: el del editor que coincide con
// su fondo, su tinta y su fuente, o, si no hay ninguno y no es el papel blanco de siempre,
// un tema propio con ellos (`name` es el nombre que traía el fichero, si traía alguno).
// Antes el fondo de una presentación ajena se perdía, o se copiaba a cada diapositiva.
func (m *slidesModel) temaAjeno(bg, ink, font, name string) {
	bg, ink = normalizeColor("#"+strings.TrimPrefix(bg, "#")), normalizeColor("#"+strings.TrimPrefix(ink, "#"))
	if bg == "" {
		return
	}
	if ink == "" {
		ink = "000000" // sin tinta dicha, la negra de siempre
	}
	if id := themeIDFor(bg, ink, font); id != "" {
		m.Theme = id
		return
	}
	if esPapel(bg, ink, font) {
		return
	}
	if font == pptxFontDefault {
		font = ""
	}
	name = strings.TrimSpace(name)
	if name == "" || strings.EqualFold(name, "Default") {
		name = "Importado"
	}
	m.Themes = []customTheme{{ID: idTemaImportado, Name: name, Bg: "#" + bg, Ink: "#" + ink, Font: fuenteTema(font)}}
	m.Theme = idTemaImportado
}

// quitarFondosDelTema borra el fondo propio de las diapositivas que es el del tema: LibreOffice
// escribe el fondo del patrón en cada una, y no es un fondo propio de ninguna.
func (m *slidesModel) quitarFondosDelTema() {
	bg := m.theme().Bg
	for i := range m.Slides {
		if strings.EqualFold(normalizeColor(m.Slides[i].Bg), bg) {
			m.Slides[i].Bg = ""
		}
	}
}
