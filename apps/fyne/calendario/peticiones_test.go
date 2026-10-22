// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// franjasDe son las franjas de solo lectura que la pantalla pone sobre la vista.
func franjasDe(p *Pantalla) []*ui.FranjaSoloLectura {
	var out []*ui.FranjaSoloLectura
	var mirar func(o fyne.CanvasObject)
	mirar = func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *ui.FranjaSoloLectura:
			out = append(out, v)
		case *fyne.Container:
			for _, h := range v.Objects {
				mirar(h)
			}
		}
	}
	mirar(p.conFranjas(canvas.NewRectangle(nil)))
	return out
}

// La franja del calendario compartido de lector (el «Equipo» del servicio falso) ofrece
// pedir acceso de edición; y cuando el Hub dice que se ha concedido ESE calendario, la
// app vuelve a leer sus calendarios sin que nadie recargue. Uno de otro calendario, no.
func TestElCompartidoDeLectorPideAccesoYSeDesbloqueaEnVivo(t *testing.T) {
	s := &servicioFalso{}
	a := appDePrueba(t, s)
	a.Arrancar()
	fs := franjasDe(a.P)
	if len(fs) != 1 || fs[0].Accion == nil {
		t.Fatalf("el calendario de lector no tiene su franja con «Pedir acceso de edición»: %d franjas", len(fs))
	}
	antes := func() int { s.mu.Lock(); defer s.mu.Unlock(); return s.listados }
	n := antes()
	ui.RepartirAviso(api.HubNotification{ID: "x", Type: "calendar.access.granted", Params: map[string]string{"resource": "otro"}})
	fyne.DoAndWait(func() {})
	if antes() != n {
		t.Error("un aviso de OTRO calendario recargó este")
	}
	ui.RepartirAviso(api.HubNotification{ID: "y", Type: "calendar.access.granted", Params: map[string]string{"resource": "suyo"}})
	fyne.DoAndWait(func() {})
	if antes() != n+1 {
		t.Errorf("concedido «suyo», la app no volvió a leer los calendarios (%d → %d)", n, antes())
	}
}
