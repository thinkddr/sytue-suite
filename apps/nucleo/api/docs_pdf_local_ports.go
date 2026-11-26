// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import "context"

// Interfaces OPCIONALES de un puerto de datos de Drive que solo cambian algo si el puerto
// las cumple (el patrón de DriveFileReplacer). Las usa la edición local, donde los PDF
// son ficheros del disco y no viven todos en la carpeta «PDF»
// (apps/fyne/PLAN-EDICION-LOCAL.md §6.3, P-01, P-09, P-11). Un puerto que no las cumple
// —el repositorio privado de la edición conectada— sigue exactamente como estaba.

// DrivePathResolver resuelve una ruta en su fichero sin recorrer el árbol carpeta a
// carpeta. En local la ruta puede ser la absoluta de un fichero suelto del disco, que no
// cuelga de la raíz. Un fichero que no existe es un *Error 404, como en PorRuta.
type DrivePathResolver interface {
	FileByPath(context.Context, string) (File, error)
}

// PDFLister elige lo que lista la portada de CertiPDF ([PDF.Lista]): en local, los PDF
// abiertos hace poco, estén donde estén, además de los de la carpeta «PDF».
type PDFLister interface {
	ListPDFs(context.Context) ([]File, error)
}

// PDFAuthorNamer da el nombre con que se firman las respuestas y los estados de las
// anotaciones. Sin él, firma el sujeto del alcance (el identificador de la cuenta).
type PDFAuthorNamer interface {
	PDFAuthorName() string
}

func (p *PDF) pathResolver() (DrivePathResolver, bool) {
	if p == nil || p.drive == nil || p.drive.data == nil {
		return nil, false
	}
	r, ok := p.drive.data.(DrivePathResolver)
	return r, ok
}

func (p *PDF) pdfLister() (PDFLister, bool) {
	if p == nil || p.drive == nil || p.drive.data == nil {
		return nil, false
	}
	l, ok := p.drive.data.(PDFLister)
	return l, ok
}

// annotationAuthor es quien firma en las anotaciones locales.
func (p *PDF) annotationAuthor() string {
	if n, ok := p.drive.data.(PDFAuthorNamer); ok {
		if nombre := n.PDFAuthorName(); nombre != "" {
			return nombre
		}
	}
	return p.drive.data.PrivateStorageScope().Subject
}
