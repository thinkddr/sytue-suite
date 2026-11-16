// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func (r *Repository) Scheduled(parent context.Context) ([]api.MailScheduled, error) {
	if err := r.DiscoverOutbound(parent); err != nil {
		return nil, err
	}
	var out []api.MailScheduled
	err := r.run(parent, true, func(ctx context.Context, _ storage.AtomicCatalogSnapshot) error {
		rows, err := r.cfg.Journal.List(r.outboundOwner(), outboundNamespace)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if err := ctx.Err(); err != nil {
				return err
			}
			var work outboundIntent
			if json.Unmarshal(row.Value, &work) != nil || work.Attempt.Operation != row.ID || r.validateOutbound(work) != nil {
				return ErrRecord
			}
			clear(work.Envelope)
			if work.Canceled || work.Receipt != nil && work.Receipt.Accepted() {
				continue
			}
			out = append(out, api.MailScheduled{ID: work.Attempt.Operation, From: work.Input.From, Recipients: slices.Clone(work.Input.Recipients), Subject: work.Message.Subject, ReleaseAt: work.ReleaseAt, CreatedAt: work.Message.Date})
		}
		slices.SortFunc(out, func(a, b api.MailScheduled) int {
			if n := a.ReleaseAt.Compare(b.ReleaseAt); n != 0 {
				return n
			}
			return a.CreatedAt.Compare(b.CreatedAt)
		})
		return nil
	})
	return out, err
}

func (r *Repository) Reschedule(parent context.Context, id string, when time.Time) error {
	if !canonicalDraftUUID(id) || !when.After(time.Now()) {
		return ErrRecord
	}
	return r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		return r.cfg.Journal.Exclusive(ctx, r.outboundOwner(), outboundNamespace, id, func() error {
			work, err := r.loadOutbound(id)
			if errors.Is(err, ErrNotFound) {
				work, err = r.adoptSharedOutbound(ctx, s, id)
			}
			if err != nil {
				return err
			}
			defer clear(work.Envelope)
			return r.proposeOutboundControl(ctx, s, &work, when, false)
		})
	})
}

func (r *Repository) CancelOutbound(ctx context.Context, id string) error {
	return r.CancelPrivateOutbound(ctx, api.MailOutgoingAttempt{Scope: r.cfg.Scope, Epoch: r.cfg.MigrationState.Epoch, Operation: id})
}
