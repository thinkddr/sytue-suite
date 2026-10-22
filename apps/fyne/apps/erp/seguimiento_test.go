// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// El selector de responsable dice quién es «Yo» y no lo repite entre los demás; la
// persona elegida sigue siendo la de su fila.
func TestElResponsableYoLlevaSuNombre(t *testing.T) {
	p := nuevoPlan(traductor(t), []api.ERPPersona{
		{Sub: "a", Nombre: "Ana"}, {Sub: "d", Nombre: "Daniel", Yo: true}, {Sub: "l", Correo: "luis@x.es"},
	})
	if quiero := []string{"Yo (Daniel)", "Ana", "luis@x.es"}; !slices.Equal(p.quien.Opciones, quiero) {
		t.Fatalf("opciones = %q, quiero %q", p.quien.Opciones, quiero)
	}
	p.quien.Elegida = 2
	if r := p.responsable(); r.Sub != "l" {
		t.Errorf("la tercera fila es Luis, no %+v", r)
	}
}
