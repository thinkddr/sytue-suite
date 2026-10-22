// SPDX-License-Identifier: Apache-2.0
package callaccess

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// FlushIdentity guards every ID transaction, including SCIM, dynamic groups and organization merges.
func FlushIdentity(ctx context.Context, tx pgx.Tx, revoker SubjectRevoker) error {
	var enabled bool
	if err := tx.QueryRow(ctx, `SELECT enabled FROM identity_call_guard_state WHERE singleton`).Scan(&enabled); err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT subject::text,org_slug,version FROM identity_call_access_fences WHERE transaction_id=pg_current_xact_id_if_assigned() AND version>acknowledged_version ORDER BY subject,org_slug`)
	if err != nil {
		return err
	}
	type fence struct {
		subject, tenant string
		version         int64
	}
	var pending []fence
	for rows.Next() {
		var f fence
		if err := rows.Scan(&f.subject, &f.tenant, &f.version); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, f := range pending {
		if revoker == nil {
			return ErrUnavailable
		}
		if err := revoker.RevokeSubjectCalls(ctx, f.subject, f.tenant, f.version); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE identity_call_access_fences SET acknowledged_version=$3 WHERE subject=$1 AND org_slug=$2 AND version=$3`, f.subject, f.tenant, f.version); err != nil {
			return err
		}
	}
	return nil
}
