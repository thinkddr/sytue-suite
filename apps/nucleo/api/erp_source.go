// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"io"
	"net/url"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const maxPrivateERPRowBytes = 32 << 20

func privateERPSourceKind(kind string) bool {
	switch kind {
	case "record", "activity":
		return true
	default:
		return false
	}
}

func privateERPSourceRow(row storage.StreamSourceRow) bool {
	return privateERPSourceKind(row.Kind) && row.Validate() == nil && row.Size >= 2 && row.Size <= maxPrivateERPRowBytes
}

func (e *ERP) PrivateStorageState(ctx context.Context, scope storage.Scope) (storage.AppMigrationState, error) {
	return privateAppMigrationState(ctx, e.c, scope, "erp", "")
}

func (e *ERP) BeginPrivateStorageMigration(ctx context.Context, scope storage.Scope, epoch string) (storage.AppMigrationState, error) {
	if !storage.MigrationEpoch(epoch) {
		return storage.AppMigrationState{}, storage.ErrRecord
	}
	return privateAppMigrationState(ctx, e.c, scope, "erp", epoch)
}

func (e *ERP) PrivateStorageSourcePage(ctx context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (storage.StreamSourcePage, error) {
	if after != (storage.LegacySourceCursor{}) && !privateERPSourceKind(after.Kind) {
		return storage.StreamSourcePage{}, storage.ErrRecord
	}
	request, err := privateSourcePageRequest(scope, "erp", epoch, after)
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	page, err := JSON[storage.StreamSourcePage](ctx, e.c, request)
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	if page.Validate(scope, epoch, after) != nil {
		return storage.StreamSourcePage{}, storage.ErrRecord
	}
	for _, row := range page.Rows {
		if !privateERPSourceRow(row) {
			return storage.StreamSourcePage{}, storage.ErrRecord
		}
	}
	return page, nil
}

func (e *ERP) OpenPrivateStorageSource(ctx context.Context, scope storage.Scope, epoch string, row storage.StreamSourceRow) (io.ReadCloser, error) {
	request, err := privateAppRequest(scope, "erp", "/private-storage/migration")
	if err != nil || !storage.MigrationEpoch(epoch) || !privateERPSourceRow(row) {
		return nil, storage.ErrRecord
	}
	request.Path += "/" + epoch + "/source/" + url.PathEscape(row.Kind) + "/" + url.PathEscape(row.ID)
	return openPrivateSourceResponse(ctx, e.c, request, row.Size, row.SHA256)
}
