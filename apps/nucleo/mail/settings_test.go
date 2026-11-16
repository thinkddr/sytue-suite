// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestPrivateMailSettingsMaterializeAuthenticatedArchiveWithoutChangingV2Receipt(t *testing.T) {
	f := setupImport(t)
	if err := f.repo.ImportRetainedSource(t.Context(), f.source); err != nil {
		t.Fatal(err)
	}
	snapshot, err := storage.LoadAtomicCatalog(t.Context(), f.crypto, testScope)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := f.repo.loadImportReceipt(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range receipt.Sources {
		if item.Source.Kind != "filter" && item.Source.Kind != "vacation" {
			continue
		}
		if item.Projection != nil {
			t.Fatal("historical receipt was rewritten")
		}
		id := item.Source.ID
		if item.Source.Kind == "vacation" {
			id = "vacation"
		}
		raw, err := f.repo.document(t.Context(), snapshot, item.Source.Kind, id)
		if err != nil {
			t.Fatal("archived setting not materialized", err)
		}
		original := f.source.bodies[storage.LegacySourceCursor{Kind: item.Source.Kind, ID: item.Source.ID}]
		if !bytes.Equal(raw, original) {
			t.Fatal("original JSON bytes changed")
		}
	}
	pages, opens, puts := f.source.pages, f.source.opens, f.crypto.puts
	f.repo.Close()
	reopened := f.reopen(t)
	if err := reopened.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if f.source.pages != pages || f.source.opens != opens || f.crypto.puts != puts {
		t.Fatal("recovery reimported SQL or republished settings")
	}
	if value, err := reopened.Vacation(t.Context()); err != nil || value.Body != "Private auto-reply" || value.PrivateRevision == "" {
		t.Fatal(value, err)
	}
}

func TestPrivateMailSettingsCRUDPreservesJSONAndRejectsStaleWriters(t *testing.T) {
	f := setup(t, true)
	f.open(t)
	for _, seed := range []struct {
		id, date string
		priority int
	}{{"second", "2026-10-30T12:00:00Z", 1}, {"first", "2026-10-30T11:00:00Z", 1}, {"earliest-priority", "2026-10-30T13:00:00Z", 0}} {
		f.crypto.seed(t, "filter", seed.id, json.RawMessage(`{"id":"`+seed.id+`","name":"Rule","priority":`+string(rune('0'+seed.priority))+`,"enabled":true,"match_from":"sender","mark_read":true,"created_at":"`+seed.date+`","account_id":"retained-owner","extension":{"precise":900719925474099512345}}`))
	}
	filters, err := f.repo.Filters(t.Context())
	if err != nil || len(filters) != 3 || filters[0].ID != "earliest-priority" || filters[1].ID != "first" {
		t.Fatal("priority/created_at precedence changed", filters, err)
	}
	old := filters[1]
	changed := old
	changed.Name = "Edited rule"
	if err := f.repo.UpdateFilter(t.Context(), changed); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.UpdateFilter(t.Context(), old); !errors.Is(err, ErrConflict) {
		t.Fatal("stale editor overwrote settings", err)
	}
	snapshot, _ := storage.LoadAtomicCatalog(t.Context(), f.crypto, testScope)
	raw, err := f.repo.document(t.Context(), snapshot, "filter", old.ID)
	if err != nil || !bytes.Contains(raw, []byte(`900719925474099512345`)) || !bytes.Contains(raw, []byte(`"account_id":"retained-owner"`)) || !bytes.Contains(raw, []byte(`2026-10-30T11:00:00Z`)) {
		t.Fatal("edit discarded unknown fields", string(raw), err)
	}
	if _, err := f.repo.CreateFilter(t.Context(), api.MailFilter{Name: "Invalid"}); !errors.Is(err, model.ErrInvalidFilter) {
		t.Fatal("domain rule weakened", err)
	}
	if _, err := f.repo.CreateFilter(t.Context(), api.MailFilter{MatchFrom: "x", MoveTo: "11111111-1111-1111-1111-111111111111"}); !errors.Is(err, model.ErrInvalidFilter) {
		t.Fatal("foreign mailbox accepted", err)
	}
	created, err := f.repo.CreateFilter(t.Context(), api.MailFilter{Name: "New", MatchFrom: "sender", MarkFlagged: true})
	if err != nil || created.ID == "" || created.PrivateRevision == "" {
		t.Fatal(created, err)
	}
	if err := f.repo.DeleteFilter(t.Context(), created.ID); err != nil {
		t.Fatal(err)
	}
	filters, err = f.repo.Filters(t.Context())
	if err != nil || slices.ContainsFunc(filters, func(v api.MailFilter) bool { return v.ID == created.ID }) {
		t.Fatal("deleted rule resurfaced", err)
	}
	box, err := f.repo.CreateMailbox(t.Context(), "Private destination")
	if err != nil {
		t.Fatal(err)
	}
	withDestination, err := f.repo.CreateFilter(t.Context(), api.MailFilter{Name: "Move", MatchFrom: "sender", MoveTo: box.ID})
	if err != nil {
		t.Fatal("existing same-owner destination rejected", err)
	}
	if _, err := f.repo.DeleteMailbox(t.Context(), box.ID); !errors.Is(err, api.ErrPrivateMailOperation) {
		t.Fatal("folder deletion left a dangling rule", err)
	}
	if err := f.repo.DeleteFilter(t.Context(), withDestination.ID); err != nil {
		t.Fatal(err)
	}
	if refusal, err := f.repo.DeleteMailbox(t.Context(), box.ID); err != nil || refusal != "" {
		t.Fatal("unreferenced folder deletion failed", refusal, err)
	}
	start := time.Date(2026, 10, 30, 0, 0, 0, 123456789, time.UTC)
	f.crypto.seed(t, "vacation", "vacation", json.RawMessage(`{"enabled":true,"subject":"Away","body":"Original","account_id":"owner","extension":{"precise":900719925474099512345}}`))
	vac, err := f.repo.Vacation(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	stale := vac
	vac.Body = "Changed"
	vac.StartsAt = &start
	if err := f.repo.SetVacation(t.Context(), vac); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.SetVacation(t.Context(), stale); !errors.Is(err, ErrConflict) {
		t.Fatal("stale vacation overwrote settings", err)
	}
	vac, err = f.repo.Vacation(t.Context())
	if err != nil || vac.StartsAt == nil || !vac.StartsAt.Equal(start) || vac.UpdatedAt.IsZero() {
		t.Fatal("vacation lost full model", vac, err)
	}
	snapshot, _ = storage.LoadAtomicCatalog(t.Context(), f.crypto, testScope)
	raw, err = f.repo.document(t.Context(), snapshot, "vacation", "vacation")
	if err != nil || !bytes.Contains(raw, []byte(`900719925474099512345`)) || !bytes.Contains(raw, []byte(`"account_id":"owner"`)) {
		t.Fatal("vacation discarded original fields", err)
	}
}

func TestPrivateMailFilterCreateRetriesAcceptedLostACKWithAssignedID(t *testing.T) {
	f := setup(t, true)
	f.open(t)
	f.crypto.loseIndex = true
	value, err := f.repo.CreateFilter(t.Context(), api.MailFilter{Name: "Retry", MatchSubject: "subject", MarkRead: true})
	if err == nil || value.ID == "" {
		t.Fatal("fixture did not lose accepted ACK", err)
	}
	retried, err := f.repo.CreateFilter(t.Context(), value)
	if err != nil || retried.ID != value.ID {
		t.Fatal("creation retry lost operation ID", err)
	}
	filters, err := f.repo.Filters(t.Context())
	if err != nil || len(filters) != 1 {
		t.Fatal("creation retry duplicated rule", filters, err)
	}
	owner := testScope.Tenant + "\x00" + testScope.Subject + "\x00mail"
	pending, err := f.repo.cfg.Journal.List(owner, writesNamespace)
	if err != nil || len(pending) != 0 {
		t.Fatal("acknowledged creation left private work pending", len(pending), err)
	}
}

func TestPrivateMailSettingsUpgradeRetriesAcceptedLostACKWithoutReimport(t *testing.T) {
	f := setupImport(t)
	if err := f.repo.ImportRetainedSource(t.Context(), f.source); err != nil {
		t.Fatal(err)
	}
	// Reconstruct an older V2 device: its receipt and all authenticated
	// archives exist, but this version has not materialized settings yet.
	for ref, record := range f.crypto.records {
		if record.ObjectID == objectID("filter", "large") || record.ObjectID == objectID("vacation", "vacation") {
			delete(f.crypto.records, ref)
			for key, value := range f.crypto.transactions {
				if value == ref {
					delete(f.crypto.transactions, key)
				}
			}
		}
	}
	f.repo.Close()
	pages, opens := f.source.pages, f.source.opens
	f.crypto.loseIndex = true
	upgraded := f.reopen(t)
	if err := upgraded.Initialize(t.Context()); err == nil {
		t.Fatal("materialization did not lose accepted ACK")
	}
	if _, err := upgraded.Filters(t.Context()); !errors.Is(err, ErrNotReady) {
		t.Fatal("partial materialization activated", err)
	}
	upgraded.Close()
	upgraded = f.reopen(t)
	if err := upgraded.Initialize(t.Context()); err != nil {
		t.Fatal("materialization did not resume", err)
	}
	if f.source.pages != pages || f.source.opens != opens {
		t.Fatal("settings upgrade reimported SQL")
	}
	if values, err := upgraded.Filters(t.Context()); err != nil || len(values) != 1 || values[0].ID != "large" {
		t.Fatal("settings upgrade duplicated/lost archive", values, err)
	}
	snapshot, err := storage.LoadAtomicCatalog(t.Context(), f.crypto, testScope)
	if err != nil {
		t.Fatal(err)
	}
	if len(heads(snapshot, objectID("filter", "large"))) != 1 {
		t.Fatal("accepted ACK retry forked settings")
	}
}

func TestPrivateMailSettingsConflictingBranchesBlockReadsAndWrites(t *testing.T) {
	f := setup(t, true)
	f.open(t)
	first := f.crypto.seed(t, "filter", "conflict", api.MailFilter{ID: "conflict", MatchFrom: "first", MarkRead: true})
	f.crypto.seed(t, "filter", "conflict", api.MailFilter{ID: "conflict", MatchFrom: "left", MarkRead: true}, first)
	f.crypto.seed(t, "filter", "conflict", api.MailFilter{ID: "conflict", MatchFrom: "right", MarkRead: true}, first)
	if _, err := f.repo.Filters(t.Context()); !errors.Is(err, ErrConflict) {
		t.Fatal("conflict chose a visible winner", err)
	}
	if err := f.repo.DeleteFilter(t.Context(), "conflict"); !errors.Is(err, ErrConflict) {
		t.Fatal("delete merged conflicting branches", err)
	}
}
