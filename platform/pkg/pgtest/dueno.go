// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package pgtest

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// Dueno dice cómo migra un servicio en el piloto una vez separados los roles
// (INF-01, deploy/helm/bin/separar-roles.sh): con el rol que es
// DUEÑO de su base, sin SUPERUSER ni BYPASSRLS, y con los permisos que le da el
// despliegue y ninguno más.
type Dueno struct {
	// TraspasoSQL es la ruta de deploy/helm/sql/traspasar-propiedad.sql:
	// la base pasa al dueño con el mismo SQL que en el piloto.
	TraspasoSQL string
	// ParametrosSQL es la ruta de deploy/helm/sql/conceder-parametros.sql,
	// que le da SET sobre los parámetros app.* de las funciones. Vacía, el dueño no
	// lo recibe: así se comprueba que sin ese permiso las migraciones fallan.
	ParametrosSQL string
	// SuperusuarioHasta es la versión hasta la que migra el superusuario ANTES del
	// traspaso, como pasó en el piloto, donde toda base nació con el superusuario.
	// Cero: el dueño las aplica todas. Solo hace falta para las que hacen lo que el
	// dueño no puede por diseño (crear el rol auxiliar `docs_app`, concederse su
	// pertenencia), que en producción se hizo antes de traspasar.
	SuperusuarioHasta uint
}

// parametrosMu ordena, dentro del proceso, lo que escribe en pg_parameter_acl:
// esa fila es UNA por parámetro y del servidor, y dos GRANT o DROP OWNED a la vez
// sobre ella acaban en «tuple concurrently updated». Los subtests de un mismo
// paquete corren en paralelo contra el mismo servidor.
var parametrosMu sync.Mutex

// MigrarComoDueno aplica las migraciones de un servicio en una base nueva como las
// aplica el piloto con los roles separados, y devuelve el error de las que aplica
// el dueño (nil si entran todas).
//
// POR QUÉ. New migra con el superusuario del contenedor, y PostgreSQL no le niega
// nada: ni crear un rol, ni una extensión que no sea de confianza, ni escribir un
// parámetro propio en una función. El 2026-11-16 la v3.20.3 llegó al piloto con
// funciones que llevan `SET app.maintenance = 'on'`, que desde PostgreSQL 15 exige
// `GRANT SET ON PARAMETER` a quien no es superusuario, y doce servicios se quedaron
// con la migración sucia: en CI todo estaba en verde. Esto es el camino real.
//
// Los fallos de infraestructura (arrancar, crear la base, el traspaso) acaban el
// test con t.Fatal; lo que se devuelve es solo lo que dicen las migraciones.
func MigrarComoDueno(t *testing.T, fsys fs.FS, dir string, d Dueno) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	dsn, _, terminate, err := baseVacia(ctx)
	if err != nil {
		t.Fatalf("pgtest: base vacía: %v", err)
	}
	var soltar func()
	t.Cleanup(func() {
		// Primero la base y después el rol: el rol no se deja borrar mientras
		// tenga algo suyo.
		terminate()
		if soltar != nil {
			soltar()
		}
	})

	if d.SuperusuarioHasta > 0 {
		if err := migrarHasta(fsys, dir, dsn, d.SuperusuarioHasta); err != nil {
			t.Fatalf("pgtest: el superusuario migra hasta la %04d: %v", d.SuperusuarioHasta, err)
		}
	}

	duenoDSN, soltarDueno, err := traspasarAUnDueno(ctx, dsn, d.TraspasoSQL)
	if err != nil {
		t.Fatalf("pgtest: traspaso al dueño: %v", err)
	}
	soltar = soltarDueno

	if d.ParametrosSQL != "" {
		if err := concederParametros(ctx, dsn, d.ParametrosSQL, duenoDSN); err != nil {
			t.Fatalf("pgtest: permisos sobre los parámetros: %v", err)
		}
	}
	return pg.MigrateUp(fsys, dir, duenoDSN)
}

// migrarHasta aplica, con el DSN dado, las migraciones hasta la versión dada
// incluida. golang-migrate cuenta pasos, no versiones: se cuentan aquí.
func migrarHasta(fsys fs.FS, dir, dsn string, version uint) error {
	nombres, err := fs.Glob(fsys, path.Join(dir, "*.up.sql"))
	if err != nil {
		return err
	}
	pasos := 0
	for _, nombre := range nombres {
		numero, _, _ := strings.Cut(path.Base(nombre), "_")
		v, err := strconv.ParseUint(numero, 10, 64)
		if err != nil {
			return fmt.Errorf("versión de %s: %w", nombre, err)
		}
		if uint(v) <= version {
			pasos++
		}
	}
	if pasos == 0 {
		return fmt.Errorf("no hay migraciones hasta la %04d", version)
	}
	mg, err := pg.NewMigrator(fsys, dir, dsn)
	if err != nil {
		return err
	}
	defer func() { _ = mg.Close() }()
	return mg.Steps(pasos)
}

// concederParametros ejecuta, con el rol privilegiado, el fichero de los
// parámetros nombrando al dueño en `sytue.roles`, como separar-roles.sh.
func concederParametros(ctx context.Context, dsn, ruta, duenoDSN string) error {
	sql, err := os.ReadFile(ruta) //nolint:gosec // G304: la ruta la da el test.
	if err != nil {
		return fmt.Errorf("leer %s: %w", ruta, err)
	}
	cfg, err := pgx.ParseConfig(duenoDSN)
	if err != nil {
		return err
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()

	parametrosMu.Lock()
	defer parametrosMu.Unlock()
	if _, err := conn.Exec(ctx, `SELECT set_config('sytue.roles', $1, false)`, cfg.User); err != nil {
		return err
	}
	_, err = conn.Exec(ctx, string(sql))
	return err
}
