// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"io"
	"net/url"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func (p *Photos) PrivateStorageState(ctx context.Context, scope storage.Scope) (PrivateAppMigrationState, error) {
	return privateAppMigrationState(ctx, p.c, scope, "photos", "")
}

func (p *Photos) BeginPrivateStorageMigration(ctx context.Context, scope storage.Scope, epoch string) (PrivateAppMigrationState, error) {
	if !storage.MigrationEpoch(epoch) {
		return PrivateAppMigrationState{}, storage.ErrRecord
	}
	return privateAppMigrationState(ctx, p.c, scope, "photos", epoch)
}

func (p *Photos) PrivateStorageSourcePage(ctx context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (storage.StreamSourcePage, error) {
	request, err := privateSourcePageRequest(scope, "photos", epoch, after)
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	page, err := JSON[storage.StreamSourcePage](ctx, p.c, request)
	if err == nil {
		err = page.Validate(scope, epoch, after)
	}
	return page, err
}

func (p *Photos) OpenPrivateStorageSource(ctx context.Context, scope storage.Scope, epoch string, row storage.StreamSourceRow) (io.ReadCloser, error) {
	request, err := privateAppRequest(scope, "photos", "/private-storage/migration")
	if err != nil || !storage.MigrationEpoch(epoch) || row.Validate() != nil {
		return nil, storage.ErrRecord
	}
	request.Path += "/" + epoch + "/source/" + row.Kind + "/" + url.PathEscape(row.ID)
	return openPrivateSourceResponse(ctx, p.c, request, row.Size, row.SHA256)
}
