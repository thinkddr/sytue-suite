// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"
)

// Public record inventory fixture only. Production still requires the scoped
// maintained SDK. Native integration covers the actual encrypted event transport.
type atomicCatalogPort struct {
	*catalogPort
	records map[Reference]Record
	onRead  func()
	reads   int
}

func (p *atomicCatalogPort) ReadStorageRecord(_ context.Context, ref Reference, object, role string) (Record, error) {
	p.reads++
	if p.onRead != nil {
		p.onRead()
	}
	record, ok := p.records[ref]
	if !ok {
		return Record{}, ErrIncompleteCatalog
	}
	return record, nil
}

func atomicPayload(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func atomicTestPort(t *testing.T, count int, commit bool) *atomicCatalogPort {
	t.Helper()
	p := &atomicCatalogPort{catalogPort: &catalogPort{binding: Binding{Scope: catalogScope, Target: PublicationTarget{RoomID: "!private:test", DeviceID: "DEVICE"}, Rooms: []string{"!private:test"}}, pages: map[string]map[string]ObjectPage{}}, records: map[Reference]Record{}}
	entries := []RecordEntry{}
	members := []AtomicMember{}
	for i := 0; i < count; i++ {
		old := catalogEntry(fmt.Sprint("old", i), "r0", "live")
		old.Record.ObjectID = fmt.Sprint("object", i)
		entries = append(entries, old)
		prepared := catalogEntry(fmt.Sprint("prepared", i), "r1", "deleted", old.Reference)
		prepared.Record.ObjectID = old.Record.ObjectID
		var body ObjectCommit
		if err := json.Unmarshal(prepared.Record.Payload, &body); err != nil {
			t.Fatal(err)
		}
		prepared.Record.Payload = atomicPayload(t, AtomicPrepared{Kind: ObjectPreparedKind, BatchID: "batch", Ordinal: uint64(i), Commit: body})
		entries = append(entries, prepared)
		sum, err := AtomicRecordSHA256(prepared.Record)
		if err != nil {
			t.Fatal(err)
		}
		members = append(members, AtomicMember{Reference: prepared.Reference, ObjectID: prepared.Record.ObjectID, RevisionID: prepared.Record.RevisionID, SHA256: sum})
	}
	var last *AtomicPagePointer
	for start := 0; start < len(members); start += 37 {
		end := min(start+37, len(members))
		revision := fmt.Sprint("page", start)
		ref := Reference{RoomID: "!private:test", EventID: "$" + revision}
		page := AtomicBatchPage{Kind: ObjectBatchPageKind, BatchID: "batch", Start: uint64(start), Members: members[start:end], Previous: last}
		record := Record{Version: 1, Scope: catalogScope, ObjectID: AtomicBatchObjectID("batch"), RevisionID: revision, Role: "manifest", Payload: atomicPayload(t, page)}
		sum, err := AtomicRecordSHA256(record)
		if err != nil {
			t.Fatal(err)
		}
		p.records[ref] = record
		last = &AtomicPagePointer{Reference: ref, RevisionID: revision, SHA256: sum}
	}
	if commit {
		root := RecordEntry{Reference: Reference{RoomID: "!private:test", EventID: "$commit"}, Record: Record{Version: 1, Scope: catalogScope, ObjectID: AtomicBatchObjectID("batch"), RevisionID: "commit", Role: "index", Payload: atomicPayload(t, AtomicBatchCommit{Kind: ObjectBatchKind, BatchID: "batch", Count: uint64(count), Last: *last})}}
		entries = append(entries, root)
	}
	// Bound history pages too: all entries survive even when no indexes appear in
	// an intermediate network page. This shares the production complete-room scan.
	for start := 0; start < len(entries); start += 100 {
		end := min(start+100, len(entries))
		cursor := ""
		if start > 0 {
			cursor = fmt.Sprint(start)
		}
		next := ""
		if end < len(entries) {
			next = fmt.Sprint(end)
		}
		if p.pages["!private:test"] == nil {
			p.pages["!private:test"] = map[string]ObjectPage{}
		}
		p.pages["!private:test"][cursor] = ObjectPage{Entries: entries[start:end], Next: next}
	}
	return p
}

func atomicEntry(p *atomicCatalogPort, ref string) *RecordEntry {
	for cursor, page := range p.pages["!private:test"] {
		for i := range page.Entries {
			if page.Entries[i].Reference.EventID == ref {
				return &page.Entries[i]
			}
		}
		p.pages["!private:test"][cursor] = page
	}
	return nil
}

func TestAtomicCatalogCommitActivatesAllAndRetainsExactRawHistory(t *testing.T) {
	p := atomicTestPort(t, 83, true)
	s, err := LoadAtomicCatalog(t.Context(), p, catalogScope)
	if err != nil || len(s.Heads) != 83 || len(s.History) != 166 || len(s.Entries) != 167 || len(s.Pages) != 3 || p.reads != 3 {
		t.Fatal("atomic inventory lost or truncated", err, len(s.Heads), len(s.Entries), len(s.Pages), p.reads)
	}
	for _, head := range s.Heads {
		if head.Commit.State != "deleted" {
			t.Fatal("subset of prepared revisions activated")
		}
	}
	raw := s.Entries[Reference{RoomID: "!private:test", EventID: "$prepared0"}]
	if !bytes.Contains(raw.Record.Payload, []byte(ObjectPreparedKind)) {
		t.Fatal("raw prepared envelope normalized away")
	}
	before := bytes.Clone(raw.Record.Payload)
	atomicEntry(p, "$prepared0").Record.Payload[0] ^= 1
	if !bytes.Equal(s.Entries[raw.Reference].Record.Payload, before) {
		t.Fatal("snapshot aliases source buffers")
	}
	if !bytes.Contains(s.History[0].Commit.Metadata, []byte("9007199254740993")) {
		t.Fatal("domain integer rounded")
	}
}

func TestAtomicCatalogOrphansKeepPriorHistoryAndLegacyReadersFailClosed(t *testing.T) {
	p := atomicTestPort(t, 3, false)
	s, err := LoadAtomicCatalog(t.Context(), p, catalogScope)
	if err != nil || len(s.Heads) != 3 || len(s.History) != 3 || len(s.Entries) != 6 || len(s.Pages) != 0 || p.reads != 0 {
		t.Fatal("orphan work hid old history or became visible", err)
	}
	for _, head := range s.Heads {
		if head.Commit.State != "live" {
			t.Fatal("uncommitted object visible")
		}
	}
	if s, err := LoadCatalog(t.Context(), p, catalogScope); err == nil || s.Entries != nil {
		t.Fatal("legacy reader accepted prepared protocol")
	}
}

func TestAtomicCatalogNoArbitraryTenThousandObjectBatchLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("large retained graph")
	}
	p := atomicTestPort(t, 10017, true)
	s, err := LoadAtomicCatalog(t.Context(), p, catalogScope)
	if err != nil || len(s.Heads) != 10017 || len(s.History) != 20034 || len(s.Entries) != 20035 || len(s.Pages) != 271 {
		t.Fatal("large batch capped or partially applied", err, len(s.Heads), len(s.Pages))
	}
}

func TestAtomicCatalogBrokenCommitNeverReturnsSubset(t *testing.T) {
	for _, fault := range []string{"member-missing", "member-bytes", "member-object", "member-scope", "ordinal-collision", "root-collision", "page-missing", "page-bytes", "page-scope", "page-revision", "page-kind", "page-gap", "page-cycle", "page-unknown-field", "root-count", "root-foreign-room", "binding-after-manifest", "cancel-after-manifest"} {
		t.Run(fault, func(t *testing.T) {
			p := atomicTestPort(t, 83, true)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			rootEntry := atomicEntry(p, "$commit")
			var root AtomicBatchCommit
			_ = json.Unmarshal(rootEntry.Record.Payload, &root)
			switch fault {
			case "member-missing":
				e := atomicEntry(p, "$prepared82")
				e.Reference.EventID = "$other"
				e.Record.Payload = atomicPayload(t, AtomicPrepared{Kind: ObjectPreparedKind, BatchID: "uncommitted", Ordinal: 0, Commit: ObjectCommit{Kind: ObjectKind, State: "deleted", Metadata: []byte("null")}})
			case "member-bytes":
				e := atomicEntry(p, "$prepared0")
				var v AtomicPrepared
				_ = json.Unmarshal(e.Record.Payload, &v)
				v.Commit.Metadata = []byte(`{"unknown":9007199254740995}`)
				e.Record.Payload = atomicPayload(t, v)
			case "member-object":
				atomicEntry(p, "$prepared0").Record.ObjectID = "foreign"
			case "member-scope":
				atomicEntry(p, "$prepared0").Record.Scope.Product = "mail"
			case "ordinal-collision":
				var v AtomicPrepared
				_ = json.Unmarshal(atomicEntry(p, "$prepared1").Record.Payload, &v)
				v.Ordinal = 0
				atomicEntry(p, "$prepared1").Record.Payload = atomicPayload(t, v)
			case "root-collision":
				page := p.pages["!private:test"]["100"]
				cloned := *rootEntry
				cloned.Reference.EventID = "$duplicate-root"
				page.Entries = append(page.Entries, cloned)
				p.pages["!private:test"]["100"] = page
			case "page-missing":
				delete(p.records, root.Last.Reference)
			case "page-bytes":
				r := p.records[root.Last.Reference]
				r.Payload = bytes.Replace(r.Payload, []byte("batch-page"), []byte("batch-pxge"), 1)
				p.records[root.Last.Reference] = r
			case "page-scope":
				r := p.records[root.Last.Reference]
				r.Scope.Subject = "other"
				p.records[root.Last.Reference] = r
			case "page-revision":
				r := p.records[root.Last.Reference]
				r.RevisionID = "foreign"
				p.records[root.Last.Reference] = r
			case "page-kind", "page-gap", "page-cycle", "page-unknown-field":
				r := p.records[root.Last.Reference]
				var page AtomicBatchPage
				_ = json.Unmarshal(r.Payload, &page)
				switch fault {
				case "page-kind":
					page.Kind = "unknown"
				case "page-gap":
					page.Start++
				case "page-cycle":
					cloned := root.Last
					page.Previous = &cloned
				case "page-unknown-field":
				}
				r.Payload = atomicPayload(t, page)
				if fault == "page-unknown-field" {
					r.Payload = append(r.Payload[:len(r.Payload)-1], []byte(",\"unknown\":1}")...)
				}
				p.records[root.Last.Reference] = r
				root.Last.SHA256, _ = AtomicRecordSHA256(r)
				rootEntry.Record.Payload = atomicPayload(t, root)
			case "root-count":
				root.Count--
				rootEntry.Record.Payload = atomicPayload(t, root)
			case "root-foreign-room":
				root.Last.Reference.RoomID = "!foreign:test"
				rootEntry.Record.Payload = atomicPayload(t, root)
			case "binding-after-manifest":
				p.onRead = func() { p.binding.Target.DeviceID = "CHANGED" }
			case "cancel-after-manifest":
				p.onRead = cancel
			}
			s, err := LoadAtomicCatalog(ctx, p, catalogScope)
			if err == nil || s.Entries != nil || s.History != nil || s.Heads != nil || s.Binding.Valid() {
				t.Fatal("broken commit exposed partial projection", fault, err)
			}
			if fault == "cancel-after-manifest" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation masked", err)
			}
		})
	}
}

func TestAtomicCatalogConcurrentBranchesRemainExplicit(t *testing.T) {
	p := atomicTestPort(t, 1, true)
	concurrent := catalogEntry("concurrent", "r2", "live", Reference{RoomID: "!private:test", EventID: "$old0"})
	concurrent.Record.ObjectID = "object0"
	page := p.pages["!private:test"][""]
	page.Entries = append(page.Entries, concurrent)
	slices.Reverse(page.Entries)
	p.pages["!private:test"][""] = page
	s, err := LoadAtomicCatalog(t.Context(), p, catalogScope)
	if err != nil || len(s.Heads) != 2 || s.Heads[0].Commit.State == s.Heads[1].Commit.State {
		t.Fatal("clock or arrival order chose branch", err, s.Heads)
	}
}
