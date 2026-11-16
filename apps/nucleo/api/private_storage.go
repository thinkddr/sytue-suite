// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func privateAppRequest(scope storage.Scope, product, path string) (Request, error) {
	value, err := storage.EncodeScopeHeader(scope)
	if err != nil || scope.Product != product {
		return Request{}, storage.ErrRecord
	}
	return Request{Path: path, Header: http.Header{storage.ScopeHeader: {value}}}, nil
}

func privateStorageRequest(scope storage.Scope) (Request, error) {
	return privateDriveRequest(scope, "/private-storage/migration")
}

func privateDriveRequest(scope storage.Scope, path string) (Request, error) {
	return privateAppRequest(scope, "drive", path)
}

func privateMigrationState(ctx context.Context, c *Client, scope storage.Scope, product string) (storage.LegacyMigrationState, error) {
	request, err := privateAppRequest(scope, product, "/private-storage/migration")
	if err != nil {
		return storage.LegacyMigrationState{}, err
	}
	state, err := JSON[storage.LegacyMigrationState](ctx, c, request)
	if err == nil {
		err = state.Validate(scope)
	}
	return state, err
}

func privateMigrationRequest(scope storage.Scope, product, epoch string) (Request, error) {
	request, err := privateAppRequest(scope, product, "/private-storage/migration")
	if err != nil {
		return Request{}, err
	}
	if epoch != "" {
		if !storage.MigrationEpoch(epoch) {
			return Request{}, storage.ErrRecord
		}
		request.Method = http.MethodPost
		request.Header.Set("Content-Type", "application/json")
		request.Body, _ = json.Marshal(struct {
			Epoch string `json:"epoch"`
		}{Epoch: epoch})
	}
	return request, nil
}

func beginPrivateMigration(ctx context.Context, c *Client, scope storage.Scope, product, epoch string) (storage.LegacyMigrationState, error) {
	if !storage.MigrationEpoch(epoch) {
		return storage.LegacyMigrationState{}, storage.ErrRecord
	}
	request, err := privateMigrationRequest(scope, product, epoch)
	if err != nil {
		return storage.LegacyMigrationState{}, err
	}
	return frozenPrivateMigrationResponse(ctx, c, scope, epoch, request)
}

func frozenPrivateMigrationResponse(ctx context.Context, c *Client, scope storage.Scope, epoch string, request Request) (storage.LegacyMigrationState, error) {
	state, err := JSON[storage.LegacyMigrationState](ctx, c, request)
	if err == nil {
		err = state.Validate(scope)
	}
	if err == nil && (state.Mode != "frozen" || state.Epoch != epoch) {
		err = storage.ErrRecord
	}
	return state, err
}

func privateAppMigrationState(ctx context.Context, c *Client, scope storage.Scope, product, epoch string) (PrivateAppMigrationState, error) {
	request, err := privateMigrationRequest(scope, product, epoch)
	if err != nil {
		return PrivateAppMigrationState{}, err
	}
	state, err := JSON[PrivateAppMigrationState](ctx, c, request)
	if err == nil {
		err = state.Validate(scope)
	}
	if err == nil && epoch != "" && (state.Mode != "frozen" || state.Epoch != epoch) {
		err = storage.ErrRecord
	}
	return state, err
}

func privateSourcePageRequest(scope storage.Scope, product, epoch string, after storage.LegacySourceCursor) (Request, error) {
	request, err := privateAppRequest(scope, product, "/private-storage/migration")
	if err != nil || !storage.MigrationEpoch(epoch) {
		return Request{}, storage.ErrRecord
	}
	request.Path += "/" + epoch + "/rows"
	encoded, err := storage.EncodeSourceCursor(after)
	if err != nil {
		return Request{}, err
	}
	if encoded != "" {
		request.Query = url.Values{"after": {encoded}}
	}
	return request, nil
}

func openPrivateSourceResponse(ctx context.Context, c *Client, request Request, size int64, sha256 string) (io.ReadCloser, error) {
	if size < 0 || size > storage.MaxFileBytes {
		return nil, storage.ErrRecord
	}
	response, err := c.Do(ctx, request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK || response.ContentLength >= 0 && response.ContentLength != size || response.Header.Get("Content-Type") != "application/octet-stream" || response.Header.Get("Content-Encoding") != "" || sha256 != "" && response.Header.Get("X-Sytue-Source-SHA256") != sha256 {
		_ = response.Body.Close()
		return nil, storage.ErrRecord
	}
	return &exactResponseBody{ReadCloser: response.Body, remaining: size}, nil
}

func (d *Drive) PrivateStorageState(ctx context.Context, scope storage.Scope) (storage.LegacyMigrationState, error) {
	return privateMigrationState(ctx, d.c, scope, "drive")
}

func (d *Drive) BeginPrivateStorageMigration(ctx context.Context, scope storage.Scope, epoch string) (storage.LegacyMigrationState, error) {
	return beginPrivateMigration(ctx, d.c, scope, "drive", epoch)
}

func (d *Drive) PrivateStorageSourcePage(ctx context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (storage.LegacySourcePage, error) {
	request, err := privateSourcePageRequest(scope, "drive", epoch, after)
	if err != nil {
		return storage.LegacySourcePage{}, err
	}
	page, err := JSON[storage.LegacySourcePage](ctx, d.c, request)
	if err == nil {
		err = page.Validate(scope, epoch)
	}
	if err == nil && len(page.Rows) > 0 && after.Kind != "" {
		first := page.Rows[0]
		if first.Kind < after.Kind || first.Kind == after.Kind && first.ID <= after.ID {
			err = storage.ErrRecord
		}
	}
	return page, err
}

// OpenPrivateStorageSource streams a retained original. Its consumer verifies
// declared length and the source/replacement hashes before acknowledging import.
func (d *Drive) OpenPrivateStorageSource(ctx context.Context, scope storage.Scope, epoch, kind, id string, size int64) (io.ReadCloser, error) {
	request, err := privateStorageRequest(scope)
	if err != nil || !storage.MigrationEpoch(epoch) || kind != "file" && kind != "version" || !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: kind, ID: id}) {
		return nil, storage.ErrRecord
	}
	request.Path += "/" + epoch + "/blobs/" + kind + "/" + url.PathEscape(id)
	return openPrivateSourceResponse(ctx, d.c, request, size, "")
}

// PrivateAppMigrationState is the common retained-source capability contract.
type PrivateAppMigrationState = storage.AppMigrationState
