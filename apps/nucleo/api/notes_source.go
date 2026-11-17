// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"io"
	"net/url"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// These source methods belong to the retained legacy transport, never to a
// private facade. Only an explicit, verified-device import consumes their data.
func (n *Notes) PrivateStorageState(ctx context.Context, scope storage.Scope) (PrivateAppMigrationState, error) {
	return privateAppMigrationState(ctx, n.c, scope, "notes", "")
}

func (n *Notes) BeginPrivateStorageMigration(ctx context.Context, scope storage.Scope, epoch string) (PrivateAppMigrationState, error) {
	if !storage.MigrationEpoch(epoch) {
		return PrivateAppMigrationState{}, storage.ErrRecord
	}
	return privateAppMigrationState(ctx, n.c, scope, "notes", epoch)
}

func (n *Notes) PrivateStorageSourcePage(ctx context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (storage.StreamSourcePage, error) {
	request, err := privateSourcePageRequest(scope, "notes", epoch, after)
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	page, err := JSON[storage.StreamSourcePage](ctx, n.c, request)
	if err == nil {
		err = page.Validate(scope, epoch, after)
	}
	return page, err
}

func (n *Notes) OpenPrivateStorageSource(ctx context.Context, scope storage.Scope, epoch string, row storage.StreamSourceRow) (io.ReadCloser, error) {
	request, err := privateAppRequest(scope, "notes", "/private-storage/migration")
	if err != nil || !storage.MigrationEpoch(epoch) || row.Validate() != nil {
		return nil, storage.ErrRecord
	}
	request.Path += "/" + epoch + "/source/" + row.Kind + "/" + url.PathEscape(row.ID)
	return openPrivateSourceResponse(ctx, n.c, request, row.Size, row.SHA256)
}
