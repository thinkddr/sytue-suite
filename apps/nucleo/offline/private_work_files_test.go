// SPDX-License-Identifier: Apache-2.0
//go:build !js

package offline

import (
	"os"
	"path/filepath"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
)

func TestOwnedPendingCountSeparatesAccountsAndWorksWithoutKeys(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenWithSecrets(dir, new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{"acme\x00alice\x00notes", "acme\x00bob\x00notes", "other\x00alice\x00notes"} {
		if err := store.Write(owner, "notes-private-v1", "snapshot", map[string]string{"private": "pending"}); err != nil {
			t.Fatal(err)
		}
	}
	owner := "acme\x00alice\x00notes"
	if count, err := store.CountOwner(owner, "notes-private-v1"); err != nil || count != 1 {
		t.Fatal("owned count included another scope", count, err)
	}
	if count, err := CountDirectoryOwner(dir, owner, "notes-private-v1"); err != nil || count != 1 {
		t.Fatal("keyless owned count changed", count, err)
	}
	// Metadata cannot claim a clean shutdown when a journal path is unreadable.
	bad := filepath.Join(dir, clave(owner, "broken"))
	if err := os.WriteFile(bad, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CountDirectoryOwner(dir, owner, "broken"); err == nil {
		t.Fatal("failed pending read became zero")
	}
}

func TestPendingRecordIDsStreamExactOwnerAndRejectUnknownTemporaryWork(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenWithSecrets(dir, new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	owner := "acme\x00alice\x00notes"
	for _, id := range []string{"snapshot", "import_accepted"} {
		if err := store.Write(owner, "notes-private-v1", id, map[string]string{"secret": "body"}); err != nil {
			t.Fatal(err)
		}
	}
	ids, err := store.RecordIDs(owner, "notes-private-v1")
	if err != nil || len(ids) != 2 {
		t.Fatal("exact pending IDs unavailable", ids, err)
	}
	prefix := filepath.Join(dir, clave(owner, "notes-private-v1"))
	if err := os.WriteFile(filepath.Join(prefix, "unknown.json.tmp-interrupted"), []byte("private unconfirmed bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordIDs(owner, "notes-private-v1"); err == nil {
		t.Fatal("ignored unconfirmed atomic write")
	}
	if _, err := CountDirectoryOwner(dir, owner, "notes-private-v1"); err == nil {
		t.Fatal("keyless pending count ignored unknown atomic write")
	}
}

func TestSessionClearRetainsOtherAccountJournalsAndDiscardIsScoped(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenWithSecrets(dir, new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{"alice", "bob"} {
		for _, namespace := range []string{"notes-private-v1", "mail-private-source-stage-v1", "http:notes"} {
			if err := store.Write(owner, namespace, "retained", owner); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := ClearDirectoryKeeping(dir, "notes-private-v1", "mail-private-source-stage-v1"); err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{"alice", "bob"} {
		if count, err := store.CountOwner(owner, "notes-private-v1", "mail-private-source-stage-v1"); err != nil || count != 2 {
			t.Fatal("session clear lost durable source", owner, count, err)
		}
		if count, err := store.CountOwner(owner, "http:notes"); err != nil || count != 0 {
			t.Fatal("ephemeral response survived session clear", count, err)
		}
	}
	if err := DiscardDirectoryOwner(dir, "alice", "notes-private-v1", "mail-private-source-stage-v1"); err != nil {
		t.Fatal(err)
	}
	if count, err := store.CountOwner("alice", "notes-private-v1"); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if count, err := store.CountOwner("bob", "notes-private-v1", "mail-private-source-stage-v1"); err != nil || count != 2 {
		t.Fatal("explicit discard crossed account boundary", count, err)
	}
}
