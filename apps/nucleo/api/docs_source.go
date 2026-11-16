// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/url"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func (d *Docs) privateSourceClient() (*Client, error) {
	if d == nil || d.c == nil || d.private != nil || d.privateMode {
		return nil, ErrPrivateDataOnly
	}
	return d.c, nil
}

func (s *Slides) privateSourceClient() (*Client, error) {
	if s == nil || s.c == nil || s.private != nil || s.privateMode {
		return nil, ErrPrivateDataOnly
	}
	return s.c, nil
}

func privateOfficeState(ctx context.Context, c *Client, scope storage.Scope, product, epoch string) (PrivateAppMigrationState, error) {
	if epoch != "" && !storage.MigrationEpoch(epoch) {
		return PrivateAppMigrationState{}, storage.ErrRecord
	}
	return privateAppMigrationState(ctx, c, scope, product, epoch)
}

func privateOfficeSourcePage(ctx context.Context, c *Client, scope storage.Scope, product, epoch string, after storage.LegacySourceCursor) (storage.StreamSourcePage, error) {
	request, err := privateSourcePageRequest(scope, product, epoch, after)
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	page, err := JSON[storage.StreamSourcePage](ctx, c, request)
	if err == nil {
		err = page.Validate(scope, epoch, after)
	}
	return page, err
}

func openPrivateOfficeSource(ctx context.Context, c *Client, scope storage.Scope, product, epoch string, row storage.StreamSourceRow) (io.ReadCloser, error) {
	request, err := privateAppRequest(scope, product, "/private-storage/migration")
	if err != nil || !storage.MigrationEpoch(epoch) || row.Validate() != nil {
		return nil, storage.ErrRecord
	}
	request.Path += "/" + epoch + "/source/" + url.PathEscape(row.Kind) + "/" + url.PathEscape(row.ID)
	return openPrivateSourceResponse(ctx, c, request, row.Size, row.SHA256)
}

func (d *Docs) PrivateStorageState(ctx context.Context, scope storage.Scope) (storage.AppMigrationState, error) {
	c, err := d.privateSourceClient()
	if err != nil {
		return storage.AppMigrationState{}, err
	}
	return privateOfficeState(ctx, c, scope, "docs", "")
}

func (d *Docs) BeginPrivateStorageMigration(ctx context.Context, scope storage.Scope, epoch string) (storage.AppMigrationState, error) {
	c, err := d.privateSourceClient()
	if err != nil {
		return storage.AppMigrationState{}, err
	}
	return privateOfficeState(ctx, c, scope, "docs", epoch)
}

func (d *Docs) PrivateStorageSourcePage(ctx context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (storage.StreamSourcePage, error) {
	c, err := d.privateSourceClient()
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	return privateOfficeSourcePage(ctx, c, scope, "docs", epoch, after)
}

func (d *Docs) OpenPrivateStorageSource(ctx context.Context, scope storage.Scope, epoch string, row storage.StreamSourceRow) (io.ReadCloser, error) {
	c, err := d.privateSourceClient()
	if err != nil {
		return nil, err
	}
	return openPrivateOfficeSource(ctx, c, scope, "docs", epoch, row)
}

// SheetsSourceClient adapts a Docs client to the "sheets" private storage scope.
type SheetsSourceClient struct {
	*Docs
}

// SheetsSource returns the migration source client for Sytue Calcus ("sheets").
func (d *Docs) SheetsSource() SheetsSourceClient {
	return SheetsSourceClient{Docs: d}
}

func (s SheetsSourceClient) PrivateStorageState(ctx context.Context, scope storage.Scope) (storage.AppMigrationState, error) {
	c, err := s.privateSourceClient()
	if err != nil {
		return storage.AppMigrationState{}, err
	}
	return privateOfficeState(ctx, c, scope, "sheets", "")
}

func (s SheetsSourceClient) BeginPrivateStorageMigration(ctx context.Context, scope storage.Scope, epoch string) (storage.AppMigrationState, error) {
	c, err := s.privateSourceClient()
	if err != nil {
		return storage.AppMigrationState{}, err
	}
	return privateOfficeState(ctx, c, scope, "sheets", epoch)
}

func (s SheetsSourceClient) PrivateStorageSourcePage(ctx context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (storage.StreamSourcePage, error) {
	c, err := s.privateSourceClient()
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	return privateOfficeSourcePage(ctx, c, scope, "sheets", epoch, after)
}

func (s SheetsSourceClient) OpenPrivateStorageSource(ctx context.Context, scope storage.Scope, epoch string, row storage.StreamSourceRow) (io.ReadCloser, error) {
	c, err := s.privateSourceClient()
	if err != nil {
		return nil, err
	}
	return openPrivateOfficeSource(ctx, c, scope, "sheets", epoch, row)
}

func (s *Slides) PrivateStorageState(ctx context.Context, scope storage.Scope) (storage.AppMigrationState, error) {
	c, err := s.privateSourceClient()
	if err != nil {
		return storage.AppMigrationState{}, err
	}
	return privateOfficeState(ctx, c, scope, "slides", "")
}

func (s *Slides) BeginPrivateStorageMigration(ctx context.Context, scope storage.Scope, epoch string) (storage.AppMigrationState, error) {
	c, err := s.privateSourceClient()
	if err != nil {
		return storage.AppMigrationState{}, err
	}
	return privateOfficeState(ctx, c, scope, "slides", epoch)
}

func (s *Slides) PrivateStorageSourcePage(ctx context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (storage.StreamSourcePage, error) {
	c, err := s.privateSourceClient()
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	return privateOfficeSourcePage(ctx, c, scope, "slides", epoch, after)
}

func (s *Slides) OpenPrivateStorageSource(ctx context.Context, scope storage.Scope, epoch string, row storage.StreamSourceRow) (io.ReadCloser, error) {
	c, err := s.privateSourceClient()
	if err != nil {
		return nil, err
	}
	return openPrivateOfficeSource(ctx, c, scope, "slides", epoch, row)
}
