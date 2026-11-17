// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type historicalProjectionFixture struct {
	reader, writer         *Storage
	migration              *MigrationStorage
	readerPort, writerPort *storagePort
	binding                *storage.Binding
	policies               *Policies
	checks                 int
	afterRecoverable       func()
	policyError            error
	afterPolicy            func(int)
}

func newHistoricalProjectionFixture(t *testing.T) *historicalProjectionFixture {
	t.Helper()
	f := &historicalProjectionFixture{policies: NewPolicies()}
	identity := fixtureIdentity()
	f.binding = &storage.Binding{Scope: identity.Scope, Target: storage.PublicationTarget{RoomID: "!legacy:test", DeviceID: identity.Binding.DeviceID}, Rooms: []string{"!legacy:test"}}
	handle := func(method Method, _ []byte) ([]byte, error) {
		switch method {
		case StoragePrepare, StorageBindings:
			return json.Marshal(f.binding)
		case StorageRecoverable:
			if f.afterRecoverable != nil {
				f.afterRecoverable()
			}
			return nil, nil
		default:
			return nil, ErrRejected
		}
	}
	f.readerPort = &storagePort{sourcePort: sourcePort{id: strings.Repeat("a", 64), identity: identity}, handle: handle}
	f.writerPort = &storagePort{sourcePort: sourcePort{id: strings.Repeat("b", 64), identity: identity}, handle: handle}
	var err error
	f.reader, err = OpenStorage(t.Context(), f.readerPort, NewSources(), f.policies)
	if err != nil {
		t.Fatal(err)
	}
	f.writer, err = OpenStorage(t.Context(), f.writerPort, NewSources(), f.policies)
	if err != nil {
		t.Fatal(err)
	}
	f.migration, err = NewMigrationStorage(f.reader, f.writer)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *historicalProjectionFixture) authorized(ctx context.Context, t *testing.T) context.Context {
	t.Helper()
	bound, err := f.migration.RequireAuthorizedStoragePublication(ctx, f.binding.Scope, f.binding.Target, func(ctx context.Context) error {
		f.checks++
		if f.afterPolicy != nil {
			f.afterPolicy(f.checks)
		}
		if ctx.Value(migrationPrincipal{}) != "original" {
			return ErrRejected
		}
		if f.policyError != nil {
			return f.policyError
		}
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	return bound
}

func TestHistoricalPublicationProjectionPreservesOriginalCallerCancellationAndSourcePolicy(t *testing.T) {
	f := newHistoricalProjectionFixture(t)
	parent, cancel := context.WithCancel(context.WithValue(t.Context(), migrationPrincipal{}, "original"))
	defer cancel()
	bound := f.authorized(parent, t)
	projected, err := f.migration.HistoricalStoragePublicationContext(bound)
	if err != nil || projected.Value(migrationPrincipal{}) != "original" || f.checks != 2 {
		t.Fatal("historical projection lost original caller or source policy", err, f.checks)
	}
	proof := projected.Value(publicationContext{}).(publicationProof)
	if proof.owner != f.writer || proof.target != f.binding.Target || proof.policy == nil || len(f.policies.entries) != 0 {
		t.Fatal("historical projection promoted reader or minted publication authority")
	}
	cancel()
	if !errors.Is(projected.Err(), context.Canceled) {
		t.Fatal("historical projection detached caller cancellation")
	}
}

func TestHistoricalPublicationProjectionRejectsMissingForeignOrIsolatedWriter(t *testing.T) {
	f := newHistoricalProjectionFixture(t)
	parent := context.WithValue(t.Context(), migrationPrincipal{}, "original")
	if _, err := f.migration.HistoricalStoragePublicationContext(parent); !errors.Is(err, ErrRejected) {
		t.Fatal("unbound historical writer admitted", err)
	}
	recoverable, err := f.writer.RequireRecoverableStorage(parent, f.binding.Scope, f.binding.Target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.writer.HistoricalStoragePublicationContext(recoverable); !errors.Is(err, ErrRejected) {
		t.Fatal("recoverability alone authorized historical source", err)
	}
	bound := f.authorized(parent, t)
	if _, err := f.reader.HistoricalStoragePublicationContext(bound); !errors.Is(err, ErrRejected) {
		t.Fatal("historical reader borrowed writer policy", err)
	}
	readonly, err := NewMigrationStorage(f.reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readonly.HistoricalStoragePublicationContext(bound); !errors.Is(err, ErrRejected) {
		t.Fatal("missing historical writer fell back to reader", err)
	}
	isolatedIdentity := f.writer.identity
	isolatedIdentity.Binding.MatrixUserID = storage.IsolatedMatrixUserPrefix(isolatedIdentity.Scope) + "test:matrix.test"
	isolatedPort := &storagePort{sourcePort: sourcePort{id: strings.Repeat("c", 64), identity: isolatedIdentity}, handle: f.writerPort.handle}
	isolated, err := OpenStorage(t.Context(), isolatedPort, NewSources(), f.policies)
	if err != nil {
		t.Fatal(err)
	}
	isolatedBound, err := isolated.RequireAuthorizedStoragePublication(parent, f.binding.Scope, f.binding.Target, func(ctx context.Context) error { return ctx.Err() })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := isolated.HistoricalStoragePublicationContext(isolatedBound); !errors.Is(err, ErrRejected) {
		t.Fatal("isolated writer entered historical projection", err)
	}
}

func TestHistoricalPublicationProjectionRechecksOriginalSourceAndBothOpaquePorts(t *testing.T) {
	for _, fault := range []string{"source", "late-source", "writer-rotation", "reader-retired", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			f := newHistoricalProjectionFixture(t)
			parent, cancel := context.WithCancel(context.WithValue(t.Context(), migrationPrincipal{}, "original"))
			defer cancel()
			bound := f.authorized(parent, t)
			switch fault {
			case "source":
				f.policyError = errors.New("original SQL source withdrawn")
			case "late-source":
				f.afterPolicy = func(checks int) {
					if checks == 2 {
						f.policyError = errors.New("source withdrawn after SDK recovery")
					}
				}
			case "writer-rotation":
				f.afterRecoverable = func() {
					f.binding.Target.RoomID = "!replacement:test"
					f.binding.Rooms = []string{f.binding.Target.RoomID}
				}
			case "reader-retired":
				f.afterRecoverable = func() { f.readerPort.identity.Scope.Subject = "retired" }
			case "cancelled":
				cancel()
			}
			if _, err := f.migration.HistoricalStoragePublicationContext(bound); err == nil {
				t.Fatal("historical projection ignored original source or port retirement", fault)
			}
		})
	}
}
