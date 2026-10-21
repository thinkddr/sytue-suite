// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// CATÁLOGO COMÚN DE TEMAS de Docus, Calcus y Diapos (ROADMAP-OFFICE plantillas-1).
//
// Hasta ahora solo Diapos tenía temas (slideThemePresets: fondo, tinta y fuente). El
// catálogo común los REUTILIZA —mismos ids, mismos colores— y les añade lo que un
// documento y una hoja necesitan: un acento (el color de los títulos y de la cabecera de
// una tabla), la tinta que va sobre él y las tipografías de títulos y de cuerpo. Un tema,
// los tres productos.
//
// Y añade la MARCA de la organización: un tema más, con los colores y las tipografías que
// fija quien administra, versionado (cada cambio es una versión nueva y las anteriores se
// conservan). Se aplica al crear un documento y a uno que ya existe.

const (
	// IDTemaPapel es el tema neutro: aplicarlo quita lo que otro tema puso.
	IDTemaPapel = "papel"
	// IDTemaMarca es el id del tema de la organización.
	IDTemaMarca = "marca"
)

// TemaComun es un tema del catálogo o la marca de la organización. Los colores van como
// «#RRGGBB» en mayúsculas; una fuente vacía es la de por defecto del producto.
type TemaComun struct {
	ID string `json:"id"`
	// Marca: es el tema de la organización (no del catálogo). Name y Version solo los lleva ella.
	Marca       bool   `json:"brand,omitempty"`
	Name        string `json:"name,omitempty"`
	Version     int    `json:"version,omitempty"`
	Bg          string `json:"bg"`         // fondo de una diapositiva
	Ink         string `json:"ink"`        // tinta de una diapositiva
	Accent      string `json:"accent"`     // títulos de un documento, cabecera de tabla y de hoja
	AccentInk   string `json:"accent_ink"` // tinta sobre el acento
	HeadingFont string `json:"heading_font,omitempty"`
	BodyFont    string `json:"body_font,omitempty"`
	// Logo es el logotipo de la organización (solo la marca, y solo si lo tiene): se
	// incrusta con ella en la cabecera de un documento y en la portada de una presentación
	// (tema_logo.go). No se serializa: a la web solo le llega su versión.
	Logo     *LogoMarca `json:"-"`
	LogoHash string     `json:"logo,omitempty"`
}

// LogoMarca es el logotipo listo para incrustar: un PNG ya validado y recodificado por el
// servicio (internal/logoorg), su versión y su tamaño en píxeles.
type LogoMarca struct {
	PNG   []byte
	Hash  string
	Ancho int
	Alto  int
}

// ConLogo devuelve el tema con el logotipo de la organización (nil lo quita).
func (t TemaComun) ConLogo(l *LogoMarca) TemaComun {
	t.Logo, t.LogoHash = l, ""
	if l != nil {
		t.LogoHash = l.Hash
	}
	return t
}

// acentosTema es el acento y su tinta de cada preset de Diapos: oscuros, que son los que
// leen bien como título sobre el folio blanco de un documento.
var acentosTema = map[string][2]string{
	"carbon":  {"1F2937", "FFFFFF"},
	"pizarra": {"334155", "FFFFFF"},
	"arena":   {"9A3412", "FFFFFF"},
	"menta":   {"166534", "FFFFFF"},
	"cielo":   {"1E40AF", "FFFFFF"},
}

// CatalogoTemas es el catálogo común: el papel y los presets de Diapos.
func CatalogoTemas() []TemaComun {
	out := make([]TemaComun, 0, 1+len(slideThemePresets))
	out = append(out, TemaComun{ID: IDTemaPapel, Bg: "#" + temaPapel.Bg, Ink: "#" + temaPapel.Ink, Accent: "#0A0A0A", AccentInk: "#FFFFFF"})
	for _, t := range slideThemePresets {
		a := acentosTema[t.ID]
		out = append(out, TemaComun{
			ID: t.ID, Bg: "#" + t.Bg, Ink: "#" + t.Ink,
			Accent: "#" + a[0], AccentInk: "#" + a[1], HeadingFont: t.Font, BodyFont: t.Font,
		})
	}
	return out
}

// MarcaOrg es una versión de la marca de la organización.
type MarcaOrg struct {
	Version     int       `json:"version"`
	Name        string    `json:"name"`
	Bg          string    `json:"bg"`
	Ink         string    `json:"ink"`
	Accent      string    `json:"accent"`
	HeadingFont string    `json:"heading_font,omitempty"`
	BodyFont    string    `json:"body_font,omitempty"`
	CreatedBy   string    `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// Normalizar sanea la marca que llega de fuera: colores «#RRGGBB», nombre acotado y
// fuentes de verdad (van a un atributo de estilo y a nombres de fuente de los ficheros).
func (m *MarcaOrg) Normalizar() error {
	m.Name = strings.TrimSpace(m.Name)
	if m.Name == "" || utf8.RuneCountInString(m.Name) > maxNombreTema {
		return fmt.Errorf("%w: nombre de la marca", ErrInvalidOp)
	}
	for _, c := range []*string{&m.Bg, &m.Ink, &m.Accent} {
		n := normalizeColor(*c)
		if n == "" {
			return fmt.Errorf("%w: color de la marca", ErrInvalidOp)
		}
		*c = "#" + n
	}
	for _, f := range []*string{&m.HeadingFont, &m.BodyFont} {
		if strings.TrimSpace(*f) == "" {
			*f = ""
			continue
		}
		if *f = fuenteTema(*f); *f == "" {
			return fmt.Errorf("%w: tipografía de la marca", ErrInvalidOp)
		}
	}
	return nil
}

// Tema es la marca como un tema más. La tinta sobre el acento es blanca u oscura según
// la luminancia del acento: así una marca clara también deja el texto legible.
func (m MarcaOrg) Tema() TemaComun {
	ink := "#FFFFFF"
	if luminancia(strings.TrimPrefix(m.Accent, "#")) > 150 {
		ink = "#0A0A0A"
	}
	return TemaComun{
		ID: IDTemaMarca, Marca: true, Name: m.Name, Version: m.Version,
		Bg: m.Bg, Ink: m.Ink, Accent: m.Accent, AccentInk: ink,
		HeadingFont: m.HeadingFont, BodyFont: m.BodyFont,
	}
}

// TemaPorID resuelve un id contra el catálogo y la marca (nil = la organización no tiene).
func TemaPorID(id string, marca *MarcaOrg) (TemaComun, bool) {
	if id == IDTemaMarca {
		if marca == nil {
			return TemaComun{}, false
		}
		return marca.Tema(), true
	}
	for _, t := range CatalogoTemas() {
		if t.ID == id {
			return t, true
		}
	}
	return TemaComun{}, false
}
