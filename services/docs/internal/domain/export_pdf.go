// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/pdfsimple"
)

// Motor PDF clean-room MÍNIMO (solo stdlib) para exportar los tres tipos a PDF.
//
// Elección documentada: en vez de embeber un motor de render AGPL (PDF.js,
// wkhtmltopdf, LibreOffice headless) generamos PDF 1.4 a mano con las 14 fuentes
// estándar (Helvetica normal/negrita/cursiva) y codificación WinAnsi. Es un PDF
// de TEXTO fluido (con paginación y ajuste de línea) del contenido del modelo:
// suficiente para "descargar como PDF" sin dependencias ni licencias virales.
// Limitaciones asumidas (ponytail): el formato inline (negrita/cursiva dentro de
// un párrafo) se aplana al estilo de bloque; imágenes/formas de diapositiva se
// anotan como marcador; scripts no latinos caen a '?'. La fidelidad PLENA vive en
// las rutas OOXML/ODF; el PDF es de lectura.

// El ESCRITOR vive ahora en platform/pkg/pdfsimple: Sytue Notepads exporta a PDF
// exactamente igual —texto paginado— y dos copias del ensamblado de objetos,
// xref y trailer serían dos sitios donde se desalinea un offset. Aquí se queda
// lo que SÍ es de Sytue Docs: cómo se convierte un documento, una hoja o una
// presentación en llamadas a ese escritor.
// Las fuentes se reexportan porque otros ficheros del paquete las nombran; las
// métricas de página no, que solo las usaba el escritor.
const (
	fontRegular = pdfsimple.FuenteNormal
	fontBold    = pdfsimple.FuenteNegrita
	fontItalic  = pdfsimple.FuenteCursiva
)

// Alias de los ayudantes del formato, que otros ficheros de este paquete usan
// (pdfoverlay, pdfform…). Se reexportan aquí en vez de tocar sus veinte usos:
// el que cambia de sitio es el escritor, no ellos.
var (
	ftoa      = pdfsimple.Ftoa
	pdfEscape = pdfsimple.Escapar
	pad10     = pdfsimple.Pad10
)

func exportPDF(kind Kind, title string, s *State) ([]byte, string, string, error) {
	w := pdfsimple.Nuevo()
	switch kind {
	case KindDocument:
		pdfDocument(w, title, s)
	case KindSpreadsheet:
		pdfSpreadsheet(w, title, s)
	case KindPresentation:
		pdfPresentation(w, title, s)
	default:
		return nil, "", "", ErrInvalidKind
	}
	return w.Bytes(), "application/pdf", "pdf", nil
}

// --- contenido por tipo ---

func pdfDocument(w *pdfsimple.Escritor, title string, s *State) {
	if title != "" {
		w.Texto(title, fontBold, 22)
		w.Hueco(8)
	}
	for _, blk := range documentBlocks(KindDocument, s) {
		switch {
		case blk.para != nil:
			pdfParagraph(w, *blk.para)
		case blk.table != nil:
			pdfTable(w, blk.table)
		case blk.hr:
			w.Texto(strings.Repeat("—", 24), fontRegular, 11)
			w.Hueco(4)
		}
	}
}

func pdfParagraph(w *pdfsimple.Escritor, p xpara) {
	text := strings.TrimSpace(p.plain())
	if text == "" {
		w.Hueco(8)
		return
	}
	font, size := fontRegular, 11.0
	switch {
	case p.style == "Heading1":
		font, size = fontBold, 20
	case p.style == "Heading2":
		font, size = fontBold, 16
	case p.style == "Heading3":
		font, size = fontBold, 14
	case strings.HasPrefix(p.style, "Heading"):
		font, size = fontBold, 12
	case p.style == "Quote":
		font = fontItalic
	}
	prefix := strings.Repeat("    ", p.level)
	switch p.list {
	case "bullet":
		prefix += "• "
	case "number":
		prefix += "- "
	}
	w.Texto(prefix+text, font, size)
	if strings.HasPrefix(p.style, "Heading") {
		w.Hueco(4)
	}
}

func pdfTable(w *pdfsimple.Escritor, t *xtable) {
	for _, row := range t.rows {
		cells := make([]string, 0, len(row))
		for _, c := range row {
			if c.covered {
				continue
			}
			var parts []string
			for _, p := range c.paras {
				if txt := strings.TrimSpace(p.plain()); txt != "" {
					parts = append(parts, txt)
				}
			}
			cells = append(cells, strings.Join(parts, " "))
		}
		w.Texto(strings.Join(cells, "  |  "), fontRegular, 11)
	}
	w.Hueco(6)
}

func pdfSpreadsheet(w *pdfsimple.Escritor, title string, s *State) {
	if title != "" {
		w.Texto(title, fontBold, 20)
		w.Hueco(8)
	}
	// Los VALORES, fila a fila, separados por « | »: una celda vacía deja su hueco.
	for _, g := range sheetGrids(sheetModelOrEmpty(s)) {
		w.Texto(g.name, fontBold, 14)
		w.Hueco(4)
		for _, row := range g.rows {
			fin := len(row)
			for fin > 0 && row[fin-1] == "" {
				fin--
			}
			w.Texto(strings.Join(row[:fin], " | "), fontRegular, 11)
		}
		w.Hueco(10)
	}
}

func pdfPresentation(w *pdfsimple.Escritor, title string, s *State) {
	model := slidesModelOf(title, s)
	for i, sl := range model.Slides {
		if i > 0 {
			w.NuevaPagina()
		}
		w.Texto("Diapositiva "+strconv.Itoa(i+1), fontBold, 16)
		w.Hueco(6)
		for _, box := range sl.Boxes {
			pdfSlideBox(w, box)
		}
		if notes := strings.TrimSpace(sl.Notes); notes != "" {
			w.Hueco(6)
			w.Texto("Notas:", fontBold, 11)
			for _, line := range splitParagraphs(notes) {
				w.Texto(line, fontItalic, 11)
			}
		}
	}
}

// pdfSlideBox vuelca una caja de diapositiva al PDF: imágenes y formas como
// marcador textual, texto como párrafos fluidos.
func pdfSlideBox(w *pdfsimple.Escritor, box slideBox) {
	switch box.Kind {
	case "image":
		w.Texto("[imagen]", fontItalic, 11)
	case "shape":
		shape := box.Shape
		if shape == "" {
			shape = "forma"
		}
		w.Texto("[forma: "+shape+"]", fontItalic, 11)
	default:
		for _, blk := range parseHTMLBlocks(box.HTML) {
			if blk.para == nil {
				continue
			}
			if txt := strings.TrimSpace(blk.para.plain()); txt != "" {
				font := fontRegular
				if strings.HasPrefix(blk.para.style, "Heading") {
					font = fontBold
				}
				w.Texto(txt, font, 12)
			}
		}
	}
}
