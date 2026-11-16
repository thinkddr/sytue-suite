// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// conDedo hace de teléfono mientras dura la prueba.
func conDedo(t *testing.T) {
	t.Helper()
	antes := esMovil
	esMovil = func() bool { return true }
	t.Cleanup(func() { esMovil = antes })
}

// arrastra manda un arrastre de `desde` a `hasta` en dos tramos, como lo trocea Fyne.
func arrastra(r *Rejilla, desde, hasta fyne.Position) {
	mitad := fyne.NewPos((desde.X+hasta.X)/2, (desde.Y+hasta.Y)/2)
	r.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: mitad}, Dragged: fyne.NewDelta(mitad.X-desde.X, mitad.Y-desde.Y)})
	r.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: hasta}, Dragged: fyne.NewDelta(hasta.X-mitad.X, hasta.Y-mitad.Y)})
	r.DragEnd()
}

// TestTactilArrastrarSobreLaHojaLaDesplaza: con el dedo, arrastrar sobre una celda corre
// la vista (el dedo sube, la hoja baja) y no marca rango, que era lo que dejaba la hoja
// quieta en un teléfono.
func TestTactilArrastrarSobreLaHojaLaDesplaza(t *testing.T) {
	conDedo(t)
	r := rejillaDePrueba(600, 400)
	r.Seleccionar(1, 1, false)
	desde := fyne.NewPos(AnchoCabecera+AnchoCelda*3+30, AltoCelda*8+5)
	arrastra(r, desde, desde.SubtractXY(AnchoCelda, 4*AltoCelda))
	if r.ext != nil {
		t.Fatalf("el arrastre marcó un rango hasta %v", *r.ext)
	}
	if r.desX != AnchoCelda || r.desY != 4*AltoCelda {
		t.Fatalf("la vista quedó en (%v, %v); se esperaba (%v, %v)", r.desX, r.desY, AnchoCelda, 4*AltoCelda)
	}
	if r.sel != (Celda{C: 1, F: 1}) {
		t.Fatalf("desplazar movió la celda ancla a %v", r.sel)
	}
}

// TestTactilElTiradorDeAbajoExtiendeElRango: coger el tirador de la esquina de abajo y
// llevarlo a D6 marca B2:D6, con B2 de ancla.
func TestTactilElTiradorDeAbajoExtiendeElRango(t *testing.T) {
	conDedo(t)
	r := rejillaDePrueba(600, 400)
	r.Seleccionar(1, 1, false)
	_, fin, _, ver := r.tiradores()
	if !ver {
		t.Fatal("con el dedo y una celda seleccionada no hay tirador de abajo")
	}
	// El dedo coge el círculo un poco descentrado, y lo suelta en el centro de D6.
	desde := fin.AddXY(5, 3)
	d6 := fyne.NewPos(AnchoCabecera+AnchoCelda*3+AnchoCelda/2, AltoCelda+AltoCelda*5+AltoCelda/2)
	arrastra(r, desde, d6.AddXY(AnchoCelda/2+5, AltoCelda/2+3))
	c1, f1, c2, f2 := r.Rango()
	if c1 != 1 || f1 != 1 || c2 != 3 || f2 != 5 {
		t.Fatalf("el rango es (%d,%d)-(%d,%d); se esperaba B2:D6", c1, f1, c2, f2)
	}
	if r.sel != (Celda{C: 1, F: 1}) {
		t.Fatalf("la ancla pasó a %v", r.sel)
	}
	if r.desX != 0 || r.desY != 0 {
		t.Fatalf("llevar el tirador desplazó la vista a (%v, %v)", r.desX, r.desY)
	}
}

// TestTactilElTiradorDeArribaMueveSuEsquina: el de arriba a la izquierda deja quieta la
// esquina de abajo, que pasa a ser la ancla.
func TestTactilElTiradorDeArribaMueveSuEsquina(t *testing.T) {
	conDedo(t)
	r := rejillaDePrueba(600, 400)
	r.Seleccionar(2, 2, false)
	r.Seleccionar(3, 4, true) // C3:D5
	ini, _, ver, _ := r.tiradores()
	if !ver {
		t.Fatal("no hay tirador de arriba")
	}
	b2 := fyne.NewPos(AnchoCabecera+AnchoCelda+AnchoCelda/2, AltoCelda+AltoCelda+AltoCelda/2)
	arrastra(r, ini, b2.SubtractXY(AnchoCelda/2, AltoCelda/2))
	c1, f1, c2, f2 := r.Rango()
	if c1 != 1 || f1 != 1 || c2 != 3 || f2 != 4 {
		t.Fatalf("el rango es (%d,%d)-(%d,%d); se esperaba B2:D5", c1, f1, c2, f2)
	}
	if r.sel != (Celda{C: 3, F: 4}) {
		t.Fatalf("la ancla es %v; se esperaba D5, la esquina quieta", r.sel)
	}
}

// TestTactilElTiradorDeArribaDeUnaCombinadaVuelveASuAncla: en la combinada B2:C3 sola, el
// tirador de arriba deja de ancla C3, que está cubierta. Soltarlo dentro de la combinada
// tiene que dejar la combinada con su ancla B2, y no C3 de celda activa.
func TestTactilElTiradorDeArribaDeUnaCombinadaVuelveASuAncla(t *testing.T) {
	conDedo(t)
	r := rejillaDePrueba(600, 400)
	r.Seleccionar(1, 1, false)
	r.Seleccionar(2, 2, true)
	if _, ok := r.L.Combinar(1, 1, 2, 2, false); !ok {
		t.Fatal("no se combinó B2:C3")
	}
	r.Seleccionar(1, 1, false)
	ini, _, ver, _ := r.tiradores()
	if !ver {
		t.Fatal("no hay tirador de arriba")
	}
	b2 := fyne.NewPos(AnchoCabecera+AnchoCelda+AnchoCelda/2, AltoCelda+AltoCelda+AltoCelda/2)
	arrastra(r, ini, b2.SubtractXY(AnchoCelda/2-4, AltoCelda/2-4))
	if r.sel != (Celda{C: 1, F: 1}) || r.ext != nil {
		t.Fatalf("la selección quedó en %v hasta %v; se esperaba B2 sola", r.sel, r.ext)
	}
}

// TestTactilTocarUnTiradorNoDeshaceElRango: un toque sin arrastre sobre el tirador no
// selecciona la celda que hay debajo.
func TestTactilTocarUnTiradorNoDeshaceElRango(t *testing.T) {
	conDedo(t)
	r := rejillaDePrueba(600, 400)
	r.Seleccionar(1, 1, false)
	r.Seleccionar(2, 3, true)
	_, fin, _, _ := r.tiradores()
	r.Tapped(&fyne.PointEvent{Position: fin.AddXY(3, 3)})
	if r.ext == nil || *r.ext != (Celda{C: 2, F: 3}) {
		t.Fatalf("tocar el tirador deshizo el rango: ext = %v", r.ext)
	}
}

// TestTactilSePintanLosTiradoresYNoElAsa: con el dedo se ven los dos círculos centrados en
// las esquinas, y no el asa de autorrelleno; con el ratón, al revés.
func TestTactilSePintanLosTiradoresYNoElAsa(t *testing.T) {
	r := rejillaDePrueba(600, 400)
	r.Seleccionar(1, 1, false)
	r.Seleccionar(2, 2, true)
	rr := test.WidgetRenderer(r).(*rejillaRenderer)
	rr.Layout(r.Size())
	if rr.tiradores[0].Visible() || rr.tiradores[1].Visible() || !rr.asa.Visible() {
		t.Fatal("con el ratón tiene que verse el asa y no los tiradores")
	}
	conDedo(t)
	rr.Layout(r.Size())
	if !rr.tiradores[0].Visible() || !rr.tiradores[1].Visible() || rr.asa.Visible() {
		t.Fatal("con el dedo tienen que verse los tiradores y no el asa")
	}
	fin := rr.tiradores[1].Position().AddXY(RadioTirador, RadioTirador)
	if want := fyne.NewPos(AnchoCabecera+3*AnchoCelda, AltoCelda+3*AltoCelda); fin != want {
		t.Fatalf("el tirador de abajo está centrado en %v; la esquina de C3 es %v", fin, want)
	}
}

// TestRejillaConRatonElArrastreSigueMarcandoRango: en el escritorio no cambia nada.
func TestRejillaConRatonElArrastreSigueMarcandoRango(t *testing.T) {
	r := rejillaDePrueba(600, 400)
	b2 := fyne.NewPos(AnchoCabecera+AnchoCelda+10, AltoCelda+AltoCelda+5)
	arrastra(r, b2, b2.AddXY(2*AnchoCelda, 2*AltoCelda))
	if r.ext == nil || *r.ext != (Celda{C: 3, F: 3}) || r.desX != 0 || r.desY != 0 {
		t.Fatalf("con el ratón el arrastre tiene que marcar B2:D4 sin desplazar: ext=%v des=(%v,%v)", r.ext, r.desX, r.desY)
	}
}
