// SPDX-License-Identifier: Apache-2.0
package drive

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// PrivateMigrationReceipt returns the SDK-authenticated original import index
// in an initialized isolated catalog. A caller must finish MigratePrivateCatalog
// before using it to acknowledge an identity cutover; this method alone does
// not assert that post-import history or another device's pending work moved.
func (r *Repository) PrivateMigrationReceipt(parent context.Context) (storage.Reference, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return storage.Reference{}, err
	}
	defer done()
	version, ok := r.cfg.Crypto.(interface {
		StorageIdentityVersion() storage.IdentityVersion
	})
	if !ok || version.StorageIdentityVersion() != storage.IsolatedIdentity {
		return storage.Reference{}, ErrSource
	}
	if err := r.requireReady(); err != nil {
		return storage.Reference{}, err
	}
	if err := r.refresh(ctx); err != nil {
		return storage.Reference{}, err
	}
	state, err := r.cfg.Source.PrivateStorageState(ctx, r.cfg.Scope)
	if err != nil {
		return storage.Reference{}, err
	}
	if state.Validate(r.cfg.Scope) != nil || state.Mode == "legacy" {
		return storage.Reference{}, ErrSource
	}
	complete, err := r.completed(ctx, state)
	if err != nil {
		return storage.Reference{}, err
	}
	if !complete {
		return storage.Reference{}, ErrNotReady
	}
	versions := r.versions("drive_migration_" + digest([]byte(state.Epoch)))
	if len(versions) != 1 {
		return storage.Reference{}, ErrConflict
	}
	if err := ctx.Err(); err != nil {
		return storage.Reference{}, err
	}
	return versions[0].Reference, nil
}
