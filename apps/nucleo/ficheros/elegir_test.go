// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ficheros

import (
	"slices"
	"testing"
)

// TestLasRutasSonUnaPorLinea: zenity, kdialog, osascript y PowerShell dan una ruta por
// línea; una ruta con espacios es UNA ruta, y el final de línea de Windows no se cuela.
func TestLasRutasSonUnaPorLinea(t *testing.T) {
	got := rutasDe("/home/ana/Fotos/playa uno.jpg\r\n/home/ana/b.png\n\n")
	if quiero := []string{"/home/ana/Fotos/playa uno.jpg", "/home/ana/b.png"}; !slices.Equal(got, quiero) {
		t.Errorf("rutasDe dio %q y tocaba %q", got, quiero)
	}
	if rutasDe("") != nil {
		t.Error("una salida vacía (cancelar) tiene que dar ninguna ruta")
	}
}

// TestUnNombreConSaltoDeLineaNoEligeOtroFichero (FYS-01): «informe.pdf⏎.netrc» no
// devuelve «.netrc», relativo al directorio de trabajo.
func TestUnNombreConSaltoDeLineaNoEligeOtroFichero(t *testing.T) {
	for _, got := range [][]string{
		rutasDe("/home/ana/Descargas/informe.pdf\n.ssh/id_ed25519\n"),
		lineas([]byte("/home/ana/Descargas/informe.pdf\n.netrc\n")),
	} {
		if len(got) != 1 || got[0] != "/home/ana/Descargas/informe.pdf" {
			t.Errorf("rutas: %q", got)
		}
	}
}
