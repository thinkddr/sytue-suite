//go:build !js

// SPDX-License-Identifier: Apache-2.0
package atomic

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// This witness blocks an admitted SDK history request until its context is
// cancelled. It models cancellation propagation, not a real Matrix SDK logout.
type blockedReceiptSDK struct {
	*batchPublisherWitness
	entered chan struct{}
	stop    chan struct{}
	once    sync.Once
}

func (sdk *blockedReceiptSDK) StorageObjectHistory(ctx context.Context, _, _ string, _ int) (storage.ObjectPage, error) {
	sdk.once.Do(func() { close(sdk.entered) })
	select {
	case <-ctx.Done():
		return storage.ObjectPage{}, ctx.Err()
	case <-sdk.stop:
		return storage.ObjectPage{}, errors.New("blocked SDK witness stopped")
	}
}

func TestFinalizationReceiptCancellationUnblocksSDKRead(t *testing.T) {
	for _, phase := range []string{"capture", "readback"} {
		t.Run(phase, func(t *testing.T) {
			plan, sdk := batchStageFixture(t)
			if _, err := PublishInputBatch(t.Context(), plan, sdk, nil); err != nil {
				t.Fatal(err)
			}
			lease := func(ctx context.Context) (func(), error) { return func() {}, ctx.Err() }
			if phase == "readback" {
				if err := CaptureFinalizationReceipt(t.Context(), plan, sdk, nil, lease); err != nil {
					t.Fatal(err)
				}
			}
			owner := atomicOwner(plan.data.root.Scope)
			active := storage.AtomicJournalNamespace(plan.data.root.Scope.Product)
			history := storage.AtomicHistoryNamespace(plan.data.root.Scope.Product)
			before := receiptJournalSnapshot(t, plan.data.journal, owner, active, history)
			if len(before[active]) == 0 {
				t.Fatal("missing active work")
			}
			puts, prepares := sdk.puts, sdk.prepare
			blocked := &blockedReceiptSDK{batchPublisherWitness: sdk, entered: make(chan struct{}), stop: make(chan struct{})}
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan struct{})
			defer func() {
				cancel()
				close(blocked.stop)
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					t.Error("blocked SDK witness did not finish during cleanup")
				}
			}()
			var acquired, released atomic.Int32
			lease = func(ctx context.Context) (func(), error) {
				acquired.Add(1)
				return func() { released.Add(1) }, ctx.Err()
			}
			result := make(chan error, 1)
			go func() {
				defer close(done)
				if phase == "capture" {
					result <- CaptureFinalizationReceipt(ctx, plan, blocked, nil, lease)
				} else {
					result <- CheckFinalizationReceipt(ctx, plan.data.journal, plan.data.root.Scope, plan.data.root.Operation, blocked, nil, lease)
				}
			}()
			select {
			case <-blocked.entered:
			case err := <-result:
				t.Fatal("operation ended before the SDK request blocked", err)
			case <-time.After(15 * time.Second):
				t.Fatal("operation never reached the SDK request")
			}
			if acquired.Load() != 1 || released.Load() != 0 {
				t.Fatal("blocked SDK request did not hold the lease", acquired.Load(), released.Load())
			}
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) || released.Load() != 1 {
					t.Fatal("cancelled SDK request did not release the lease", released.Load(), err)
				}
			case <-time.After(15 * time.Second):
				t.Fatal("SDK request did not stop on cancellation")
			}
			after := receiptJournalSnapshot(t, plan.data.journal, owner, active, history)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("cancelled observation changed durable journal IDs or values")
			}
			if sdk.puts != puts || sdk.prepare != prepares {
				t.Fatal("cancelled observation resent the publication")
			}
			lease = func(ctx context.Context) (func(), error) { return func() {}, ctx.Err() }
			if phase == "capture" {
				if err := CaptureFinalizationReceipt(t.Context(), plan, sdk, nil, lease); err != nil {
					t.Fatal("capture could not resume after cancellation", err)
				}
			}
			if err := CheckFinalizationReceipt(t.Context(), plan.data.journal, plan.data.root.Scope, plan.data.root.Operation, sdk, nil, lease); err != nil {
				t.Fatal("readback could not resume after cancellation", err)
			}
		})
	}
}
