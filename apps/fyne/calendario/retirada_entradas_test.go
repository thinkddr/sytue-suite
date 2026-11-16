// SPDX-License-Identifier: Apache-2.0
package calendario

import (
	"context"
	"net/url"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestEntradasRetiradasNoReabrenNiRepintanCalendar(t *testing.T) {
	fecha := time.Date(2031, 3, 4, 10, 0, 0, 0, time.UTC)
	p := &Pantalla{
		T: traductor(t), Cursor: fecha, Zona: time.UTC,
		Calendarios: []api.Calendar{{ID: "personal", DisplayName: "Personal"}},
	}
	a := NuevaAppConVida(context.Background(), p, nil)
	w := test.NewWindow(a.Objeto())
	t.Cleanup(w.Close)
	t.Cleanup(a.Cerrar)
	w.Resize(fyne.NewSize(1280, 800))
	entrada := &url.URL{Scheme: "sytue-calendar", RawQuery: "action=create&title=privado"}
	a.AtenderEnlace(entrada)
	if w.Canvas().Overlays().Top() == nil {
		t.Fatal("el enlace activo no abrió el formulario")
	}
	ui.Descartar(w.Canvas())

	vinculados := 0
	a.ConServiciosVinculados(func() { vinculados++ })
	accionVinculados := p.AlServiciosVinculados
	accionEvento := p.AlCrearEvento
	accionVista := p.AlCambiarVista
	a.ConImportar(nil)
	a.avisar(ui.AvisoInfo, "toast.eventCreated")
	if len(a.avisos.Avisos()) != 1 {
		t.Fatal("el escenario no generó el aviso")
	}
	antes := a.raiz.Objects[0]
	a.Cerrar()
	if len(a.avisos.Avisos()) != 0 {
		t.Fatal("el aviso de la identidad retirada sigue activo")
	}
	if a.raiz.Objects[0] != antes {
		t.Fatal("PararImportador repintó la identidad después de cancelarla")
	}
	a.AtenderEnlace(entrada)
	if a.Atajo('c') {
		t.Error("la instancia retirada aceptó un atajo")
	}
	accionEvento(fecha)
	accionVista(VistaDia)
	accionVinculados()
	a.PonerGestorRecursos(true)
	a.pintar() // También llega desde el importador y los callbacks de petición.
	a.avisar(ui.AvisoInfo, "toast.eventCreated")
	if w.Canvas().Overlays().Top() != nil || vinculados != 0 || p.Vista == VistaDia || p.PuedeGestionarRecursos ||
		a.raiz.Objects[0] != antes || len(a.avisos.Avisos()) != 0 {
		t.Fatal("una entrada tardía modificó o mostró la interfaz de la identidad retirada")
	}
}
