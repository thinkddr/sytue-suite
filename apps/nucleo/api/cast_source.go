// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"io"
	"net/url"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func (c *Cast) PrivateStorageState(ctx context.Context, scope storage.Scope) (PrivateAppMigrationState, error) {
	return privateAppMigrationState(ctx, c.c, scope, "cast", "")
}

func (c *Cast) BeginPrivateStorageMigration(ctx context.Context, scope storage.Scope, epoch string) (PrivateAppMigrationState, error) {
	if !storage.MigrationEpoch(epoch) {
		return PrivateAppMigrationState{}, storage.ErrRecord
	}
	return privateAppMigrationState(ctx, c.c, scope, "cast", epoch)
}

func (c *Cast) PrivateStorageSourcePage(ctx context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (storage.StreamSourcePage, error) {
	request, err := privateSourcePageRequest(scope, "cast", epoch, after)
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	page, err := JSON[storage.StreamSourcePage](ctx, c.c, request)
	if err == nil {
		err = page.Validate(scope, epoch, after)
	}
	return page, err
}

func (c *Cast) OpenPrivateStorageSource(ctx context.Context, scope storage.Scope, epoch string, row storage.StreamSourceRow) (io.ReadCloser, error) {
	request, err := privateAppRequest(scope, "cast", "/private-storage/migration")
	if err != nil || !storage.MigrationEpoch(epoch) || row.Validate() != nil {
		return nil, storage.ErrRecord
	}
	request.Path += "/" + epoch + "/source/" + row.Kind + "/" + url.PathEscape(row.ID)
	return openPrivateSourceResponse(ctx, c.c, request, row.Size, row.SHA256)
}
