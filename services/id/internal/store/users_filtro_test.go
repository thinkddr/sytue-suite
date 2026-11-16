// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import "testing"

// Los filtros del directorio son los nombres de los recuentos de
// /admin/insights: si uno se queda sin predicado, el aviso de la portada
// enlazaría a una lista que el servicio rechaza con 400.
func TestFiltroUsuarios(t *testing.T) {
	for _, f := range []FiltroUsuarios{"", FiltroSinMFA, FiltroSuspendidos, FiltroSinVerificar} {
		if !FiltroUsuariosValido(f) {
			t.Errorf("%q debería ser un filtro válido", f)
		}
	}
	for _, f := range []FiltroUsuarios{"todos", "SUSPENDED", "suspended ", "deleted"} {
		if FiltroUsuariosValido(f) {
			t.Errorf("%q no debería ser un filtro válido", f)
		}
	}
	// Los nombres son el contrato con la consola web y los avisos: no se renombran.
	quiero := map[FiltroUsuarios]string{
		FiltroSinMFA: "without_mfa", FiltroSuspendidos: "suspended", FiltroSinVerificar: "unverified_email",
	}
	for f, nombre := range quiero {
		if string(f) != nombre || condicionFiltro[f] == "" {
			t.Errorf("filtro %q: nombre o predicado inesperado (%q)", f, condicionFiltro[f])
		}
	}
}
