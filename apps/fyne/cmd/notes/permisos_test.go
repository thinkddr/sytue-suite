// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

func TestAdministraCuadernosSoloEnLaSesionYOrganizacionVigentes(t *testing.T) {
	actual := sesion.Estado{Sujeto: "ana", GeneracionSesion: "sesion-actual"}
	base := auth.Reclamos{Sub: "ana", Tenant: "equipo", OrgKind: "org", Roles: []string{"admin"}}
	for _, caso := range []struct {
		nombre          string
		reclamos        auth.Reclamos
		generacion, org string
		estado          sesion.Estado
		quiere          bool
	}{
		{"admin actual", base, "sesion-actual", "equipo", actual, true},
		{"owner actual", auth.Reclamos{Sub: "ana", Tenant: "equipo", Roles: []string{"owner"}}, "sesion-actual", "equipo", actual, true},
		{"miembro", auth.Reclamos{Sub: "ana", Tenant: "equipo", Roles: []string{"member"}}, "sesion-actual", "equipo", actual, false},
		{"permiso delegado no es admin", auth.Reclamos{Sub: "ana", Tenant: "equipo", Perms: []string{"users.read"}}, "sesion-actual", "equipo", actual, false},
		{"otra persona", auth.Reclamos{Sub: "luis", Tenant: "equipo", Roles: []string{"admin"}}, "sesion-actual", "equipo", actual, false},
		{"otra organizacion", base, "sesion-actual", "otra", actual, false},
		{"sesion anterior", base, "sesion-anterior", "equipo", actual, false},
		{"sin generacion", base, "", "equipo", actual, false},
		{"sin organizacion", base, "sesion-actual", "", actual, false},
		{"sin sesion", base, "sesion-actual", "equipo", sesion.Estado{}, false},
		{"cuenta personal", auth.Reclamos{Sub: "ana", Tenant: "equipo", OrgKind: "personal", Roles: []string{"owner"}}, "sesion-actual", "equipo", actual, false},
		{"plan personal anterior", auth.Reclamos{Sub: "ana", Tenant: "equipo", Plan: "personal_free", Roles: []string{"admin"}}, "sesion-actual", "equipo", actual, false},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			if got := administraCuadernos(caso.reclamos, caso.generacion, caso.estado, caso.org); got != caso.quiere {
				t.Errorf("administraCuadernos = %v, quiero %v", got, caso.quiere)
			}
		})
	}
}
