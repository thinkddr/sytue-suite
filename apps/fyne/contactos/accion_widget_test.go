// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"net/url"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestNuevoContactoDelWidgetAbreElFormulario: la acción «Nuevo contacto» del widget
// (`sytue-contacts:/?action=create`, la lee api.LeerAccion) abre el formulario de crear
// vacío, también si llega antes que la lista (la app arrancando por el widget): se abre
// al llegar. El enlace no crea nada: se guarda con el botón del formulario.
func TestNuevoContactoDelWidgetAbreElFormulario(t *testing.T) {
	u, err := url.Parse("sytue-contacts:/?action=create")
	if err != nil || api.LeerAccion("contacts", u) != "create" {
		t.Fatalf("la acción del widget no se lee: %v", err)
	}
	s := &servicioFalso{contactos: []api.Contact{persona("ana", "Ana")}}
	a := appDePrueba(t, s)
	a.PedirNuevo()
	if !a.nuevoPedido || a.lienzo().Overlays().Top() != nil {
		t.Fatal("sin la lista, el formulario se aplaza")
	}
	a.Cargar()
	if a.nuevoPedido || a.lienzo().Overlays().Top() == nil {
		t.Fatal("al llegar la lista no se abrió el formulario de crear")
	}
	if len(s.escrituras) != 0 {
		t.Fatalf("el enlace escribió: %+v", s.escrituras)
	}
	a.lienzo().Overlays().Remove(a.lienzo().Overlays().Top())
	a.PedirNuevo()
	if a.nuevoPedido || a.lienzo().Overlays().Top() == nil {
		t.Fatal("con la lista ya cargada, el formulario se abre en el acto")
	}
}
