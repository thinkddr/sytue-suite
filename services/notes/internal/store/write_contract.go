// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	notesdb "git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// Keep the existing transaction/RLS helpers. The capability is explicitly
// stamped by this binary inside each write transaction, never by its caller.
func (s *Store) inWriteTx(ctx context.Context, fn func(pgx.Tx) error) error {
	return s.pool.InTenantTx(ctx, withWriteContract(ctx, fn))
}

func (s *Store) inMaintenanceWriteTx(ctx context.Context, fn func(pgx.Tx) error) error {
	return s.pool.InMaintenanceTx(ctx, withWriteContract(ctx, fn))
}

func withWriteContract(ctx context.Context, fn func(pgx.Tx) error) func(pgx.Tx) error {
	return func(tx pgx.Tx) error {
		if err := notesdb.MarkWriteTransaction(ctx, tx); err != nil {
			return err
		}
		err := fn(tx)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "55000" && pgErr.ConstraintName == notesdb.WriteContractConstraint {
			return domain.ErrBackendWriteUpgrade
		}
		return err
	}
}
