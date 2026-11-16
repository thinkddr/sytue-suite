// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

var _ offline.PrivateWork = (*Repository)(nil)

func (r *Repository) Pending(ctx context.Context) (offline.PendingWork, error) {
	return r.engine.Pending(ctx)
}
func (r *Repository) Drain(ctx context.Context) error { return r.engine.Drain(ctx) }
func (r *Repository) Resume() error                   { return r.engine.Resume() }
