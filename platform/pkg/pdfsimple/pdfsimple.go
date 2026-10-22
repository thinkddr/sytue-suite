// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package pdfsimple escribe PDF de TEXTO con solo la biblioteca estándar.
//
// Sale de `services/docs/internal/domain/export_pdf.go`, donde nació para
// exportar documentos, hojas y presentaciones. Se mueve aquí sin cambiarle una
// línea de comportamiento porque Sytue Notepads necesita exactamente lo mismo
// —volcar texto paginado a un PDF— y la alternativa era una segunda copia de
// ciento ochenta líneas de ensamblado de objetos, xref y trailer: dos copias de
// un formato binario son dos sitios donde se desalinea un offset.
//
// LO QUE ESTE PAQUETE NO ES: un motor de maquetación. No hay imágenes, ni
// tablas, ni colores, ni fuentes embebidas —usa las catorce estándar del
// formato—. Es deliberado: el motivo original era no embeber un motor de render
// AGPL para producir un fichero que casi siempre es texto con títulos.
package pdfsimple

import (
	"bytes"
	"strconv"
	"strings"
)

// Medidas de página en puntos: A4 con márgenes de ~2 cm.
const (
	PageW  = 595.28
	PageH  = 841.89
	Margin = 56.7
)

// Las tres fuentes que el escritor declara en el recurso de cada página. Son
// nombres del PDF, no de la tipografía: el visor las resuelve a las catorce
// estándar del formato, que no hay que embeber.
const (
	FuenteNormal  = "F1"
	FuenteNegrita = "F2"
	FuenteCursiva = "F3"
)

type Escritor struct {
	pages []string
	cur   bytes.Buffer
	y     float64
}

func Nuevo() *Escritor {
	return &Escritor{y: PageH - Margin}
}

// text escribe una cadena con ajuste de línea y paginación automática.
func (w *Escritor) Texto(s, font string, size float64) {
	maxW := PageW - 2*Margin
	for _, line := range wrapText(s, size, maxW) {
		if w.y < Margin+size {
			w.NuevaPagina()
		}
		w.cur.WriteString("BT /" + font + " " + Ftoa(size) + " Tf " +
			Ftoa(Margin) + " " + Ftoa(w.y) + " Td (" + Escapar(line) + ") Tj ET\n")
		w.y -= size * 1.45
	}
}

func (w *Escritor) Hueco(h float64) { w.y -= h }

func (w *Escritor) NuevaPagina() {
	w.pages = append(w.pages, w.cur.String())
	w.cur.Reset()
	w.y = PageH - Margin
}

// render ensambla el fichero PDF (objetos + xref + trailer).
func (w *Escritor) Bytes() []byte {
	// Volcar la página en curso (siempre al menos una).
	w.pages = append(w.pages, w.cur.String())

	// Objetos: 1 Catalog, 2 Pages, 3-5 Fonts, luego por página (contenido + page).
	objs := make([]string, 0, 5+2*len(w.pages))
	objs = append(objs,
		"<< /Type /Catalog /Pages 2 0 R >>",
		"", // placeholder Pages (obj 2), se rellena al final
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Oblique /Encoding /WinAnsiEncoding >>",
	)

	kids := make([]string, 0, len(w.pages))
	for _, content := range w.pages {
		if content == "" {
			content = "BT ET\n"
		}
		contentNum := len(objs) + 1
		objs = append(objs, "<< /Length "+strconv.Itoa(len(content))+" >>\nstream\n"+content+"endstream")
		pageNum := len(objs) + 1
		objs = append(objs, "<< /Type /Page /Parent 2 0 R "+
			"/MediaBox [0 0 "+Ftoa(PageW)+" "+Ftoa(PageH)+"] "+
			"/Resources << /Font << /F1 3 0 R /F2 4 0 R /F3 5 0 R >> >> "+
			"/Contents "+strconv.Itoa(contentNum)+" 0 R >>")
		kids = append(kids, strconv.Itoa(pageNum)+" 0 R")
	}
	objs[1] = "<< /Type /Pages /Kids [" + strings.Join(kids, " ") + "] /Count " + strconv.Itoa(len(kids)) + " >>"

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, len(objs)+1)
	for i, body := range objs {
		offsets[i+1] = buf.Len()
		buf.WriteString(strconv.Itoa(i+1) + " 0 obj\n" + body + "\nendobj\n")
	}
	xref := buf.Len()
	buf.WriteString("xref\n0 " + strconv.Itoa(len(objs)+1) + "\n")
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(objs); i++ {
		buf.WriteString(Pad10(offsets[i]) + " 00000 n \n")
	}
	buf.WriteString("trailer\n<< /Size " + strconv.Itoa(len(objs)+1) + " /Root 1 0 R >>\n")
	buf.WriteString("startxref\n" + strconv.Itoa(xref) + "\n%%EOF")
	return buf.Bytes()
}

// --- helpers de texto/codificación ---

// wrapText parte el texto en líneas que caben en maxW usando el ancho medio de
// Helvetica (~0.5 em). Aproximado a propósito: el objetivo es no desbordar el
// margen, no la justificación tipográfica exacta.
func wrapText(s string, size, maxW float64) []string {
	if s == "" {
		return []string{""}
	}
	maxChars := int(maxW / (size * 0.5))
	if maxChars < 8 {
		maxChars = 8
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := ""
		for _, wd := range words {
			// Palabra sola más larga que la línea: trocearla.
			for len(wd) > maxChars {
				if line != "" {
					out = append(out, line)
					line = ""
				}
				out = append(out, wd[:maxChars])
				wd = wd[maxChars:]
			}
			switch {
			case line == "":
				line = wd
			case len(line)+1+len(wd) <= maxChars:
				line += " " + wd
			default:
				out = append(out, line)
				line = wd
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// pdfEscape codifica a WinAnsi y escapa los caracteres especiales de cadena PDF.
// Escapar prepara una cadena para ir dentro de un literal `( )` de PDF.
func Escapar(s string) string {
	var b strings.Builder
	for _, r := range s {
		c := winAnsiByte(r)
		switch c {
		case '\\', '(', ')':
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	return b.String()
}

// winAnsiByte mapea un rune a su byte WinAnsi (CP1252). Latin-1 (0xA0-0xFF)
// coincide con Unicode; se añaden los tipográficos comunes; el resto cae a '?'.
func winAnsiByte(r rune) byte {
	switch {
	case r >= 0 && r < 0x80:
		return byte(r)
	case r >= 0xA0 && r <= 0xFF:
		return byte(r)
	}
	switch r {
	case '€': // €
		return 0x80
	case '‘': // ‘
		return 0x91
	case '’': // ’
		return 0x92
	case '“': // “
		return 0x93
	case '”': // ”
		return 0x94
	case '•': // •
		return 0x95
	case '–': // –
		return 0x96
	case '—': // —
		return 0x97
	default:
		return '?'
	}
}

// Ftoa formatea un número como los espera el formato: dos decimales, sin
// notación científica.
func Ftoa(f float64) string { return strconv.FormatFloat(f, 'f', 2, 64) }

// Pad10 rellena a diez dígitos, como exige cada entrada de la tabla xref.
func Pad10(n int) string {
	s := strconv.Itoa(n)
	if len(s) >= 10 {
		return s
	}
	return strings.Repeat("0", 10-len(s)) + s
}
