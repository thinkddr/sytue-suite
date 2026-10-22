// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLasBarrasDelMesCruzanLasCeldas: un evento de día completo del 8 al 10 de
// septiembre es UNA barra que cruza martes, miércoles y jueves en el carril de su
// semana —a 30 px del borde de arriba de la semana—, no una píldora en el día 8. Y la
// celda del 8 le deja sitio: sus píldoras bajan una fila de barra.
func TestLasBarrasDelMesCruzanLasCeldas(t *testing.T) {
	p := pantallaDeMuestra(t)
	tinta := colorDe("#6b3e9e")
	o := ocurrencia("v", "Vacaciones", time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC), true)
	o.Color = tinta
	p.Eventos = append(p.Eventos, o)
	m := NuevoMes(p.locale(), p.Cursor)
	m.Zona, m.Eventos = time.UTC, p.Eventos
	d := m.disponer()
	if len(d.semanas[1]) != 1 || d.semanas[1][0].Col != 1 || d.semanas[1][0].Ancho != 3 {
		t.Fatalf("la barra no cruza martes a jueves de la segunda semana: %+v", d.semanas[1])
	}
	for i, c := range d.chips {
		for _, x := range c {
			if x.Event.ID == "v" {
				t.Fatalf("el evento de varios días sale además como píldora en el día %d", i)
			}
		}
	}

	img := pintar(t, p)
	alto := AltoSemana(altoUtil - altoCab)
	y := int(altoCab + AltoDiasSemana + tema.Borde + alto + CarrilBarras + AltoBarra/2)
	col := Cuadricula.AnchoColumna(anchoCont)
	for _, dia := range []float32{1.5, 2.5, 3.5} { // martes, miércoles y jueves
		x := int(tema.AnchoLateral + dia*col)
		if got := px(img, x, y); !mismoColor(got, tinta) {
			t.Errorf("en (%d,%d), dentro de la barra, se pinta %v y el evento es %v", x, y, got, tinta)
		}
	}
	// El lunes 7 no está en la barra.
	if got := px(img, int(tema.AnchoLateral+0.5*col), y); mismoColor(got, tinta) {
		t.Error("la barra se sale al lunes")
	}
	guardar(t, img, "calendario-mes-barras")
}
