// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pg

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// InTx ejecuta fn dentro de una transacción. Si fn devuelve error o entra en
// pánico, hace rollback; en caso contrario, commit. El rollback es seguro tras
// commit (no-op), de modo que el defer no enmascara el error de commit.
func (p *Pool) InTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return runInTx(ctx, p.Pool, pgx.TxOptions{}, fn)
}

// InTxOpts es como InTx pero permite especificar opciones (nivel de aislamiento).
func (p *Pool) InTxOpts(ctx context.Context, opts pgx.TxOptions, fn func(tx pgx.Tx) error) error {
	return runInTx(ctx, p.Pool, opts, fn)
}

func runInTx(ctx context.Context, pool *pgxpool.Pool, opts pgx.TxOptions, fn func(tx pgx.Tx) error) (err error) {
	tx, err := pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("pg: begin tx: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				err = errors.Join(err, fmt.Errorf("pg: rollback: %w", rbErr))
			}
		}
	}()

	if err = fn(tx); err != nil { //nolint:gocritic // sloppyReassign: err es el named return que lee el defer de rollback; := lo rompería.
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("pg: commit: %w", err)
	}
	return nil
}

// InTenantTx ejecuta fn dentro de una transacción con la variable de sesión
// `app.tenant` fijada al slug del tenant del contexto. Las políticas RLS
// referencian `current_setting('app.tenant', true)` para aislar filas por
// tenant. El ajuste es LOCAL (vive solo en la transacción).
func (p *Pool) InTenantTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	return p.InTx(ctx, func(tx pgx.Tx) error {
		if err := SetTenant(ctx, tx, t.Slug); err != nil {
			return err
		}
		return fn(tx)
	})
}

// SetTenant fija `app.tenant` LOCAL a la transacción/sesión actual. Usa
// set_config con parámetro para evitar inyección SQL.
func SetTenant(ctx context.Context, q Querier, slug string) error {
	if !tenancy.ValidSlug(slug) {
		return fmt.Errorf("pg: slug de tenant inválido: %q", slug)
	}
	// set_config(setting, value, is_local=true): solo afecta a la transacción.
	if _, err := q.Exec(ctx, "SELECT set_config('app.tenant', $1, true)", slug); err != nil {
		return fmt.Errorf("pg: set_config app.tenant: %w", err)
	}
	return nil
}
