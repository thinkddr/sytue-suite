// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestPermisosDeCuadernoSeparanDuenoYAdminOrg(t *testing.T) {
	for _, caso := range []struct {
		nombre, dueno, yo, visibilidad string
		admin, borrar, administrar     bool
	}{
		{"dueno personal", "ana", "ana", "personal", false, true, true},
		{"dueno organizacion", "ana", "ana", "organization", false, true, true},
		{"miembro organizacion", "ana", "luis", "organization", false, false, false},
		{"admin organizacion", "ana", "luis", "organization", true, false, true},
		{"admin no adquiere privado ajeno", "ana", "luis", "private", true, false, false},
		{"admin no adquiere personal ajeno", "ana", "luis", "personal", true, false, false},
		{"admin no adquiere publico ajeno", "ana", "luis", "public", true, false, false},
		{"sin identidad", "ana", "", "organization", false, false, false},
		{"dueno desconocido", "", "luis", "organization", false, false, false},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			p := &Pantalla{Yo: func() string { return caso.yo }, AdminOrg: func() bool { return caso.admin }}
			c := DeCuaderno(api.Notebook{OwnerSub: caso.dueno, Visibility: caso.visibilidad})
			if got := p.esDuenoCuaderno(&c); got != caso.borrar {
				t.Errorf("borrar = %v, quiero %v", got, caso.borrar)
			}
			if got := p.puedeAdministrarCuaderno(&c); got != caso.administrar {
				t.Errorf("administrar = %v, quiero %v", got, caso.administrar)
			}
		})
	}
	p := &Pantalla{svc: &api.Notes{}}
	c := DeCuaderno(api.Notebook{OwnerSub: "ana", Visibility: "organization"})
	if p.esDuenoCuaderno(&c) || p.puedeAdministrarCuaderno(&c) {
		t.Fatal("un servicio sin identidad no puede asumir que el cuaderno es suyo")
	}
}

func TestAjustesDeAdminOrgConservanAccesoSinOfrecerBorradoAjeno(t *testing.T) {
	w := test.NewWindow(nil)
	w.SetPadded(false)
	defer w.Close()
	p := NuevaPantalla(trInterno(t), "es", nil, nil, w)
	p.Yo = func() string { return "luis" }
	p.AdminOrg = func() bool { return true }
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	c := DeCuaderno(api.Notebook{ID: "c1", OwnerSub: "ana", Title: "Equipo", Visibility: "organization", Color: ColoresCuaderno[0]})
	p.dialogoAjustes(&c, "")
	defer p.cerrarAjustes()
	vistos := map[string]bool{}
	recorrerInterno(w.Canvas().Overlays().Top(), func(o fyne.CanvasObject) {
		if x, ok := o.(*canvas.Text); ok {
			vistos[x.Text] = true
		}
		if b, ok := o.(*ui.Boton); ok {
			vistos[b.Texto] = true
		}
	})
	if vistos[p.tr.T("notebook.delete", nil)] {
		t.Error("administrar la organizacion no permite borrar el cuaderno ajeno")
	}
	for _, clave := range []string{"visibility.label", "crypto.title", "index.label", "common.save"} {
		if !vistos[p.tr.T(clave, nil)] {
			t.Errorf("el admin no ve el control permitido %q", clave)
		}
	}
}

func TestCambioDeSesionRetiraLosAjustesDelDuenoAnterior(t *testing.T) {
	w := test.NewWindow(nil)
	w.SetPadded(false)
	defer w.Close()
	p := NuevaPantalla(trInterno(t), "es", nil, nil, w)
	yo := "ana"
	p.Yo = func() string { return yo }
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	c := DeCuaderno(api.Notebook{ID: "c1", OwnerSub: "ana", Title: "Privado de Ana", Visibility: "personal", Color: ColoresCuaderno[0]})
	p.dialogoAjustes(&c, "")
	if p.ajustes == nil || w.Canvas().Overlays().Top() == nil {
		t.Fatal("el dueño no abrió sus ajustes")
	}
	yo = "luis"
	p.ReiniciarSesion()
	if p.ajustes != nil || w.Canvas().Overlays().Top() != nil {
		t.Fatal("el inicio nuevo conserva controles de los ajustes del dueño anterior")
	}
}
