// SPDX-License-Identifier: Apache-2.0
package records

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestPendingContentReadsOwnedExactIntentWithoutPublication(t *testing.T) {
	r, probe, _ := retainedReaderFixture(t)
	target := r.binding.Target
	work := mutation{Version: 1, Scope: r.cfg.Scope, Operation: "recipe", Input: hashBytes([]byte("input")), Entity: "edit", ID: "edit", State: "live", JSON: []byte(`{"id":"edit","unknown":900719925474099312345}`), Content: true, Target: &target, ContentInput: []byte(`{"origin":"immutable","base":"old"}`)}
	key := "mutation_" + hashBytes([]byte(work.Operation+"\x00"+work.Input))
	if err := r.cfg.Journal.Write(r.owner(), r.namespace(), key, work); err != nil {
		t.Fatal(err)
	}
	pending, err := r.ReadPendingContent(t.Context(), "recipe", "edit", "edit")
	if err != nil || pending == nil || !bytes.Equal(pending.JSON, work.JSON) || !bytes.Equal(pending.Captured, work.ContentInput) || pending.Target != target || pending.InputFingerprint != work.Input {
		t.Fatal("pending read replaced the original intent", err)
	}
	clear(pending.JSON)
	clear(pending.Captured)
	var saved mutation
	found, err := r.cfg.Journal.Read(r.owner(), r.namespace(), key, &saved)
	if err != nil || !found || !bytes.Equal(saved.JSON, work.JSON) || !bytes.Equal(saved.ContentInput, work.ContentInput) || probe.acknowledgments != 0 || probe.downloads != 0 {
		t.Fatal("pending read mutated or downloaded content", err)
	}
	for _, selector := range [][3]string{{"other", "edit", "edit"}, {"recipe", "asset", "edit"}, {"recipe", "edit", "other"}} {
		if result, err := r.ReadPendingContent(t.Context(), selector[0], selector[1], selector[2]); result != nil || err != nil {
			t.Fatal("pending read crossed its selector", selector, err)
		}
	}
	r.binding.Target.DeviceID = "OTHER"
	if result, err := r.ReadPendingContent(t.Context(), "recipe", "edit", "edit"); result != nil || !errors.Is(err, ErrSource) {
		t.Fatal("pending read adapted a different target", err)
	}
}

func TestPendingContentsPagesMoreThan128JobsWithoutChangingTheirTargets(t *testing.T) {
	r, probe, _ := retainedReaderFixture(t)
	target := r.binding.Target
	for i := range 211 {
		work := mutation{Version: 1, Scope: r.cfg.Scope, Operation: "ingest", Input: hashBytes(fmt.Appendf(nil, "input_%d", i)), Entity: "asset", ID: fmt.Sprintf("asset_%d", i), State: "live", JSON: fmt.Appendf(nil, `{"id":"asset_%d"}`, i), Content: true, Target: &target, ContentInput: []byte(`{"staged":"owned-source"}`)}
		if i >= 200 {
			work.Operation = "other"
		}
		key := "mutation_" + hashBytes([]byte(work.Operation+"\x00"+work.Input))
		if err := r.cfg.Journal.Write(r.owner(), r.namespace(), key, work); err != nil {
			t.Fatal(err)
		}
		if err := r.cfg.Journal.Write(r.owner(), r.namespace(), key+"_publication", map[string]string{"unrelated": "publication"}); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	cursor := ""
	pages := 0
	for {
		page, err := r.PendingContents(t.Context(), "ingest", "asset", cursor, 7)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		if len(page.Items) > 7 {
			t.Fatal("page exceeded its work budget")
		}
		for _, item := range page.Items {
			if item.Target != target || item.EntityID == "" || seen[item.EntityID] || !bytes.Contains(item.JSON, []byte(item.EntityID)) {
				t.Fatal("page replaced, omitted or repeated durable intent")
			}
			seen[item.EntityID] = true
			clear(item.JSON)
			clear(item.Captured)
		}
		if page.Next == "" {
			break
		}
		if page.Next == cursor {
			t.Fatal("empty page did not advance")
		}
		cursor = page.Next
	}
	if len(seen) != 200 || pages < 29 || probe.downloads != 0 || probe.acknowledgments != 0 {
		t.Fatal("enumeration truncated or mutated queued jobs", len(seen), pages)
	}
	page, err := r.PendingContents(t.Context(), "ingest", "asset", "", 1)
	if err != nil || page.Next == "" {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		clear(item.JSON)
		clear(item.Captured)
	}
	if _, err := r.PendingContents(t.Context(), "other", "asset", page.Next, 1); !errors.Is(err, ErrSource) {
		t.Fatal("cursor crossed its operation", err)
	}
	r.cfg.Scope.Subject = "other"
	if _, err := r.PendingContents(t.Context(), "ingest", "asset", page.Next, 1); !errors.Is(err, ErrSource) {
		t.Fatal("cursor crossed its scope", err)
	}
}

func TestPendingContentsRefusesRetargetingAndKeepsReadyGate(t *testing.T) {
	r, _, _ := retainedReaderFixture(t)
	target := r.binding.Target
	work := mutation{Version: 1, Scope: r.cfg.Scope, Operation: "ingest", Input: hashBytes([]byte("source")), Entity: "asset", ID: "asset", State: "live", JSON: []byte(`{"id":"asset"}`), Content: true, Target: &target, ContentInput: []byte(`{"staged":"source"}`)}
	key := "mutation_" + hashBytes([]byte(work.Operation+"\x00"+work.Input))
	if err := r.cfg.Journal.Write(r.owner(), r.namespace(), key, work); err != nil {
		t.Fatal(err)
	}
	r.ready = false
	if _, err := r.PendingContents(t.Context(), "ingest", "asset", "", 8); !errors.Is(err, ErrNotReady) {
		t.Fatal("enumeration bypassed readiness", err)
	}
	r.ready = true
	r.binding.Target.DeviceID = "OTHER"
	if page, err := r.PendingContents(t.Context(), "ingest", "asset", "", 8); !errors.Is(err, ErrSource) || len(page.Items) != 0 {
		t.Fatal("enumeration silently retargeted original input", err)
	}
}

func TestPendingContentRejectsAmbiguousOrInvalidJournal(t *testing.T) {
	for _, change := range []string{"scope", "key", "capture", "legacy", "multiple", "limit", "canceled", "not ready"} {
		t.Run(change, func(t *testing.T) {
			r, _, _ := retainedReaderFixture(t)
			target := r.binding.Target
			work := mutation{Version: 1, Scope: r.cfg.Scope, Operation: "recipe", Input: "input", Entity: "edit", ID: "edit", State: "live", JSON: []byte(`{"id":"edit"}`), Content: true, Target: &target, ContentInput: []byte(`{"base":"old"}`)}
			key := "mutation_" + hashBytes([]byte(work.Operation+"\x00"+work.Input))
			switch change {
			case "scope":
				work.Scope.Subject = "other"
			case "key":
				key = "mutation_wrong"
			case "capture":
				work.ContentInput = []byte("invalid")
			case "legacy":
				work.Content = false
			case "not ready":
				r.ready = false
			}
			if err := r.cfg.Journal.Write(r.owner(), r.namespace(), key, work); err != nil {
				t.Fatal(err)
			}
			if change == "multiple" || change == "limit" {
				count := 1
				if change == "limit" {
					count = 129
				}
				for i := range count {
					other := work
					other.Input = hashBytes([]byte{byte(i)})
					if change == "limit" {
						other.ID = "unrelated"
					}
					otherKey := "mutation_" + hashBytes([]byte(other.Operation+"\x00"+other.Input))
					if err := r.cfg.Journal.Write(r.owner(), r.namespace(), otherKey, other); err != nil {
						t.Fatal(err)
					}
				}
			}
			ctx := t.Context()
			if change == "canceled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if result, err := r.ReadPendingContent(ctx, "recipe", "edit", "edit"); result != nil || err == nil {
				t.Fatal("unsafe pending intent became resumable", change, err)
			}
		})
	}
}
