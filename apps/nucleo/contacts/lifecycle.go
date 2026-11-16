// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

var _ offline.PrivateWork = (*Repository)(nil)

func (r *Repository) Drain(ctx context.Context) error { return r.core.Drain(ctx) }
func (r *Repository) Resume() error                   { return r.core.Resume() }
func (r *Repository) Pending(ctx context.Context) (offline.PendingWork, error) {
	return r.core.Pending(ctx)
}

func (r *Repository) RefreshPrivateData(ctx context.Context) error {
	return r.core.RefreshPrivateData(ctx)
}
