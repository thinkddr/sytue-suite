// SPDX-License-Identifier: Apache-2.0
package records

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type epochCrypto struct {
	Crypto
	binding storage.Binding
}

func (c epochCrypto) PrepareStorage(context.Context, storage.Scope) (storage.Binding, error) {
	return c.binding, nil
}

func (c epochCrypto) CheckStoragePublicationTarget(target storage.PublicationTarget) error {
	if target != c.binding.Target {
		return ErrSource
	}
	return nil
}

type epochSource struct {
	Source
	state            SnapshotState
	epoch            string
	failure          error
	captures, begins int
	begun            string
	changed          bool
}

func (s *epochSource) State(context.Context, storage.Scope) (SnapshotState, error) {
	return s.state, nil
}

func (s *epochSource) SnapshotEpoch(_ context.Context, scope storage.Scope) (string, error) {
	s.captures++
	if scope != s.state.Scope {
		return "", ErrSource
	}
	return s.epoch, s.failure
}

func (s *epochSource) Begin(_ context.Context, scope storage.Scope, epoch string) (SnapshotState, error) {
	s.begins++
	s.begun = epoch
	now := time.Now().UTC()
	state := s.state
	state.Mode = "frozen"
	state.StartedAt = &now
	state.Epoch = epoch
	if s.changed {
		state.Epoch = uuid.NewString()
	}
	return state, nil
}

func epochRepository(t *testing.T) (*Repository, *epochSource) {
	t.Helper()
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "photos"}
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	source := &epochSource{state: SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}}}
	binding := storage.Binding{Scope: scope, Target: storage.PublicationTarget{RoomID: "!photos:test", DeviceID: "DEVICE"}, Rooms: []string{"!photos:test"}}
	return &Repository{cfg: Config{Scope: scope, Product: "photos", Journal: journal, Source: source, Crypto: epochCrypto{binding: binding}}}, source
}

func TestSnapshotEpochRecoversAcceptedPlanOnlyForNewLegacyWAL(t *testing.T) {
	for _, retained := range []string{"", uuid.NewString()} {
		t.Run(retained, func(t *testing.T) {
			r, source := epochRepository(t)
			source.epoch = retained
			state, work, err := r.prepareSnapshot(t.Context())
			if err != nil || source.captures != 1 || source.begins != 1 || !storage.MigrationEpoch(work.Epoch) || state.Epoch != work.Epoch {
				t.Fatal("new legacy WAL did not retain source epoch", err)
			}
			if retained != "" && work.Epoch != retained {
				t.Fatal("accepted plan received a new epoch")
			}
			before := work
			source.epoch = uuid.NewString()
			_, work, err = r.prepareSnapshot(t.Context())
			if err != nil || work != before || source.captures != 1 || source.begun != before.Epoch {
				t.Fatal("retry rewrote existing WAL epoch or target", err)
			}
		})
	}
}

func TestSnapshotEpochFailureOrChangedScopeCannotBeginOrCreateWAL(t *testing.T) {
	for _, change := range []string{"invalid", "error", "scope"} {
		t.Run(change, func(t *testing.T) {
			r, source := epochRepository(t)
			source.epoch = uuid.NewString()
			switch change {
			case "invalid":
				source.epoch = "not-an-epoch"
			case "error":
				source.failure = errors.New("synthetic opaque plan lookup failed")
			case "scope":
				source.state.Scope.Subject = "other"
			}
			if _, _, err := r.prepareSnapshot(t.Context()); err == nil || source.begins != 0 {
				t.Fatal("untrusted plan began source", err)
			}
			ids, err := r.cfg.Journal.RecordIDs(r.owner(), r.namespace())
			if err != nil || len(ids) != 0 {
				t.Fatal("untrusted plan created WAL", ids, err)
			}
		})
	}
	r, source := epochRepository(t)
	source.epoch = uuid.NewString()
	source.changed = true
	if _, _, err := r.prepareSnapshot(t.Context()); !errors.Is(err, ErrSource) || source.begun != source.epoch {
		t.Fatal("changed Begin epoch accepted", err)
	}
	var saved snapshotWork
	if found, err := r.cfg.Journal.Read(r.owner(), r.namespace(), "snapshot", &saved); err != nil || !found || saved.Epoch != source.epoch {
		t.Fatal("changed source discarded original WAL", err)
	}
}

func TestSnapshotEpochIsNotConsultedForAnAlreadyFrozenSource(t *testing.T) {
	r, source := epochRepository(t)
	now := time.Now().UTC()
	source.state.Mode = "frozen"
	source.state.Epoch = uuid.NewString()
	source.state.StartedAt = &now
	source.epoch = "invalid"
	source.failure = errors.New("must not query accepted plan")
	_, work, err := r.prepareSnapshot(t.Context())
	if err != nil || work.Epoch != source.state.Epoch || source.captures != 0 || source.begins != 0 {
		t.Fatal("frozen epoch was changed", err)
	}
}

func TestSnapshotEpochKeepsSourcesWithoutTheOptionalPortUnchanged(t *testing.T) {
	r, source := epochRepository(t)
	source.failure = errors.New("optional epoch port must not be called")
	r.cfg.Source = struct{ Source }{Source: source}
	_, work, err := r.prepareSnapshot(t.Context())
	if err != nil || !storage.MigrationEpoch(work.Epoch) || source.captures != 0 || source.begins != 1 {
		t.Fatal("ordinary source changed epoch behavior", err)
	}
}
