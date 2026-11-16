// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// ChangePrivateSubmissionControl fences stale authorized devices without ever
// retaining the job's scheduled time, recipients or MIME on the relay.
func (s *Store) ChangePrivateSubmissionControl(ctx context.Context, owner string, change model.SubmissionControlChange) (model.SubmissionReceipt, error) {
	var receipt model.SubmissionReceipt
	if change.Validate() != nil {
		return receipt, model.ErrSubmission
	}
	err := s.submissionTx(ctx, owner, change.Epoch, func(tx pgx.Tx) error {
		tenant := tenantOf(ctx)
		var fingerprint, control string
		var generation int64
		var canceled bool
		err := tx.QueryRow(ctx, "SELECT fingerprint,control_generation,control_commitment,canceled FROM mail_private_submissions WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND operation=$4 FOR UPDATE", tenant, owner, change.Epoch, change.Operation).Scan(&fingerprint, &generation, &control, &canceled)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if fingerprint != change.Fingerprint {
			return model.ErrSubmissionConflict
		}
		// Exact repeat after a lost HTTP ACK never creates another generation.
		if generation == change.Generation+1 && control == change.NextControl && canceled == change.Cancel {
			var err error
			receipt, err = readSubmissionReceipt(ctx, tx, tenant, owner, change.Epoch, change.Operation)
			return err
		}
		if generation != change.Generation || control != change.Control {
			return model.ErrSubmissionControl
		}
		if canceled {
			return model.ErrSubmissionStarted
		}
		var started bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM mail_private_submission_recipients WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND operation=$4 AND state NOT IN ('ready','failed'))", tenant, owner, change.Epoch, change.Operation).Scan(&started); err != nil {
			return err
		}
		if started {
			return model.ErrSubmissionStarted
		}
		if _, err := tx.Exec(ctx, "UPDATE mail_private_submissions SET control_generation=control_generation+1,control_commitment=$5,canceled=$6 WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND operation=$4", tenant, owner, change.Epoch, change.Operation, change.NextControl, change.Cancel); err != nil {
			return err
		}
		if change.Cancel {
			if _, err := tx.Exec(ctx, "UPDATE mail_private_submission_recipients SET state='canceled' WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND operation=$4", tenant, owner, change.Epoch, change.Operation); err != nil {
				return err
			}
		}
		receipt, err = readSubmissionReceipt(ctx, tx, tenant, owner, change.Epoch, change.Operation)
		return err
	})
	return receipt, err
}
