// SPDX-License-Identifier: Apache-2.0
package records

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type sourceAdapterFixture struct {
	ctx    context.Context
	scope  storage.Scope
	epoch  string
	after  storage.LegacySourceCursor
	row    storage.StreamSourceRow
	state  storage.AppMigrationState
	page   storage.StreamSourcePage
	reader io.ReadCloser
	err    error
	calls  int
}

func (s *sourceAdapterFixture) PrivateStorageState(ctx context.Context, scope storage.Scope) (storage.AppMigrationState, error) {
	s.ctx, s.scope = ctx, scope
	s.calls++
	return s.state, s.err
}

func (s *sourceAdapterFixture) BeginPrivateStorageMigration(ctx context.Context, scope storage.Scope, epoch string) (storage.AppMigrationState, error) {
	s.ctx, s.scope, s.epoch = ctx, scope, epoch
	s.calls++
	return s.state, s.err
}

func (s *sourceAdapterFixture) PrivateStorageSourcePage(ctx context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (storage.StreamSourcePage, error) {
	s.ctx, s.scope, s.epoch, s.after = ctx, scope, epoch, after
	s.calls++
	return s.page, s.err
}

func (s *sourceAdapterFixture) OpenPrivateStorageSource(ctx context.Context, scope storage.Scope, epoch string, row storage.StreamSourceRow) (io.ReadCloser, error) {
	s.ctx, s.scope, s.epoch, s.row = ctx, scope, epoch, row
	s.calls++
	return s.reader, s.err
}

func TestAppSourceRejectsNilWithoutCallingAnAPI(t *testing.T) {
	var typedNil *sourceAdapterFixture
	for _, client := range []AppSource{nil, typedNil} {
		if source, err := NewAppSource(client); source != nil || !errors.Is(err, ErrSource) {
			t.Fatal("nil source accepted", err)
		}
	}
}

func TestAppSourcePreservesOriginalContextScopeEpochAndStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scope := storage.Scope{Tenant: "tenant", Subject: "alice", Product: "calendar"}
	started := time.Date(2026, 10, 30, 1, 0, 0, 0, time.UTC)
	epoch := "cc0d13ef-c18d-455c-bb4e-60275f253d0b"
	after := storage.LegacySourceCursor{Kind: "request", ID: "calendar-id/subject"}
	row := storage.StreamSourceRow{Kind: "object", ID: "retained-id", Size: 4, SHA256: strings.Repeat("a", 64)}
	fixture := &sourceAdapterFixture{
		state:  storage.AppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "frozen", Epoch: epoch, StartedAt: &started, Rows: 7}, Shared: true},
		page:   storage.StreamSourcePage{Scope: scope, Epoch: epoch, Rows: []storage.StreamSourceRow{row}},
		reader: io.NopCloser(strings.NewReader("data")),
	}
	source, err := NewAppSource(fixture)
	if err != nil {
		t.Fatal(err)
	}
	state, err := source.State(ctx, scope)
	if err != nil || state.Version != fixture.state.Version || state.LegacyMigrationState != fixture.state.LegacyMigrationState || !state.Shared || fixture.ctx != ctx || fixture.scope != scope || fixture.calls != 1 {
		t.Fatal("source state or authority changed", err)
	}
	state, err = source.Begin(ctx, scope, epoch)
	if err != nil || state.Epoch != epoch || !state.Shared || fixture.ctx != ctx || fixture.scope != scope || fixture.epoch != epoch || fixture.calls != 2 {
		t.Fatal("begin changed source identity", err)
	}
	page, err := source.Page(ctx, scope, epoch, after)
	if err != nil || page.Scope != scope || page.Epoch != epoch || len(page.Rows) != 1 || page.Rows[0] != row || fixture.ctx != ctx || fixture.scope != scope || fixture.epoch != epoch || fixture.after != after || fixture.calls != 3 {
		t.Fatal("page authority or descriptor changed", err)
	}
	reader, err := source.Open(ctx, scope, epoch, row)
	if err != nil || reader != fixture.reader || fixture.ctx != ctx || fixture.scope != scope || fixture.epoch != epoch || fixture.row != row || fixture.calls != 4 {
		t.Fatal("source stream was substituted", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAppSourcePreservesFailuresAndDoesNotNormalizeSourceState(t *testing.T) {
	sentinel := errors.New("source API denied the request")
	fixture := &sourceAdapterFixture{state: storage.AppMigrationState{Version: 99, LegacyMigrationState: storage.LegacyMigrationState{Scope: storage.Scope{Tenant: "other", Subject: "other", Product: "mail"}, Mode: "legacy"}}, err: sentinel}
	source, err := NewAppSource(fixture)
	if err != nil {
		t.Fatal(err)
	}
	scope := storage.Scope{Tenant: "tenant", Subject: "alice", Product: "calendar"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	state, err := source.State(ctx, scope)
	if !errors.Is(err, sentinel) || state.Version != 99 || state.Scope != fixture.state.Scope || state.Mode != "legacy" {
		t.Fatal("state failure was hidden or rewritten", err)
	}
	if _, err := source.Begin(ctx, scope, "epoch"); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if _, err := source.Page(ctx, scope, "epoch", storage.LegacySourceCursor{}); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if _, err := source.Open(ctx, scope, "epoch", storage.StreamSourceRow{}); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if fixture.ctx != ctx || fixture.scope != scope || fixture.calls != 4 {
		t.Fatal("cancellation or scope was replaced")
	}
}
