// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package peticion

import (
	"runtime"
	"testing"
	"weak"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

func opcionesEnVivo(alConceder func()) Opciones {
	return Opciones{
		SinPermiso: true, Concedido: "calendar.access.granted", Recurso: "c1",
		AlConceder: alConceder, Recomponer: func() {},
		T: func(string, i18n.Vars) string { return "" },
	}
}

// TestLaEscuchaDelHubNoRetieneLaPantalla: mientras la pantalla vive, el aviso de acceso
// concedido le llega; cuando se va, la escucha del Hub —que es de la app entera— no la
// mantiene viva. VISTO EN ROJO con la escucha guardando las Opciones directamente: la
// pantalla seguía viva al acabar.
func TestLaEscuchaDelHubNoRetieneLaPantalla(t *testing.T) {
	test.NewTempApp(t)
	concedido := make(chan struct{}, 1)
	e := &Estado{}
	Enganchar(e, api.Peticiones{}, opcionesEnVivo(func() { concedido <- struct{}{} }))
	ui.RepartirAviso(api.HubNotification{Type: "calendar.access.granted", Params: map[string]string{"resource": "c1"}})
	fyne.DoAndWait(func() {})
	select {
	case <-concedido:
	default:
		t.Fatal("con la pantalla viva, el acceso concedido no le llega")
	}

	pantalla := func() weak.Pointer[struct{ e *Estado }] {
		p := &struct{ e *Estado }{e: &Estado{}}
		Enganchar(p.e, api.Peticiones{}, opcionesEnVivo(func() { _ = p }))
		return weak.Make(p)
	}()
	for range 3 {
		runtime.GC()
	}
	if pantalla.Value() != nil {
		t.Error("la escucha del Hub mantiene viva una pantalla que ya no existe")
	}
	e.Soltar()
}
