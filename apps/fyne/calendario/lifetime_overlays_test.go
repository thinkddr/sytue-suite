// SPDX-License-Identifier: Apache-2.0
package calendario

import (
	"context"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestRetirarIdentidadCierraCapasRealesDeCalendar(t *testing.T) {
	for _, caso := range []struct {
		nombre string
		abrir  func(*testing.T, *App, fyne.Canvas)
		minimo int
	}{
		{"menu", func(_ *testing.T, a *App, _ fyne.Canvas) {
			a.menuCalendario(api.Calendar{ID: "personal"}, a.raiz)
		}, 1},
		{"busqueda", func(_ *testing.T, a *App, _ fyne.Canvas) { a.abrirBusqueda() }, 1},
		{"selector anidado", func(t *testing.T, a *App, c fyne.Canvas) {
			a.crearEn(a.P.Cursor)
			var selector *ui.Selector
			enCapas(c, func(o fyne.CanvasObject) {
				if s, ok := o.(*ui.Selector); ok && len(s.Opciones) > 0 && selector == nil {
					selector = s
				}
			})
			if selector == nil {
				t.Fatal("el formulario no tiene selector")
			}
			selector.Tapped(nil)
		}, 2},
		{"fecha anidada", func(t *testing.T, a *App, c fyne.Canvas) {
			a.crearEn(a.P.Cursor)
			var boton *ui.BotonIcono
			enCapas(c, func(o fyne.CanvasObject) {
				if b, ok := o.(*ui.BotonIcono); ok && b.Rotulo == a.P.t("event.start", nil) {
					boton = b
				}
			})
			if boton == nil {
				t.Fatal("el formulario no tiene selector de fecha inicial")
			}
			boton.Tapped(nil)
		}, 2},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			p := &Pantalla{
				T: traductor(t), Cursor: time.Date(2031, 3, 4, 10, 0, 0, 0, time.UTC), Zona: time.UTC,
				Calendarios: []api.Calendar{{ID: "personal", DisplayName: "Personal"}},
			}
			a := NuevaAppConVida(context.Background(), p, nil)
			w := test.NewWindow(a.Objeto())
			t.Cleanup(w.Close)
			t.Cleanup(a.Cerrar)
			w.Resize(fyne.NewSize(1280, 800))
			caso.abrir(t, a, w.Canvas())
			if n := len(w.Canvas().Overlays().List()); n < caso.minimo {
				t.Fatalf("el escenario no abrió las capas esperadas: %d", n)
			}
			a.Cerrar()
			if n := len(w.Canvas().Overlays().List()); n != 0 || w.Canvas().Focused() != nil || len(a.dialogos) != 0 {
				t.Fatalf("la identidad retirada conserva %d capas o foco", n)
			}
		})
	}
}

func TestRetiroNoDescartaLaNuevaIdentidadNiOtrasVentanas(t *testing.T) {
	p := &Pantalla{T: traductor(t), Cursor: time.Now(), Zona: time.UTC}
	a := NuevaAppConVida(context.Background(), p, nil)
	// La otra ventana se abre ANTES: el driver de pruebas de Fyne resuelve
	// CanvasForObject con la última ventana creada, sea cual sea el objeto, y así
	// esa última es la del presentador, como la resolvería el driver real.
	otra := test.NewWindow(container.NewWithoutLayout())
	t.Cleanup(otra.Close)
	w := test.NewWindow(a.Objeto())
	t.Cleanup(w.Close)
	noDescartable := ui.NuevoDialogo("identidad anterior", container.NewWithoutLayout())
	noDescartable.NoDescartable = true
	descartado := false
	noDescartable.AlCerrar = func() { descartado = true; noDescartable.Cerrar() }
	a.mostrarDialogo(noDescartable, w.Canvas())
	ajeno := ui.NuevoDialogo("otra ventana", container.NewWithoutLayout())
	ajeno.Mostrar(otra.Canvas())
	a.Cerrar()
	if descartado || w.Canvas().Overlays().Top() != nil || otra.Canvas().Overlays().Top() == nil {
		t.Fatal("el retiro ejecutó una acción de usuario o afectó otra ventana")
	}
	// Cerrar must release the dialog's own layer, not merely remove it from the
	// canvas: a removed-but-still-open Dialogo silently refuses to show again.
	noDescartable.Mostrar(w.Canvas())
	if w.Canvas().Overlays().Top() == nil {
		t.Fatal("el diálogo conservó internamente su capa tras el retiro")
	}
	noDescartable.Cerrar()
	nuevo := ui.NuevoDialogo("identidad nueva", container.NewWithoutLayout())
	nuevo.Mostrar(w.Canvas())
	a.Cerrar()
	if w.Canvas().Overlays().Top() == nil {
		t.Fatal("un segundo retiro cerró la identidad ya montada")
	}
	nuevo.Cerrar()
	ajeno.Cerrar()
}
