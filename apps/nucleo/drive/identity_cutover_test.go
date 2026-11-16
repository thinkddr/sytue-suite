// SPDX-License-Identifier: Apache-2.0
package drive

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type cutoverServiceFixture struct {
	state         storage.IdentityCutoverState
	begin         func(storage.IdentityCutoverState) storage.IdentityCutoverState
	beforeBegin   func()
	beforeFinish  func()
	finishError   error
	acceptedError bool
	calls         []string
}

func (s *cutoverServiceFixture) MatrixStorageMigration(_ context.Context, scope storage.Scope) (storage.IdentityCutoverState, error) {
	s.calls = append(s.calls, "state")
	if scope != s.state.Scope {
		return storage.IdentityCutoverState{}, ErrSource
	}
	return s.state, nil
}

func (s *cutoverServiceFixture) BeginMatrixStorageMigration(_ context.Context, scope storage.Scope, epoch string) (storage.IdentityCutoverState, error) {
	s.calls = append(s.calls, "begin")
	if scope != s.state.Scope || !storage.MigrationEpoch(epoch) {
		return storage.IdentityCutoverState{}, ErrSource
	}
	if s.beforeBegin != nil {
		s.beforeBegin()
	}
	s.state.Phase, s.state.Epoch = "frozen", epoch
	if s.begin != nil {
		return s.begin(s.state), nil
	}
	return s.state, nil
}

func (s *cutoverServiceFixture) CompleteMatrixStorageMigration(_ context.Context, scope storage.Scope, epoch string, receipt storage.Reference) (storage.IdentityCutoverState, error) {
	s.calls = append(s.calls, "complete")
	if scope != s.state.Scope || epoch != s.state.Epoch || s.state.Phase != "frozen" || !receipt.Valid() {
		return storage.IdentityCutoverState{}, ErrSource
	}
	if s.beforeFinish != nil {
		s.beforeFinish()
	}
	if s.finishError != nil && !s.acceptedError {
		return storage.IdentityCutoverState{}, s.finishError
	}
	s.state.Phase, s.state.Receipt = "isolated", &receipt
	return s.state, s.finishError
}

func newCutoverFixture(t *testing.T) (*catalogTransferFixture, *cutoverServiceFixture, IdentityCutoverConfig) {
	t.Helper()
	f := newCatalogTransferFixture(t)
	s := &cutoverServiceFixture{state: storage.IdentityCutoverState{Version: 1, Scope: readerScope, Phase: "legacy", SourceRooms: slices.Clone(f.old.binding.Rooms)}}
	cfg := IdentityCutoverConfig{Scope: readerScope, Service: s, Legacy: f.source, Isolated: f.target, Authorize: func(ctx context.Context) error { return ctx.Err() }, CopyFile: func(context.Context, storage.PublicationTarget, string, storage.Reference, string, string) (storage.Reference, error) {
		t.Error("record fixture unexpectedly requested file content")
		return storage.Reference{}, ErrSource
	}}
	return f, s, cfg
}

func TestIdentityCutoverOrdersCompleteGraphBeforeAcknowledgment(t *testing.T) {
	f, service, cfg := newCutoverFixture(t)
	service.beforeBegin = func() {
		if !f.source.ready || len(f.next.records) != 0 {
			t.Error("identity freeze preceded source preparation or followed premature copy")
		}
	}
	service.beforeFinish = func() {
		snapshot, _, err := f.target.load(t.Context())
		if err != nil || len(snapshot.Entries) != len(f.versions) || !f.target.ready {
			t.Error("routing acknowledged before the complete verified graph", err)
		}
	}
	cutover, err := NewIdentityCutover(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = cutover.Prepare(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(service.calls, []string{"state", "begin", "complete"}) || service.state.Phase != "isolated" {
		t.Fatal("wrong cutover order", service.calls)
	}
	f.assertNoPlaintextSource(t)
}

func TestIdentityCutoverHoldsTheOriginalWorkLeaseAcrossBackendPhases(t *testing.T) {
	f, service, cfg := newCutoverFixture(t)
	var held atomic.Int32
	targetConfig := f.target.cfg
	targetConfig.WorkLease = func(context.Context) (func(), error) {
		held.Add(1)
		return func() { held.Add(-1) }, nil
	}
	target, err := New(targetConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	cfg.Isolated = target
	check := func() {
		if held.Load() < 1 {
			t.Error("backend phase ran outside its original device session lease")
		}
	}
	service.beforeBegin, service.beforeFinish = check, check
	cutover, err := NewIdentityCutover(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = cutover.Prepare(t.Context()); err != nil || held.Load() != 0 {
		t.Fatal("cutover did not release its full preparation lease", err)
	}
	f.assertNoPlaintextSource(t)
}

func TestIdentityCutoverRetriesUnacceptedAndLostFinalAcknowledgment(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(map[bool]string{false: "not-accepted", true: "accepted"}[accepted], func(t *testing.T) {
			f, service, cfg := newCutoverFixture(t)
			failure := errors.New("synthetic final acknowledgment loss")
			service.finishError, service.acceptedError = failure, accepted
			cutover, err := NewIdentityCutover(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if err = cutover.Prepare(t.Context()); !errors.Is(err, failure) {
				t.Fatal("final acknowledgment fault was hidden", err)
			}
			writes := f.next.writes
			service.finishError = nil
			if accepted {
				// A fresh lifecycle must open the isolated destination without
				// requiring access to the old device or attempting SQL import.
				cfg.Legacy, cfg.CopyFile = nil, nil
				cutover, err = NewIdentityCutover(cfg)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err = cutover.Prepare(t.Context()); err != nil {
				t.Fatal(err)
			}
			if f.next.writes != writes || service.state.Phase != "isolated" {
				t.Fatal("acknowledgment retry recopied the graph")
			}
			f.assertNoPlaintextSource(t)
		})
	}
}

func TestIdentityCutoverRejectsChangedFreezeBeforeCopy(t *testing.T) {
	for _, fault := range []string{"scope", "epoch", "rooms", "phase"} {
		t.Run(fault, func(t *testing.T) {
			f, service, cfg := newCutoverFixture(t)
			service.begin = func(state storage.IdentityCutoverState) storage.IdentityCutoverState {
				switch fault {
				case "scope":
					state.Scope.Subject = "another-person"
				case "epoch":
					state.Epoch = readerEpoch
				case "rooms":
					state.SourceRooms = []string{"!different:matrix.test"}
				case "phase":
					state.Phase = "freezing"
				}
				return state
			}
			cutover, err := NewIdentityCutover(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if err = cutover.Prepare(t.Context()); err == nil || len(f.next.records) != 0 || slices.Contains(service.calls, "complete") {
				t.Fatal("changed or incomplete freeze allowed copy", err)
			}
		})
	}
}

func TestIdentityCutoverRejectsRevokedWitnessBeforeAcknowledgment(t *testing.T) {
	f, service, cfg := newCutoverFixture(t)
	failure := errors.New("synthetic session witness changed")
	checks := 0
	cfg.Authorize = func(context.Context) error {
		checks++
		if checks >= 3 {
			return failure
		}
		return nil
	}
	cutover, err := NewIdentityCutover(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = cutover.Prepare(t.Context()); !errors.Is(err, failure) || !f.target.ready || slices.Contains(service.calls, "complete") {
		t.Fatal("changed session acknowledged the cutover", err)
	}
	f.assertNoPlaintextSource(t)
}

func TestIdentityCutoverRetriesCopyBeforeCompleting(t *testing.T) {
	f, service, cfg := newCutoverFixture(t)
	f.next.loseIndex = true
	cutover, err := NewIdentityCutover(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = cutover.Prepare(t.Context()); err == nil || slices.Contains(service.calls, "complete") {
		t.Fatal("partial graph acknowledged", err)
	}
	if err = cutover.Prepare(t.Context()); err != nil || service.state.Phase != "isolated" {
		t.Fatal("copy failed to resume", err)
	}
	f.assertNoPlaintextSource(t)
}

func TestIdentityCutoverChecksAuthenticatedReceiptWhenOpeningIsolated(t *testing.T) {
	f, service, cfg := newCutoverFixture(t)
	cutover, err := NewIdentityCutover(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = cutover.Prepare(t.Context()); err != nil {
		t.Fatal(err)
	}
	forged := storage.Reference{RoomID: f.next.binding.Target.RoomID, EventID: "$different-receipt"}
	service.state.Receipt = &forged
	cfg.Legacy, cfg.CopyFile = nil, nil
	cutover, err = NewIdentityCutover(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = cutover.Prepare(t.Context()); !errors.Is(err, ErrSource) {
		t.Fatal("backend reference replaced the authenticated SDK receipt", err)
	}
	f.assertNoPlaintextSource(t)
}

func TestIdentityCutoverRejectsWrongIdentityBeforeSourceSideEffects(t *testing.T) {
	f, service, cfg := newCutoverFixture(t)
	cfg.Isolated = f.source
	if _, err := NewIdentityCutover(cfg); err == nil {
		t.Fatal("same source/destination accepted")
	}
	cfg.Legacy = nil
	cutover, err := NewIdentityCutover(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = cutover.Prepare(t.Context()); !errors.Is(err, ErrSource) || len(service.calls) != 0 {
		t.Fatal("legacy target reached routing service", err)
	}
	f.assertNoPlaintextSource(t)
}
