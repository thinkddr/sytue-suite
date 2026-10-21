// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// «Que entren como invitados» (decidido por el dueño el 2026-10-16): quien viene de otra
// organización a la reunión de un evento llega con un token de la SUYA. Para llevarle a
// la anfitriona hay que saber dónde vive la sala; se sabe solo de las salas de un evento.
//
// Visto en rojo contra main: sin la política de la 0042 y esta búsqueda, la reunión no
// aparecía desde la otra organización y el invitado no podía ni intentar entrar.
func TestLaReunionDeUnEventoLlevaAlDeFueraASuAnfitriona(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	acme := tenantCtx("acme")
	var deEvento, suelta string
	if err := h.Pool.InTenantTx(acme, func(tx pgx.Tx) error {
		if err := tx.QueryRow(acme, `INSERT INTO rooms (tenant, name, title, metadata)
			VALUES ('acme', 'acme_evento', 'Evento', '{"source":{"kind":"calendar","ref_id":"uid-1"}}')
			RETURNING id::text`).Scan(&deEvento); err != nil {
			return err
		}
		return tx.QueryRow(acme, `INSERT INTO rooms (tenant, name, title) VALUES ('acme', 'acme_suelta', 'Suelta')
			RETURNING id::text`).Scan(&suelta)
	}); err != nil {
		t.Fatal(err)
	}
	bg := context.Background()
	if got, err := st.AnfitrionDeReunionDeEvento(bg, "otra", uuid.MustParse(deEvento)); err != nil || got != "acme" {
		t.Fatalf("la reunión del evento, desde otra organización: %q (%v)", got, err)
	}
	if got, _ := st.AnfitrionDeReunionDeEvento(bg, "otra", uuid.MustParse(suelta)); got != "" {
		t.Fatalf("una reunión directa no se abre a nadie de fuera: %q", got)
	}
	if got, _ := st.AnfitrionDeReunionDeEvento(bg, "acme", uuid.MustParse(deEvento)); got != "" {
		t.Fatalf("desde su propia organización no hay que cambiar de sitio: %q", got)
	}
}
