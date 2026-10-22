// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Firma de correo en el SERVIDOR (columna accounts.signature, migración 0014).
// El ceiling que cierra: vivía en localStorage, así que no existía fuera del
// navegador donde se escribió. Lo que hay que demostrar es que persiste por
// cuenta, que no se ve entre organizaciones (RLS de `accounts`, que ya iba con
// FORCE) y que ninguna cuenta hereda la firma de otra.
package store_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

func TestSignaturePersisteYEsPorCuenta(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	ada := seedAccount(t, ctx, st, "ada@sytue.eu")
	bob := seedAccount(t, ctx, st, "bob@sytue.eu")

	// Cuenta recién creada: sin firma, y eso no es un error.
	if sig, err := st.GetSignature(ctx, ada.ID); err != nil || sig != "" {
		t.Fatalf("firma inicial = %q, err=%v (quiero vacía y sin error)", sig, err)
	}

	const firma = "Ada Lovelace\nCTO · Sytue\n+34 600 000 000"
	if err := st.SetSignature(ctx, ada.ID, firma); err != nil {
		t.Fatalf("SetSignature: %v", err)
	}
	// Persiste tal cual, saltos de línea incluidos: son la firma.
	got, err := st.GetSignature(ctx, ada.ID)
	if err != nil {
		t.Fatalf("GetSignature: %v", err)
	}
	if got != firma {
		t.Fatalf("firma = %q, quiero %q", got, firma)
	}
	// Y es de ESA cuenta: el compañero de organización no la hereda.
	if sig, err := st.GetSignature(ctx, bob.ID); err != nil || sig != "" {
		t.Fatalf("otra cuenta ve la firma: %q (err=%v)", sig, err)
	}

	// Sobrescribir con vacío la borra (el usuario puede quitarla).
	if err := st.SetSignature(ctx, ada.ID, ""); err != nil {
		t.Fatalf("SetSignature(vacía): %v", err)
	}
	if sig, _ := st.GetSignature(ctx, ada.ID); sig != "" {
		t.Fatalf("no se borró: %q", sig)
	}
}

// Aislamiento entre organizaciones: `accounts` va con ENABLE *y* FORCE ROW LEVEL
// SECURITY desde 0001_init, así que la columna nueva queda cubierta sin política
// propia. Esto lo comprueba en vez de suponerlo.
func TestSignatureNoCruzaOrganizaciones(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ada := seedAccount(t, tenantCtx("acme"), st, "ada@sytue.eu")
	if err := st.SetSignature(tenantCtx("acme"), ada.ID, "Ada · Acme"); err != nil {
		t.Fatalf("SetSignature: %v", err)
	}
	// Otro tenant no la LEE...
	if sig, err := st.GetSignature(tenantCtx("other"), ada.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto al leer: %q (err=%v)", sig, err)
	}
	// ...ni la ESCRIBE (el UPDATE no ve la fila: 0 filas ⇒ ErrNotFound, no un
	// éxito silencioso que dejara creer que se guardó).
	if err := st.SetSignature(tenantCtx("other"), ada.ID, "pisada"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto al escribir: %v", err)
	}
	if sig, _ := st.GetSignature(tenantCtx("acme"), ada.ID); sig != "Ada · Acme" {
		t.Fatalf("la firma se pisó desde otra organización: %q", sig)
	}
}

// Una cuenta que no existe es ErrNotFound, no una firma vacía: si no, un id
// equivocado parecería «cuenta sin firma».
func TestSignatureCuentaInexistente(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	if _, err := st.GetSignature(ctx, uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetSignature(inexistente) = %v, quiero ErrNotFound", err)
	}
	if err := st.SetSignature(ctx, uuid.New(), "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("SetSignature(inexistente) = %v, quiero ErrNotFound", err)
	}
}
