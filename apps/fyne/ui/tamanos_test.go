// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestLosTamanosSeEscribenConFormatBytes: un tamaño de fichero se escribe con
// `i18n.FormatBytes`, que es el `formatBytes` de @sytue/ui, y un resultado del Hub se
// lee con `DetalleHit`. El 2026-09-28 el Hub enseñaba «1.0 KiB» en «Reciente» —el
// fragmento del servidor, en inglés— y quedaban seis formateadores propios (Rooms ×2,
// Mail, Notes, Descargas, CertiPDF) con tres criterios de decimales y el punto inglés.
//
// Se mira el CÓDIGO FUENTE porque el fallo se comete al escribir: una tabla de unidades
// hecha a mano pasa todas las pruebas de su app. Las excepciones se comparan por
// IGUALDAD, así que también falla cuando una sobra.
//
// VISTO EN ROJO devolviendo `it.Snippet` en `lanzador.go` y con el `TamanoLegible` de
// Rooms restaurado.
func TestLosTamanosSeEscribenConFormatBytes(t *testing.T) {
	unidad := regexp.MustCompile(`\b[kKMGT]i?B"|"kb"`)
	fragmento := regexp.MustCompile(`\.Snippet\b`)
	// Quién puede leer `Snippet` y por qué.
	excepciones := map[string]string{
		"ui/barrabuscador.go":         "`DetalleHit` es quien decide entre el tamaño y el fragmento",
		"apps/notes/paneles.go":       "es un `NoteHit` de Notes: su fragmento es texto de la nota, no un tamaño",
		"apps/mail/correo_externo.go": "es un `ExternalMailMessage`: su fragmento es una vista previa de texto del correo, no un tamaño ni un `HubHit`",
	}
	var malos []string
	vistos := map[string]bool{}
	err := filepath.WalkDir("..", func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(strings.TrimPrefix(ruta, "../"))
		if d.IsDir() {
			if rel == "upstream" || (ruta != ".." && strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		fuente, err := os.ReadFile(ruta) //nolint:gosec // G304: ficheros del propio módulo
		if err != nil {
			return err
		}
		for n, linea := range strings.Split(string(fuente), "\n") {
			if strings.HasPrefix(strings.TrimSpace(linea), "//") {
				continue
			}
			if unidad.MatchString(linea) {
				malos = append(malos, rel+":"+strconv.Itoa(n+1)+": unidad a mano, usa i18n.FormatBytes")
			}
			if fragmento.MatchString(linea) {
				if _, ok := excepciones[rel]; !ok {
					malos = append(malos, rel+":"+strconv.Itoa(n+1)+": `Snippet` sin pasar por ui.DetalleHit")
				}
				vistos[rel] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for rel := range excepciones {
		if !vistos[rel] {
			malos = append(malos, rel+": ya no lee `Snippet`, quita su excepción")
		}
	}
	slices.Sort(malos)
	for _, m := range malos {
		t.Error(m)
	}
}
