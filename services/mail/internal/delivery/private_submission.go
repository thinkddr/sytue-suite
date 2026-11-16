// SPDX-License-Identifier: Apache-2.0
package delivery

import (
	"bytes"
	"context"
	"errors"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// ErrSMTPAmbiguous means DATA may already have been accepted. Never retry it
// automatically, including another MX. HTTP reconciliation reads the receipt.
var ErrSMTPAmbiguous = errors.New("mail: SMTP delivery outcome is uncertain")

type PrivateSender interface {
	SendPrivate(context.Context, string, []string, []byte) (bool, error)
}

func (s *SMTPSender) SendPrivate(ctx context.Context, from string, to []string, raw []byte) (bool, error) {
	private := *s
	private.privateTransit = true
	return private.Send(ctx, from, to, bytes.NewReader(raw))
}

// WithPrivateSender enables transient delivery. It does not enable the legacy
// Sent/S3/outbound queue for a frozen private account.
func (out *Outbound) WithPrivateSender(sender PrivateSender) *Outbound {
	out.privateSender = sender
	return out
}

func (out *Outbound) SubmitPrivate(ctx context.Context, owner string, input model.Submission, raw []byte) (model.SubmissionReceipt, error) {
	var empty model.SubmissionReceipt
	fingerprint, signed, err := out.preparePrivateSubmission(ctx, owner, input, raw)
	if err != nil {
		return empty, err
	}
	defer clear(signed)
	receipt, err := out.store.BeginPrivateSubmission(ctx, owner, input, fingerprint)
	if err != nil {
		return empty, err
	}
	for ordinal, state := range receipt.States {
		if state != "ready" {
			continue
		}
		if err := out.deliverPrivateRecipient(ctx, owner, input, ordinal, signed); err != nil {
			return receipt, err
		}
	}
	return out.store.PrivateSubmissionReceipt(ctx, owner, input.Epoch, input.Operation)
}

func (out *Outbound) preparePrivateSubmission(ctx context.Context, owner string, input model.Submission, raw []byte) (string, []byte, error) {
	if out.privateSender == nil || input.Validate() != nil || input.Generation <= 0 || len(raw) == 0 {
		return "", nil, model.ErrSubmission
	}
	if int64(len(raw)) > out.maxBytes {
		return "", nil, domain.ErrMessageTooLarge
	}
	raw = bytes.Clone(raw)
	defer clear(raw)
	fingerprint, err := input.Fingerprint(raw)
	if err != nil {
		return "", nil, err
	}
	account, err := out.store.AccountBySubject(ctx, owner)
	if err != nil {
		return "", nil, err
	}
	resolved, err := out.store.LookupAddress(ctx, input.From)
	if err != nil {
		return "", nil, err
	}
	if resolved.AccountID != account.ID {
		return "", nil, domain.ErrFromNoAutorizado
	}
	if err := exigirFromDeLaCuenta(ctx, out.store, raw, input.From, account.ID); err != nil {
		return "", nil, err
	}
	signed, err := out.signer.Sign(raw)
	if err != nil {
		return "", nil, err
	}
	return fingerprint, signed, nil
}

func (out *Outbound) deliverPrivateRecipient(ctx context.Context, owner string, input model.Submission, ordinal int, signed []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	claimed, err := out.store.ClaimPrivateSubmission(ctx, owner, input, ordinal)
	if err != nil || !claimed {
		return err
	}
	permanent, sendErr := out.privateSender.SendPrivate(ctx, input.From, []string{input.Recipients[ordinal]}, signed)
	final := "accepted"
	if sendErr != nil {
		final = "failed"
		if !permanent {
			final = "ready"
		}
		if errors.Is(sendErr, ErrSMTPAmbiguous) {
			final = "uncertain"
		}
	}
	// A disconnected UI must not erase the durable outcome after SMTP accepts.
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	finishErr := out.store.FinishPrivateSubmission(finishCtx, owner, input, ordinal, final)
	cancel()
	return finishErr
}

func (out *Outbound) PrivateSubmissionReceipt(ctx context.Context, owner, epoch, operation string) (model.SubmissionReceipt, error) {
	return out.store.PrivateSubmissionReceipt(ctx, owner, epoch, operation)
}

func (out *Outbound) RegisterPrivateSubmission(ctx context.Context, owner string, registration model.SubmissionRegistration) (model.SubmissionReceipt, error) {
	if out.privateSender == nil || registration.Input.Generation != 1 {
		return model.SubmissionReceipt{}, model.ErrSubmission
	}
	account, err := out.store.AccountBySubject(ctx, owner)
	if err != nil {
		return model.SubmissionReceipt{}, err
	}
	resolved, err := out.store.LookupAddress(ctx, registration.Input.From)
	if err != nil {
		return model.SubmissionReceipt{}, err
	}
	if account.ID != resolved.AccountID {
		return model.SubmissionReceipt{}, domain.ErrFromNoAutorizado
	}
	return out.store.RegisterPrivateSubmission(ctx, owner, registration.Input, registration.Fingerprint)
}

func (out *Outbound) ChangePrivateSubmissionControl(ctx context.Context, owner string, change model.SubmissionControlChange) (model.SubmissionReceipt, error) {
	return out.store.ChangePrivateSubmissionControl(ctx, owner, change)
}
