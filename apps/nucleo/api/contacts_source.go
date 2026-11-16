// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"io"
	"net/url"

	"git.sytue.eu/sytue/one/apps/nucleo/contacts/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func (c *Contacts) PrivateStorageState(ctx context.Context, scope storage.Scope) (PrivateAppMigrationState, error) {
	return c.contactsSourceState(ctx, scope, "")
}

func (c *Contacts) BeginPrivateStorageMigration(ctx context.Context, scope storage.Scope, epoch string) (PrivateAppMigrationState, error) {
	if !storage.MigrationEpoch(epoch) {
		return PrivateAppMigrationState{}, storage.ErrRecord
	}
	return c.contactsSourceState(ctx, scope, epoch)
}

func (c *Contacts) contactsSourceState(ctx context.Context, scope storage.Scope, epoch string) (PrivateAppMigrationState, error) {
	state, err := privateAppMigrationState(ctx, c.c, scope, "contacts", epoch)
	if err == nil && state.Shared {
		err = storage.ErrRecord
	}
	return state, err
}

func (c *Contacts) PrivateStorageSourcePage(ctx context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (storage.StreamSourcePage, error) {
	request, err := privateSourcePageRequest(scope, "contacts", epoch, after)
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	page, err := JSON[storage.StreamSourcePage](ctx, c.c, request)
	if err == nil {
		err = page.Validate(scope, epoch, after)
	}
	if err == nil {
		for _, row := range page.Rows {
			if !model.SourceKind(row.Kind) || row.Size > model.MaxRowBytes {
				return storage.StreamSourcePage{}, storage.ErrRecord
			}
		}
	}
	return page, err
}

func (c *Contacts) OpenPrivateStorageSource(ctx context.Context, scope storage.Scope, epoch string, row storage.StreamSourceRow) (io.ReadCloser, error) {
	request, err := privateAppRequest(scope, "contacts", "/private-storage/migration")
	if err != nil || !storage.MigrationEpoch(epoch) || row.Validate() != nil || !model.SourceKind(row.Kind) || row.Size > model.MaxRowBytes {
		return nil, storage.ErrRecord
	}
	request.Path += "/" + epoch + "/blobs/" + row.Kind + "/" + url.PathEscape(row.ID)
	return openPrivateSourceResponse(ctx, c.c, request, row.Size, row.SHA256)
}
