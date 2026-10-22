// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la BAJA DE COLABORADOR de Sytue Cloud contra PostgreSQL real:
// quitar a alguien arrastra los tokens de API que emitió, y no ser colaborador es una
// respuesta firme y no una laguna que se rellene con el rol de la organización.
//
// Hallazgo de la caza del patrón «autorización apoyada en la fila equivocada»:
//   - `humanHasRole` caía al claim de rol del JWT cuando el usuario no tenía fila de
//     membresía. Ese fallback existe para el ARRANQUE (el primer owner entra antes de
//     que haya membresías), pero se aplicaba a cualquiera sin fila, así que RETIRARLE la
//     membresía a un member/billing que fuera admin en la organización le CONCEDÍA
//     admin/owner en Cloud: exec de shell en pods de producción, emitir tokens de CI,
//     re-añadirse, restore de servicios, spend-limit y el secreto HMAC del webhook git.
//     Los vocabularios de rol de Sytue ID y de Cloud son idénticos, así que encajaba.
//   - `DeleteMembership` no tocaba `api_tokens`, cuyo `expires_at` es OPCIONAL: el token
//     del saliente seguía sirviendo para siempre.
package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// TestDeleteMembershipRevokesAPITokens: la baja arrastra los tokens que emitió esa
// persona y NO toca los de los demás.
func TestDeleteMembershipRevokesAPITokens(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	if _, err := st.UpsertMembership(ctx, "saliente", domain.RoleMember); err != nil {
		t.Fatalf("UpsertMembership(saliente): %v", err)
	}
	if _, err := st.UpsertMembership(ctx, "queda", domain.RoleAdmin); err != nil {
		t.Fatalf("UpsertMembership(queda): %v", err)
	}

	// Un token SIN caducidad de cada uno: es el caso peligroso (ResolveAPIToken acepta
	// expires_at NULL, así que vive indefinidamente).
	for _, sub := range []string{"saliente", "queda"} {
		if _, err := st.CreateAPIToken(ctx, store.NewAPITokenRecord{
			Name: "ci-" + sub, TokenHash: "hash-" + sub,
			Scope: domain.ScopeDeploy, CreatedBy: sub,
		}); err != nil {
			t.Fatalf("CreateAPIToken(%s): %v", sub, err)
		}
	}
	if toks, err := st.ListAPITokens(ctx); err != nil {
		t.Fatalf("ListAPITokens: %v", err)
	} else if len(toks) != 2 {
		t.Fatalf("esperaba 2 tokens antes de la baja, hay %d", len(toks))
	}

	// BAJA del colaborador.
	if err := st.DeleteMembership(ctx, "saliente"); err != nil {
		t.Fatalf("DeleteMembership: %v", err)
	}

	toks, err := st.ListAPITokens(ctx)
	if err != nil {
		t.Fatalf("ListAPITokens tras la baja: %v", err)
	}
	if len(toks) != 1 {
		t.Fatalf("la baja debe arrastrar SOLO los tokens del saliente; quedan %d", len(toks))
	}
	if toks[0].CreatedBy != "queda" {
		t.Fatalf("se ha revocado el token equivocado: created_by=%q", toks[0].CreatedBy)
	}
}

// TestEffectiveRoleNoFallbackOnPopulatedTenant: con el tenant ya poblado, quien no es
// colaborador NO hereda el rol de la organización; el fallback solo vive en el arranque.
func TestEffectiveRoleNoFallbackOnPopulatedTenant(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	// Tenant VACÍO: no hay membresías ⇒ el arranque decide por el claim del JWT.
	if n, err := st.CountMemberships(ctx); err != nil {
		t.Fatalf("CountMemberships: %v", err)
	} else if n != 0 {
		t.Fatalf("el tenant debería arrancar vacío, hay %d membresías", n)
	}

	// Tenant POBLADO: a partir de aquí, la ausencia de fila es una respuesta.
	if _, err := st.UpsertMembership(ctx, "owner-inicial", domain.RoleOwner); err != nil {
		t.Fatalf("UpsertMembership: %v", err)
	}
	if n, err := st.CountMemberships(ctx); err != nil {
		t.Fatalf("CountMemberships: %v", err)
	} else if n != 1 {
		t.Fatalf("esperaba 1 membresía, hay %d", n)
	}
	// El sujeto sin fila no tiene rol EN CLOUD, y CountMemberships > 0 es lo que impide
	// que EffectiveRole delegue en el claim de la organización.
	if _, err := st.GetMembershipRole(ctx, "ajeno"); err == nil {
		t.Fatal("un sujeto sin membresía no debe tener rol en Cloud")
	}
}
