// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func historicalContextFixture(t testing.TB) (*Client, *Client, storage.PublicationTarget, storagePublication, *storageAdmission) {
	t.Helper()
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	source := &Client{cfg: Config{Tenant: scope.Tenant, Subject: scope.Subject, Bootstrap: Bootstrap{StorageProduct: scope.Product, StorageIdentityVersion: storage.LegacyIdentity, StorageRoomID: "!source:matrix.test", StorageRoomIDs: []string{"!source:matrix.test"}}}}
	source.session.DeviceID = "SOURCE"
	target := &Client{cfg: Config{Tenant: scope.Tenant, Subject: scope.Subject, Bootstrap: Bootstrap{StorageProduct: scope.Product, StorageIdentityVersion: storage.IsolatedIdentity, StorageRoomID: "!target:matrix.test", StorageRoomIDs: []string{"!target:matrix.test"}}}}
	target.session.DeviceID = "TARGET"
	endpoint := storage.PublicationTarget{RoomID: source.cfg.Bootstrap.StorageRoomID, DeviceID: source.DeviceID()}
	proof := storagePublication{client: source, scope: scope, target: endpoint, identity: storage.LegacyIdentity, policy: func(context.Context) error { return nil }}
	return source, target, endpoint, proof, &storageAdmission{client: target, scope: scope}
}

func TestHistoricalStoragePublicationContextRequiresOriginalAuthorizedSDK1(t *testing.T) {
	source, target, _, proof, fixed := historicalContextFixture(t)
	original := context.WithValue(t.Context(), storageAdmissionKey{}, fixed)
	original = context.WithValue(original, storageRecordAdmissionKey{}, &storageRecordAdmission{fixed: fixed})
	original = context.WithValue(original, storagePublicationContext{}, proof)
	if _, err := source.admissionFor(original); !errors.Is(err, ErrIdentity) {
		t.Fatal("foreign admission guard was weakened", err)
	}
	projected, err := source.HistoricalStoragePublicationContext(original)
	if err != nil {
		t.Fatal(err)
	}
	retained, ok := projected.Value(storagePublicationContext{}).(storagePublication)
	if !ok || retained.client != proof.client || retained.scope != proof.scope || retained.target != proof.target || retained.identity != proof.identity || retained.policy == nil {
		t.Fatal("historical policy/target replaced")
	}
	if marker, err := source.admissionFor(projected); err != nil || marker != nil {
		t.Fatal("isolated authority leaked into historical source", err)
	}
	if _, err := target.bindStoragePublication(projected, proof.scope, storage.PublicationTarget{RoomID: target.cfg.Bootstrap.StorageRoomID, DeviceID: target.DeviceID()}); !errors.Is(err, ErrIdentity) {
		t.Fatal("source projection rebound target SDK2 publication", err)
	}
	if projected.Value(storageRecordAdmissionKey{}) != nil || original.Value(storageAdmissionKey{}) != fixed {
		t.Fatal("projection changed original context")
	}
	for _, fault := range []string{"missing-proof", "target-proof", "missing-policy", "withdrawn-policy", "scope", "device", "version", "same-client", "marker-scope", "marker-version", "record", "typednil"} {
		t.Run(fault, func(t *testing.T) {
			source, target, _, proof, fixed := historicalContextFixture(t)
			switch fault {
			case "target-proof":
				proof.client = target
				proof.identity = storage.IsolatedIdentity
			case "missing-policy":
				proof.policy = nil
			case "withdrawn-policy":
				proof.policy = func(context.Context) error { return ErrIdentity }
			case "scope":
				proof.scope.Subject = "other"
			case "device":
				proof.target.DeviceID = "OTHER"
			case "version":
				source.cfg.Bootstrap.StorageIdentityVersion = storage.IsolatedIdentity
			case "same-client":
				fixed.client = source
			case "marker-scope":
				fixed.scope.Product = "drive"
			case "marker-version":
				target.cfg.Bootstrap.StorageIdentityVersion = storage.LegacyIdentity
			}
			ctx := context.WithValue(t.Context(), storageAdmissionKey{}, fixed)
			if fault != "missing-proof" {
				ctx = context.WithValue(ctx, storagePublicationContext{}, proof)
			}
			if fault == "record" {
				ctx = context.WithValue(ctx, storageRecordAdmissionKey{}, &storageRecordAdmission{fixed: &storageAdmission{client: target, scope: fixed.scope}})
			}
			if fault == "typednil" {
				ctx = context.WithValue(ctx, storageAdmissionKey{}, (*storageAdmission)(nil))
			}
			if _, err := source.HistoricalStoragePublicationContext(ctx); err == nil {
				t.Fatal("unapproved historical projection", fault)
			}
		})
	}
}

func TestHistoricalStoragePublicationProjectionPreservesOriginalLeaseDeadlineAndRevocation(t *testing.T) {
	source, _, _, proof, fixed := historicalContextFixture(t)
	life, cancelLife := context.WithCancel(t.Context())
	defer cancelLife()
	deadline := time.Now().Add(time.Minute)
	caller, cancel := context.WithDeadline(life, deadline)
	defer cancel()
	type callerKey struct{}
	caller = context.WithValue(caller, callerKey{}, "original")
	var mutex sync.RWMutex
	lease := offline.ReentrantWorkLease(func(ctx context.Context) (func(), error) { mutex.RLock(); return mutex.RUnlock, ctx.Err() })
	outer := offline.NewWorkGateWithLease(life, lease)
	defer outer.Close()
	admitted, release, err := outer.Start(caller)
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	done := func() { once.Do(release) }
	defer done()
	admitted = context.WithValue(admitted, storageAdmissionKey{}, fixed)
	denied := false
	calls := 0
	proof.policy = func(ctx context.Context) error {
		source.stateMu.Lock()
		source.stateMu.Unlock() //nolint:gocritic,staticcheck // badLock/SA2001: deliberate empty critical section; no SDK lock may surround policy.
		calls++
		if denied {
			return ErrIdentity
		}
		return ctx.Err()
	}
	admitted = context.WithValue(admitted, storagePublicationContext{}, proof)
	projected, err := source.HistoricalStoragePublicationContext(admitted)
	if err != nil {
		t.Fatal(err)
	}
	if value := projected.Value(callerKey{}); value != "original" {
		t.Fatal("lost original caller")
	}
	if got, ok := projected.Deadline(); !ok || !got.Equal(deadline) {
		t.Fatal("lost deadline")
	}
	writerReady, writerDone := make(chan struct{}), make(chan struct{})
	go func() {
		close(writerReady)
		mutex.Lock()
		mutex.Unlock() //nolint:gocritic,staticcheck // badLock/SA2001: intentional barrier wait for reader lease release.
		close(writerDone)
	}()
	<-writerReady
	waitUntil := time.Now().Add(time.Second)
	for mutex.TryRLock() {
		mutex.RUnlock()
		if time.Now().After(waitUntil) {
			done()
			t.Fatal("writer never queued")
		}
		runtime.Gosched()
	}
	nested := offline.NewWorkGateWithLease(life, lease)
	defer nested.Close()
	type result struct {
		ctx  context.Context
		done func()
		err  error
	}
	started := make(chan result, 1)
	go func() { ctx, done, err := nested.Start(projected); started <- result{ctx, done, err} }()
	var borrowed result
	select {
	case borrowed = <-started:
	case <-time.After(time.Second):
		done()
		select {
		case abandoned := <-started:
			if abandoned.done != nil {
				abandoned.done()
			}
		case <-time.After(time.Second):
		}
		t.Fatal("projection dropped original work-lease witness")
	}
	if borrowed.err != nil || borrowed.done == nil {
		t.Fatal("borrow failed", borrowed.err)
	}
	denied = true
	if err := storagePublicationPolicy(borrowed.ctx); !errors.Is(err, ErrIdentity) {
		t.Fatal("projection bypassed later original authority", err)
	}
	borrowed.done()
	cancel()
	if !errors.Is(projected.Err(), context.Canceled) {
		t.Fatal("projection detached original cancellation")
	}
	done()
	select {
	case <-writerDone:
	case <-time.After(time.Second):
		t.Fatal("historical projection retained work after completion")
	}
	if calls < 3 {
		t.Fatal("original policy was not preserved", calls)
	}
}
