// SPDX-License-Identifier: Apache-2.0
package callaccess

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Flush runs before the enclosing Docs write, import, move or purge commits.
// An empty tenant is reserved for platform merge transactions (app.merge=on).
// Other service databases have no document fence table and require no action.
func Flush(ctx context.Context, tx pgx.Tx, tenant string, revoker Revoker) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('public.doc_call_access_fences') IS NOT NULL`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return nil
	}
	var enabled bool
	if err := tx.QueryRow(ctx, `SELECT enabled FROM doc_call_guard_state WHERE singleton`).Scan(&enabled); err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT document_id::text,tenant,version FROM doc_call_access_fences WHERE ($1='' OR tenant=$1) AND transaction_id=pg_current_xact_id_if_assigned() AND version>acknowledged_version ORDER BY document_id`, tenant)
	if err != nil {
		return err
	}
	type fence struct {
		id, tenant string
		version    int64
	}
	var changed []fence
	for rows.Next() {
		var f fence
		if err := rows.Scan(&f.id, &f.tenant, &f.version); err != nil {
			rows.Close()
			return err
		}
		changed = append(changed, f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, f := range changed {
		if revoker == nil {
			return ErrUnavailable
		}
		if err := revoker.RevokeDocumentCalls(ctx, f.id, f.tenant, f.version); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE doc_call_access_fences SET acknowledged_version=$2 WHERE document_id=$1 AND version=$2`, f.id, f.version); err != nil {
			return err
		}
	}
	return nil
}
