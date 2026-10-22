// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El CHECK `shares_link_token` es una verja de seguridad, no una formalidad del
// esquema: un share kind='link' SIN credencial sería una URL pública que abre
// cualquiera que dé con la ruta. La migración 0019 lo reescribió para aceptar el
// hash (0018 dejó de guardar el token en claro) y al reescribir un CHECK es fácil
// dejarlo aceptándolo todo — el resto de la suite no lo notaría, porque el store
// siempre rellena el hash. Esto sí.
package store_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

func TestDriveEnlaceSinCredencialRechazado(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	root, err := store.New(h.Pool).EnsureRoot(tenantCtx("acme"))
	if err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}

	// SQL crudo y con el rol dueño a propósito: lo que se comprueba es que lo
	// para la BASE, no que el store no lo intente.
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, h.DSN)
	if err != nil {
		t.Fatalf("conectar: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	// Sin nombrar `token`: 0020 borró esa columna. Nombrarla haría fallar el
	// INSERT por «column does not exist», que es un error distinto del que este
	// test viene a comprobar —y lo daría por bueno igual.
	_, err = conn.Exec(ctx,
		`INSERT INTO shares (tenant, folder_id, kind, permission, token_hash)
		 VALUES ('acme', $1, 'link', 'read', NULL)`, root.ID)
	if err == nil {
		t.Fatal("se insertó un enlace público sin token ni hash: lo abriría cualquiera")
	}
	if !strings.Contains(err.Error(), "shares_link_token") {
		t.Fatalf("lo rechazó otra cosa, no la verja: %v", err)
	}

	// Y con hash SÍ entra: la verja no puede quedar cerrada para el camino bueno,
	// que es justo el que 0018 estrenó.
	if _, err := conn.Exec(ctx,
		`INSERT INTO shares (tenant, folder_id, kind, permission, token_hash)
		 VALUES ('acme', $1, 'link', 'read', repeat('a', 64))`, root.ID); err != nil {
		t.Fatalf("un enlace con solo hash debe entrar: %v", err)
	}
}

// TestDriveTokenEnClaroYaNoExiste comprueba el contract de 0020.
//
// El par expand→contract empezó en 0018 (guardar el HASH en vez del token) y se
// cierra aquí borrando la columna. Mientras existía, los enlaces creados ANTES de
// 0018 conservaban su cadena en TEXTO PLANO, y un enlace público de Drive es una
// credencial portadora: quien la tiene, abre el fichero. Cualquier vía que lea la
// tabla —una copia de seguridad, un volcado para depurar, un SELECT de alguien
// con acceso al motor— los entregaba enteros.
//
// Se comprueban las dos mitades, porque una sola no demuestra nada: que la
// columna YA NO ESTÁ, y que crear y resolver un enlace sigue funcionando sin
// ella. Borrar la columna y romper los enlaces no sería un arreglo.
func TestDriveTokenEnClaroYaNoExiste(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := tenantCtx("acme")
	st := store.New(h.Pool)

	var existe bool
	if err := h.Pool.QueryRow(context.Background(),
		`SELECT EXISTS (
		   SELECT 1 FROM information_schema.columns
		   WHERE table_name = 'shares' AND column_name = 'token')`).Scan(&existe); err != nil {
		t.Fatalf("consultar el esquema: %v", err)
	}
	if existe {
		t.Error("la columna shares.token sigue en el esquema: los tokens de antes de 0018 " +
			"continúan en claro en la base de datos")
	}

	// La verja tiene que SEGUIR ahí. `DROP COLUMN` borra en cascada los CHECK que
	// nombran la columna, así que este es el caso que separa un contract correcto
	// de uno que abre un agujero sin decirlo.
	var verja bool
	if err := h.Pool.QueryRow(context.Background(),
		`SELECT EXISTS (
		   SELECT 1 FROM pg_constraint
		   WHERE conname = 'shares_link_token' AND conrelid = 'shares'::regclass)`).Scan(&verja); err != nil {
		t.Fatalf("consultar el constraint: %v", err)
	}
	if !verja {
		t.Error("el CHECK shares_link_token desapareció con la columna: se podría crear " +
			"un enlace público SIN credencial, que abre cualquiera que dé con la ruta")
	}

	// Y el enlace tiene que seguir funcionando sin la columna.
	root, err := st.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}
	token := strings.Repeat("t", 43)
	sh, err := st.CreateShare(ctx, store.NewShareRecord{
		FolderID: &root.ID, Kind: "link", Permission: "read",
		Token: token, CreatedBy: "ana",
	})
	if err != nil {
		t.Fatalf("CreateShare tras el contract: %v", err)
	}
	got, _, _, err := st.ResolveLink(context.Background(), token)
	if err != nil {
		t.Fatalf("ResolveLink tras el contract: %v", err)
	}
	if got.ID != sh.ID {
		t.Errorf("el enlace resuelve a %s y no a %s", got.ID, sh.ID)
	}
}
