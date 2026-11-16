// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"
	"errors"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// RunOutbound sends only previously consented, encrypted admitted operations.
// It never fabricates a new operation for an uncertain SMTP result.
func (r *Repository) RunOutbound(ctx context.Context, changed func(api.MailOutgoingStatus, error)) error {
	nextDiscovery := time.Time{}
	for {
		if !time.Now().Before(nextDiscovery) {
			err := r.DiscoverOutbound(ctx)
			if err != nil && isTerminalOutboundError(ctx, err) {
				return err
			}
			nextDiscovery = time.Now().Add(30 * time.Second)
		}
		if err := r.runOutboundStep(ctx, changed); err != nil {
			return err
		}
		timer := time.NewTimer(5 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func isTerminalOutboundError(ctx context.Context, err error) bool {
	return errors.Is(err, offline.ErrWorkClosed) || errors.Is(err, offline.ErrWorkPaused) || ctx.Err() != nil
}

func (r *Repository) runOutboundStep(ctx context.Context, changed func(api.MailOutgoingStatus, error)) error {
	var pending []api.MailOutgoingAttempt
	count, err := r.cfg.Journal.CountOwner(r.outboundOwner(), outboundNamespace)
	if err == nil && count > 0 {
		pending, err = r.PendingOutbound(ctx)
	}
	if err != nil {
		if isTerminalOutboundError(ctx, err) {
			return err
		}
		return nil
	}
	for _, attempt := range pending {
		status, err := r.CommitOutbound(ctx, attempt)
		if changed != nil {
			changed(status, err)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return nil
}
