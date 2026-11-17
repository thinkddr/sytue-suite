// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// La hoja con el dedo. Con el ratón, arrastrar sobre la rejilla marca el rango; en un
// teléfono eso dejaba la hoja sin desplazar, porque el dedo no tiene rueda. Como en la web
// (donde lo hace el navegador) y en las hojas de Android e iOS, con el dedo arrastrar
// desplaza, y el rango se marca con los dos tiradores circulares de sus esquinas
// (`tiradores.ts` en la web). El asa de autorrelleno no se pinta: con el dedo no se coge
// (llega por MouseDown, que solo da el escritorio) y se confundiría con el tirador de abajo.

// esMovil es [ui.EsTactil]; variable para que la prueba lo sea, como en Docus.
var esMovil = ui.EsTactil

// Medidas de un tirador: el círculo que se ve y la zona donde lo coge un dedo, que es
// mayor (2 × 22 son los 44 px de objetivo táctil). Las de la web y las de Docus.
const (
	RadioTirador float32 = 7
	ZonaTirador  float32 = 22
)

// Qué lleva el dedo: el tirador de arriba a la izquierda, el de abajo a la derecha, o
// ninguno (el arrastre desplaza la hoja). tiradorSinDecidir es antes del primer movimiento.
const (
	tiradorSinDecidir = iota
	tiradorInicio
	tiradorFin
	tiradorNinguno
)

// esquinasRango es el rectángulo que se ve seleccionado: el rango o, con una celda
// combinada sola, la combinada entera.
func (r *Rejilla) esquinasRango() (c1, f1, c2, f2 int) {
	c1, f1, c2, f2 = r.Rango()
	if u, ok := r.L.FusionEn(r.sel.C, r.sel.F); ok && r.ext == nil {
		c1, f1, c2, f2 = u.C1, u.F1, u.C2, u.F2
	}
	return c1, f1, c2, f2
}

// tiradores son los centros de los dos tiradores, en px de la rejilla: las esquinas del
// rango. Solo con el dedo, fuera de la edición y si la esquina se ve (no ha pasado bajo
// las cabeceras al desplazar). Con una celda sola también están: es como se empieza un
// rango sin ratón.
func (r *Rejilla) tiradores() (ini, fin fyne.Position, verIni, verFin bool) {
	if !esMovil() || r.editando {
		return ini, fin, false, false
	}
	vw := r.vista()
	c1, f1, c2, f2 := r.esquinasRango()
	ini = fyne.NewPos(vw.x(c1), vw.y(f1))
	fin = fyne.NewPos(vw.x(c2+1), vw.y(f2)+vw.alto(f2))
	// Un píxel hacia dentro, la esquina es de SU celda y no de la vecina o la cabecera.
	enCelda := func(p fyne.Position) bool { z, _ := r.zonaEn(p); return z == zonaCelda }
	return ini, fin, enCelda(ini.AddXY(1, 1)), enCelda(fin.SubtractXY(1, 1))
}

// tiradorEn dice qué tirador hay en p (px de la rejilla). El de abajo gana si se tocan,
// que es el que se suele agrandar.
func (r *Rejilla) tiradorEn(p fyne.Position) int {
	ini, fin, verIni, verFin := r.tiradores()
	cerca := func(c fyne.Position) bool {
		dx, dy := p.X-c.X, p.Y-c.Y
		return dx*dx+dy*dy <= ZonaTirador*ZonaTirador
	}
	switch {
	case verFin && cerca(fin):
		return tiradorFin
	case verIni && cerca(ini):
		return tiradorInicio
	}
	return tiradorNinguno
}

// arrastrarTactil es Dragged con el dedo. Devuelve el evento que sigue el camino del
// rango (el del ratón, con su autodesplazamiento en el borde), o nil si el gesto era
// desplazar y ya está hecho.
//
// Al coger un tirador, la esquina CONTRARIA pasa a ser la celda ancla y el arrastre es el
// de celdas: así cruzar el tirador al otro lado da la vuelta al rango, como en la web
// (`destinoTirador`). El dedo no coge el círculo por el centro: lo que había entre el dedo
// y la esquina al empezar (`agarre`) se suma a cada punto, y un poco hacia dentro del
// rango, para que la esquina caiga en su celda.
func (r *Rejilla) arrastrarTactil(e *fyne.DragEvent) *fyne.DragEvent {
	if r.tirador == tiradorSinDecidir {
		inicio := e.Position.Subtract(e.Dragged)
		r.tirador = r.tiradorEn(inicio)
		if r.tirador != tiradorNinguno {
			ini, fin, _, _ := r.tiradores()
			c1, f1, c2, f2 := r.esquinasRango()
			esquina := fin
			r.sel, r.ext = Celda{C: c1, F: f1}, &Celda{C: c2, F: f2}
			if r.tirador == tiradorInicio {
				esquina = ini
				r.sel, r.ext = Celda{C: c2, F: f2}, &Celda{C: c1, F: f1}
			}
			r.agarre = esquina.Subtract(inicio)
			r.arrastre = zonaCelda
		}
	}
	if r.tirador == tiradorNinguno {
		r.Desplazar(-e.Dragged.DX, -e.Dragged.DY)
		return nil
	}
	ajuste := r.agarre.AddXY(1, 1)
	if r.tirador == tiradorFin {
		ajuste = r.agarre.SubtractXY(1, 1)
	}
	out := &fyne.DragEvent{PointEvent: fyne.PointEvent{Position: e.Position.Add(ajuste)}, Dragged: e.Dragged}
	if !e.AbsolutePosition.IsZero() {
		out.AbsolutePosition = e.AbsolutePosition.Add(ajuste)
	}
	return out
}

// recogerEnFusion: el tirador de arriba deja de ancla la esquina de abajo del rango, que en
// una combinada es una celda CUBIERTA. Si el rango vuelve a no salir de esa combinada, la
// selección es la combinada sola, por su ancla: con la cubierta de celda activa, lo que se
// escribiera iría a una celda que no se ve.
func (r *Rejilla) recogerEnFusion() {
	u, ok := r.L.FusionEn(r.sel.C, r.sel.F)
	if !ok || (r.sel.C == u.C1 && r.sel.F == u.F1) {
		return
	}
	if c1, f1, c2, f2 := r.Rango(); c1 >= u.C1 && f1 >= u.F1 && c2 <= u.C2 && f2 <= u.F2 {
		r.sel, r.ext = Celda{C: u.C1, F: u.F1}, nil
	}
}

// pintarTiradores pone los dos círculos en `primary-500` con borde de papel, sobre las
// celdas y bajo las cabeceras: van pegados a las celdas y se esconden con ellas.
func (rr *rejillaRenderer) pintarTiradores(v tema.Variante) {
	ini, fin, verIni, verFin := rr.r.tiradores()
	for i, c := range []fyne.Position{ini, fin} {
		t := rr.tiradores[i]
		if ver := []bool{verIni, verFin}[i]; !ver {
			t.Hide()
			continue
		}
		t.FillColor, t.StrokeColor, t.StrokeWidth = tema.Color("primary-500", v), Papel, 1.5
		t.Move(fyne.NewPos(c.X-RadioTirador, c.Y-RadioTirador))
		t.Resize(fyne.NewSize(2*RadioTirador, 2*RadioTirador))
		t.Show()
		t.Refresh()
	}
}

// nuevosTiradores son los dos círculos, escondidos hasta que haya dedo.
func nuevosTiradores() [2]*canvas.Circle {
	var t [2]*canvas.Circle
	for i := range t {
		t[i] = canvas.NewCircle(Papel)
		t[i].Hide()
	}
	return t
}
