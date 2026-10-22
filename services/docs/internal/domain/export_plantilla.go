// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strings"
)

// ComoPlantilla convierte el paquete que ha exportado Export (OOXML u ODF) en una
// plantilla de Office u OpenDocument: el mismo contenido con el tipo declarado de
// plantilla (.dotx .xltx .potx · .ott .ots .otp). Devuelve los bytes, el content-type y
// la extensión. Sobre otro formato, ErrInvalidFormat.
//
// Es un cambio de DECLARACIÓN y no de contenido: lo que Word, Excel, PowerPoint o
// LibreOffice hacen distinto con una plantilla (abrirla como documento nuevo) lo deciden
// ellos por ese tipo, y por eso es lo único que se toca.
func ComoPlantilla(kind Kind, format BlobFormat, data []byte) ([]byte, string, string, error) {
	switch format {
	case FormatOOXML:
		var mime, ext, de, a string
		switch kind {
		case KindDocument:
			mime, ext = "application/vnd.openxmlformats-officedocument.wordprocessingml.template", "dotx"
			de, a = "wordprocessingml.document.main+xml", "wordprocessingml.template.main+xml"
		case KindSpreadsheet:
			mime, ext = "application/vnd.openxmlformats-officedocument.spreadsheetml.template", "xltx"
			de, a = "spreadsheetml.sheet.main+xml", "spreadsheetml.template.main+xml"
		case KindPresentation:
			mime, ext = "application/vnd.openxmlformats-officedocument.presentationml.template", "potx"
			de, a = "presentationml.presentation.main+xml", "presentationml.template.main+xml"
		default:
			return nil, "", "", ErrInvalidKind
		}
		out, err := reescribirZip(data, map[string]cambioZip{"[Content_Types].xml": {de, a}}, false)
		return out, mime, ext, err
	case FormatODF:
		var base, ext string
		switch kind {
		case KindDocument:
			base, ext = "application/vnd.oasis.opendocument.text", "ott"
		case KindSpreadsheet:
			base, ext = "application/vnd.oasis.opendocument.spreadsheet", "ots"
		case KindPresentation:
			base, ext = "application/vnd.oasis.opendocument.presentation", "otp"
		default:
			return nil, "", "", ErrInvalidKind
		}
		// El `mimetype` es la primera entrada, SIN comprimir (lo exige ODF), y el
		// manifiesto repite el tipo del paquete.
		out, err := reescribirZip(data, map[string]cambioZip{
			"mimetype":              {base, base + "-template"},
			"META-INF/manifest.xml": {`manifest:media-type="` + base + `"`, `manifest:media-type="` + base + `-template"`},
		}, true)
		return out, base + "-template", ext, err
	default:
		return nil, "", "", fmt.Errorf("%w: solo OOXML y ODF tienen plantilla", ErrInvalidFormat)
	}
}

// cambioZip es una sustitución de texto dentro de una entrada de un ZIP.
type cambioZip struct{ buscar, poner string }

// reescribirZip copia un ZIP cambiando, en las entradas dadas, una cadena por otra. Con
// `exacto`, el contenido entero del `mimetype` debe ser `buscar` (el de ODF). Las demás
// entradas se copian tal cual, sin recomprimir y en el mismo orden.
func reescribirZip(data []byte, cambios map[string]cambioZip, exacto bool) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: paquete ZIP inválido: %w", ErrInvalidFormat, err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	tocadas := 0
	for _, f := range zr.File {
		c, ok := cambios[f.Name]
		if !ok {
			if err := zw.Copy(f); err != nil {
				return nil, err
			}
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		txt := string(b)
		switch {
		case exacto && f.Name == "mimetype":
			if strings.TrimSpace(txt) != c.buscar {
				return nil, fmt.Errorf("%w: el paquete no es del tipo esperado", ErrInvalidFormat)
			}
			txt = c.poner
		case !strings.Contains(txt, c.buscar):
			return nil, fmt.Errorf("%w: %s no declara el tipo esperado", ErrInvalidFormat, f.Name)
		default:
			txt = strings.Replace(txt, c.buscar, c.poner, 1)
		}
		h := &zip.FileHeader{Name: f.Name, Method: f.Method, Modified: f.Modified}
		if f.Name == "mimetype" {
			h.Method = zip.Store
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(txt)); err != nil {
			return nil, err
		}
		tocadas++
	}
	if tocadas != len(cambios) {
		return nil, fmt.Errorf("%w: faltan partes del paquete", ErrInvalidFormat)
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
