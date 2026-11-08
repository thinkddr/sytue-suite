// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package auth

import (
	"context"
	"testing"

	"github.com/zalando/go-keyring"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

// TestRenovarNoRotaLaEpocaDeLosWidgets: cada renovación guarda un refresh nuevo y
// cambia la marca; la época de los widgets se queda y queda confirmada con la marca
// nueva. Otra persona y el cierre la rotan. Era la causa del widget de Calendar en
// blanco tras ~15 min con la app abierta (ADR 0078, enmienda del 2026-11-06).
func TestRenovarNoRotaLaEpocaDeLosWidgets(t *testing.T) {
	aparte(t)
	keyring.MockInit()
	a := (&OIDC{Issuer: "https://id.example"}).Almacen()
	guardar := func(tk Tokens) ajustes.Estado {
		t.Helper()
		// Como en la suite: el guardado va siempre dentro del cerrojo de la sesión.
		soltar, err := ajustes.CerrojoSesion(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer soltar()
		if err := a.Guardar(tk); err != nil {
			t.Fatal(err)
		}
		e := ajustes.Leer()
		if e.Epoca == "" || e.EpocaConfirmada != e.Sesion {
			t.Fatalf("la época no queda confirmada con la marca del guardado: %+v", e)
		}
		return e
	}
	antes := guardar(Tokens{Refresh: "r1", Subject: "ana", Organizacion: "acme"})
	despues := guardar(Tokens{Refresh: "r2", Subject: "ana", Organizacion: "acme"})
	if despues.Epoca != antes.Epoca || despues.Sesion == antes.Sesion {
		t.Fatalf("renovar ha rotado la época o no ha cambiado la marca: %+v → %+v", antes, despues)
	}
	otra := guardar(Tokens{Refresh: "r3", Subject: "bea", Organizacion: "acme"})
	if otra.Epoca == despues.Epoca {
		t.Fatal("otra persona hereda la época de la anterior")
	}
	if cierre := guardar(Tokens{}); cierre.Epoca == otra.Epoca {
		t.Fatal("cerrar la sesión no rota la época")
	}
}
