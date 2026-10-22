// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"testing"
	"time"
)

// TestAlcanceEfectivo vigila las tres respuestas por las que existe: que un
// owner sale con TODO (y no vacío, que es lo que enseñaba mirar `perms`), que
// una elevación caducada no concede nada, y que de cada permiso se dice de
// dónde sale —y se queda la fuente más estable cuando llega por varias—.
func TestAlcanceEfectivo(t *testing.T) {
	t.Parallel()
	ahora := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	viva := ahora.Add(time.Hour)
	muerta := ahora.Add(-time.Minute)

	busca := func(cs []Concesion, p Permission) (Concesion, bool) {
		for _, c := range cs {
			if c.Permiso == string(p) {
				return c, true
			}
		}
		return Concesion{}, false
	}

	// Un owner sin ningún rol personalizado tiene el catálogo entero por su rol.
	owner := AlcanceEfectivo(string(RoleOwner), nil, nil, ahora)
	if len(owner) != len(AllPermissions) {
		t.Fatalf("un owner tiene %d permisos, quiero los %d del catálogo", len(owner), len(AllPermissions))
	}
	if c, _ := busca(owner, PermRolesManage); c.Fuente != FuenteRolIntegrado {
		t.Errorf("roles.manage de un owner sale de %q, quiero %q", c.Fuente, FuenteRolIntegrado)
	}

	// Un miembro con un rol personalizado: solo lo de ese rol, y se dice cuál.
	rol := CustomRole{Name: "Auditoría", Permissions: []string{string(PermAuditView)}}
	miembro := AlcanceEfectivo(string(RoleMember), []CustomRole{rol}, nil, ahora)
	if len(miembro) != 1 {
		t.Fatalf("quiero un solo permiso, tengo %+v", miembro)
	}
	if miembro[0].Fuente != FuenteRolPersonalizado || miembro[0].Detalle != "Auditoría" {
		t.Errorf("no dice de qué rol sale: %+v", miembro[0])
	}
	if miembro[0].Hasta != nil {
		t.Error("una asignación permanente no caduca")
	}

	// Una elevación PIM viva concede, y con su hora de apagado.
	conPIM := AlcanceEfectivo(string(RoleMember), nil, []ElevacionViva{{
		Nombre: "Soporte", Perms: []string{string(PermUsersManage)},
		ExpiresAt: viva, Status: ActivationActive,
	}}, ahora)
	c, ok := busca(conPIM, PermUsersManage)
	if !ok || c.Fuente != FuenteElevacionTemporal {
		t.Fatalf("la elevación viva tiene que conceder: %+v", conPIM)
	}
	if c.Hasta == nil || !c.Hasta.Equal(viva) {
		t.Errorf("una elevación sin hora de apagado no se distingue de un permiso permanente: %+v", c)
	}

	// Caducada: NO concede. Es lo que hace que «just-in-time» signifique algo.
	if a := AlcanceEfectivo(string(RoleMember), nil, []ElevacionViva{{
		Nombre: "Soporte", Perms: []string{string(PermUsersManage)},
		ExpiresAt: muerta, Status: ActivationActive,
	}}, ahora); len(a) != 0 {
		t.Errorf("una elevación caducada no concede nada, tengo %+v", a)
	}
	// Pendiente de aprobación tampoco.
	if a := AlcanceEfectivo(string(RoleMember), nil, []ElevacionViva{{
		Nombre: "Soporte", Perms: []string{string(PermUsersManage)},
		ExpiresAt: viva, Status: ActivationPending,
	}}, ahora); len(a) != 0 {
		t.Errorf("una activación pendiente no concede nada, tengo %+v", a)
	}

	// Por dos caminos: gana el más estable, que es el que hay que tocar para
	// quitarlo de verdad.
	dos := AlcanceEfectivo(string(RoleMember),
		[]CustomRole{{Name: "Auditoría", Permissions: []string{string(PermAuditView)}}},
		[]ElevacionViva{{Nombre: "Soporte", Perms: []string{string(PermAuditView)}, ExpiresAt: viva, Status: ActivationActive}},
		ahora)
	if len(dos) != 1 || dos[0].Fuente != FuenteRolPersonalizado {
		t.Errorf("gana la fuente estable: %+v", dos)
	}

	// Una elevación a rol integrado concede el conjunto DELEGABLE, nunca roles.manage.
	builtin := AlcanceEfectivo(string(RoleMember), nil, []ElevacionViva{{
		Nombre: "admin", Builtin: true, ExpiresAt: viva, Status: ActivationActive,
	}}, ahora)
	if _, ok := busca(builtin, PermRolesManage); ok {
		t.Error("una elevación PIM no puede conceder roles.manage (auto-escalada)")
	}
	if _, ok := busca(builtin, PermUsersManage); !ok {
		t.Error("una elevación a rol integrado concede el resto del catálogo")
	}
}
