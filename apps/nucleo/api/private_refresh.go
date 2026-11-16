// SPDX-License-Identifier: Apache-2.0
package api

import "context"

// PrivateDataRefresher refreshes a complete authenticated device projection.
// It is explicit; ordinary reads and thumbnail requests never trigger a reload.
type PrivateDataRefresher interface{ RefreshPrivateData(context.Context) error }

func (c *Contacts) RefreshPrivateData(ctx context.Context) error {
	if c == nil {
		return ErrPrivateContactsOperation
	}
	if c.data == nil {
		return nil
	}
	refresh, ok := c.data.(PrivateDataRefresher)
	if !ok {
		return ErrPrivateContactsOperation
	}
	return refresh.RefreshPrivateData(ctx)
}

func (p *Photos) RefreshPrivateData(ctx context.Context) error {
	if p == nil {
		return ErrPrivatePhotosOperation
	}
	if p.data == nil {
		return nil
	}
	refresh, ok := p.data.(PrivateDataRefresher)
	if !ok {
		return ErrPrivatePhotosOperation
	}
	return refresh.RefreshPrivateData(ctx)
}
func (p *Photos) UsesPrivateData() bool { return p != nil && p.data != nil }
