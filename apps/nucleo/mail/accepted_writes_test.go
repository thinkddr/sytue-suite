// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func retainedWrite(t *testing.T, f *fixture) (string, string, writeIntent) {
	t.Helper()
	owner := testScope.Tenant + "\x00" + testScope.Subject + "\x00mail"
	ids, err := f.repo.cfg.Journal.RecordIDs(owner, writesNamespace)
	if err != nil || len(ids) != 1 {
		t.Fatal("expected one original retained intent", err, len(ids))
	}
	var work writeIntent
	found, err := f.repo.cfg.Journal.Read(owner, writesNamespace, ids[0], &work)
	if err != nil || !found {
		t.Fatal(err)
	}
	return owner, ids[0], work
}

func TestAcceptedDocumentWriteKeyPreservesOriginalBytesAndExistingWAL(t *testing.T) {
	for _, state := range []string{"live", "deleted"} {
		for _, payload := range [][]byte{[]byte(`{"precise":900719925474099312345}`), []byte(`{"unicode":"漢字"}`)} {
			raw := make([]byte, 0, 1024)
			raw = append(raw, []byte(`{"kind":"original-header"}`)...)
			original := bytes.Clone(raw)
			expected := digest(append(append(bytes.Clone(raw), []byte("\x00"+state+"\x00")...), payload...))
			if actual := documentWriteKey(raw, payload, state); actual != expected || !bytes.Equal(raw, original) {
				t.Fatal("original immutable WAL key bytes changed")
			}
		}
	}
	f := setup(t, true)
	f.open(t)
	f.crypto.loseIndex = true
	if err := f.repo.SetSignature(t.Context(), "accepted before restart"); err == nil {
		t.Fatal("fixture did not retain its lost index ACK")
	}
	owner, id, work := retainedWrite(t, f)
	defer clear(work.Payload)
	raw, _ := json.Marshal(work.Header)
	original := digest(append(append(bytes.Clone(raw), []byte("\x00live\x00")...), work.Payload...))
	if id != original {
		t.Fatal("existing WAL key was regenerated")
	}
	cfg := f.repo.cfg
	f.repo.Close()
	f.crypto.binding.Target.DeviceID = "RECOVERED-B"
	reopened, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	before := f.crypto.puts
	if err = reopened.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if n, err := cfg.Journal.CountOwner(owner, writesNamespace); err != nil || n != 0 || f.crypto.puts != before {
		t.Fatal("current device failed exact read-only cleanup of old accepted intent", n, err)
	}
}

func TestAcceptedDocumentRecoveryPreservesNewerVisibleEditAndOtherNamespaces(t *testing.T) {
	f := setup(t, true)
	f.open(t)
	f.crypto.loseIndex = true
	if err := f.repo.SetSignature(t.Context(), "old accepted"); err == nil {
		t.Fatal("missing lost ACK")
	}
	owner, id, work := retainedWrite(t, f)
	defer clear(work.Payload)
	if err := f.repo.SetSignature(t.Context(), "newer visible edit"); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.cfg.Journal.Write(owner, writesNamespace, id, work); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.cfg.Journal.Write(owner, "other-private-work", id, work); err != nil {
		t.Fatal(err)
	}
	f.crypto.binding.Target.DeviceID = "RECOVERED-B"
	before := f.crypto.puts
	got, err := f.repo.Signature(t.Context())
	if err != nil || got != "newer visible edit" || f.crypto.puts != before {
		t.Fatal("historical ACK cleanup republished or chose an old visible value", err)
	}
	if n, err := f.repo.cfg.Journal.CountOwner(owner, writesNamespace); err != nil || n != 0 {
		t.Fatal("accepted metadata WAL retained", n, err)
	}
	if n, err := f.repo.cfg.Journal.CountOwner(owner, "other-private-work"); err != nil || n != 1 {
		t.Fatal("cleanup crossed its original namespace", n, err)
	}
}

func TestAcceptedDocumentRecoveryNeverResendsUnacceptedAndUsesRealPages(t *testing.T) {
	f := setup(t, true)
	f.open(t)
	f.crypto.loseIndex = true
	if err := f.repo.SetSignature(t.Context(), "accepted"); err == nil {
		t.Fatal("missing lost ACK")
	}
	owner, _, accepted := retainedWrite(t, f)
	defer clear(accepted.Payload)
	for i := 0; i < 130; i++ {
		work := accepted
		work.Header.ID = fmt.Sprintf("never-accepted-%d", i)
		work.Content = nil
		work.Index = nil
		raw, _ := json.Marshal(work.Header)
		id := documentWriteKey(raw, work.Payload, "live")
		if err := f.repo.cfg.Journal.Write(owner, writesNamespace, id, work); err != nil {
			t.Fatal(err)
		}
	}
	before := f.crypto.puts
	f.crypto.binding.Target.DeviceID = "RECOVERED-B"
	if _, err := f.repo.Signature(t.Context()); err != nil {
		t.Fatal(err)
	}
	if n, err := f.repo.cfg.Journal.CountOwner(owner, writesNamespace); err != nil || n != 130 || f.crypto.puts != before {
		t.Fatal("missing publication became accepted, resent or disappeared across pages", n, err)
	}
}

func TestAcceptedDocumentRecoveryRejectsForeignKeyPayloadAndScope(t *testing.T) {
	for _, fault := range []string{"scope", "key", "payload", "target"} {
		t.Run(fault, func(t *testing.T) {
			f := setup(t, true)
			f.open(t)
			f.crypto.loseIndex = true
			if err := f.repo.SetSignature(t.Context(), "accepted original"); err == nil {
				t.Fatal("missing lost ACK")
			}
			owner, id, work := retainedWrite(t, f)
			defer clear(work.Payload)
			switch fault {
			case "scope":
				work.Scope.Subject = "foreign"
			case "key":
				if err := f.repo.cfg.Journal.Delete(owner, writesNamespace, id); err != nil {
					t.Fatal(err)
				}
				id = strings.Repeat("a", 64)
			case "payload":
				work.Payload = []byte(`"forged accepted payload"`)
				raw, _ := json.Marshal(work.Header)
				if err := f.repo.cfg.Journal.Delete(owner, writesNamespace, id); err != nil {
					t.Fatal(err)
				}
				id = documentWriteKey(raw, work.Payload, "live")
			case "target":
				work.Target.RoomID = "!foreign:test"
			}
			if err := f.repo.cfg.Journal.Write(owner, writesNamespace, id, work); err != nil {
				t.Fatal(err)
			}
			before := f.crypto.puts
			if _, err := f.repo.Signature(t.Context()); !errors.Is(err, ErrRecord) {
				t.Fatal("foreign or changed accepted intent was acknowledged", fault, err)
			}
			if n, err := f.repo.cfg.Journal.CountOwner(owner, writesNamespace); err != nil || n != 1 || f.crypto.puts != before {
				t.Fatal("unverified work was erased or replayed", n, err)
			}
		})
	}
}

func TestAcceptedDocumentRoutingDeleteAndChangedMetadataDoNotStrandACK(t *testing.T) {
	f := setupDrafts(t)
	metadata := newRoutingFixture()
	f.repo.cfg.Metadata = metadata
	alias, err := f.repo.CreateRoutingAlias(t.Context(), "new@example.test")
	if err != nil {
		t.Fatal(err)
	}
	f.crypto.loseIndex = true
	if err = f.repo.DeleteRoutingAlias(t.Context(), alias.ID); err == nil {
		t.Fatal("missing accepted private delete ACK")
	}
	before := f.crypto.puts
	if err = f.repo.DeleteRoutingAlias(t.Context(), alias.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("already absent address changed existing notFound semantics", err)
	}
	owner := testScope.Tenant + "\x00" + testScope.Subject + "\x00mail"
	if n, err := f.repo.cfg.Journal.CountOwner(owner, writesNamespace); err != nil || n != 0 || metadata.deletes != 1 || f.crypto.puts != before {
		t.Fatal("private deletion ACK stranded or repeated public deletion", n, err)
	}
	f.crypto.loseIndex = true
	if _, err = f.repo.RefreshRouting(t.Context()); err != nil { // identical existing projection is an intentional no-op
		t.Fatal(err)
	}
	metadata.value.Aliases = nil
	f.crypto.loseIndex = true
	if _, err = f.repo.RefreshRouting(t.Context()); err == nil {
		t.Fatal("changed public projection did not lose its accepted ACK")
	}
	metadata.value.Primary = "new-primary@example.test"
	if _, err = f.repo.RefreshRouting(t.Context()); err != nil {
		t.Fatal(err)
	}
	if n, err := f.repo.cfg.Journal.CountOwner(owner, writesNamespace); err != nil || n != 0 {
		t.Fatal("new public metadata stranded earlier accepted projection", n, err)
	}
}

func operationForTest(t *testing.T, id string) context.Context {
	t.Helper()
	ctx, err := WithDocumentOperation(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func TestAcceptedDocumentRecoveredRetryAfterReopenCannotRepublishButExplicitNewIntentCan(t *testing.T) {
	f := setup(t, true)
	f.open(t)
	f.crypto.loseIndex = true
	if err := f.repo.SetSignature(t.Context(), "original accepted"); err == nil {
		t.Fatal("missing lost ACK")
	}
	if err := f.repo.SetSignature(t.Context(), "newer edit"); err != nil {
		t.Fatal(err)
	}
	owner := testScope.Tenant + "\x00" + testScope.Subject + "\x00mail"
	cfg := f.repo.cfg
	f.repo.Close()
	f.crypto.binding.Target.DeviceID = "RECOVERED-B"
	reopened, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err = reopened.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	before := f.crypto.puts
	for i := 0; i < 2; i++ {
		if err = reopened.SetSignature(t.Context(), "original accepted"); err != nil {
			t.Fatal(err)
		}
	}
	if f.crypto.puts != before {
		t.Fatal("reopened retry created a new historical publication")
	}
	if got, err := reopened.Signature(t.Context()); err != nil || got != "newer edit" {
		t.Fatal("retry replaced a newer visible edit", err)
	}
	if n, err := cfg.Journal.CountOwner(owner, writesNamespace); err != nil || n != 0 {
		t.Fatal("accepted pending work stranded", n, err)
	}
	if n, err := cfg.Journal.CountOwner(owner, storage.MailWriteHistoryNamespace); err != nil || n < 1 {
		t.Fatal("accepted intent history was discarded", n, err)
	}
	explicit := operationForTest(t, "11111111-1111-4111-8111-111111111111")
	if err = reopened.SetSignature(explicit, "original accepted"); err != nil {
		t.Fatal(err)
	}
	if f.crypto.puts <= before {
		t.Fatal("deliberate new edit reused the accepted old operation")
	}
	before = f.crypto.puts
	if err = reopened.SetSignature(explicit, "original accepted"); err != nil || f.crypto.puts != before {
		t.Fatal("explicit operation retry duplicated", err)
	}
	if err = reopened.SetSignature(explicit, "different payload with same intent"); !errors.Is(err, ErrRecord) || f.crypto.puts != before {
		t.Fatal("explicit intent changed immutable payload", err)
	}
	if got, err := reopened.Signature(t.Context()); err != nil || got != "original accepted" {
		t.Fatal("explicit return to old value failed", err)
	}
}

func TestAcceptedDocumentRecoveryPreservesOriginalFilterCreationProtocol(t *testing.T) {
	f := setup(t, true)
	f.open(t)
	if err := f.repo.SetSignature(t.Context(), "existing display value"); err != nil {
		t.Fatal(err)
	}
	owner := testScope.Tenant + "\x00" + testScope.Subject + "\x00mail"
	id := "22222222-2222-4222-8222-222222222222"
	if err := f.repo.cfg.Journal.Write(owner, writesNamespace, filterCreateKey(id), map[string]string{"original": "filter-protocol"}); err != nil {
		t.Fatal(err)
	}
	before := f.crypto.puts
	if _, err := f.repo.Signature(t.Context()); err != nil || f.crypto.puts != before {
		t.Fatal("ordinary recovery interpreted filter creation DTO", err)
	}
	if count, err := f.repo.cfg.Journal.CountOwner(owner, writesNamespace); err != nil || count != 1 {
		t.Fatal("filter creation WAL disappeared", count, err)
	}
}

func TestAcceptedDocumentExplicitNoopIntentKeepsImmutablePayload(t *testing.T) {
	f := setup(t, true)
	f.open(t)
	if err := f.repo.SetSignature(t.Context(), "current value"); err != nil {
		t.Fatal(err)
	}
	explicit := operationForTest(t, "33333333-3333-4333-8333-333333333333")
	if err := f.repo.SetSignature(explicit, "current value"); err != nil {
		t.Fatal(err)
	}
	before := f.crypto.puts
	if err := f.repo.SetSignature(explicit, "different payload"); !errors.Is(err, ErrRecord) || f.crypto.puts != before {
		t.Fatal("explicit no-op forgot its immutable payload", err)
	}
	if err := f.repo.SetSignature(explicit, "current value"); err != nil || f.crypto.puts != before {
		t.Fatal("explicit same-value intent retry republished", err)
	}
}
