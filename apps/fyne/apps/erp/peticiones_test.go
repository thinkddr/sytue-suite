// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Un módulo en `lectura` lleva la franja con «Pedir acceso de edición»; con `usuario`,
// no. Y cuando el Hub dice que la administración lo ha concedido, la pantalla vuelve a
// leer sus permisos (y solo si el aviso es de ESE módulo).
func TestUnModuloEnLecturaPideAccesoYSeDesbloqueaEnVivo(t *testing.T) {
	p := NuevaPantalla(traductor(t), api.NewERP("http://127.0.0.1:1", nil, nil, "es"), nil)
	p.Catalogo = catalogoEjemplo()
	p.lista.Modelo = p.Catalogo.Modulos[0].Modelos[0]
	recargas := 0
	p.fondo = func(func()) { recargas++ }
	p.SabePermisos = true
	p.Permisos = Permisos{Modulos: map[string]string{"facturacion": api.ERPLectura}}
	fs := p.franjasPeticion()
	if len(fs) != 1 {
		t.Fatalf("un módulo en lectura lleva %d franjas y es una", len(fs))
	}
	if f, ok := fs[0].(*ui.FranjaSoloLectura); !ok || f.Accion == nil {
		t.Fatal("la franja no ofrece «Pedir acceso de edición»")
	}
	ui.RepartirAviso(api.HubNotification{ID: "a", Type: "erp.access.granted", Params: map[string]string{"resource": "contactos"}})
	fyne.DoAndWait(func() {})
	if recargas != 0 {
		t.Error("el aviso de OTRO módulo recargó los permisos")
	}
	ui.RepartirAviso(api.HubNotification{ID: "b", Type: "erp.access.granted", Params: map[string]string{"resource": "facturacion"}})
	fyne.DoAndWait(func() {})
	if recargas != 1 {
		t.Errorf("concedido, la pantalla no volvió a leer los permisos (%d)", recargas)
	}
	p.Permisos = Permisos{Modulos: map[string]string{"facturacion": api.ERPUsuario}}
	if fs := p.franjasPeticion(); len(fs) != 0 {
		t.Error("con permiso de usuario sigue la franja de solo lectura")
	}
}
