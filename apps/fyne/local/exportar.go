// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"context"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
	"git.sytue.eu/sytue/one/apps/nucleo/local/exportar"
)

// «Exportar en formatos abiertos»: lo de cada app vive en `nucleo/local/exportar`, que
// también usa el Hub para exportarlas todas a la vez; estos son sus nombres de siempre.

// ExportarBiblioteca es el AlExportarTodo de Docus, Calcus y Diapos: cada documento de
// la biblioteca con la extensión ext (docx, xlsx, pptx en OOXML; odt, ods, odp en ODF),
// con su título por nombre. La papelera no se exporta. Es [exportar.Biblioteca].
func ExportarBiblioteca(b *biblioteca.Biblioteca, ext string) func(context.Context, string) error {
	return exportar.Biblioteca(b, ext)
}

// EscribirExportado deja datos en carpeta/nombre, sin salirse de carpeta.
func EscribirExportado(carpeta, nombre string, datos []byte) error {
	return exportar.Escribir(carpeta, nombre, datos)
}

// CopiarExportado vuelca r en carpeta/nombre y lo cierra.
func CopiarExportado(carpeta, nombre string, r io.ReadCloser) error {
	return exportar.Copiar(carpeta, nombre, r)
}
