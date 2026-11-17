// SPDX-License-Identifier: Apache-2.0
package records

import (
	"context"
	"io"
	"reflect"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// AppSource is the immutable-source contract exposed by each product's API.
// The product client validates capabilities and the requested account scope.
type AppSource interface {
	PrivateStorageState(context.Context, storage.Scope) (storage.AppMigrationState, error)
	BeginPrivateStorageMigration(context.Context, storage.Scope, string) (storage.AppMigrationState, error)
	PrivateStorageSourcePage(context.Context, storage.Scope, string, storage.LegacySourceCursor) (storage.StreamSourcePage, error)
	OpenPrivateStorageSource(context.Context, storage.Scope, string, storage.StreamSourceRow) (io.ReadCloser, error)
}

type appSource struct{ client AppSource }

// NewAppSource adapts a retained API source without importing the API package.
// It never prepares a target, changes identity, or treats a source as ready.
func NewAppSource(client AppSource) (Source, error) {
	if client == nil {
		return nil, ErrSource
	}
	value := reflect.ValueOf(client)
	kind := value.Kind()
	if (kind == reflect.Chan || kind == reflect.Func || kind == reflect.Interface || kind == reflect.Map || kind == reflect.Pointer || kind == reflect.Slice) && value.IsNil() {
		return nil, ErrSource
	}
	return appSource{client: client}, nil
}

func (s appSource) State(ctx context.Context, scope storage.Scope) (SnapshotState, error) {
	state, err := s.client.PrivateStorageState(ctx, scope)
	return sourceSnapshotState(state), err
}

func (s appSource) Begin(ctx context.Context, scope storage.Scope, epoch string) (SnapshotState, error) {
	state, err := s.client.BeginPrivateStorageMigration(ctx, scope, epoch)
	return sourceSnapshotState(state), err
}

func (s appSource) Page(ctx context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (SnapshotPage, error) {
	return s.client.PrivateStorageSourcePage(ctx, scope, epoch, after)
}

func (s appSource) Open(ctx context.Context, scope storage.Scope, epoch string, row SnapshotRow) (io.ReadCloser, error) {
	return s.client.OpenPrivateStorageSource(ctx, scope, epoch, row)
}

func sourceSnapshotState(state storage.AppMigrationState) SnapshotState {
	return SnapshotState{Version: state.Version, LegacyMigrationState: state.LegacyMigrationState, Shared: state.Shared}
}
