// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de la LIBERTAD DE ELECCIÓN contra PostgreSQL real: qué organización sirve cada
// producto de la suite con qué aplicación conectada.
//
// El harness conecta con un rol sin BYPASSRLS (platform/pkg/pgtest), así que el
// aislamiento que se comprueba aquí lo impone de verdad la política de la migración
// 0007, no un predicado de Go.
package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	integrationsdb "git.sytue.eu/sytue/one/services/integrations/db"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

// activeInstance siembra una instancia ACTIVA: designar sustituto exige que lo esté.
func activeInstance(ctx context.Context, t *testing.T, st *store.Store, name string) string {
	t.Helper()
	inst, err := st.CreateInstance(ctx, "demo", name, json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("CreateInstance(%s): %v", name, err)
	}
	if _, err := st.UpdateInstance(ctx, inst.ID, inst.Name, inst.Config, domain.StatusActive, nil); err != nil {
		t.Fatalf("activar %s: %v", name, err)
	}
	return inst.ID
}

// TestSubstitutionLifecycle: elegir, cambiar de opinión y volver al producto nativo.
func TestSubstitutionLifecycle(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	st := store.New(h.Pool, newCipher(t))
	seedTemplate(t, h, "demo")
	ctx := tenantCtx("acme")

	// Sin filas: TODO lo sirve Sytue. La ausencia ES el estado por defecto, no un dato
	// que falte, así que aquí no hay nada que sembrar.
	if subs, err := st.ListSubstitutions(ctx); err != nil {
		t.Fatalf("ListSubstitutions: %v", err)
	} else if len(subs) != 0 {
		t.Fatalf("una organización nueva no debe tener sustituciones, tiene %d", len(subs))
	}

	uno := activeInstance(ctx, t, st, "Google Workspace")
	dos := activeInstance(ctx, t, st, "Microsoft 365")

	if err := st.SetSubstitution(ctx, "sheets", uno); err != nil {
		t.Fatalf("SetSubstitution: %v", err)
	}
	// Cambiar de opinión SUSTITUYE, no acumula: sustituir es elegir. Con dos filas para
	// el mismo producto el lanzador tendría que desempatar.
	if err := st.SetSubstitution(ctx, "sheets", dos); err != nil {
		t.Fatalf("SetSubstitution (cambio): %v", err)
	}
	subs, err := st.ListSubstitutions(ctx)
	if err != nil {
		t.Fatalf("ListSubstitutions: %v", err)
	}
	if len(subs) != 1 || subs[0].Product != "sheets" || subs[0].InstanceID != dos {
		t.Fatalf("elegir dos veces debe dejar UNA fila con la última: %+v", subs)
	}

	// Volver al nativo.
	if err := st.ClearSubstitution(ctx, "sheets"); err != nil {
		t.Fatalf("ClearSubstitution: %v", err)
	}
	if subs, err := st.ListSubstitutions(ctx); err != nil || len(subs) != 0 {
		t.Fatalf("tras limpiar debe quedar vacío (err=%v, subs=%+v)", err, subs)
	}
	// Limpiar lo que ya está limpio es un NO-OP: el resultado deseado («que lo sirva
	// Sytue») ya se cumple, y devolver ErrNotFound obligaría a la UI a distinguir dos
	// casos que para quien lo usa son el mismo.
	if err := st.ClearSubstitution(ctx, "sheets"); err != nil {
		t.Fatalf("limpiar dos veces no debe fallar: %v", err)
	}
}

// TestSubstitutionRequiresActiveInstance: designar como sustituto de Sytue Mail una
// conexión desactivada dejaría a la organización sin correo en el lanzador, y el fallo
// aparecería después, lejos de la acción que lo causó.
func TestSubstitutionRequiresActiveInstance(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	st := store.New(h.Pool, newCipher(t))
	seedTemplate(t, h, "demo")
	ctx := tenantCtx("acme")

	// Instancia en borrador (no activa).
	inst, err := st.CreateInstance(ctx, "demo", "A medio configurar", json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if err := st.SetSubstitution(ctx, "mail", inst.ID); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, quiero ErrInvalidInput: una conexión no activa no puede servir un producto", err)
	}
	// Y una que no existe es 404, no un 500 ni un éxito silencioso.
	if err := st.SetSubstitution(ctx, "mail", "00000000-0000-0000-0000-000000000000"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, quiero ErrNotFound", err)
	}
}

// TestSubstitutionOrgIsolation: la elección de una organización no la ve ni la puede
// tocar otra. Con el harness sin BYPASSRLS, quien lo impide es la POLÍTICA de la
// migración, no un WHERE de Go.
func TestSubstitutionOrgIsolation(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	st := store.New(h.Pool, newCipher(t))
	seedTemplate(t, h, "demo")
	ctxA, ctxB := tenantCtx("org-a"), tenantCtx("org-b")

	instA := activeInstance(ctxA, t, st, "La de A")
	if err := st.SetSubstitution(ctxA, "docs", instA); err != nil {
		t.Fatalf("A SetSubstitution: %v", err)
	}

	if subs, err := st.ListSubstitutions(ctxB); err != nil {
		t.Fatalf("B ListSubstitutions: %v", err)
	} else if len(subs) != 0 {
		t.Fatalf("aislamiento roto: B ve la elección de A: %+v", subs)
	}
	// B tampoco puede apuntar a la instancia de A: para B esa conexión no existe.
	if err := st.SetSubstitution(ctxB, "docs", instA); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B designa la conexión de A (err=%v)", err)
	}
	// Y limpiar en B no toca la elección de A.
	if err := st.ClearSubstitution(ctxB, "docs"); err != nil {
		t.Fatalf("B ClearSubstitution: %v", err)
	}
	if subs, err := st.ListSubstitutions(ctxA); err != nil || len(subs) != 1 {
		t.Fatalf("A ha perdido su elección (err=%v, subs=%+v)", err, subs)
	}
}

// TestSubstitutionDiesWithItsInstance: borrar la conexión devuelve el producto al nativo.
// Sin la cascada, el lanzador seguiría apuntando a algo que ya no existe.
func TestSubstitutionDiesWithItsInstance(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	st := store.New(h.Pool, newCipher(t))
	seedTemplate(t, h, "demo")
	ctx := tenantCtx("acme")

	inst := activeInstance(ctx, t, st, "Se va a borrar")
	if err := st.SetSubstitution(ctx, "rooms", inst); err != nil {
		t.Fatalf("SetSubstitution: %v", err)
	}
	// Antes de borrar, la baja puede saber qué productos deja huérfanos (para avisar).
	if prods, err := st.SubstitutedProducts(ctx, inst); err != nil {
		t.Fatalf("SubstitutedProducts: %v", err)
	} else if len(prods) != 1 || prods[0] != "rooms" {
		t.Fatalf("productos servidos por la conexión = %v, quiero [rooms]", prods)
	}

	if err := st.DeleteInstance(ctx, inst); err != nil {
		t.Fatalf("DeleteInstance: %v", err)
	}
	if subs, err := st.ListSubstitutions(ctx); err != nil {
		t.Fatalf("ListSubstitutions tras borrar: %v", err)
	} else if len(subs) != 0 {
		t.Fatalf("la sustitución debe morir con su conexión, quedan %+v", subs)
	}
}
