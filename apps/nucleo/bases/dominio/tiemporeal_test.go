// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package dominio

import "testing"

// Editar una vista cambia lo que otra pestaña tiene en pantalla (sus filtros, su orden):
// tiene que viajar por el flujo como crearla o borrarla, para que se vuelva a pedir.
func TestLasVistasViajanPorElFlujo(t *testing.T) {
	for _, accion := range []string{"view.create", "view.update", "view.delete"} {
		c, ok := CambioParaElFlujo(accion, "t1", "", "", "ana", "", RolLector, RolLector)
		if !ok || c.Action != accion || c.TableID != "t1" {
			t.Errorf("%s: %+v %v", accion, c, ok)
		}
		if _, ok := CambioParaElFlujo(accion, "t1", "", "", "ana", "", RolLector, ""); ok {
			t.Errorf("%s llega a quien no lee la tabla", accion)
		}
	}
}
