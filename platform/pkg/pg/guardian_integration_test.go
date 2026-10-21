// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El guardián de arranque contra un PostgreSQL de verdad, con los dos roles que
// importan: el privilegiado del arnés —superusuario, que es con el que conectaba
// el piloto entero el 2026-10-06 (INF-01)— y el de aplicación.
package pg_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
)

func TestConnectRechazaUnRolQueEludeLaRLS(t *testing.T) {
	h := pgtest.New(t, os.DirFS("."), "migrations")
	ctx := context.Background()

	// 1. Superusuario (h.DSN): Connect debe negarse a abrir el pool.
	priv, err := pg.Connect(ctx, pg.Config{DSN: h.DSN, MaxConns: 1})
	if err == nil {
		priv.Close()
		t.Fatal("Connect aceptó un rol superusuario que elude la RLS")
	}
	if !strings.Contains(err.Error(), "elude la seguridad por filas") ||
		!strings.Contains(err.Error(), "superusuario=true") {
		t.Fatalf("el error no explica que el rol es superusuario: %v", err)
	}

	// 2. Rol sin superusuario pero con BYPASSRLS: también elude todas las políticas.
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	rolBypass := "bypass_" + hex.EncodeToString(b[:])
	claveBypass := "clave_" + hex.EncodeToString(b[:])

	adm, err := pgx.Connect(ctx, h.DSN)
	if err != nil {
		t.Fatalf("conectar con pgx al rol privilegiado: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adm.Exec(ctx, `DROP ROLE IF EXISTS `+rolBypass)
		_ = adm.Close(ctx)
	})
	if _, err := adm.Exec(ctx,
		`CREATE ROLE `+rolBypass+` LOGIN PASSWORD '`+claveBypass+`' NOSUPERUSER BYPASSRLS`); err != nil {
		t.Fatalf("crear rol con BYPASSRLS: %v", err)
	}
	u, err := url.Parse(h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(rolBypass, claveBypass)

	byp, err := pg.Connect(ctx, pg.Config{DSN: u.String(), MaxConns: 1})
	if err == nil {
		byp.Close()
		t.Fatal("Connect aceptó un rol con BYPASSRLS")
	}
	if !strings.Contains(err.Error(), "elude la seguridad por filas") ||
		!strings.Contains(err.Error(), "bypassrls=true") {
		t.Fatalf("el error no explica que el rol tiene BYPASSRLS: %v", err)
	}

	// 3. Pool principal con rol de aplicación pero DATABASE_DIRECT_URL apuntando a superusuario.
	t.Run("direct_dsn_superusuario", func(t *testing.T) {
		t.Setenv(pg.DirectDSNEnv, h.DSN)
		p, err := pg.Connect(ctx, pg.Config{DSN: h.AppDSN, MaxConns: 1})
		if err == nil {
			p.Close()
			t.Fatal("Connect aceptó DATABASE_DIRECT_URL con un rol superusuario")
		}
		if !strings.Contains(err.Error(), "elude la seguridad por filas") {
			t.Fatalf("el error de DATABASE_DIRECT_URL no explica el motivo: %v", err)
		}
	})

	// 4. Con el rol de aplicación (NOSUPERUSER, NOBYPASSRLS), Connect abre el pool.
	app, err := pg.Connect(ctx, pg.Config{DSN: h.AppDSN, MaxConns: 1})
	if err != nil {
		t.Fatalf("conectar con el rol de aplicación: %v", err)
	}
	app.Close()
}

// Resolve probaba que el rol de aplicación se pudiera ASUMIR, no qué se asumía:
// un `docs_app` con BYPASSRLS pasaba por «RLS + rol dedicado» con las políticas
// apagadas.
func TestResolveRechazaUnRolDeAplicacionQueEludeLaRLS(t *testing.T) {
	h := pgtest.New(t, os.DirFS("."), "migrations")
	ctx := context.Background()

	u, err := url.Parse(h.AppDSN)
	if err != nil {
		t.Fatalf("AppDSN: %v", err)
	}
	// Los roles son del servidor, no de la base, y el servidor se comparte entre
	// tests: nombre propio y limpieza al acabar.
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	rol := "trampa_" + hex.EncodeToString(b[:]) + "_app"

	priv, err := pgx.Connect(ctx, h.DSN)
	if err != nil {
		t.Fatalf("conectar con el rol privilegiado: %v", err)
	}
	t.Cleanup(func() {
		_, _ = priv.Exec(ctx, `DROP OWNED BY `+rol)
		_, _ = priv.Exec(ctx, `DROP ROLE IF EXISTS `+rol)
		_ = priv.Close(ctx)
	})
	for _, sql := range []string{
		`CREATE ROLE ` + rol + ` NOLOGIN BYPASSRLS`,
		`GRANT ` + rol + ` TO ` + u.User.Username(),
		`GRANT USAGE ON SCHEMA public TO ` + rol,
		`GRANT SELECT ON ALL TABLES IN SCHEMA public TO ` + rol,
	} {
		if _, err := priv.Exec(ctx, sql); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}

	_, err = pg.NewAppRole(rol).Resolve(ctx, h.Pool)
	if err == nil {
		t.Fatal("Resolve aceptó un rol de aplicación con BYPASSRLS")
	}
	if !strings.Contains(err.Error(), "elude RLS") {
		t.Fatalf("el error no explica el motivo: %v", err)
	}

	// Lo que bloquea es el atributo, no el rol: sin BYPASSRLS se asume y queda activo.
	if _, err := priv.Exec(ctx, `ALTER ROLE `+rol+` NOBYPASSRLS`); err != nil {
		t.Fatalf("quitar BYPASSRLS: %v", err)
	}
	ar := pg.NewAppRole(rol)
	if _, err := ar.Resolve(ctx, h.Pool); err != nil {
		t.Fatalf("sin BYPASSRLS debería aceptar: %v", err)
	}
	if !ar.Activo() {
		t.Fatal("el rol se puede asumir y no elude RLS, pero no quedó activo")
	}
}
