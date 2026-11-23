// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"context"
	_ "embed"
	"fmt"
	"io/fs"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

//go:embed scope_admission_v1.sql
var scopeAdmissionV1 []byte

func WithScopeAdmissionDDL(source fs.FS) fs.FS {
	return withMigrationDDL(source, migrationDDLFragment{marker: "-- sytue:scope-admission-v1", sql: scopeAdmissionV1})
}

// BindScopeAdmissionSnapshot captures authority's transaction boundary before
// application callbacks. The trigger compares transition XIDs with this original
// snapshot, including when Read Committed subsequently sees a reopened row.
func BindScopeAdmissionSnapshot(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `SELECT set_config('app.scope_admission_snapshot',pg_current_snapshot()::text,true)`)
	if err != nil {
		return fmt.Errorf("pg: bind scope admission snapshot: %w", err)
	}
	return nil
}

func ScopeAdmissionInstalled(ctx context.Context, q Querier) (bool, error) {
	var installed bool
	err := q.QueryRow(ctx, `SELECT to_regclass('private_scope_admission_control') IS NOT NULL AND to_regprocedure('sytue_scope_admission_transition(uuid,text,text,text)') IS NOT NULL`).Scan(&installed)
	return installed, err
}

// TransitionScopeAdmission only changes infrastructure admission metadata.
// It never rewrites an application's tenant, pin, root, reference or content.
func TransitionScopeAdmission(ctx context.Context, tx pgx.Tx, job uuid.UUID, source, target, phase string) error {
	if job == uuid.Nil || !tenancy.ValidSlug(source) || !tenancy.ValidSlug(target) || source == target {
		return fmt.Errorf("pg: invalid scope admission merge")
	}
	if phase != "close" && phase != "abort" && phase != "complete" {
		return fmt.Errorf("pg: invalid scope admission phase")
	}
	if err := SetMerge(ctx, tx); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `SELECT sytue_scope_admission_transition($1,$2,$3,$4)`, job, source, target, phase)
	return err
}

// RequireScopeAdmissionCoverage rejects a partially deployed or disabled guard.
// A table being Scope-bound is not proof that its future writes are fenced.
func RequireScopeAdmissionCoverage(ctx context.Context, q Querier, inventory TenantMergeInventory) error {
	installed, err := ScopeAdmissionInstalled(ctx, q)
	if err != nil {
		return err
	}
	if !installed {
		return fmt.Errorf("pg: scope admission schema is required")
	}
	for _, group := range []struct {
		tables []string
		class  string
	}{{inventory.ScopeBound, "bound"}, {inventory.PrivateStates, "state"}} {
		for _, table := range group.tables {
			if group.class == "state" {
				var immutable bool
				if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid=to_regclass($1) AND NOT t.tgisinternal AND t.tgenabled IN('O','A') AND t.tgtype=27 AND t.tgfoid IN (to_regprocedure('sytue_private_storage_state_scope_guard()'),to_regprocedure('sytue_private_storage_state_scope_guard_v2()')))`, table).Scan(&immutable); err != nil {
					return err
				}
				if !immutable {
					return fmt.Errorf("pg: immutable storage state guard missing or disabled on %s", table)
				}
			}
			args := append([]byte(group.class), 0)
			var covered bool
			err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid=to_regclass($1) AND NOT t.tgisinternal AND t.tgenabled IN('O','A') AND t.tgname='scope_bound_admission' AND t.tgtype=23 AND t.tgfoid=to_regprocedure('sytue_scope_admission_data_guard()') AND t.tgargs=$2::bytea)
 AND EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid=to_regclass($1) AND NOT t.tgisinternal AND t.tgenabled IN('O','A') AND t.tgname='scope_bound_admission_commit' AND t.tgtype=21 AND t.tgdeferrable AND t.tginitdeferred AND t.tgfoid=to_regprocedure('sytue_scope_admission_data_guard()') AND t.tgargs=$2::bytea)`, table, args).Scan(&covered)
			if err != nil {
				return err
			}
			if !covered {
				return fmt.Errorf("pg: scope admission guard missing or disabled on %s", table)
			}
		}
	}
	return nil
}
