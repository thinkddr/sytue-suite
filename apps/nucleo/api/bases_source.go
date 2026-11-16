// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"io"
	"net/url"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const maxPrivateBasesRowBytes = 32 << 20

func privateBasesSourceKind(kind string) bool {
	switch kind {
	case "base", "table", "field", "view", "record":
		return true
	default:
		return false
	}
}

func privateBasesSourceRow(row storage.StreamSourceRow) bool {
	return privateBasesSourceKind(row.Kind) && row.Validate() == nil && row.Size >= 2 && row.Size <= maxPrivateBasesRowBytes
}

func (b *Bases) PrivateStorageState(ctx context.Context, scope storage.Scope) (storage.AppMigrationState, error) {
	return privateAppMigrationState(ctx, b.c, scope, "bases", "")
}

func (b *Bases) BeginPrivateStorageMigration(ctx context.Context, scope storage.Scope, epoch string) (storage.AppMigrationState, error) {
	if !storage.MigrationEpoch(epoch) {
		return storage.AppMigrationState{}, storage.ErrRecord
	}
	return privateAppMigrationState(ctx, b.c, scope, "bases", epoch)
}

func (b *Bases) PrivateStorageSourcePage(ctx context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (storage.StreamSourcePage, error) {
	if after != (storage.LegacySourceCursor{}) && !privateBasesSourceKind(after.Kind) {
		return storage.StreamSourcePage{}, storage.ErrRecord
	}
	request, err := privateSourcePageRequest(scope, "bases", epoch, after)
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	page, err := JSON[storage.StreamSourcePage](ctx, b.c, request)
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	if page.Validate(scope, epoch, after) != nil {
		return storage.StreamSourcePage{}, storage.ErrRecord
	}
	for _, row := range page.Rows {
		if !privateBasesSourceRow(row) {
			return storage.StreamSourcePage{}, storage.ErrRecord
		}
	}
	return page, nil
}

func (b *Bases) OpenPrivateStorageSource(ctx context.Context, scope storage.Scope, epoch string, row storage.StreamSourceRow) (io.ReadCloser, error) {
	request, err := privateAppRequest(scope, "bases", "/private-storage/migration")
	if err != nil || !storage.MigrationEpoch(epoch) || !privateBasesSourceRow(row) {
		return nil, storage.ErrRecord
	}
	request.Path += "/" + epoch + "/source/" + url.PathEscape(row.Kind) + "/" + url.PathEscape(row.ID)
	return openPrivateSourceResponse(ctx, b.c, request, row.Size, row.SHA256)
}
