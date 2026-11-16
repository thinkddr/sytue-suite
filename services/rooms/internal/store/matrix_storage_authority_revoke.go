// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Lifecycle callers already hold the identity or tenant denial lock. Existing
// isolated routing enumerates scopes without granting cross-owner access to the
// root table. A durable denial and its authority generation commit together,
// before any remote device/room withdrawal can fail or be delayed.
func revokeMatrixAuthoritiesTx(ctx context.Context, tx pgx.Tx, subject, tenantFilter, source string, version int64) error {
	var originalTenant string
	if err := tx.QueryRow(ctx, `SELECT coalesce(current_setting('app.tenant',true),'')`).Scan(&originalTenant); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT tenant,product FROM matrix_storage_isolated_rooms WHERE subject=$1 AND ($2='' OR tenant=$2) ORDER BY tenant,product`, subject, tenantFilter)
	if err != nil {
		return err
	}
	scopes := []storage.Scope{}
	for rows.Next() {
		var scope storage.Scope
		scope.Subject = subject
		if err := rows.Scan(&scope.Tenant, &scope.Product); err != nil {
			rows.Close()
			return err
		}
		scopes = append(scopes, scope)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, scope := range scopes {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant',$1,true),set_config('app.matrix_authority_owner',$2,true),set_config('app.matrix_authority_product',$3,true),set_config('app.matrix_authority_change','revoke',true)`, scope.Tenant, scope.Subject, scope.Product); err != nil {
			return err
		}
		var root string
		var generation int64
		err := tx.QueryRow(ctx, `UPDATE matrix_storage_authorities SET generation=generation+1,status='revoked',updated_at=now() WHERE tenant=$1 AND subject=$2 AND product=$3 AND status='active' RETURNING master_key,generation`, scope.Tenant, scope.Subject, scope.Product).Scan(&root, &generation)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		evidence, err := json.Marshal(map[string]any{"v": 1, "source": source, "lifecycle_version": version})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO matrix_storage_authority_history(tenant,subject,product,generation,master_key,status,reason,device_id,evidence) VALUES($1,$2,$3,$4,$5,'revoked','revoke','server_lifecycle',$6::jsonb)`, scope.Tenant, scope.Subject, scope.Product, generation, root, evidence); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `SELECT set_config('app.tenant',$1,true),set_config('app.matrix_authority_owner','',true),set_config('app.matrix_authority_product','',true),set_config('app.matrix_authority_change','',true)`, originalTenant)
	return err
}
