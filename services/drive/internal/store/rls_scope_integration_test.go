// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// DEMOSTRACIÓN de que la RLS se EJERCITA de verdad en el harness (pgtest conecta
// con un rol de aplicación sin superusuario y sin BYPASSRLS).
//
// Estos tests son deliberadamente distintos del resto del paquete: NO pasan por el
// store, hablan SQL crudo contra h.Pool. Así el único mecanismo que puede ocultar
// una fila es la POLÍTICA de la migración — no hay predicado Go, ni parent_id, ni
// tenant=$1 que pueda estar haciendo el trabajo por ella. Si alguien borrase la
// política `folders_scope_isolation` de la migración 0014 (o la 0010, que la
// introdujo), o si el harness volviera a conectar con un superusuario, estos tests
// fallarían. Es la razón de su existencia: hasta ahora NADA verificaba el ámbito de
// unidad de equipo ni el de canal, porque el pool saltaba la RLS.
//
// Lo que NO prueban: quién puede leer un adjunto (eso es la verja de membresía del
// canal en service/channelfiles.go, verificada contra Sytue Rooms).
package store_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
)

// countFolders cuenta TODAS las carpetas visibles en la transacción, sin ningún
// filtro: el recuento es exactamente «lo que la RLS deja ver» con las GUC dadas.
// scope aplica las GUC de ámbito (app.team_drive / app.channel) o ninguna.
func countFolders(t *testing.T, h *pgtest.Harness, tenant string, scope func(pgx.Tx) error) int {
	t.Helper()
	var n int
	err := h.Pool.InTx(context.Background(), func(tx pgx.Tx) error {
		if err := pg.SetTenant(context.Background(), tx, tenant); err != nil {
			return err
		}
		if scope != nil {
			if err := scope(tx); err != nil {
				return err
			}
		}
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM folders`).Scan(&n)
	})
	if err != nil {
		t.Fatalf("contar carpetas (tenant=%s): %v", tenant, err)
	}
	return n
}

func setGUC(key, val string) func(pgx.Tx) error {
	return func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), "SELECT set_config($1, $2, true)", key, val)
		return err
	}
}

// TestRLSEnforcedByHarness comprueba primero lo que hace posible todo lo demás: el
// rol del pool NO es superusuario y NO tiene BYPASSRLS. Sin esto, PostgreSQL no
// evalúa ninguna política y los tests de aislamiento de la suite son vacíos.
func TestRLSEnforcedByHarness(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()

	var user string
	var super, bypass bool
	if err := h.Pool.QueryRow(ctx,
		`SELECT current_user, rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user`).
		Scan(&user, &super, &bypass); err != nil {
		t.Fatalf("consultar pg_roles: %v", err)
	}
	t.Logf("rol del pool: %s (rolsuper=%v, rolbypassrls=%v)", user, super, bypass)
	if super || bypass {
		t.Fatalf("el pool conecta como %q con rolsuper=%v rolbypassrls=%v: PostgreSQL NO aplica RLS a ese rol, "+
			"así que NINGÚN test de aislamiento de la suite prueba nada", user, super, bypass)
	}

	// Deny-by-default: sin `app.tenant` fijado la política no casa ninguna fila, y
	// el WITH CHECK rechaza el INSERT. Es el contrato de pg.TenantRLSPolicySQL.
	_, err := h.Pool.Exec(ctx, `INSERT INTO folders (tenant, name, is_root) VALUES ('acme', 'raiz', true)`)
	if err == nil {
		t.Fatal("INSERT sin app.tenant aceptado: el WITH CHECK de la política RLS no se está evaluando")
	}
	t.Logf("INSERT sin app.tenant rechazado por la RLS, como debe: %v", err)
}

// TestRLSTeamDriveScopeIsEnforcedByThePolicy es la prueba de fuego del ámbito de
// unidad de equipo (migración 0010). Ninguna consulta de lectura del store filtra
// por `team_drive_id` —es el diseño declarado en teamdrive.go: «el ámbito lo pone la
// política, no la consulta»—, así que este aislamiento SOLO lo garantiza la RLS.
func TestRLSTeamDriveScopeIsEnforcedByThePolicy(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	drive := uuid.New()

	// Carpeta de una unidad de equipo: hay que insertarla DENTRO del ámbito, o el
	// WITH CHECK de la política la rechaza (ya es una garantía en sí).
	if err := h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, "acme"); err != nil {
			return err
		}
		if err := setGUC("app.team_drive", drive.String())(tx); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO folders (tenant, name, is_root, team_drive_id) VALUES ('acme', 'solo-unidad', false, $1)`,
			drive)
		return err
	}); err != nil {
		t.Fatalf("insertar carpeta de unidad: %v", err)
	}

	// Insertarla desde el ámbito PERSONAL debe ser imposible (WITH CHECK).
	if err := h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, "acme"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO folders (tenant, name, is_root, team_drive_id) VALUES ('acme', 'colada', false, $1)`,
			drive)
		return err
	}); err == nil {
		t.Fatal("se pudo insertar contenido de una unidad desde el ámbito personal: la política no filtra el WITH CHECK")
	}

	if n := countFolders(t, h, "acme", setGUC("app.team_drive", drive.String())); n != 1 {
		t.Fatalf("dentro de la unidad se ven %d carpetas, quiero 1", n)
	}
	// Lo que la RLS y solo la RLS impide: verla desde «Mi unidad».
	if n := countFolders(t, h, "acme", nil); n != 0 {
		t.Fatalf("el ámbito personal ve %d carpetas de la unidad de equipo, quiero 0 "+
			"(fuga: la política folders_scope_isolation no está aplicándose)", n)
	}
	// Ni desde OTRA unidad del mismo tenant.
	if n := countFolders(t, h, "acme", setGUC("app.team_drive", uuid.New().String())); n != 0 {
		t.Fatalf("otra unidad ve %d carpetas ajenas, quiero 0", n)
	}
	// Ni desde otro tenant, ni acertando el uuid de la unidad.
	if n := countFolders(t, h, "globex", setGUC("app.team_drive", drive.String())); n != 0 {
		t.Fatalf("otro tenant ve %d carpetas, quiero 0 (fuga CROSS-TENANT)", n)
	}
}

// TestRLSChannelScopeIsEnforcedByThePolicy hace lo propio con el ámbito de canal de
// la migración 0014 (`app.channel`), el que impide que un adjunto de un canal
// privado o de un DM sea visible en «Mi unidad» —que es un espacio compartido del
// tenant— y por tanto descargable por cualquier miembro de la organización.
func TestRLSChannelScopeIsEnforcedByThePolicy(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	channel, otro := uuid.New(), uuid.New()

	if err := h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, "acme"); err != nil {
			return err
		}
		if err := setGUC("app.channel", channel.String())(tx); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO folders (tenant, name, is_root, channel_id) VALUES ('acme', $1, false, $2)`,
			channel.String(), channel)
		return err
	}); err != nil {
		t.Fatalf("insertar carpeta de canal: %v", err)
	}

	if n := countFolders(t, h, "acme", setGUC("app.channel", channel.String())); n != 1 {
		t.Fatalf("dentro del canal se ven %d carpetas, quiero 1", n)
	}
	if n := countFolders(t, h, "acme", nil); n != 0 {
		t.Fatalf("«Mi unidad» ve %d carpetas del canal, quiero 0 "+
			"(fuga: un adjunto de canal privado sería descargable por toda la org)", n)
	}
	if n := countFolders(t, h, "acme", setGUC("app.channel", otro.String())); n != 0 {
		t.Fatalf("otro canal ve %d carpetas ajenas, quiero 0", n)
	}
	if n := countFolders(t, h, "globex", setGUC("app.channel", channel.String())); n != 0 {
		t.Fatalf("otro tenant ve %d carpetas del canal, quiero 0 (fuga CROSS-TENANT)", n)
	}
	// Los ámbitos son EXCLUYENTES: desde una unidad de equipo tampoco se ve.
	if n := countFolders(t, h, "acme", func(tx pgx.Tx) error {
		return setGUC("app.team_drive", uuid.New().String())(tx)
	}); n != 0 {
		t.Fatalf("una unidad de equipo ve %d carpetas de canal, quiero 0", n)
	}
}

// TestRLSOwnerScopeIsEnforcedByThePolicy comprueba que la política de la
// migración 0032 aísla «Mi unidad» (team_drive_id IS NULL AND channel_id IS NULL)
// por persona (`app.owner_sub`) dentro de la misma organización.
func TestRLSOwnerScopeIsEnforcedByThePolicy(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()

	if err := h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, "acme"); err != nil {
			return err
		}
		if err := setGUC("app.owner_sub", "alice")(tx); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO folders (tenant, name, is_root, owner_sub) VALUES ('acme', 'privada-alice', false, 'alice')`)
		return err
	}); err != nil {
		t.Fatalf("insertar carpeta personal de alice: %v", err)
	}

	// Bob no puede insertar una carpeta a nombre de Alice desde su propio ámbito (WITH CHECK).
	if err := h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, "acme"); err != nil {
			return err
		}
		if err := setGUC("app.owner_sub", "bob")(tx); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO folders (tenant, name, is_root, owner_sub) VALUES ('acme', 'suplantada', false, 'alice')`)
		return err
	}); err == nil {
		t.Fatal("bob pudo insertar una carpeta con owner_sub='alice': el WITH CHECK de la política no filtra por app.owner_sub")
	}

	if n := countFolders(t, h, "acme", setGUC("app.owner_sub", "alice")); n != 1 {
		t.Fatalf("alice ve %d carpetas en su unidad, quiero 1", n)
	}
	if n := countFolders(t, h, "acme", setGUC("app.owner_sub", "bob")); n != 0 {
		t.Fatalf("bob ve %d carpetas de la unidad personal de alice, quiero 0 (fuga entre personas del mismo tenant)", n)
	}
	if n := countFolders(t, h, "acme", nil); n != 0 {
		t.Fatalf("sin app.owner_sub se ven %d carpetas de alice, quiero 0", n)
	}
}

