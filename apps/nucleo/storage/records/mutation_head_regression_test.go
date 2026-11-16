// SPDX-License-Identifier: Apache-2.0
package records

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// The transport is deliberately in-memory. The test exercises the real
// records.Mutate and encrypted journal, not Matrix, crypto or power-loss durability.
type mutationHeadServer struct {
	next    int
	objects map[storage.Reference]storage.ObjectVersion
	records map[storage.Reference]storage.Record
}

type mutationHeadPort struct {
	Crypto
	server          *mutationHeadServer
	binding         storage.Binding
	nextTargetError error
}

func (p *mutationHeadPort) StorageIdentityVersion() storage.IdentityVersion {
	return storage.IsolatedIdentity
}

func (p *mutationHeadPort) StorageBindings(context.Context) (storage.Binding, error) {
	return p.binding, nil
}

func (p *mutationHeadPort) CheckStoragePublicationTarget(target storage.PublicationTarget) error {
	if p.nextTargetError != nil {
		err := p.nextTargetError
		p.nextTargetError = nil
		return err
	}
	if target != p.binding.Target {
		return ErrSource
	}
	return nil
}

func (p *mutationHeadPort) StorageObjectHistory(_ context.Context, room, cursor string, _ int) (storage.ObjectPage, error) {
	if room != p.binding.Target.RoomID || cursor != "" {
		return storage.ObjectPage{}, ErrSource
	}
	var page storage.ObjectPage
	for _, version := range p.server.objects {
		payload, err := json.Marshal(version.Commit)
		if err != nil {
			return storage.ObjectPage{}, err
		}
		previous := make([]string, 0, len(version.Commit.Parents))
		for _, parent := range version.Commit.Parents {
			previous = append(previous, parent.EventID)
		}
		page.Entries = append(page.Entries, storage.RecordEntry{Reference: version.Reference, Record: storage.Record{
			Version: 1, Scope: p.binding.Scope, ObjectID: version.ObjectID, RevisionID: version.RevisionID,
			Role: "index", Previous: previous, Payload: payload,
		}})
	}
	return page, nil
}

func (p *mutationHeadPort) reference() storage.Reference {
	p.server.next++
	return storage.Reference{RoomID: p.binding.Target.RoomID, EventID: fmt.Sprintf("$event%d", p.server.next)}
}

func (p *mutationHeadPort) PutStorageRecordAt(_ context.Context, target storage.PublicationTarget, _ string, record storage.Record) (storage.Reference, error) {
	if err := p.CheckStoragePublicationTarget(target); err != nil {
		return storage.Reference{}, err
	}
	ref := p.reference()
	record.Payload = bytes.Clone(record.Payload)
	record.Previous = slices.Clone(record.Previous)
	p.server.records[ref] = record
	return ref, nil
}

func (p *mutationHeadPort) ReadStorageRecord(_ context.Context, ref storage.Reference, object, role string) (storage.Record, error) {
	record, ok := p.server.records[ref]
	if !ok || record.ObjectID != object || record.Role != role {
		return storage.Record{}, ErrSource
	}
	record.Payload = bytes.Clone(record.Payload)
	record.Previous = slices.Clone(record.Previous)
	return record, nil
}

func (p *mutationHeadPort) PutStorageObjectAt(_ context.Context, target storage.PublicationTarget, _, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	if err := p.CheckStoragePublicationTarget(target); err != nil {
		return storage.ObjectVersion{}, err
	}
	ref := p.reference()
	version := storage.ObjectVersion{Reference: ref, ObjectID: object, RevisionID: revision, Commit: commit}
	p.server.objects[ref] = cloneVersion(version)
	return cloneVersion(version), nil
}

func (p *mutationHeadPort) StorageObject(_ context.Context, ref storage.Reference, object string) (storage.ObjectVersion, error) {
	version, ok := p.server.objects[ref]
	if !ok || version.ObjectID != object {
		return storage.ObjectVersion{}, ErrSource
	}
	return cloneVersion(version), nil
}

func mutationHeadClient(t *testing.T, server *mutationHeadServer, journal *offline.Store, device string) *Repository {
	t.Helper()
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "contacts"}
	binding := storage.Binding{Scope: scope, Target: storage.PublicationTarget{RoomID: "!contacts:test", DeviceID: device}, Rooms: []string{"!contacts:test"}}
	port := &mutationHeadPort{server: server, binding: binding}
	// ready is established by the fixture; actual source import is not tested here.
	r := &Repository{cfg: Config{
		Context: t.Context(), Scope: scope, Product: "contacts", EntityKind: "contact-entity", ReceiptKind: "contact-receipt", Crypto: port, Gateway: &api.Drive{}, Journal: journal,
		ValidateRow: func(_ context.Context, _, _ string, raw []byte) error {
			if !json.Valid(raw) {
				return ErrSource
			}
			return nil
		},
	}, gate: offline.NewWorkGate(t.Context()), ready: true}
	t.Cleanup(r.Close)
	return r
}

func mutationHeadJournal(t *testing.T) *offline.Store {
	t.Helper()
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	return journal
}

func TestRetainedMutationDoesNotAdoptAChangedHead(t *testing.T) {
	for _, scenario := range []struct {
		name        string
		sameJournal bool
		publication bool
	}{
		{"before-publication/different-device-and-journal", false, false},
		{"before-publication/same-device-and-journal", true, false},
		{"after-publication-plan/different-device-and-journal", false, true},
		{"after-publication-plan/same-device-and-journal", true, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := t.Context()
			server := &mutationHeadServer{objects: map[storage.Reference]storage.ObjectVersion{}, records: map[storage.Reference]storage.Record{}}
			journalA := mutationHeadJournal(t)
			a := mutationHeadClient(t, server, journalA, "DEVICE_A")
			journalB, deviceB := mutationHeadJournal(t), "DEVICE_B"
			if scenario.sameJournal {
				journalB, deviceB = journalA, "DEVICE_A"
			}
			b := mutationHeadClient(t, server, journalB, deviceB)
			const kind, id = "contact", "person"
			initial := []byte(`{"note":"initial","photo":"old"}`)
			photoA := []byte(`{"note":"initial","photo":"new"}`)
			noteB := []byte(`{"note":"changed by B","photo":"old"}`)
			seed := func(raw []byte) func(context.Context) (string, string, string, []byte, error) {
				return func(context.Context) (string, string, string, []byte, error) {
					return kind, id, "live", bytes.Clone(raw), nil
				}
			}
			if _, err := a.Mutate(ctx, "seed", id, seed(initial)); err != nil {
				t.Fatal("seed", err)
			}
			oldHead, err := a.ReadyVersion(kind, id)
			if err != nil {
				t.Fatal(err)
			}
			stop := errors.New("synthetic interruption after mutation WAL write")
			valid := a.cfg.ValidateRow
			if scenario.publication {
				a.cfg.Crypto.(*mutationHeadPort).nextTargetError = stop
			} else {
				a.cfg.ValidateRow = func(context.Context, string, string, []byte) error { return stop }
			}
			input := struct{ ID, ETag, Photo string }{id, "observed-etag", "new"}
			calls := 0
			prepareA := func(context.Context) (string, string, string, []byte, error) {
				calls++
				observed, err := a.ReadyVersion(kind, id)
				if err != nil || observed.Reference != oldHead.Reference {
					t.Fatal("A did not prepare from the original head", err)
				}
				return kind, id, "live", bytes.Clone(photoA), nil
			}
			if _, err := a.Mutate(ctx, "set-photo-by-id", input, prepareA); !errors.Is(err, stop) || calls != 1 {
				t.Fatal("failed to stop after retaining A's intent", err, calls)
			}
			a.cfg.ValidateRow = valid
			encoded, _ := json.Marshal(input)
			key := "mutation_" + hashBytes([]byte("set-photo-by-id\x00"+hashBytes(encoded)))
			var retained mutation
			if found, err := journalA.Read(a.owner(), a.namespace(), key, &retained); err != nil || !found || !bytes.Equal(retained.JSON, photoA) {
				t.Fatal("A's exact mutation was not retained", err)
			}
			var before publication
			if found, err := journalA.Read(a.owner(), a.namespace(), key+"_publication", &before); err != nil || found != scenario.publication {
				t.Fatal("the cut did not retain the expected publication plan", err)
			}
			if scenario.publication && !slices.Equal(before.Parents, []storage.Reference{oldHead.Reference}) {
				t.Fatal("publication plan did not pin the original head")
			}
			if _, err := b.Mutate(ctx, "set-note", struct{ ID, Note string }{id, "changed by B"}, seed(noteB)); err != nil {
				t.Fatal("independent writer B", err)
			}
			newHead, err := b.ReadyVersion(kind, id)
			if err != nil || newHead.Reference == oldHead.Reference || !slices.Equal(newHead.Commit.Parents, []storage.Reference{oldHead.Reference}) {
				t.Fatal("B did not advance the original head", err)
			}
			_, retryErr := a.Mutate(ctx, "set-photo-by-id", input, prepareA)
			if calls != 1 {
				t.Fatal("retry silently re-prepared A against another revision")
			}
			// Rejection must retain work. Alternatively A can publish a sibling of B,
			// keeping both branches, but it must never make stale A a child of B.
			if retryErr != nil {
				if !errors.Is(retryErr, ErrConflict) {
					t.Fatal("retry failed for a reason other than the changed head", retryErr)
				}
				if found, err := journalA.Read(a.owner(), a.namespace(), key, &retained); err != nil || !found {
					t.Fatal("failed retry discarded the original WAL", err)
				}
			}
			catalog, err := storage.LoadCatalog(ctx, a.cfg.Crypto, a.cfg.Scope)
			if err != nil {
				t.Fatal("cannot inspect complete history", err)
			}
			wantHeads := 1
			if retryErr == nil {
				wantHeads = 2
			}
			if len(catalog.Heads) != wantHeads {
				t.Fatal("retry did not retain the expected heads", len(catalog.Heads), retryErr)
			}
			foundB, foundA := false, false
			for _, head := range catalog.Heads {
				if head.ObjectID != oldHead.ObjectID || head.Commit.Content == nil || head.Commit.Content.Kind != "record" {
					t.Fatal("unexpected head or content pointer")
				}
				record, err := a.cfg.Crypto.ReadStorageRecord(ctx, head.Commit.Content.Reference, head.ObjectID, "content")
				if err != nil {
					t.Fatal("cannot read head content", err)
				}
				var value content
				if json.Unmarshal(record.Payload, &value) != nil {
					t.Fatal("cannot decode head content")
				}
				switch head.Reference {
				case newHead.Reference:
					foundB = bytes.Equal(value.JSON, noteB)
				default:
					foundA = bytes.Equal(value.JSON, photoA) && slices.Equal(head.Commit.Parents, []storage.Reference{oldHead.Reference})
				}
			}
			if !foundB || foundA != (retryErr == nil) {
				t.Fatal("retry lost B's exact content or did not preserve A as its sibling")
			}
		})
	}
}
