// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"archive/zip"
	"bytes"
	"io"
	"path"
	"strings"
)

// Plantillas de Office y de OpenDocument (.dotx .xltx .potx · .ott .ots .otp).
//
// Una plantilla es el mismo paquete que su documento con otro tipo de contenido
// declarado, así que se IMPORTAN con el mismo lector que .docx, .xlsx, .pptx, .odt,
// .ods y .odp: lo único que cambia es lo que significa abrirla. Abrir una plantilla no
// edita el fichero, crea un documento NUEVO a partir de él, y por eso el resultado
// lleva `Template`: quien lo abre desde Drive no ancla el documento al fichero (anclarlo
// haría que abrir la plantilla dos veces diera el mismo documento, que es justo lo
// contrario de una plantilla).

// extPlantilla son las extensiones de plantilla y el formato del paquete que las lee.
// Las variantes con macros (.dotm .xltm .potm) se leen igual: las macros no se
// ejecutan ni se conservan.
var extPlantilla = map[string]srcFormat{
	".dotx": fmtDocx, ".dotm": fmtDocx,
	".xltx": fmtXlsx, ".xltm": fmtXlsx,
	".potx": fmtPptx, ".potm": fmtPptx,
	".ott": fmtOdt, ".ots": fmtOds, ".otp": fmtOdp,
}

// mimetypesOdfPlantilla es el `mimetype` de un paquete OpenDocument de plantilla y el
// formato que lo lee.
var mimetypesOdfPlantilla = map[string]srcFormat{
	"application/vnd.oasis.opendocument.text-template":         fmtOdt,
	"application/vnd.oasis.opendocument.spreadsheet-template":  fmtOds,
	"application/vnd.oasis.opendocument.presentation-template": fmtOdp,
}

// esPlantillaDeOficina dice si el fichero es una plantilla: por la extensión o, si
// viene renombrado, por lo que el paquete declara (el `mimetype` de ODF o el tipo de
// contenido de la parte principal de OOXML). Solo lee esas dos entradas del ZIP.
func esPlantillaDeOficina(filename string, data []byte) bool {
	if _, ok := extPlantilla[strings.ToLower(path.Ext(filename))]; ok {
		return true
	}
	if !isZipPackage(data) {
		return false
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	for _, f := range zr.File {
		if f.Name != "mimetype" && f.Name != "[Content_Types].xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(rc, 1<<20))
		rc.Close()
		txt := string(b)
		if f.Name == "mimetype" {
			if _, ok := mimetypesOdfPlantilla[strings.TrimSpace(txt)]; ok {
				return true
			}
			continue
		}
		if strings.Contains(txt, ".template.main+xml") || strings.Contains(txt, "macroEnabledTemplate") {
			return true
		}
	}
	return false
}
