// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package store implementa la persistencia de Sytue ID sobre PostgreSQL con
// pgx (datastore único, sin Redis/Valkey). Todas las consultas son
// parametrizadas (sin SQL dinámico) por seguridad (docs/CONVENTIONS.md §0.7).
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Store agrupa el acceso a datos de Sytue ID. Sus métodos reciben un pg.Querier
// (pool o tx) como primer argumento, de modo que cada operación funciona dentro
// o fuera de una transacción sin duplicar consultas.
type Store struct {
	db *pg.Pool
	// keyCipher cifra/descifra el material sensible de claves (private_pem) en
	// reposo (F9). nil ⇒ sin cifrado (solo dev); en prod se exige (ver main).
	keyCipher *keycrypto.Cipher
	// suiteDomain es el dominio del que cuelgan los subdominios de organización
	// (sytue.eu ⇒ acme.sytue.eu), base de las direcciones de correo que se asignan
	// al dar de alta. Lo fija SetSuiteDomain; vacío ⇒ no se reparten buzones.
	suiteDomain              string
	externalAccess           ExternalAccessGuard
	callIdentity             callaccess.SubjectRevoker
	callIdentityProofEnabled bool
}

// New construye un Store sobre el pool dado, sin cifrado de claves (dev).
func New(db *pg.Pool) *Store { return &Store{db: db} }

// NewWithCipher construye un Store con cifrado en reposo del material de claves.
func NewWithCipher(db *pg.Pool, c *keycrypto.Cipher) *Store {
	return &Store{db: db, keyCipher: c}
}

// DB devuelve el Querier del pool para operaciones fuera de transacción.
func (s *Store) DB() pg.Querier { return s.db.Pool }

// Healthz delega en el pool (para /readyz).
func (s *Store) Healthz(ctx context.Context) error { return s.db.Healthz(ctx) }

// InTx ejecuta fn dentro de una transacción; el Querier pasado es la tx.
func (s *Store) InTx(ctx context.Context, fn func(q pg.Querier) error) error {
	return s.db.InTx(ctx, func(t pgx.Tx) error {
		if err := fn(t); err != nil {
			return err
		}
		return s.flushCallIdentity(ctx, t)
	})
}

// pgErr convierte errores de pgx a errores de dominio estables.
func pgErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" { // unique_violation
		return domain.ErrConflict
	}
	return err
}

func errf(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("store: %s: %w", op, pgErr(err))
}
