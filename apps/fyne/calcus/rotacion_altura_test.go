// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Son alturas ÚTILES tras el área segura, incluida la barra de suite real. Una
// rejilla de alto positivo puede enseñar solo el encabezado: hay que ver A1 y A2.
func TestLaRotacionDeCalcusConPocaAlturaEnseñaDosFilasYLasCabeceras(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	s := &servicioHojas{rol: "editor", lamport: 4, hoja: `{"active":0,"sheets":[{"name":"Datos","cells":{"A1":"10","A2":"=A1*2"}}]}`}
	a := appDePrueba(t, s)
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		t.Fatal(err)
	}
	a.T = tr.T
	barra := ui.NuevaBarraSuite(tr, nil, iconos.Sheets)
	t.Cleanup(barra.Parar)
	w := test.NewTempWindow(t, barra.Montar(a.Objeto()))
	w.SetPadded(false)
	a.Arrancar()
	a.AbrirID("h1", "Cuentas")
	lz := a.Abierto()
	for _, tam := range []fyne.Size{{Width: 390, Height: 763}, {Width: 568, Height: 272}, {Width: 756, Height: 299}, {Width: 756, Height: 369}, {Width: 1024, Height: 748}, {Width: 568, Height: 272}} {
		w.Resize(tam)
		if tam.Width <= tam.Height || tam.Height >= 400 {
			continue
		}
		vista := lz.Rejilla.vista()
		finSegundaFila := vista.y(1) + vista.alto()
		if vista.y(0) < AltoCelda || finSegundaFila > lz.Rejilla.Size().Height {
			t.Fatalf("%v: cabecera y dos filas necesitan %g px; rejilla=%v", tam, finSegundaFila, lz.Rejilla.Size())
		}
		if lz.Libro.Texto(0, 0) != "10" || lz.Libro.Texto(0, 1) != "20" {
			t.Fatal("girar perdió las celdas o su fórmula")
		}
		if dir := os.Getenv("SYTUE_ROTACION_CAPTURAS"); dir != "" {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				t.Fatal(err)
			}
			t.Setenv("SYTUE_CAPTURA", filepath.Join(dir, fmt.Sprintf("native-rotacion-calcus-area-segura-%gx%g.png", tam.Width, tam.Height)))
			guarda(t, w.Canvas().Capture())
		}
	}
}
