// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Las barras de 24 y las fichas que caben enteras: el arreglo de la web del 2026-11-25
// (`barras-y-fichas.test.tsx`), aquí.

// TestLasFichasQueCabenSonLasDeLaWeb son los casos de `fichasQueCaben`: a 1280×800 una
// semana mide 107,8; con una fila de barras, sus 24 px; en una ventana de 1920×1080, unas
// 155 por semana.
func TestLasFichasQueCabenSonLasDeLaWeb(t *testing.T) {
	for _, c := range []struct {
		alto          float32
		filas         int
		todas, conMas int
	}{
		{107.8, 0, 2, 2}, // sin barras caben dos; con tres, dos y «+1 más»
		{107.8, 1, 1, 1}, // con una fila de barras, una entera y «+1 más», no dos cortadas
		{155, 0, 4, 3},   // en una ventana alta caben más
		{40, 2, 0, 0},    // nunca menos de cero
	} {
		todas, conMas := fichas(c.alto, c.filas)
		if todas != c.todas || conMas != c.conMas {
			t.Errorf("fichas(%v, %d) = %d, %d; la web da %d, %d", c.alto, c.filas, todas, conMas, c.todas, c.conMas)
		}
	}
}

// TestLasBarrasYLaFichaSolaDelMes: las barras de varios días miden 24 (como las
// píldoras, el objetivo de WCAG 2.5.8) a 26 de paso, en el mes y en la franja «todo el
// día» de la semana; y las píldoras de un día van en dos líneas si caben así en vez de
// cortarse, con la hora y el título en el mismo párrafo.
//
// VISTO EN ROJO con las barras de 18 a 22 de paso y la píldora de 20 en una línea.
func TestLasBarrasYLaFichaSolaDelMes(t *testing.T) {
	p := pantallaDeMuestra(t)
	dia := func(d, h int) time.Time { return time.Date(2026, 9, d, h, 0, 0, 0, time.UTC) }
	evento := func(id, titulo string, ini, fin time.Time, todo bool) Ocurrencia {
		return Ocurrencia{Instance: api.Instance{
			Event: api.Event{ID: id, Summary: titulo, AllDay: todo, DTStart: ini}, Start: ini, End: fin,
		}, Color: AcentoPorDefecto}
	}
	// Dos barras apiladas en la semana del 7 (sin píldoras), una barra y una píldora en
	// la del 14 y una píldora sola en la del 31 de agosto.
	p.Eventos = []Ocurrencia{
		evento("congreso", "Congreso", dia(7, 0), dia(10, 0), true),
		evento("feria", "Feria", dia(8, 0), dia(10, 0), true),
		evento("seminario", "Seminario", dia(14, 0), dia(17, 0), true),
		evento("revision", "Revisión", dia(16, 9), dia(16, 10), false),
		evento("sola", "Reunión semanal del equipo de producto", dia(2, 8), dia(2, 9), false),
	}
	vista := p.vista()
	w := test.NewWindow(vista)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(anchoCont, altoUtil-altoCab))
	m := vista.(*container.ThemeOverride).Content.(*container.Scroll).Content.(*Mes)
	r := test.WidgetRenderer(m).(*mesRenderer)
	var ys []float32
	for w, bs := range r.barras {
		for _, b := range bs {
			if b.Size().Height != 24 {
				t.Errorf("la barra «%s» mide %v de alto; tocaban 24", b.rotulo, b.Size().Height)
			}
			if w == 1 { // la semana del 7
				ys = append(ys, b.Position().Y)
			}
		}
	}
	if len(ys) != 2 || (ys[1]-ys[0] != 26 && ys[0]-ys[1] != 26) {
		t.Errorf("dos barras apiladas en %v: tocaban 26 de paso", ys)
	}
	if PasoBarra != 26 || AltoBarra != 24 {
		t.Errorf("la franja «todo el día» apila barras de %v a %v; tocaban 24 a 26", AltoBarra, PasoBarra)
	}
	pildoraDel := func(d int) (*pildora, *celdaMes) {
		for _, c := range r.celdas {
			if c.dia.Month() == time.September && c.dia.Day() == d {
				cr := test.WidgetRenderer(c).(*celdaRenderer)
				if len(cr.pildoras) != 1 {
					t.Fatalf("el %d pinta %d píldoras", d, len(cr.pildoras))
				}
				return cr.pildoras[0], c
			}
		}
		t.Fatalf("no está el %d", d)
		return nil, nil
	}
	sola, celda := pildoraDel(2)
	pr := test.WidgetRenderer(sola).(*pildoraRenderer)
	if !sola.dosLineas || sola.Size().Height != AltoPildoraDosLineas || !pr.resto.Visible() || pr.resto.Text == "" {
		t.Errorf("la píldora sola mide %v (dos líneas: %v, segunda «%s»)", sola.Size().Height, sola.dosLineas, pr.resto.Text)
	}
	if fin := sola.Position().Y + sola.Size().Height; fin > celda.Size().Height-RellenoCelda {
		t.Errorf("la píldora en dos líneas acaba en %v y la celda en %v", fin, celda.Size().Height)
	}
	if pr.resto.Position().X != pr.hora.Position().X {
		t.Errorf("la segunda línea empieza en %v y la hora en %v", pr.resto.Position().X, pr.hora.Position().X)
	}
	// Con una barra en su semana también va en dos líneas si cabe bajo ella (desde el
	// 2026-11-25, como `fichasEnDosLineas` de la web), sin salirse de su celda.
	conBarra, celdaBarra := pildoraDel(16)
	if !conBarra.dosLineas || conBarra.Size().Height != AltoPildoraDosLineas {
		t.Errorf("con una barra en la semana la píldora mide %v (dos líneas: %v)", conBarra.Size().Height, conBarra.dosLineas)
	}
	if fin := conBarra.Position().Y + conBarra.Size().Height; fin > celdaBarra.Size().Height-RellenoCelda {
		t.Errorf("la píldora bajo la barra acaba en %v y la celda en %v", fin, celdaBarra.Size().Height)
	}
}
