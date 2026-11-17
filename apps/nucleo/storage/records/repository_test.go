// SPDX-License-Identifier: Apache-2.0
package records

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestPrivateRecordsConfigurationRequiresDomainValidationAndExactProduct(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "contacts"}
	for _, cfg := range []Config{
		{},
		{Context: t.Context(), Scope: scope, Product: "notes"},
		{Context: t.Context(), Scope: scope, Product: "contacts", EntityKind: "entity", ReceiptKind: "receipt"},
	} {
		if _, err := New(cfg); !errors.Is(err, ErrSource) {
			t.Fatal("invalid engine configuration accepted", err)
		}
	}
}

func TestPrivateRecordsPublicationIdentitySupportsRetainedCompositeIDs(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "contacts"}
	r := &Repository{cfg: Config{Scope: scope, Product: "contacts", EntityKind: "org.sytue.contacts.entity.v1"}}
	id := "id:original/with unicode é"
	raw := []byte(`{"id":"retained","n":900719925474099312345}`)
	summary, _ := json.Marshal(entity{Kind: r.cfg.EntityKind, Entity: "member", ID: id, SHA256: hashBytes(raw)})
	version := storage.ObjectVersion{ObjectID: r.object("member", id), Commit: storage.ObjectCommit{Metadata: summary}}
	got, err := r.description(version)
	if err != nil || got.ID != id {
		t.Fatal("composite source identity changed", err)
	}
	for _, invalid := range []string{"", "bad\nrow", "bad\x00row"} {
		summary, _ = json.Marshal(entity{Kind: r.cfg.EntityKind, Entity: "member", ID: invalid, SHA256: hashBytes(raw)})
		version.Commit.Metadata = summary
		if _, err := r.description(version); !errors.Is(err, ErrSource) {
			t.Fatal("invalid source identity accepted", invalid, err)
		}
	}
}

func TestPrivateRecordsViewsOwnTheirMetadataAndCheckEntryIdentity(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "contacts"}
	r := &Repository{cfg: Config{Scope: scope, Product: "contacts", EntityKind: "entity", ReceiptKind: "receipt"}, ready: true, heads: map[string][]storage.ObjectVersion{}}
	summary, _ := json.Marshal(entity{Kind: "entity", Entity: "member", ID: "id:original", SHA256: hashBytes(nil)})
	version := storage.ObjectVersion{ObjectID: r.object("member", "id:original"), Commit: storage.ObjectCommit{State: "live", Metadata: summary, Parents: []storage.Reference{{RoomID: "!p:test", EventID: "$p"}}}}
	r.heads[version.ObjectID] = []storage.ObjectVersion{version}
	rows, err := (catalogView{r: r}).Entries()
	if err != nil || len(rows) != 1 {
		t.Fatal(err)
	}
	rows[0].Version.Commit.Metadata[0] = '!'
	rows[0].Version.Commit.Parents[0].EventID = "$mutated"
	if !bytes.Equal(r.heads[version.ObjectID][0].Commit.Metadata, summary) || r.heads[version.ObjectID][0].Commit.Parents[0].EventID != "$p" {
		t.Fatal("catalog view aliases engine memory")
	}
	selected, err := r.Selected("member")
	if err != nil || len(selected) != 1 {
		t.Fatal(err)
	}
	selected[0].Commit.Metadata[0] = '!'
	if !bytes.Equal(r.heads[version.ObjectID][0].Commit.Metadata, summary) {
		t.Fatal("selection aliases engine memory")
	}
	rows, err = (catalogView{r: r}).Entries()
	if err != nil {
		t.Fatal(err)
	}
	rows[0].ID = "other"
	if _, err := (catalogView{r: r}).ReadJSON(context.Background(), rows[0]); !errors.Is(err, ErrSource) {
		t.Fatal("altered entry routed to crypto", err)
	}
}

func TestPrivateRecordsPublicCatalogKeepsReadinessAndCloseGates(t *testing.T) {
	r := &Repository{cfg: Config{Context: t.Context()}, gate: offline.NewWorkGate(t.Context())}
	if _, err := r.Catalog(); !errors.Is(err, ErrNotReady) {
		t.Fatal("incomplete catalog exposed", err)
	}
	r.ready = true
	view, err := r.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	r.ready = false
	if _, err := view.Entries(); !errors.Is(err, ErrNotReady) {
		t.Fatal("old view skipped reinitialization gate", err)
	}
	if _, err := view.ReadJSON(t.Context(), Entry{}); !errors.Is(err, ErrNotReady) {
		t.Fatal("old view exposed body during reinitialization", err)
	}
	r.closed = true
	if _, err := view.Entries(); !errors.Is(err, ErrClosed) {
		t.Fatal("old view skipped close gate", err)
	}
	if _, err := r.ReadJSON(t.Context(), storage.ObjectVersion{}); !errors.Is(err, ErrClosed) {
		t.Fatal("public body skipped close gate", err)
	}
}

type recordOperationCallerKey struct{}

func TestPrivateRecordsBindTheOriginalAdmittedOperation(t *testing.T) {
	active, called := 0, 0
	gate := offline.NewWorkGateWithLease(t.Context(), func(context.Context) (func(), error) { active++; return func() { active-- }, nil })
	r := &Repository{cfg: Config{Context: t.Context(), BindPublicationContext: func(ctx context.Context) (context.Context, error) {
		called++
		if active != 1 || ctx.Value(recordOperationCallerKey{}) != "original" {
			t.Fatal("record binding discarded original lease/caller")
		}
		return context.WithValue(ctx, struct{ Publication bool }{true}, "bound"), nil
	}}, gate: gate}
	defer r.Close()
	job, done, err := r.Start(context.WithValue(t.Context(), recordOperationCallerKey{}, "original"))
	if err != nil || called != 1 || active != 1 || job.Value(struct{ Publication bool }{true}) != "bound" {
		t.Fatal("record operation did not use shared binding", err)
	}
	done()
	if active != 0 {
		t.Fatal("record lease leaked")
	}
	r.cfg.BindPublicationContext = func(context.Context) (context.Context, error) { return context.Background(), nil }
	if _, done, err := r.Start(t.Context()); err == nil || done != nil || active != 0 {
		t.Fatal("discarded record caller admitted work", err)
	}
}
