// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"io"
	"net/url"

	"git.sytue.eu/sytue/one/apps/nucleo/calendar/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// These calls keep the original authenticated Calendar transport separate from
// ordinary private CRUD. Historical frozen data never invents ingress readiness.
func (c *Calendars) PrivateStorageState(ctx context.Context, scope storage.Scope) (storage.AppMigrationState, error) {
	return privateAppMigrationState(ctx, c.c, scope, "calendar", "")
}

func (c *Calendars) BeginPrivateStorageMigration(ctx context.Context, scope storage.Scope, epoch string) (storage.AppMigrationState, error) {
	if !storage.MigrationEpoch(epoch) {
		return storage.AppMigrationState{}, storage.ErrRecord
	}
	return privateAppMigrationState(ctx, c.c, scope, "calendar", epoch)
}

func (c *Calendars) PrivateStorageSourcePage(ctx context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (storage.StreamSourcePage, error) {
	if after != (storage.LegacySourceCursor{}) && !model.SourceKind(after.Kind) {
		return storage.StreamSourcePage{}, storage.ErrRecord
	}
	request, err := privateSourcePageRequest(scope, "calendar", epoch, after)
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	page, err := JSON[storage.StreamSourcePage](ctx, c.c, request)
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	if page.Validate(scope, epoch, after) != nil {
		return storage.StreamSourcePage{}, storage.ErrRecord
	}
	for _, row := range page.Rows {
		if !privateCalendarSourceRow(row) {
			return storage.StreamSourcePage{}, storage.ErrRecord
		}
	}
	return page, nil
}

func privateCalendarSourceRow(row storage.StreamSourceRow) bool {
	return model.SourceKind(row.Kind) && row.Validate() == nil && row.Size >= 2 && row.Size <= model.MaxRowBytes
}

func (c *Calendars) OpenPrivateStorageSource(ctx context.Context, scope storage.Scope, epoch string, row storage.StreamSourceRow) (io.ReadCloser, error) {
	request, err := privateAppRequest(scope, "calendar", "/private-storage/migration")
	if err != nil || !storage.MigrationEpoch(epoch) || !privateCalendarSourceRow(row) {
		return nil, storage.ErrRecord
	}
	request.Path += "/" + epoch + "/source/" + url.PathEscape(row.Kind) + "/" + url.PathEscape(row.ID)
	return openPrivateSourceResponse(ctx, c.c, request, row.Size, row.SHA256)
}
