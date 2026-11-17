// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"encoding/hex"
	"errors"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

func (s *Store) submissionTx(ctx context.Context, owner, epoch string, fn func(pgx.Tx) error) error {
	return s.privateTx(ctx, owner, func(tx pgx.Tx) error {
		state, err := pg.LockPrivateStorageState(ctx, tx, "mail_private_storage_states", tenantOf(ctx), owner)
		if err != nil {
			return err
		}
		if state.Epoch != epoch || (state.Mode != "frozen" && state.Mode != "private") {
			return ErrPrivateStorageEpoch
		}
		return fn(tx)
	})
}

// BeginPrivateSubmission commits a content-free deduplication fence BEFORE SMTP.
// 'sending' is deliberately never reclaimed on a lease or timeout: after a
// process crash SMTP may already have accepted the message.
func (s *Store) BeginPrivateSubmission(ctx context.Context, owner string, input model.Submission, fingerprint string) (model.SubmissionReceipt, error) {
	if input.Generation <= 0 {
		return model.SubmissionReceipt{}, model.ErrSubmissionControl
	}
	return s.beginPrivateSubmission(ctx, owner, input, fingerprint, false)
}

func (s *Store) RegisterPrivateSubmission(ctx context.Context, owner string, input model.Submission, fingerprint string) (model.SubmissionReceipt, error) {
	if input.Generation != 1 {
		return model.SubmissionReceipt{}, model.ErrSubmissionControl
	}
	return s.beginPrivateSubmission(ctx, owner, input, fingerprint, true)
}

func (s *Store) beginPrivateSubmission(ctx context.Context, owner string, input model.Submission, fingerprint string, register bool) (model.SubmissionReceipt, error) {
	var receipt model.SubmissionReceipt
	decoded, err := hex.DecodeString(fingerprint)
	if input.Validate() != nil || err != nil || len(decoded) != 32 {
		return receipt, model.ErrSubmission
	}
	err = s.submissionTx(ctx, owner, input.Epoch, func(tx pgx.Tx) error {
		tenant := tenantOf(ctx)
		if err := ensurePrivateSubmissionRow(ctx, tx, tenant, owner, input, fingerprint, register); err != nil {
			return err
		}
		var err error
		receipt, err = readSubmissionReceipt(ctx, tx, tenant, owner, input.Epoch, input.Operation)
		return err
	})
	return receipt, err
}

func ensurePrivateSubmissionRow(ctx context.Context, tx pgx.Tx, tenant, owner string, input model.Submission, fingerprint string, register bool) error {
	_, err := tx.Exec(ctx,
		"INSERT INTO mail_private_submissions(tenant,owner_sub,epoch,operation,fingerprint,recipient_count) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING",
		tenant, owner, input.Epoch, input.Operation, fingerprint, len(input.Recipients))
	if err != nil {
		return err
	}
	var original, control string
	var count int
	var generation int64
	if err := tx.QueryRow(ctx, "SELECT fingerprint,recipient_count,control_generation,control_commitment FROM mail_private_submissions WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND operation=$4 FOR UPDATE", tenant, owner, input.Epoch, input.Operation).Scan(&original, &count, &generation, &control); err != nil {
		return err
	}
	if original != fingerprint || count != len(input.Recipients) {
		return model.ErrSubmissionConflict
	}
	if _, err := tx.Exec(ctx, "INSERT INTO mail_private_submission_recipients(tenant,owner_sub,epoch,operation,ordinal) SELECT $1,$2,$3::uuid,$4::uuid,n FROM generate_series(0,$5::integer-1) n ON CONFLICT DO NOTHING", tenant, owner, input.Epoch, input.Operation, count); err != nil {
		return err
	}
	if !register {
		if generation != input.Generation || control != input.Control {
			return model.ErrSubmissionControl
		}
		return nil
	}
	return registerSubmissionControl(ctx, tx, tenant, owner, input, generation, control)
}

func registerSubmissionControl(ctx context.Context, tx pgx.Tx, tenant, owner string, input model.Submission, generation int64, control string) error {
	if generation == 0 {
		var started bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM mail_private_submission_recipients WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND operation=$4 AND state NOT IN ('ready','failed'))", tenant, owner, input.Epoch, input.Operation).Scan(&started); err != nil {
			return err
		}
		if started {
			return model.ErrSubmissionStarted
		}
		_, err := tx.Exec(ctx, "UPDATE mail_private_submissions SET control_generation=1,control_commitment=$5 WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND operation=$4", tenant, owner, input.Epoch, input.Operation, input.Control)
		return err
	}
	if generation == 1 && control != input.Control {
		return model.ErrSubmissionControl
	}
	return nil
}

func (s *Store) ClaimPrivateSubmission(ctx context.Context, owner string, input model.Submission, ordinal int) (bool, error) {
	if input.Validate() != nil || ordinal < 0 || ordinal >= len(input.Recipients) {
		return false, model.ErrSubmission
	}
	claimed := false
	err := s.submissionTx(ctx, owner, input.Epoch, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "UPDATE mail_private_submission_recipients SET state='sending' WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND operation=$4 AND ordinal=$5 AND state='ready' AND EXISTS(SELECT 1 FROM mail_private_submissions s WHERE s.tenant=mail_private_submission_recipients.tenant AND s.owner_sub=mail_private_submission_recipients.owner_sub AND s.epoch=mail_private_submission_recipients.epoch AND s.operation=mail_private_submission_recipients.operation AND NOT s.canceled AND s.control_generation=$6 AND s.control_commitment=$7)", tenantOf(ctx), owner, input.Epoch, input.Operation, ordinal, input.Generation, input.Control)
		claimed = tag.RowsAffected() == 1
		return err
	})
	return claimed, err
}

func (s *Store) FinishPrivateSubmission(ctx context.Context, owner string, input model.Submission, ordinal int, state string) error {
	if input.Validate() != nil || ordinal < 0 || ordinal >= len(input.Recipients) || (state != "accepted" && state != "failed" && state != "uncertain" && state != "ready") {
		return model.ErrSubmission
	}
	return s.submissionTx(ctx, owner, input.Epoch, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "UPDATE mail_private_submission_recipients SET state=$6 WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND operation=$4 AND ordinal=$5 AND state='sending'", tenantOf(ctx), owner, input.Epoch, input.Operation, ordinal, state)
		if err == nil && tag.RowsAffected() != 1 {
			return model.ErrSubmissionConflict
		}
		return err
	})
}

func (s *Store) PrivateSubmissionReceipt(ctx context.Context, owner, epoch, operation string) (model.SubmissionReceipt, error) {
	var out model.SubmissionReceipt
	err := s.submissionTx(ctx, owner, epoch, func(tx pgx.Tx) error {
		var err error
		out, err = readSubmissionReceipt(ctx, tx, tenantOf(ctx), owner, epoch, operation)
		return err
	})
	return out, err
}

func readSubmissionReceipt(ctx context.Context, tx pgx.Tx, tenant, owner, epoch, operation string) (model.SubmissionReceipt, error) {
	out := model.SubmissionReceipt{Operation: operation}
	var count int
	if err := tx.QueryRow(ctx, "SELECT fingerprint,recipient_count,control_generation,control_commitment,canceled FROM mail_private_submissions WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND operation=$4", tenant, owner, epoch, operation).Scan(&out.Fingerprint, &count, &out.Generation, &out.Control, &out.Canceled); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, domain.ErrNotFound
		}
		return out, err
	}
	rows, err := tx.Query(ctx, "SELECT ordinal,state FROM mail_private_submission_recipients WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND operation=$4 ORDER BY ordinal", tenant, owner, epoch, operation)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var ordinal int
		var state string
		if err := rows.Scan(&ordinal, &state); err != nil {
			return out, err
		}
		if ordinal != len(out.States) {
			return out, model.ErrSubmission
		}
		out.States = append(out.States, state)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if len(out.States) != count {
		return out, model.ErrSubmission
	}
	return out, nil
}
