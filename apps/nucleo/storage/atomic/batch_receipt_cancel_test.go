//go:build !js

// SPDX-License-Identifier: Apache-2.0
package atomic

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Snapshot logical journal values without relying on backend enumeration order.
// This checks retained IDs and bytes, not the sealed backend's physical bytes.
func receiptJournalSnapshot(t *testing.T, journal *offline.Store, owner string, namespaces ...string) map[string]map[string]json.RawMessage {
	t.Helper()
	snapshot := make(map[string]map[string]json.RawMessage, len(namespaces))
	for _, namespace := range namespaces {
		ids, err := journal.RecordIDs(owner, namespace)
		if err != nil {
			t.Fatal("enumerate receipt journal", err)
		}
		values := make(map[string]json.RawMessage, len(ids))
		for _, id := range ids {
			if _, duplicate := values[id]; duplicate {
				t.Fatal("duplicate receipt journal ID")
			}
			var raw json.RawMessage
			found, err := journal.Read(owner, namespace, id, &raw)
			if err != nil || !found {
				t.Fatal("read receipt journal record", err)
			}
			values[id] = raw
		}
		snapshot[namespace] = values
	}
	t.Cleanup(func() {
		for _, values := range snapshot {
			for _, raw := range values {
				clear(raw)
			}
		}
	})
	return snapshot
}

// A session may be closed after a receipt operation has acquired its lease
// and begun reading the SDK. It must stop without retiring pending work, and
// a later live session must still be able to complete the same observation.
func TestFinalizationReceiptCancellationDuringSDKRead(t *testing.T) {
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
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var err error
			acquired, released, reached := 0, 0, false
			lease = func(ctx context.Context) (func(), error) {
				acquired++
				return func() { released++ }, ctx.Err()
			}
			sdk.afterRootRead = func() {
				if acquired != 1 || released != 0 {
					t.Fatal("SDK read did not hold the session lease")
				}
				reached = true
				cancel()
			}
			if phase == "capture" {
				err = CaptureFinalizationReceipt(ctx, plan, sdk, nil, lease)
			} else {
				err = CheckFinalizationReceipt(ctx, plan.data.journal, plan.data.root.Scope, plan.data.root.Operation, sdk, nil, lease)
			}
			if !reached || !errors.Is(err, context.Canceled) || acquired != 1 || released != 1 {
				t.Fatal("cancellation did not stop and release admitted work", reached, acquired, released, err)
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
			if sdk.puts != puts || sdk.prepare != prepares {
				t.Fatal("resumed observation resent the publication")
			}
		})
	}
}
