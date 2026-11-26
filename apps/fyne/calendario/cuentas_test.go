// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Sytue Office: una cuenta con CrearCalendario lleva «Crear calendario» en su sección, y
// el formulario de siempre lo crea en la cuenta, no en este equipo. Sin él, la sección
// es la de siempre.
func TestCrearCalendarioEnUnaCuenta(t *testing.T) {
	s := &escrituras{}
	a, w := appConVentana(t, s)
	var nombre, color string
	g := GrupoCuenta{
		Rotulo: "Nextcloud", RotuloSincronizar: "Sincronizar", AlSincronizar: func() {},
		CrearCalendario: func(_ context.Context, n, c string) error { nombre, color = n, c; return nil },
	}
	sin := g
	sin.CrearCalendario = nil
	if n, m := len(a.P.seccionCuenta(g).Acciones), len(a.P.seccionCuenta(sin).Acciones); n != m+1 {
		t.Fatalf("acciones de la sección: %d con crear, %d sin", n, m)
	}
	a.P.alCrearEnCuenta(g)
	var entrada *ui.Entrada
	var guardar *ui.Boton
	enCapas(w.Canvas(), func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *ui.Entrada:
			entrada = v
		case *ui.Boton:
			if v.Texto == a.P.t("common.save", nil) {
				guardar = v
			}
		}
	})
	if entrada == nil || guardar == nil {
		t.Fatal("no está el formulario del calendario")
	}
	entrada.PonerTexto("Viajes")
	guardar.Tapped(nil)
	if hechas, _ := s.apuntadas(); len(hechas) != 0 || nombre != "Viajes" || color != ColorNuevo {
		t.Fatalf("creado en la cuenta: %q %q; en este equipo: %v", nombre, color, hechas)
	}
}
