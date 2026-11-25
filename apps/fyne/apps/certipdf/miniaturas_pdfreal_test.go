// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLasFilasDeMiniaturasDeUnPDFRealMidenSuPagina: la auditoría del 2026-11-24 apuntó
// filas de miniatura «de 210 px para 50 de miniatura». Con un PDF real de siete páginas
// de geometrías distintas (`testdata/paginas-variadas.pdf`, hecho con reportlab: A4
// vertical y apaisado, un A4 con /Rotate 90, un tique de 80 × 300 mm, una carta sin
// texto, una banda de 4:1 y una URL sin espacios), abierto con el motor del dispositivo,
// cada fila mide lo más alto entre su miniatura —con la proporción que da el motor, ya
// girada— y el número con las tres líneas del extracto, más el `p-1.5`; en escritorio y
// en el cajón del teléfono. Medido: 103 para 90 (A4), 78 para 45 (apaisado), 252 para
// 240 (tique). No se reprodujo una fila más alta que eso.
func TestLasFilasDeMiniaturasDeUnPDFRealMidenSuPagina(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "paginas-variadas.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("el documento privado no tendría que pedir nada por HTTP")
	}))
	defer server.Close()
	pdf, err := api.NewPDF(server.URL, server.URL, nil, nil).WithPrivateDrive(&privateDocumentData{raw: raw})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := AbrirDocumento(t.Context(), pdf, "/PDF/Private.pdf")
	if err != nil {
		t.Fatal(err)
	}
	p := NuevaPantalla(traductorES(t))
	w := ventanaSinRelleno(p.Objeto())
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(1280, 800))
	listo := make(chan struct{}, 64)
	p.trasRed = func() { listo <- struct{}{} }
	p.Abrir(doc)
	esperarRed(t, listo) // el contenido
	esperarRed(t, listo) // el primer lote
	// El número y las tres líneas del extracto, con su `mt-0.5`.
	columna := tema.TextoXS.Linea + tema.Espacio(0.5) + 3*tema.Texto2XS.Linea
	for _, tam := range []fyne.Size{{Width: 1280, Height: 800}, {Width: 390, Height: 844}} {
		restaurar := ui.TactilDePrueba(tam.Width < ui.Estrecho)
		w.Resize(tam)
		p.PanelAbierto = true
		p.Recomponer()
		if len(p.panel.miniaturas) != 7 {
			t.Fatalf("a %v el panel tiene %d miniaturas de 7", tam, len(p.panel.miniaturas))
		}
		for n, f := range p.panel.miniaturas {
			pag := f.pagina
			if pag.Ancho <= 0 || pag.Alto <= 0 {
				t.Fatalf("la página %d llega sin medida: %+v", n, pag)
			}
			mini := altoMiniatura(pag)
			if want := anchoMiniatura * float32(pag.Alto/pag.Ancho); mini != want {
				t.Errorf("a %v la miniatura %d mide %v y su proporción pide %v", tam, n, mini, want)
			}
			alto := MedidaFilaMiniatura(f).Height
			if tope := max(mini, columna) + 2*rellenoMiniatura; alto > tope+0.5 {
				t.Errorf("a %v la fila %d mide %v para una miniatura de %v (tope %v)", tam, n, alto, mini, tope)
			}
			if alto < mini+2*rellenoMiniatura-0.5 {
				t.Errorf("a %v la fila %d (%v) no cabe su miniatura de %v", tam, n, alto, mini)
			}
		}
		restaurar()
	}
}
