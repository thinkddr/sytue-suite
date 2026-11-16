// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
)

// ListPrivateIngressPending reads only committed immutable ciphertext tasks.
// Polling starts again at the first pending page after a restart: a failed task
// is retained, and a pagination cursor never acknowledges or removes it.
func ListPrivateIngressPending(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig, after string, limit int) (ingress.PendingPage, error) {
	out := ingress.PendingPage{Version: 1, Scope: cfg.Scope, Epoch: cfg.Epoch.String(), Tasks: []ingress.Task{}}
	if (after != "" && !storage.Identifier(after)) || limit < 1 || limit > ingress.MaxPendingTasks {
		return out, ingress.ErrContract
	}
	if err := BindPrivateIngressScope(ctx, tx, cfg); err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT t.id,t.kind,t.key_generation,k.key_id,t.cipher_size,t.cipher_sha256
 FROM private_ingress_tasks t JOIN private_ingress_key_plans k ON (k.tenant,k.owner_sub,k.product,k.epoch,k.key_generation)=(t.tenant,t.owner_sub,t.product,t.epoch,t.key_generation)
 WHERE t.tenant=$1 AND t.owner_sub=$2 AND t.product=$3 AND t.epoch=$4 AND t.id COLLATE "C">$5 COLLATE "C" AND t.acknowledged_at IS NULL AND t.cipher_sha256 IS NOT NULL ORDER BY t.id COLLATE "C" LIMIT $6`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, after, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		task := ingress.Task{Version: 1, Scope: cfg.Scope, Epoch: cfg.Epoch.String()}
		var size int64
		var digest string
		if err := rows.Scan(&task.ID, &task.Kind, &task.KeyGeneration, &task.KeyID, &size, &digest); err != nil {
			return out, err
		}
		if len(out.Tasks) == limit {
			out.Next = out.Tasks[len(out.Tasks)-1].ID
			break
		}
		task.Cipher = storage.StreamSourceRow{Kind: "ingress", ID: task.ID, Size: size, SHA256: digest}
		if err := task.Validate(cfg.Scope, cfg.Epoch.String()); err != nil {
			return out, err
		}
		out.Tasks = append(out.Tasks, task)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	return out, out.Validate(cfg.Scope, cfg.Epoch.String(), after)
}
