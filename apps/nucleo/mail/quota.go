// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type CipherUsageReader interface {
	CipherUsage(context.Context, storage.Scope) (storage.CipherUsage, error)
}

// Quota uses committed ciphertext accounting from the common gateway. The Mail
// service supplies the plan limit only; its historical plaintext used_bytes is
// deliberately ignored. Metadata needs the original lease, not catalog bodies.
func (r *Repository) Quota(parent context.Context) (api.MailQuota, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return api.MailQuota{}, err
	}
	defer done()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return api.MailQuota{}, ErrClosed
	}
	if !r.ready {
		return api.MailQuota{}, ErrNotReady
	}
	if r.cfg.Metadata == nil || r.cfg.Usage == nil {
		return api.MailQuota{}, api.ErrPrivateMailOperation
	}
	plan, err := r.cfg.Metadata.RoutingQuota(ctx, r.cfg.Scope)
	if err != nil {
		return api.MailQuota{}, err
	}
	if plan.QuotaBytes < 0 {
		return api.MailQuota{}, ErrRecord
	}
	value, err := r.cfg.Usage.CipherUsage(ctx, r.cfg.Scope)
	if err != nil {
		return api.MailQuota{}, err
	}
	if !value.Valid(r.cfg.Scope) {
		return api.MailQuota{}, ErrRecord
	}
	if err := ctx.Err(); err != nil {
		return api.MailQuota{}, err
	}
	return api.MailQuota{UsedBytes: value.SytueBytes, QuotaBytes: plan.QuotaBytes, ExternalUsedBytes: value.ExternalBytes, PendingDeleteBytes: value.PendingDeleteBytes, CiphertextBytes: value.Bytes}, nil
}
