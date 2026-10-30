// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ficheros

import (
	"slices"
	"strings"
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

// TestElTituloYLaRutaNoLleganAlGuionDePowerShellComoCodigo (FYS-12): tanto el título del
// selector como la ruta temporal del portapapeles de Windows van en Base64, de modo que
// una comilla recta o tipográfica (’ ‘ ‚ ‛) nunca cierra el literal ni inyecta código.
func TestElTituloYLaRutaNoLleganAlGuionDePowerShellComoCodigo(t *testing.T) {
	peligroso := "L’app‘‚‛'; Remove-Item C:\\x; '"
	for _, guion := range []string{
		guionElegirWindows(peligroso, []string{"pdf", "docx"}),
		guionCopiarImagenWindows(`C:\Users\` + peligroso + `\AppData\Local\Temp\sytue-recorte-1.png`),
	} {
		if strings.Contains(guion, "Remove-Item") || strings.ContainsAny(guion, "’‘‚‛") {
			t.Fatalf("el texto va en claro en el guion de PowerShell: %s", guion)
		}
		if !strings.Contains(guion, "FromBase64String('") {
			t.Fatalf("el guion no decodifica desde Base64: %s", guion)
		}
	}
}
