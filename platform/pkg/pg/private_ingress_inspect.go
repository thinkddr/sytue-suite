// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
)

// ReadPrivateIngressTaskState exposes public commitments and original routing,
// never the claim secret. Inspection grants neither body nor publication access.
func ReadPrivateIngressTaskState(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig, id string) (ingress.TaskState, error) {
	stored, err := ReadPrivateIngressDelivery(ctx, tx, cfg, id)
	if err != nil {
		return ingress.TaskState{}, err
	}
	return privateIngressTaskState(ctx, tx, cfg, stored)
}

func privateIngressTaskState(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig, stored PrivateIngressDelivery) (ingress.TaskState, error) {
	state := ingress.TaskState{Version: 1, Task: stored.Task, Receipt: stored.Receipt}
	var err error
	state.Plan, err = ReadPrivateIngressKey(ctx, tx, cfg, stored.Task.KeyGeneration)
	if err != nil {
		return ingress.TaskState{}, err
	}
	if stored.ClaimGeneration > 0 {
		if stored.LeaseUntil == nil {
			return ingress.TaskState{}, ingress.ErrClaim
		}
		assignment := ingress.Assignment{Target: stored.Target, Generation: stored.ClaimGeneration, ExpiresAt: *stored.LeaseUntil}
		err = tx.QueryRow(ctx, `SELECT signing_key FROM private_ingress_devices WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND device_id=$5`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, stored.Target.DeviceID).Scan(&assignment.SigningKey)
		if err != nil {
			return ingress.TaskState{}, err
		}
		state.Assignment = &assignment
	}
	return state, state.Validate(cfg.Scope, cfg.Epoch.String(), stored.Task.ID)
}

func (q PrivateIngressQueue) Inspect(ctx context.Context, scope storage.Scope, epoch, id string) (ingress.TaskState, error) {
	var result ingress.TaskState
	if q.Transaction == nil || !scope.Valid() || !storage.MigrationEpoch(epoch) || !storage.Identifier(id) {
		return result, ingress.ErrContract
	}
	err := q.Transaction(ctx, scope, epoch, "inspect", func(tx pgx.Tx, cfg AuthorizedPrivateIngressConfig) error {
		if cfg.Authority == nil {
			return ingress.ErrRevoked
		}
		authority, err := cfg.Authority.ReadStorageAuthority(ctx, scope)
		if err != nil {
			return err
		}
		if authority.RequireActive(scope, authority.Generation) != nil {
			return ingress.ErrRevoked
		}
		result, err = ReadPrivateIngressTaskState(ctx, tx, cfg.PrivateIngressConfig, id)
		return err
	})
	return result, err
}
