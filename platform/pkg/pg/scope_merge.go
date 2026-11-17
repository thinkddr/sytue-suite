// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

//go:embed scope_merge_v1.sql
var scopeMergeV1 []byte

// WithScopeBoundMergeDDL installs the common non-writing merge contract only
// where an explicit versioned migration requests it.
func WithScopeBoundMergeDDL(source fs.FS) fs.FS {
	return withMigrationDDL(source, migrationDDLFragment{marker: "-- sytue:scope-bound-merge-v1", sql: scopeMergeV1})
}

var ErrScopeBoundMerge = errors.New("pg: scope-bound storage blocks organization merge")

// TenantMergeInventory distinguishes movable rows from tables whose identity
// must never be relabelled. A missing contract remains an error, not a blocker
// that can silently be skipped.
type TenantMergeInventory struct {
	Tables        []string
	Movable       []string
	ScopeBound    []string
	PrivateStates []string
	Missing       []string
}

func ConsultaInventarioDeFusion(ctx context.Context, q Querier) (TenantMergeInventory, error) {
	var inventory TenantMergeInventory
	rows, err := q.Query(ctx, consultaTablasConTenant)
	if err != nil {
		return inventory, fmt.Errorf("pg: listar contrato de fusión: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var table string
		var movable, bound, states, declared bool
		if err := rows.Scan(&table, &movable, &bound, &states, &declared); err != nil {
			return inventory, fmt.Errorf("pg: leer contrato de fusión: %w", err)
		}
		inventory.Tables = append(inventory.Tables, table)
		switch {
		case bound:
			inventory.ScopeBound = append(inventory.ScopeBound, table)
		case declared:
			inventory.Missing = append(inventory.Missing, table)
		case movable:
			inventory.Movable = append(inventory.Movable, table)
		default:
			inventory.Missing = append(inventory.Missing, table)
		}
		if states {
			inventory.PrivateStates = append(inventory.PrivateStates, table)
		}
	}
	return inventory, rows.Err()
}

// CheckScopeBoundMerge inspects both organizations before hooks or domain writes.
// It never treats the read-only preflight capability as permission to retag a
// cryptographic scope. Cross-service admission fencing is a separate lifecycle.
func CheckScopeBoundMerge(ctx context.Context, tx pgx.Tx, inventory TenantMergeInventory, source, target string) (err error) {
	if !tenancy.ValidSlug(source) || !tenancy.ValidSlug(target) || source == target {
		return fmt.Errorf("pg: invalid merge scopes")
	}
	var previous string
	if err := tx.QueryRow(ctx, `SELECT coalesce(current_setting('app.merge',true),'')`).Scan(&previous); err != nil {
		return err
	}
	defer func() {
		if _, restore := tx.Exec(ctx, `SELECT set_config('app.merge',$1,true),set_config('app.merge_preflight','off',true),set_config('app.merge_source','',true),set_config('app.merge_target','',true)`, previous); err == nil {
			err = restore
		}
	}()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.merge','off',true),set_config('app.merge_preflight','on',true),set_config('app.merge_source',$1,true),set_config('app.merge_target',$2,true)`, source, target); err != nil {
		return err
	}
	for _, table := range inventory.ScopeBound {
		if err := checkScopeBoundRows(ctx, tx, table, source, target, ""); err != nil {
			return err
		}
	}
	if err := SetMerge(ctx, tx); err != nil {
		return err
	}
	for _, table := range inventory.PrivateStates {
		if err := checkScopeBoundRows(ctx, tx, table, source, target, " AND mode<>'legacy'"); err != nil {
			return err
		}
	}
	return nil
}

func checkScopeBoundRows(ctx context.Context, tx pgx.Tx, table, source, target, predicate string) error {
	sql := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE tenant IN($1,$2)%s)", pgx.Identifier{table}.Sanitize(), predicate)
	var exists bool
	if err := tx.QueryRow(ctx, sql, source, target).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%w: %s", ErrScopeBoundMerge, table)
	}
	return nil
}
