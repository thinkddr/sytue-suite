// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pg

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"regexp"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Migrator aplica migraciones versionadas desde un fs.FS embebido (cada
// servicio embebe su carpeta db/migrations con //go:embed).
type Migrator struct {
	m *migrate.Migrate
}

// NewMigrator construye un Migrator a partir de un fs.FS y el DSN. `dir` es la
// subcarpeta dentro del FS que contiene los ficheros .up.sql/.down.sql
// (típicamente "."). El DSN debe usar el esquema "pgx5://" o "postgres://".
//
// Si DATABASE_DIRECT_URL está definida, las migraciones usan ese DSN directo:
// golang-migrate serializa con pg_advisory_lock (lock de SESIÓN), que a través
// de PgBouncer en pool_mode=transaction se rompe (lock y unlock pueden caer en
// conexiones de servidor distintas => exclusión mutua rota y locks filtrados).
func NewMigrator(fsys fs.FS, dir, dsn string) (*Migrator, error) {
	return newMigrator(fsys, dir, dsn, "")
}

var migrationTableName = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

func migrationDSN(dsn, table string) (string, error) {
	u, err := url.Parse(normalizeDSN(directDSN(dsn)))
	if err != nil {
		return "", fmt.Errorf("pg: URL de migración: %w", err)
	}
	if table != "" {
		if !migrationTableName.MatchString(table) {
			return "", errors.New("pg: nombre de tabla de migraciones inválido")
		}
		query := u.Query()
		query.Set("x-migrations-table", table)
		u.RawQuery = query.Encode()
	}
	return u.String(), nil
}

func newMigrator(fsys fs.FS, dir, dsn, table string) (*Migrator, error) {
	target, err := migrationDSN(dsn, table)
	if err != nil {
		return nil, err
	}
	src, err := iofs.New(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("pg: fuente de migraciones: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, target)
	if err != nil {
		return nil, fmt.Errorf("pg: instancia de migración: %w", err)
	}
	return &Migrator{m: m}, nil
}

// normalizeDSN garantiza el esquema que el driver pgx/v5 de golang-migrate
// espera ("pgx5://"). Acepta DSNs "postgres://"/"postgresql://".
func normalizeDSN(dsn string) string {
	switch {
	case len(dsn) >= 7 && dsn[:7] == "pgx5://":
		return dsn
	case len(dsn) >= 11 && dsn[:11] == "postgres://":
		return "pgx5://" + dsn[11:]
	case len(dsn) >= 13 && dsn[:13] == "postgresql://":
		return "pgx5://" + dsn[13:]
	default:
		return dsn
	}
}

// Up aplica todas las migraciones pendientes. No es error si ya está al día.
func (mg *Migrator) Up() error {
	if err := mg.m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("pg: migrate up: %w", err)
	}
	return nil
}

// Steps aplica (n>0) o revierte (n<0) n migraciones (uso operativo, no en arranque).
func (mg *Migrator) Steps(n int) error {
	if err := mg.m.Steps(n); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("pg: migrate steps(%d): %w", n, err)
	}
	return nil
}

// Version devuelve la versión actual y si el esquema está "dirty".
func (mg *Migrator) Version() (version uint, dirty bool, err error) {
	v, d, err := mg.m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, nil
	}
	return v, d, err
}

// Close libera los recursos del migrador.
func (mg *Migrator) Close() error {
	srcErr, dbErr := mg.m.Close()
	return errors.Join(srcErr, dbErr)
}

// MigrateUp es un atajo: abre el migrador desde el FS, aplica Up y cierra.
func MigrateUp(fsys fs.FS, dir, dsn string) error {
	mg, err := NewMigrator(fsys, dir, dsn)
	if err != nil {
		return err
	}
	defer mg.Close()
	return mg.Up()
}

// MigrateUpTable gives a shared package its own migration ledger. The table
// option is applied after selecting DATABASE_DIRECT_URL, so production's
// PgBouncer bypass cannot silently replace it with the service's ledger.
func MigrateUpTable(fsys fs.FS, dir, dsn, table string) error {
	mg, err := newMigrator(fsys, dir, dsn, table)
	if err != nil {
		return err
	}
	defer mg.Close()
	return mg.Up()
}
