// SPDX-License-Identifier: Apache-2.0
package cifrado

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// RetainedSources is the same suite composition on native and browser clients.
type RetainedSources = offline.RetainedSources

func NewRetainedSources(resources ResourceGuard, sources ...offline.PrivateWork) (*RetainedSources, error) {
	return offline.NewRetainedSources(resources, ErrPrivateResources, sources...)
}

func NewComposedSources(resources ResourceGuard, owner offline.PrivateWork, inventory func(context.Context) (offline.PendingWork, error), sources ...offline.PrivateWork) (*RetainedSources, error) {
	return offline.NewComposedSources(resources, owner, inventory, ErrPrivateResources, sources...)
}
