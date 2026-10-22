// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Test de integración del gancho de fusión de organizaciones sobre el material
// S/MIME (`platform_merge_prepare`, migración 0013).
//
// Lo que se juega aquí no es un contador: es material criptográfico de un
// cliente. La función BORRA la autoridad de una organización y los certificados
// de sus buzones, así que cada caso está escrito para responder a una pregunta
// concreta: ¿borra cuando debe?, ¿NO borra cuando no debe?, y sobre todo ¿puede
// alguien hacer que borre desde fuera de una fusión?
//
// Ese último no es teórico. Antes de la guarda `app.merge`, una conexión normal
// del servicio —que lleva su `app.tenant` fijado y por tanto ve sus propias
// filas— podía llamar a platform_merge_prepare('mi-org','mi-org') y dejar a su
// organización sin S/MIME. El harness concede EXECUTE al rol de aplicación, así
// que ese camino es exactamente el que tiene un atacante con ejecución de SQL.
package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// seedSMIME deja una autoridad y un certificado de buzón para `tenant`.
//
// La cuenta se crea con el store —y no con un uuid inventado— porque
// `smime_identities.account_id` es una clave ajena a `accounts`: un certificado
// no existe sin su buzón. La escritura del material va dentro de una transacción
// de fusión, que bajo FORCE RLS es la única forma de sembrar varios tenants
// seguidos desde la misma sesión.
func seedSMIME(t *testing.T, st *store.Store, pool *pg.Pool, tenant, address string) {
	t.Helper()
	ctx := tenantCtx(tenant)
	acc := seedAccount(t, ctx, st, address)
	err := pool.InMergeTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO smime_authorities (tenant, cert_pem, key_enc) VALUES ($1, $2, $3)
			 ON CONFLICT (tenant) DO NOTHING`,
			tenant, "CA-"+tenant, "sellada-"+tenant); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO smime_identities (account_id, tenant, address, cert_pem, key_enc, not_after)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			acc.ID, tenant, address, "CERT-"+address, "sellada-"+address,
			time.Now().Add(300*24*time.Hour))
		return err
	})
	if err != nil {
		t.Fatalf("sembrar S/MIME de %s: %v", tenant, err)
	}
}

// contarSMIME cuenta autoridades e identidades de un tenant. Va en transacción
// de fusión para poder mirar ambos lados de una fusión desde la misma sesión.
func contarSMIME(t *testing.T, ctx context.Context, pool *pg.Pool, tenant string) (autoridades, identidades int) {
	t.Helper()
	err := pool.InMergeTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM smime_authorities WHERE tenant = $1`, tenant).Scan(&autoridades); err != nil {
			return err
		}
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM smime_identities WHERE tenant = $1`, tenant).Scan(&identidades)
	})
	if err != nil {
		t.Fatalf("contar S/MIME de %s: %v", tenant, err)
	}
	return autoridades, identidades
}

// prepare llama al gancho como lo hace el motor: dentro de InMergeTx.
func prepare(ctx context.Context, pool *pg.Pool, source, target string) (string, error) {
	var nota *string
	err := pool.InMergeTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT platform_merge_prepare($1, $2)`, source, target).Scan(&nota)
	})
	if err != nil || nota == nil {
		return "", err
	}
	return *nota, nil
}

// Las dos organizaciones tienen autoridad: la del origen no puede moverse (choca
// con la clave primaria) y tampoco debe, porque su ancla no lo tiene instalado
// nadie en la organización resultante. Se descarta con sus certificados, que se
// re-emitirán solos bajo la autoridad del destino al enviar el próximo correo.
func TestMergePrepareDescartaLaAutoridadHuerfana(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := context.Background()

	seedSMIME(t, st, h.Pool, "huerfana", "ana@huerfana.eu")
	seedSMIME(t, st, h.Pool, "destino", "eva@destino.eu")

	nota, err := prepare(ctx, h.Pool, "huerfana", "destino")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if nota == "" {
		t.Fatal("el gancho no dijo nada: descartar material de un cliente en silencio es lo que no puede pasar")
	}

	if a, i := contarSMIME(t, ctx, h.Pool, "huerfana"); a != 0 || i != 0 {
		t.Errorf("del origen quedan %d autoridades y %d identidades, quiero 0 y 0", a, i)
	}
	// Y lo que NO debe tocar: el destino, intacto.
	if a, i := contarSMIME(t, ctx, h.Pool, "destino"); a != 1 || i != 1 {
		t.Errorf("el destino tiene %d autoridades y %d identidades, quiero 1 y 1", a, i)
	}

	// Idempotente: reanudar una fusión no puede volver a borrar (ni fallar).
	if nota, err := prepare(ctx, h.Pool, "huerfana", "destino"); err != nil || nota != "" {
		t.Errorf("repetir el gancho: nota=%q err=%v, quiero silencio y sin error", nota, err)
	}
}

// El destino NO tiene autoridad: no hay conflicto, así que no se descarta nada y
// la del origen se mueve tal cual con el UPDATE de siempre. Es también el caso
// del renombrado de una organización (`id renameorg`), donde el destino todavía
// no existe: descartar ahí sería tirar una autoridad válida y obligar a
// redistribuir el ancla sin ninguna razón.
func TestMergePrepareNoTocaNadaSiElDestinoNoTieneAutoridad(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := context.Background()

	seedSMIME(t, st, h.Pool, "vieja", "p@vieja.eu")

	nota, err := prepare(ctx, h.Pool, "vieja", "nueva")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if nota != "" {
		t.Errorf("no había conflicto y el gancho descartó algo: %q", nota)
	}
	if a, i := contarSMIME(t, ctx, h.Pool, "vieja"); a != 1 || i != 1 {
		t.Errorf("el material del origen se tocó: %d autoridades, %d identidades; quiero 1 y 1", a, i)
	}
}

// La guarda que hace segura a la función. Sin ella, la RLS NO protege: una
// conexión de negocio lleva su `app.tenant` fijado, ve sus propias filas y puede
// pedirle a la función que las borre.
func TestMergePrepareSoloDentroDeUnaFusion(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := context.Background()

	seedSMIME(t, st, h.Pool, "acme", "ada@acme.eu")

	// Transacción de NEGOCIO (tenant fijado, sin app.merge): es el camino que
	// tendría quien consiguiera ejecutar SQL con las credenciales del servicio.
	err := h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant', 'acme', true)`); err != nil {
			return err
		}
		var nota *string
		return tx.QueryRow(ctx, `SELECT platform_merge_prepare($1, $2)`, "acme", "acme").Scan(&nota)
	})
	if err == nil {
		t.Fatal("la función se dejó ejecutar fuera de una fusión: una organización podría borrarse su propio S/MIME")
	}

	// Y con app.merge, pero fusionando una organización consigo misma: mismo daño.
	if _, err := prepare(ctx, h.Pool, "acme", "acme"); err == nil {
		t.Fatal("se aceptó origen = destino: eso no es fusionar, es borrar el S/MIME de una organización")
	}

	// Lo que importa de verdad: después de los dos intentos, el material sigue ahí.
	if a, i := contarSMIME(t, ctx, h.Pool, "acme"); a != 1 || i != 1 {
		t.Fatalf("se perdió material S/MIME: %d autoridades, %d identidades; quiero 1 y 1", a, i)
	}
}
