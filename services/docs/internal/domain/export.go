// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
)

// Exportador clean-room a formatos ofimáticos ABIERTOS:
//   - OOXML (ECMA-376): docx/xlsx/pptx — paquete OPC (ZIP) con XML.
//   - ODF (OASIS ODF 1.3): odt/ods/odp — ZIP con content.xml + mimetype.
//   - PDF/HTML/Markdown: export de lectura.
//
// Obra original de Sytue sobre las especificaciones públicas (ECMA-376 y OASIS
// ODF). No usa código de OnlyOffice/LibreOffice (AGPL/MPL no portado). Las
// librerías de ZIP/XML usadas son de la stdlib de Go (BSD).
//
// Esta primera entrega cubre el documento de texto (KindDocument) con fidelidad
// de párrafos; hojas y presentaciones exportan su representación textual/tabular
// base (el editor del frontend posee el modelo rico y puede ampliar el mapeo).

// Export serializa el estado materializado de un documento al formato pedido.
// Devuelve los bytes, el content-type y la extensión de fichero sugerida.
func Export(kind Kind, title string, state *State, format BlobFormat) (data []byte, contentType, ext string, err error) {
	return ExportWithComments(kind, title, state, format, nil)
}

// ExportWithComments es Export con los comentarios del documento (doc_comments), que DOCX y
// ODT llevan en su parte propia con el ancla y el hilo; los demás formatos no los llevan.
func ExportWithComments(kind Kind, title string, state *State, format BlobFormat, comments []Comment) (data []byte, contentType, ext string, err error) {
	if !format.Valid() {
		return nil, "", "", ErrInvalidFormat
	}
	switch format {
	case FormatMarkdown:
		return []byte(exportMarkdown(kind, title, state)), "text/markdown; charset=utf-8", "md", nil
	case FormatHTML:
		return []byte(exportHTML(kind, title, state)), "text/html; charset=utf-8", "html", nil
	case FormatOOXML:
		return exportOOXML(kind, title, state, comments)
	case FormatODF:
		return exportODF(kind, title, state, comments)
	case FormatPDF:
		return exportPDF(kind, title, state)
	default:
		return nil, "", "", fmt.Errorf("%w: export %q no implementado en backend", ErrInvalidFormat, format)
	}
}

// exportMarkdown produce un Markdown simple: título + párrafos del cuerpo.
func exportMarkdown(kind Kind, title string, s *State) string {
	var b strings.Builder
	if title != "" {
		b.WriteString("# ")
		b.WriteString(title)
		b.WriteString("\n\n")
	}
	if kind == KindSpreadsheet {
		sheetMarkdown(&b, s)
		return b.String()
	}
	if paras := documentHTMLParagraphs(kind, s); paras != nil {
		b.WriteString(strings.Join(paras, "\n\n"))
	} else {
		b.WriteString(s.PlainText()) // modelos no-documento / estados RGA
	}
	b.WriteString("\n")
	return b.String()
}

// exportHTML produce un HTML de documento completo. Para KindDocument el modelo
// del editor YA es HTML (registro doc:html): se emite dentro del cuerpo, saneado al
// salir (SanearHTML). Para el resto se escapa el texto plano.
func exportHTML(kind Kind, title string, s *State) string {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"es\"><head><meta charset=\"utf-8\">\n")
	b.WriteString("<title>")
	b.WriteString(html.EscapeString(title))
	b.WriteString("</title></head><body>\n")
	// El HTML del editor se sanea TAMBIÉN al salir: lo guardado antes de sanear al
	// guardar puede no estarlo, y este fichero lo abre quien lo descarga o lo ve por un
	// enlace, que no es quien lo escribió.
	if kind == KindSpreadsheet {
		sheetHTML(&b, s)
	} else if kind == KindPresentation {
		slidesHTML(&b, title, s)
	} else if body := SanearHTML(strings.TrimSpace(s.documentHTML())); kind == KindDocument && body != "" {
		b.WriteString(body)
		b.WriteString("\n")
	} else {
		for _, para := range splitParagraphs(s.PlainText()) {
			b.WriteString("<p>")
			b.WriteString(html.EscapeString(para))
			b.WriteString("</p>\n")
		}
	}
	b.WriteString("</body></html>\n")
	return b.String()
}

// documentHTML devuelve el HTML del documento que el editor guarda en el registro
// LWW "doc:html" (una cadena JSON). Vacío si no existe o no es una cadena.
func (s *State) documentHTML() string {
	return s.contentString("doc:html")
}

// contentString devuelve el contenido de un registro LWW cuyo valor es una cadena
// JSON (doc:html, doc:sheet, doc:slides): el editor guarda el modelo del tipo en
// una sola clave (CONTENT_KEY). "" si la clave no existe o no es una cadena.
func (s *State) contentString(key string) string {
	r, ok := s.Registers[key]
	if !ok {
		return ""
	}
	var v string
	if json.Unmarshal(r.Value, &v) != nil {
		return ""
	}
	return v
}

var (
	reBlockClose = regexp.MustCompile(`(?i)</(p|h[1-6]|li|div|blockquote|tr|section|article)>`)
	reLineBreak  = regexp.MustCompile(`(?i)<br\s*/?>`)
	reAnyTag     = regexp.MustCompile(`<[^>]*>`)
)

var (
	reDibujo       = regexp.MustCompile(`(?is)<figure\b[^>]*\bdata-dibujo\b[^>]*>.*?</figure>`)
	reDibujoDesc   = regexp.MustCompile(`(?is)<desc>(.*?)</desc>`)
	reDibujoTitulo = regexp.MustCompile(`(?is)<title>(.*?)</title>`)
)

// htmlToParagraphs extrae los párrafos de texto de un fragmento HTML del editor:
// los cierres de bloque y los <br> se vuelven saltos de línea, el resto de
// etiquetas se elimina y las entidades se decodifican. Sin dependencias.
func htmlToParagraphs(fragment string) []string {
	// Un dibujo de Docus no es texto: su <svg> lleva rótulos que saldrían sueltos. Queda su
	// texto alternativo.
	fragment = reDibujo.ReplaceAllStringFunc(fragment, func(fig string) string {
		for _, re := range []*regexp.Regexp{reDibujoDesc, reDibujoTitulo} {
			if m := re.FindStringSubmatch(fig); len(m) > 1 && strings.TrimSpace(m[1]) != "" {
				return "<p>[dibujo: " + strings.TrimSpace(m[1]) + "]</p>"
			}
		}
		return "<p>[dibujo]</p>"
	})
	withBreaks := reLineBreak.ReplaceAllString(reBlockClose.ReplaceAllString(fragment, "\n"), "\n")
	text := html.UnescapeString(reAnyTag.ReplaceAllString(withBreaks, ""))
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// documentHTMLParagraphs devuelve los párrafos de texto del documento tomados del
// registro doc:html del editor, o nil si no es un KindDocument con ese contenido
// (los otros modelos y los estados RGA siguen exportando por PlainText).
func documentHTMLParagraphs(kind Kind, s *State) []string {
	if kind != KindDocument {
		return nil
	}
	if strings.TrimSpace(s.documentHTML()) == "" {
		return nil
	}
	return htmlToParagraphs(s.documentHTML())
}

// exportOOXML construye un paquete OPC mínimo (ECMA-376) para el tipo dado.
func exportOOXML(kind Kind, title string, s *State, comments []Comment) ([]byte, string, string, error) {
	switch kind {
	case KindDocument:
		buf, err := buildDocx(title, s, comments)
		return buf, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "docx", err
	case KindSpreadsheet:
		buf, err := buildXlsx(s)
		return buf, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "xlsx", err
	case KindPresentation:
		buf, err := buildPptx(title, s)
		return buf, "application/vnd.openxmlformats-officedocument.presentationml.presentation", "pptx", err
	default:
		return nil, "", "", ErrInvalidKind
	}
}

// exportODF construye un paquete ODF (OASIS ODF 1.3): mimetype + content.xml con
// estilos automáticos y cuerpo del tipo correspondiente (odt/ods/odp). El mapeo
// del contenido vive en export_odt.go / export_sheet.go / export_slides.go.
func exportODF(kind Kind, title string, s *State, comments []Comment) ([]byte, string, string, error) {
	var mimetype, ext, styles, body, stylesDoc string
	var media map[string][]byte
	switch kind {
	case KindDocument:
		mimetype, ext = "application/vnd.oasis.opendocument.text", "odt"
		w := newODTWriter()
		body = odtTextBody(title, s, w, comments)
		styles = w.styles.String()
		stylesDoc = w.stylesDoc
		media = w.media
	case KindSpreadsheet:
		mimetype, ext = "application/vnd.oasis.opendocument.spreadsheet", "ods"
		styles, body, stylesDoc, media = odsBody(s)
	case KindPresentation:
		mimetype, ext = "application/vnd.oasis.opendocument.presentation", "odp"
		styles, body, stylesDoc, media = odpPackage(title, s)
	default:
		return nil, "", "", ErrInvalidKind
	}

	content := xmlHeader +
		`<office:document-content ` +
		`xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" ` +
		`xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0" ` +
		`xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" ` +
		`xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0" ` +
		`xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0" ` +
		`xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0" ` +
		`xmlns:xlink="http://www.w3.org/1999/xlink" ` +
		`xmlns:dc="http://purl.org/dc/elements/1.1/" ` +
		`xmlns:loext="urn:org:documentfoundation:names:experimental:office:xmlns:loext:1.0" ` +
		`xmlns:tableooo="http://openoffice.org/2009/table" ` + // el color de la pestaña de una hoja
		`xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0" ` +
		`xmlns:number="urn:oasis:names:tc:opendocument:xmlns:datastyle:1.0" ` +
		`xmlns:presentation="urn:oasis:names:tc:opendocument:xmlns:presentation:1.0" ` +
		// Sin él, Calc no reconoce el «of:» de table:formula y guarda cada fórmula como
		// «of:=of:=…» en error (se ve en testdata/calcus/lo-de-ods.ods).
		`xmlns:of="urn:oasis:names:tc:opendocument:xmlns:of:1.2" ` +
		`xmlns:anim="urn:oasis:names:tc:opendocument:xmlns:animation:1.0" ` +
		`xmlns:smil="urn:oasis:names:tc:opendocument:xmlns:smil-compatible:1.0" ` +
		`xmlns:sytue="` + odpNSSytue + `" ` +
		`office:version="1.3"><office:automatic-styles>` + styles + `</office:automatic-styles>` +
		`<office:body>` + body + `</office:body></office:document-content>`

	// Las presentaciones exigen una página maestra (draw:master-page), que en ODF
	// vive en styles.xml (odpPackage lo devuelve); documentos y hojas inlinean sus
	// estilos en content.xml.
	data, err := buildODFPackage(mimetype, content, stylesDoc, media)
	if err != nil {
		return nil, "", "", err
	}
	return data, mimetype, ext, nil
}

// buildODFPackage construye el ZIP ODF con `mimetype` como PRIMER miembro y SIN
// comprimir (requisito de OASIS ODF para la detección por firma). `styles` es el
// stream styles.xml opcional (páginas maestras); `media` son binarios embebidos.
func buildODFPackage(mimetype, content, styles string, media map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// mimetype: primero y STORE (sin compresión).
	mh := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	mw, err := zw.CreateHeader(mh)
	if err != nil {
		return nil, fmt.Errorf("odf: mimetype: %w", err)
	}
	if _, err := mw.Write([]byte(mimetype)); err != nil {
		return nil, err
	}

	if err := writeZipMember(zw, "content.xml", []byte(content)); err != nil {
		return nil, err
	}
	if styles != "" {
		if err := writeZipMember(zw, "styles.xml", []byte(styles)); err != nil {
			return nil, err
		}
	}
	if err := writeZipMember(zw, "META-INF/manifest.xml", []byte(odfManifest(mimetype, styles != "", media))); err != nil {
		return nil, err
	}
	for _, name := range sortedBytesKeys(media) {
		if err := writeZipMember(zw, name, media[name]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("odf: cerrar zip: %w", err)
	}
	return buf.Bytes(), nil
}

// zipPackage construye un ZIP (paquete OPC para OOXML) con partes de texto.
func zipPackage(files map[string]string) ([]byte, error) {
	parts := make(map[string][]byte, len(files))
	for k, v := range files {
		parts[k] = []byte(v)
	}
	return zipPackageBytes(parts)
}

// zipPackageBytes construye un ZIP (OPC) admitiendo partes binarias (imágenes).
func zipPackageBytes(files map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range sortedBytesKeys(files) {
		if err := writeZipMember(zw, name, files[name]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("ooxml: cerrar zip: %w", err)
	}
	return buf.Bytes(), nil
}

func writeZipMember(zw *zip.Writer, name string, body []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("zip: %s: %w", name, err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("zip: %s: %w", name, err)
	}
	return nil
}

func sortedBytesKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// --- utilidades de texto/XML ---

const xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

// xmlEscape escapa el texto para contenido XML.
func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return r.Replace(s)
}

// splitParagraphs divide el texto plano en párrafos por saltos de línea.
func splitParagraphs(text string) []string {
	if text == "" {
		return []string{""}
	}
	parts := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(parts))
	out = append(out, parts...)
	return out
}
