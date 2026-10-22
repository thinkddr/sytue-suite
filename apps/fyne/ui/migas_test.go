// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLaMigaMide26Coma25 es el número medido en `drive/archivos.ancho.json`: el botón
// «Mi unidad» del `nav[aria-label="Ruta"]` mide 26,25 de alto con `3.5px 7px` de
// relleno y radio 6.
func TestLaMigaMide26Coma25(t *testing.T) {
	m := NuevaMiga("Mi unidad", nil)
	m.Actual = true
	d := MedidaMiga(m)
	if d.Height != 30 {
		t.Errorf("la miga mide %v de alto y en producción mide 26,25", d.Height)
	}
	if got := m.MinSize(); got != d {
		t.Errorf("MinSize() = %v y MedidaMiga dice %v", got, d)
	}
	// El relleno horizontal es `px-2`, o sea 7 por lado: 14 más que el rótulo.
	if got, quiero := d.Width-Medir("Mi unidad", tema.TextoBase, SemiNegrita).Width, 2*tema.Espacio(2); got != quiero {
		t.Errorf("la miga añade %v de relleno y `px-2` con el rem de 14 son %v", got, quiero)
	}
}

// TestElUltimoSegmentoVaEnSemiNegrita: `aria-current="page"` lleva `font-semibold`
// (600) y los demás `font-medium` (500). En producción se mide `12.25px/19.25px 600`
// en el actual.
//
// No es cosmética: es lo único que dice en qué carpeta estás cuando la ruta es larga.
func TestElUltimoSegmentoVaEnSemiNegrita(t *testing.T) {
	m := NuevasMigas("Ruta", []string{"Mi unidad", "Contratos", "2026"}, nil)
	_ = test.WidgetRenderer(m)
	for i, s := range m.segmentos {
		ultimo := i == len(m.segmentos)-1
		if s.Actual != ultimo {
			t.Errorf("el segmento %d dice Actual=%v y el actual es el último", i, s.Actual)
		}
		quiero := Medio
		if ultimo {
			quiero = SemiNegrita
		}
		if s.peso() != quiero {
			t.Errorf("el segmento %d va con el peso %v y tenía que ir con %v", i, s.peso(), quiero)
		}
	}
}

// TestLosGalonesSeparanLosSegmentos es el `gap-0.5` medido a los dos lados del galón
// de 14: entre dos rótulos hay 1,75 + 14 + 1,75.
func TestLosGalonesSeparanLosSegmentos(t *testing.T) {
	m := NuevasMigas("Ruta", []string{"Mi unidad", "Contratos"}, nil)
	d := MedidaMigas(m)
	m.Resize(d)
	_ = test.WidgetRenderer(m)
	m.Refresh()
	pinta(t, d.Width+10, d.Height+10, m)

	a, b := m.segmentos[0], m.segmentos[1]
	hueco := b.Position().X - (a.Position().X + a.Size().Width)
	if quiero := 2*tema.Espacio(0.5) + LadoGalonMiga; hueco != quiero {
		t.Errorf("entre dos segmentos hay %v y el galón con sus dos `gap-0.5` son %v", hueco, quiero)
	}
	if d.Height != 30 {
		t.Errorf("la ruta mide %v de alto y su segmento mide 26,25", d.Height)
	}
}

// TestLaRutaLargaSeEncogeEnVezDeDesbordar: la web les pone `min-w-0` y `truncate`, así
// que una ruta de diez carpetas se estrecha dentro de su hueco en vez de empujar la
// cabecera. Los galones no encogen (`shrink-0`).
func TestLaRutaLargaSeEncogeEnVezDeDesbordar(t *testing.T) {
	ruta := []string{"Mi unidad", "Documentación de producto", "Contratos firmados", "2026"}
	m := NuevasMigas("Ruta", ruta, nil)
	entero := MedidaMigas(m).Width
	estrecho := entero / 2
	m.Resize(fyne.NewSize(estrecho, AltoMiga))
	_ = test.WidgetRenderer(m)
	m.Refresh()
	pinta(t, entero+10, 40, m)

	ultimo := m.segmentos[len(m.segmentos)-1]
	if fin := ultimo.Position().X + ultimo.Size().Width; fin > estrecho+0.5 {
		t.Errorf("la ruta acaba en %v y solo tenía %v de sitio: se ha desbordado", fin, estrecho)
	}
	for i, g := range m.galones {
		if got := g.Objeto().Size().Width; got != LadoGalonMiga {
			t.Errorf("el galón %d mide %v y no tiene que encoger: son 14", i, got)
		}
	}
}

// TestLaMigaLlevaANingunSitioSiNoLeDanADonde comprueba el cableado del índice, que es
// donde se cuela el error clásico de este componente: todas las migas llevando a la
// última carpeta porque la variable del bucle se captura una sola vez.
func TestLaMigaLlevaANingunSitioSiNoLeDanADonde(t *testing.T) {
	visitado := -1
	m := NuevasMigas("Ruta", []string{"Mi unidad", "Contratos", "2026"}, func(i int) { visitado = i })
	_ = test.WidgetRenderer(m)
	test.Tap(m.segmentos[1])
	if visitado != 1 {
		t.Errorf("se ha pulsado el segundo segmento y ha llevado al %d", visitado)
	}
	test.Tap(m.segmentos[0])
	if visitado != 0 {
		t.Errorf("se ha pulsado el primero y ha llevado al %d", visitado)
	}
}

// TestLasMigasDeLaBarraSonDeTextoPequeno: la ruta de «Compartido conmigo» es `text-sm`
// con el mismo `py-1`, así que cada segmento mide 15,75 + 7 = 22,75 y no 26,25; y la del
// diálogo «Mover a…» mide lo del cromo. Lo que cambia entre las tres es el cuerpo, el
// fondo al pasar por encima y el galón, y nada más.
func TestLasMigasDeLaBarraSonDeTextoPequeno(t *testing.T) {
	barra := NuevasMigas("Ruta", []string{"Proyecto", "Planos"}, nil)
	barra.Estilo = MigasEnBarra
	if got := MedidaMigas(barra).Height; got != tema.TextoSM.Linea+2*tema.Espacio(1) {
		t.Errorf("la ruta de la barra mide %v de alto", got)
	}
	dialogo := NuevasMigas("Ruta", []string{"Mi unidad"}, nil)
	dialogo.Estilo = MigasEnDialogo
	if got := MedidaMigas(dialogo).Height; got != AltoMiga {
		t.Errorf("la ruta del diálogo mide %v y es la del cromo, %v", got, AltoMiga)
	}
	if MigasEnBarra.escalon() == (EstiloMigas{}).escalon() {
		t.Error("la barra y el cromo usan el mismo cuerpo de letra")
	}
	w := test.NewWindow(barra)
	defer w.Close()
	w.Resize(fyne.NewSize(300, 30))
	w.Canvas().Capture()
}
