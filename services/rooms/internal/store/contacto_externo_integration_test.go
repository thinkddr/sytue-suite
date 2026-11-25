// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// La política de contacto con gente de fuera (mig 0083): cada organización ve la
// suya, una organización sin fila está abierta, ContactoPermitido mira las dos, y en
// una fusión la del origen se descarta si el destino tiene la suya y quien confiaba en
// el origen pasa a confiar en el destino, sin repetidos ni confiar en sí misma.
func TestPoliticaDeContactoExternoYFusion(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := context.Background()
	acme, otra, vieja := tenantCtx("acme"), tenantCtx("otra"), tenantCtx("vieja")

	if p, err := st.PoliticaContactoExterno(acme); err != nil || p.Modo != domain.ContactoAbierto {
		t.Fatalf("sin fila, abierta: %+v %v", p, err)
	}
	for c, p := range map[context.Context]domain.PoliticaContactoExterno{
		acme:  {Modo: domain.ContactoConfianza, Confianza: []string{"vieja", "otra"}},
		otra:  {Modo: domain.ContactoConfianza, Confianza: []string{"vieja"}},
		vieja: {Modo: domain.ContactoCerrado, Confianza: []string{}},
	} {
		if _, err := st.GuardarPoliticaContactoExterno(c, p, "admin"); err != nil {
			t.Fatal(err)
		}
	}
	if ok, err := st.ContactoPermitido(ctx, "otra", "acme"); err != nil || ok {
		t.Fatalf("«otra» no confía en acme: %v %v", ok, err)
	}
	if ok, err := st.ContactoPermitido(ctx, "acme", "vieja"); err != nil || ok {
		t.Fatalf("«vieja» está cerrada: %v %v", ok, err)
	}
	if ok, err := st.ContactoPermitido(ctx, "nueva", "sinfila"); err != nil || !ok {
		t.Fatalf("sin filas, abiertas: %v %v", ok, err)
	}

	// «vieja» se fusiona en «otra»: «otra» conserva la suya, y quien confiaba en
	// «vieja» confía ahora en «otra» (pero «otra» no confía en sí misma).
	err := h.Pool.InMergeTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `SELECT platform_merge_prepare('vieja','otra')`)
		return err
	})
	if err != nil {
		t.Fatalf("preparar la fusión: %v", err)
	}
	if p, _ := st.PoliticaContactoExterno(otra); p.Modo != domain.ContactoConfianza || len(p.Confianza) != 0 {
		t.Fatalf("la de «otra» tras la fusión: %+v", p)
	}
	if p, _ := st.PoliticaContactoExterno(vieja); p.Modo != domain.ContactoAbierto {
		t.Fatalf("la de «vieja» se descartó: %+v", p)
	}
	p, _ := st.PoliticaContactoExterno(acme)
	slices.Sort(p.Confianza)
	if !slices.Equal(p.Confianza, []string{"otra"}) {
		t.Fatalf("acme confía en %v, quiero solo «otra»", p.Confianza)
	}
}
