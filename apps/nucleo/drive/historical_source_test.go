// SPDX-License-Identifier: Apache-2.0
package drive

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type (
	historicalDriveCallerKey struct{}
	historicalDriveProofKey  struct{}
	historicalDriveProof     struct {
		client    *historicalDriveCrypto
		policy    storage.PublicationPolicy
		projected bool
	}
	// SDK policy/delivery are explicit component witnesses here. The original Go
	// domain imports and verifies its full inventory; separate Matrix tests use SDK.
	historicalDriveCrypto struct {
		*catalogTransferCrypto
		bindings, projections int
		projectionError       error
		nilProjection         bool
	}
)

func (c *historicalDriveCrypto) RequireAuthorizedStoragePublication(ctx context.Context, scope storage.Scope, target storage.PublicationTarget, policy storage.PublicationPolicy) (context.Context, error) {
	if scope != c.binding.Scope || target != c.binding.Target || policy == nil {
		return nil, ErrSource
	}
	c.bindings++
	if err := policy(ctx); err != nil {
		return nil, err
	}
	return context.WithValue(ctx, historicalDriveProofKey{}, historicalDriveProof{client: c, policy: policy}), nil
}

//nolint:nilnil // test double: nilProjection deliberately returns (nil, nil) to prove callers reject a nil context
func (c *historicalDriveCrypto) HistoricalStoragePublicationContext(ctx context.Context) (context.Context, error) {
	proof, ok := ctx.Value(historicalDriveProofKey{}).(historicalDriveProof)
	if !ok || proof.client != c || proof.policy == nil {
		return nil, ErrSource
	}
	c.projections++
	if c.projectionError != nil {
		return nil, c.projectionError
	}
	if c.nilProjection {
		return nil, nil
	}
	if err := proof.policy(ctx); err != nil {
		return nil, err
	}
	proof.projected = true
	return context.WithValue(ctx, historicalDriveProofKey{}, proof), nil
}

func (c *historicalDriveCrypto) checkPublication(ctx context.Context) error {
	proof, ok := ctx.Value(historicalDriveProofKey{}).(historicalDriveProof)
	if !ok || proof.client != c || proof.policy == nil || !proof.projected || ctx.Value(historicalDriveCallerKey{}) != "original" {
		return ErrSource
	}
	return proof.policy(ctx)
}

func (c *historicalDriveCrypto) PutStorageRecordAt(ctx context.Context, target storage.PublicationTarget, transaction string, record storage.Record) (storage.Reference, error) {
	if err := c.checkPublication(ctx); err != nil {
		return storage.Reference{}, err
	}
	ref, err := c.catalogTransferCrypto.PutStorageRecordAt(ctx, target, transaction, record)
	if err == nil {
		err = c.checkPublication(ctx)
	}
	return ref, err
}

func (c *historicalDriveCrypto) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, transaction, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	if err := c.checkPublication(ctx); err != nil {
		return storage.ObjectVersion{}, err
	}
	version, err := c.catalogTransferCrypto.PutStorageObjectAt(ctx, target, transaction, object, revision, commit)
	if err == nil {
		err = c.checkPublication(ctx)
	}
	return version, err
}

type historicalDriveSnapshot struct {
	state                storage.LegacyMigrationState
	row                  storage.LegacySourceRow
	reads, pages, begins int
	afterRead            func(int)
}

func (s *historicalDriveSnapshot) PrivateStorageState(ctx context.Context, scope storage.Scope) (storage.LegacyMigrationState, error) {
	s.reads++
	if s.afterRead != nil {
		s.afterRead(s.reads)
	}
	if s.state.Validate(scope) != nil {
		return storage.LegacyMigrationState{}, ErrSource
	}
	return s.state, ctx.Err()
}

func (s *historicalDriveSnapshot) BeginPrivateStorageMigration(ctx context.Context, scope storage.Scope, epoch string) (storage.LegacyMigrationState, error) {
	s.begins++
	if ctx.Value(historicalDriveCallerKey{}) != "original" || scope != s.state.Scope || s.state.Mode != "legacy" || !storage.MigrationEpoch(epoch) {
		return storage.LegacyMigrationState{}, ErrSource
	}
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	s.state.Mode = "frozen"
	s.state.Epoch = epoch
	s.state.StartedAt = &now
	s.state.Rows = 1
	return s.state, ctx.Err()
}

func (s *historicalDriveSnapshot) PrivateStorageSourcePage(ctx context.Context, scope storage.Scope, epoch string, cursor storage.LegacySourceCursor) (storage.LegacySourcePage, error) {
	s.pages++
	if scope != s.state.Scope || epoch != s.state.Epoch || cursor != (storage.LegacySourceCursor{}) {
		return storage.LegacySourcePage{}, ErrSource
	}
	return storage.LegacySourcePage{Scope: scope, Epoch: epoch, Rows: []storage.LegacySourceRow{s.row}}, ctx.Err()
}

func (*historicalDriveSnapshot) OpenPrivateStorageSource(context.Context, storage.Scope, string, string, string, int64) (io.ReadCloser, error) {
	return nil, ErrSource
}

func historicalDriveAuthorize(ctx context.Context) error {
	if ctx.Value(historicalDriveCallerKey{}) != "original" {
		return ErrSource
	}
	return ctx.Err()
}

func historicalDriveFixture(t *testing.T) (*Repository, *historicalDriveCrypto, *historicalDriveSnapshot, *cutoverServiceFixture, context.Context) {
	t.Helper()
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	source := &historicalDriveSnapshot{state: storage.LegacyMigrationState{Scope: readerScope, Mode: "frozen", Epoch: readerEpoch, StartedAt: &started, Rows: 1}, row: storage.LegacySourceRow{Kind: "folder", ID: "root", Payload: []byte("{\"id\":\"root\",\"is_root\":true,\"name\":\"Private original\",\"future_integer\":900719925474099312345}")}}
	crypto := &historicalDriveCrypto{catalogTransferCrypto: newCatalogTransferCrypto(readerScope, "source")}
	repository, err := New(Config{Context: t.Context(), Scope: readerScope, Crypto: crypto, Source: source, Gateway: api.NewDrive("https://unused.invalid", nil, nil), Journal: journal})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(repository.Close)
	routing := &cutoverServiceFixture{state: storage.IdentityCutoverState{Version: 1, Scope: readerScope, Phase: "legacy", SourceRooms: []string{crypto.binding.Target.RoomID}}}
	parent := context.WithValue(t.Context(), historicalDriveCallerKey{}, "original")
	return repository, crypto, source, routing, parent
}

func TestHistoricalDriveSourceImportsOnlyWithOriginalLivePolicyAndExactRows(t *testing.T) {
	for _, initial := range []string{"frozen", "legacy"} {
		t.Run(initial, func(t *testing.T) {
			repository, crypto, source, routing, ctx := historicalDriveFixture(t)
			if initial == "legacy" {
				source.state = storage.LegacyMigrationState{Scope: readerScope, Mode: "legacy"}
			}
			if err := repository.initializeHistoricalSource(ctx, routing.state, routing, historicalDriveAuthorize); err != nil {
				t.Fatal("original historical Drive source failed", err)
			}
			if !repository.ready || crypto.bindings != 1 || crypto.projections != 1 || crypto.writes < 4 || source.pages != 1 {
				t.Fatal("historical Drive import skipped original policy or complete inventory", crypto.bindings, crypto.projections, crypto.writes, source.pages)
			}
			versions := repository.versions(objectID("folder", source.row.ID))
			if len(versions) != 1 {
				t.Fatal("historical Drive source has no exact private head")
			}
			readCtx, done, err := repository.start(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer done()
			_, _, raw, err := repository.readEntry(readCtx, versions[0])
			defer clear(raw)
			if err != nil || !bytes.Equal(raw, source.row.Payload) {
				t.Fatal("historical Drive import changed original JSON bytes", err)
			}
		})
	}
}

func TestHistoricalDriveSourceRejectsMissingIntentFrozenIdentityAndChangedSource(t *testing.T) {
	for _, fault := range []string{"missing-intent", "frozen-identity", "source-epoch", "source-count", "source-start", "current-matrix", "current-rooms", "projector-error", "nil-projected-context", "session"} {
		t.Run(fault, func(t *testing.T) {
			repository, crypto, source, routing, ctx := historicalDriveFixture(t)
			authorize := historicalDriveAuthorize
			switch fault {
			case "missing-intent":
				if err := repository.Initialize(ctx); err == nil || crypto.writes != 0 || source.pages != 0 {
					t.Fatal("Drive imported SQL source without explicit historical capability", err)
				}
				return
			case "frozen-identity":
				routing.state.Phase = "frozen"
				routing.state.Epoch = readerEpoch
			case "source-epoch":
				source.afterRead = func(count int) {
					if count == 2 {
						source.state.Epoch = "22345678-1234-1234-1234-123456789abc"
					}
				}
			case "source-count":
				source.afterRead = func(count int) {
					if count == 2 {
						source.state.Rows = 2
					}
				}
			case "source-start":
				source.afterRead = func(count int) {
					if count == 2 {
						*source.state.StartedAt = source.state.StartedAt.Add(time.Second)
					}
				}
			case "current-matrix":
				source.afterRead = func(count int) {
					if count == 2 {
						routing.state.Phase = "frozen"
						routing.state.Epoch = readerEpoch
					}
				}
			case "current-rooms":
				source.afterRead = func(count int) {
					if count == 2 {
						routing.state.SourceRooms = nil
					}
				}
			case "projector-error":
				crypto.projectionError = errors.New("original source projection rejected")
			case "nil-projected-context":
				crypto.nilProjection = true
			case "session":
				authorize = func(context.Context) error { return errors.New("original session retired") }
			}
			if err := repository.initializeHistoricalSource(ctx, routing.state, routing, authorize); err == nil || crypto.writes != 0 || source.pages != 0 {
				t.Fatal("Drive source bypassed phase, SQL epoch, projector or session", fault, err)
			}
		})
	}
}

func TestHistoricalDriveSourceKeepsAmbiguousIntentAndRejectsLateEpochChange(t *testing.T) {
	repository, crypto, source, routing, ctx := historicalDriveFixture(t)
	original := source.state.Epoch
	crypto.afterWrite = func() { source.state.Epoch = "22345678-1234-1234-1234-123456789abc"; crypto.afterWrite = nil }
	if err := repository.initializeHistoricalSource(ctx, routing.state, routing, historicalDriveAuthorize); err == nil || repository.ready || crypto.writes != 1 {
		t.Fatal("late source withdrawal acknowledged private import", err, crypto.writes)
	}
	pending, err := repository.Pending(ctx)
	if err != nil || pending.Count == 0 {
		t.Fatal("ambiguous historical source discarded original journal", err)
	}
	source.state.Epoch = original
	if err := repository.initializeHistoricalSource(ctx, routing.state, routing, historicalDriveAuthorize); err != nil || !repository.ready {
		t.Fatal("restored original source failed retained-WAL recovery", err)
	}
}

var _ storage.HistoricalPublicationProjector = (*historicalDriveCrypto)(nil)
