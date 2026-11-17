// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"encoding/json"
	"io"
	"net/url"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Migration source methods belong to the original authenticated Mail client.
// A WithPrivateData facade has a closed transport and cannot read legacy data.
func (m *Mail) PrivateStorageState(ctx context.Context, scope storage.Scope) (storage.LegacyMigrationState, error) {
	return privateMigrationState(ctx, m.c, scope, "mail")
}

func (m *Mail) BeginPrivateStorageMigration(ctx context.Context, scope storage.Scope, epoch string) (storage.LegacyMigrationState, error) {
	return beginPrivateMigration(ctx, m.c, scope, "mail", epoch)
}

func (m *Mail) PrivateStorageSourcePage(ctx context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (storage.StreamSourcePage, error) {
	request, err := privateSourcePageRequest(scope, "mail", epoch, after)
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	page, err := JSON[storage.StreamSourcePage](ctx, m.c, request)
	if err == nil {
		err = page.Validate(scope, epoch, after)
	}
	return page, err
}

func (m *Mail) OpenPrivateStorageSource(ctx context.Context, scope storage.Scope, epoch string, row storage.StreamSourceRow) (io.ReadCloser, error) {
	request, err := privateAppRequest(scope, "mail", "/private-storage/migration")
	if err != nil || !storage.MigrationEpoch(epoch) || row.Validate() != nil {
		return nil, storage.ErrRecord
	}
	request.Path += "/" + epoch + "/source/" + url.PathEscape(row.Kind) + "/" + url.PathEscape(row.ID)
	return openPrivateSourceResponse(ctx, m.c, request, row.Size, row.SHA256)
}

func (m *Mail) PrivateStorageCapabilities(ctx context.Context, scope storage.Scope) (PrivateAppMigrationState, error) {
	return privateAppMigrationState(ctx, m.c, scope, "mail", "")
}

// ProvisionAndBeginPrivateStorageMigration is the explicit first-use action.
// It submits no MIME, source descriptors, address or profile supplied by the
// caller; the server only uses Sytue ID's signed assigned-mailbox identity.
func (m *Mail) ProvisionAndBeginPrivateStorageMigration(ctx context.Context, scope storage.Scope, epoch string) (storage.LegacyMigrationState, error) {
	if !storage.MigrationEpoch(epoch) {
		return storage.LegacyMigrationState{}, storage.ErrRecord
	}
	request, err := privateMigrationRequest(scope, "mail", epoch)
	if err != nil {
		return storage.LegacyMigrationState{}, err
	}
	request.Body, err = json.Marshal(struct {
		Epoch     string `json:"epoch"`
		Provision bool   `json:"provision"`
	}{epoch, true})
	if err != nil {
		return storage.LegacyMigrationState{}, err
	}
	defer clear(request.Body)
	return frozenPrivateMigrationResponse(ctx, m.c, scope, epoch, request)
}
