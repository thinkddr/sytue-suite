// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/agenda"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestGestoRefrescoContactsConservaFiltrosYRespetaEdicion(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{persona("a", "Ana")}}
	a := appDePrueba(t, s)
	a.Cargar()
	p := a.Pantalla
	p.Busqueda, p.SoloDestacados = "Ana", true
	r := &p.Refresco
	if !r.Puede() {
		t.Fatal("loaded address book cannot refresh")
	}
	before := r.Revision()
	r.Actualizar()
	if p.Busqueda != "Ana" || !p.SoloDestacados || r.Revision() == before {
		t.Fatal("pull discarded filters or did not reload the active address book")
	}
	p.Estado = agenda.Vacia
	if !r.Puede() {
		t.Fatal("empty address book cannot refresh")
	}
	p.DetalleAbierto = true
	if r.Puede() {
		t.Fatal("contact detail admits list refresh")
	}
	p.DetalleAbierto = false
	p.Lote = []api.Contact{persona("a", "Ana")}
	if r.Puede() {
		t.Fatal("selected contacts admit list refresh")
	}
}
