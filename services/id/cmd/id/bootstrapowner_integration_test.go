// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Alta inicial contra PostgreSQL real (pgtest). Fija el contrato que hace
// utilizable una producción recién instalada: existe un usuario, es propietario de
// la organización que opera la plataforma y es admin de plataforma. El segundo
// factor lo enrola la persona en /account: hasta entonces su rol owner mantiene
// cerrada Sytue Admin, que es el comportamiento correcto y no un fallo del alta.
package main

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	iddb "git.sytue.eu/sytue/one/services/id/db"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

func TestBootstrapOwnerDejaLaPlataformaOperable(t *testing.T) {
	h := pgtest.New(t, iddb.Migrations, iddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := context.Background()

	in, err := bootstrapOwnerInputFromEnv(entorno(map[string]string{
		bootstrapPasswordEnvVar: "contraseña-de-despliegue-larga",
	}))
	if err != nil {
		t.Fatalf("entrada: %v", err)
	}

	// DSN vacío: sin RenameFrom el bootstrap no abre ninguna base de servicio, y
	// pasarle uno aquí ataría el test a que existan las nueve.
	if err := bootstrapOwner(ctx, st, "", in); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	user, err := st.GetUserByEmail(ctx, "admin@sytue.eu")
	if err != nil {
		t.Fatalf("el admin global por defecto no existe: %v", err)
	}
	// Sin segundo factor todavía: se enrola por autoservicio en /account. Lo que
	// NO puede pasar es que quede MFAEnabled sin secreto, porque entonces ni
	// siquiera podría entrar en Sytue ID a enrolarlo.
	if user.MFAEnabled {
		t.Error("MFAEnabled sin secreto enrolado: el propietario quedaría fuera de Sytue ID")
	}
	if _, err := st.GetTOTPSecret(ctx, user.ID); err == nil {
		t.Error("el bootstrap enroló un segundo factor: debe hacerlo la persona en /account")
	}

	org, err := st.GetOrgBySlug(ctx, "corporate")
	if err != nil {
		t.Fatalf("la organización de plataforma no existe: %v", err)
	}
	roles, err := st.RolesForUserInOrg(ctx, org.ID, user.ID)
	if err != nil {
		t.Fatalf("roles: %v", err)
	}
	if len(roles) != 1 || domain.Role(roles[0]) != domain.RoleOwner {
		t.Errorf("roles inesperados en la org de plataforma: %v", roles)
	}

	esAdmin, err := st.IsPlatformAdmin(ctx, user.ID)
	if err != nil {
		t.Fatalf("IsPlatformAdmin: %v", err)
	}
	if !esAdmin {
		t.Error("el propietario inicial no quedó inscrito como admin de plataforma")
	}

	// Idempotente: el Job se ejecuta en CADA despliegue. Una segunda pasada no
	// puede duplicar la organización, degradar el rol ni perder el segundo factor.
	if err := bootstrapOwner(ctx, st, "", in); err != nil {
		t.Fatalf("segunda pasada: %v", err)
	}
	if again, err := st.GetOrgBySlug(ctx, "corporate"); err != nil || again.ID != org.ID {
		t.Errorf("la segunda pasada no reutilizó la organización (err=%v)", err)
	}
	if again, err := st.GetUserByEmail(ctx, "admin@sytue.eu"); err != nil || again.ID != user.ID {
		t.Errorf("la segunda pasada no reutilizó el usuario (err=%v)", err)
	}
	if esAdmin, err := st.IsPlatformAdmin(ctx, user.ID); err != nil || !esAdmin {
		t.Errorf("la segunda pasada perdió el alcance de plataforma (err=%v)", err)
	}
}

// Un usuario cualquiera NO es admin de plataforma: la allowlist es la única vía y
// no se deduce de tener rol elevado en una organización.
func TestSoloLaAllowlistConcedePlataforma(t *testing.T) {
	h := pgtest.New(t, iddb.Migrations, iddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := context.Background()

	hash, err := domain.HashPassword("contraseña-de-prueba-larga")
	if err != nil {
		t.Fatal(err)
	}
	var user domain.User
	var org domain.Organization
	if err := st.InTx(ctx, func(q pg.Querier) error {
		var e error
		if org, e = ensureOrg(ctx, st, q, "cliente", "Cliente, S.L."); e != nil {
			return e
		}
		if user, e = ensureUser(ctx, st, q, hash, "admin@cliente.example", "Admin Cliente", false); e != nil {
			return e
		}
		return st.UpsertMembership(ctx, q, domain.Membership{
			OrgID: org.ID, UserID: user.ID, Role: domain.RoleOwner,
		})
	}); err != nil {
		t.Fatalf("preparar organización cliente: %v", err)
	}

	esAdmin, err := st.IsPlatformAdmin(ctx, user.ID)
	if err != nil {
		t.Fatalf("IsPlatformAdmin: %v", err)
	}
	if esAdmin {
		t.Error("un owner de organización obtuvo alcance de plataforma sin estar en la allowlist")
	}
}
