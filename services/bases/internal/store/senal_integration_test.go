// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// TestLaSenalDeUnaBaseLlegaAQuienesLaVen (ADR 0078): lo que escribe una editora en la
// base de ana le llega a ana y a las personas con permiso, no a un grupo ni a quien solo
// lee el registro; y quien pierde su permiso recibe la señal de que la base ya no está en
// su widget. Fuera de una escritura con señal no se consulta nada. VISTO EN ROJO sin el
// AvisarQuienesVen de autorizar y sin el AvisarTambien de RevocarPermiso.
func TestLaSenalDeUnaBaseLlegaAQuienesLaVen(t *testing.T) {
	e := nuevo(t)
	_, err := e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: "user", PrincipalID: "bea", Role: domain.RolEditor})
	e.ok(err)
	cleoPerm, err := e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: "user", PrincipalID: "cleo", Role: domain.RolLector})
	e.ok(err)
	_, err = e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: "group", PrincipalID: "ventas", Role: domain.RolLector})
	e.ok(err)

	ctx, avisados := hubnotify.EscrituraDePrueba(e.ctx)
	_, err = e.s.CrearRegistro(ctx, bea, e.tabl.ID, dato(e.txt.ID, `"Nuevo"`), nil)
	e.ok(err)
	got := avisados()
	slices.Sort(got)
	if !slices.Equal(got, []string{"ana", "bea", "cleo"}) {
		t.Errorf("la escritura de bea avisa a %v; quiero a ana, bea y cleo", got)
	}

	// Leer no avisa a nadie, aunque vaya en una petición con señal.
	ctx, avisados = hubnotify.EscrituraDePrueba(e.ctx)
	_, err = e.s.ListarRegistros(ctx, bea, e.tabl.ID, "", 0, "")
	e.ok(err)
	if got := avisados(); len(got) != 0 {
		t.Errorf("una lectura avisa a %v", got)
	}

	ctx, avisados = hubnotify.EscrituraDePrueba(e.ctx)
	e.ok(e.s.RevocarPermiso(ctx, ana, e.base.ID, cleoPerm.ID))
	if got := avisados(); !slices.Contains(got, "cleo") {
		t.Errorf("revocar el permiso de cleo avisa a %v; quiero también a cleo", got)
	}
}
