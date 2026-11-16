// SPDX-License-Identifier: Apache-2.0
package resourcetest

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// SessionSources composes synthetic test registry witnesses while preserving
// every production Provider/SDK verification. It creates no verified token.
type SessionSources []*SessionSource

func (d SessionSources) source(scope storage.Scope, device string) *SessionSource {
	for _, s := range d {
		if s.Record.Binding.Scope == scope && s.Record.Binding.DeviceID == device {
			return s
		}
	}
	return nil
}

func (d SessionSources) CurrentResourceSession(ctx context.Context, scope storage.Scope, device string) (resource.SessionRecord, error) {
	s := d.source(scope, device)
	if s == nil {
		return resource.SessionRecord{}, resource.ErrRevoked
	}
	return s.CurrentResourceSession(ctx, scope, device)
}

func (d SessionSources) CheckResourceSession(ctx context.Context, b resource.SessionBinding) error {
	s := d.source(b.Scope, b.DeviceID)
	if s == nil {
		return resource.ErrRevoked
	}
	return s.CheckResourceSession(ctx, b)
}

func (d SessionSources) ResourceSessionDevice(ctx context.Context, scope storage.Scope, device, root string) (matrixproof.Device, error) {
	s := d.source(scope, device)
	if s == nil {
		return matrixproof.Device{}, resource.ErrRevoked
	}
	return s.ResourceSessionDevice(ctx, scope, device, root)
}
