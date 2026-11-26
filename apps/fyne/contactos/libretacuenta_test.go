// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Sytue Office (K-05): la sección de una cuenta que sabe crear libretas lleva «Nueva
// libreta», con el mismo diálogo, y la libreta se crea EN LA CUENTA (no en este equipo:
// baja con la sincronización). Una cuenta que no sabe (Google), sin la acción.
func TestNuevaLibretaEnLaCuenta(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{persona("ana", "Ana")}}
	a := appDePrueba(t, s)
	a.Cargar()
	var creada string
	p := a.Pantalla
	rotulo := p.t("nav.newAddressBook", nil)
	accion := func() *ui.AccionLateral {
		for _, x := range todosLos[*ui.AccionLateral](p.seccionCuenta(p.Cuentas[0])) {
			if x.Nombre == rotulo {
				return x
			}
		}
		return nil
	}
	p.Cuentas = []GrupoCuenta{{Rotulo: "Google"}}
	if accion() != nil {
		t.Fatal("una cuenta que no crea libretas ofrece crearlas")
	}
	p.Cuentas = []GrupoCuenta{{Rotulo: "Nextcloud", CrearLibreta: func(_ context.Context, nombre string) error { creada = nombre; return nil }}}
	ac := accion()
	if ac == nil {
		t.Fatal("sin «Nueva libreta» en la sección de la cuenta")
	}
	antes := len(s.escrituras)
	ac.AlPulsar()
	d, ok := capaDialogo(a)
	if !ok {
		t.Fatal("no se abrió el diálogo")
	}
	campos := todosLos[*ui.Entrada](d)
	if len(campos) != 1 {
		t.Fatalf("campos: %d", len(campos))
	}
	campos[0].PonerTexto("Proveedores")
	pie := botonesDe(d.Pie)
	pie[len(pie)-1].AlPulsar()
	if creada != "Proveedores" || len(s.escrituras) != antes {
		t.Fatalf("creada %q; escrituras en este equipo %v", creada, s.escrituras[antes:])
	}
	if abierto(a) {
		t.Fatal("el diálogo sigue abierto")
	}
}
