// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"io"
	"net/url"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const maxPrivateHubRowBytes = 32 << 20

func privateHubSourceKind(kind string) bool {
	switch kind {
	case "notification", "preference", "notice":
		return true
	default:
		return false
	}
}

func privateHubSourceRow(row storage.StreamSourceRow) bool {
	return privateHubSourceKind(row.Kind) && row.Validate() == nil && row.Size >= 2 && row.Size <= maxPrivateHubRowBytes
}

func (h *Hub) PrivateStorageState(ctx context.Context, scope storage.Scope) (storage.AppMigrationState, error) {
	return privateAppMigrationState(ctx, h.c, scope, "hub", "")
}

func (h *Hub) BeginPrivateStorageMigration(ctx context.Context, scope storage.Scope, epoch string) (storage.AppMigrationState, error) {
	if !storage.MigrationEpoch(epoch) {
		return storage.AppMigrationState{}, storage.ErrRecord
	}
	return privateAppMigrationState(ctx, h.c, scope, "hub", epoch)
}

func (h *Hub) PrivateStorageSourcePage(ctx context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (storage.StreamSourcePage, error) {
	if after != (storage.LegacySourceCursor{}) && !privateHubSourceKind(after.Kind) {
		return storage.StreamSourcePage{}, storage.ErrRecord
	}
	request, err := privateSourcePageRequest(scope, "hub", epoch, after)
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	page, err := JSON[storage.StreamSourcePage](ctx, h.c, request)
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	if page.Validate(scope, epoch, after) != nil {
		return storage.StreamSourcePage{}, storage.ErrRecord
	}
	for _, row := range page.Rows {
		if !privateHubSourceRow(row) {
			return storage.StreamSourcePage{}, storage.ErrRecord
		}
	}
	return page, nil
}

func (h *Hub) OpenPrivateStorageSource(ctx context.Context, scope storage.Scope, epoch string, row storage.StreamSourceRow) (io.ReadCloser, error) {
	request, err := privateAppRequest(scope, "hub", "/private-storage/migration")
	if err != nil || !storage.MigrationEpoch(epoch) || !privateHubSourceRow(row) {
		return nil, storage.ErrRecord
	}
	request.Path += "/" + epoch + "/source/" + url.PathEscape(row.Kind) + "/" + url.PathEscape(row.ID)
	return openPrivateSourceResponse(ctx, h.c, request, row.Size, row.SHA256)
}
