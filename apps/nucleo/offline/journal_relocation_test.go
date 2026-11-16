// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build !js

package offline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
)

func TestJournalRelocationResumesEveryDurableBoundaryWithoutRevivingAcknowledgedWork(t *testing.T) {
	for _, phase := range []string{"copying", "copied", "verified", "fenced"} {
		t.Run(phase, func(t *testing.T) {
			base := t.TempDir()
			sourceDir, targetDir := filepath.Join(base, "private-cache"), filepath.Join(base, "group", "mail", "offline")
			t.Cleanup(func() { _ = os.Chmod(sourceDir, 0o700) })
			keys := new(authtest.Secrets)
			source, err := OpenWithSecrets(sourceDir, keys)
			if err != nil {
				t.Fatal(err)
			}
			for _, owner := range []string{"alice", "bob"} {
				if err = source.Write(owner, "mail-private-writes-v1", "pending", owner+" 東京"); err != nil {
					t.Fatal(err)
				}
			}
			interrupted := errors.New("simulated app termination")
			stop := func(actual string) error {
				if actual == phase {
					return interrupted
				}
				return nil
			}
			if target, err := relocateJournal(t.Context(), sourceDir, targetDir, keys, stop); target != nil || !errors.Is(err, interrupted) {
				t.Fatal("cutover ignored interrupted durable phase", err)
			}
			if retired, err := JournalRelocationRetired(t.Context(), sourceDir, targetDir); retired || !errors.Is(err, ErrPendingJournal) {
				t.Fatal("unfinished copy hid retained work", retired, err)
			}
			target, err := relocateJournal(t.Context(), sourceDir, targetDir, keys, nil)
			if err != nil {
				t.Fatal("durable relocation did not resume", err)
			}
			if retired, err := JournalRelocationRetired(t.Context(), sourceDir, targetDir); !retired || err != nil {
				t.Fatal("verified retirement unavailable to closed-app metadata", retired, err)
			}
			for _, owner := range []string{"alice", "bob"} {
				var actual string
				if found, err := target.Read(owner, "mail-private-writes-v1", "pending", &actual); err != nil || !found || actual != owner+" 東京" {
					t.Fatal("resume changed another account's work", found, err)
				}
			}
			if err = target.Delete("alice", "mail-private-writes-v1", "pending"); err != nil {
				t.Fatal(err)
			}
			reopened, err := relocateJournal(t.Context(), sourceDir, targetDir, keys, nil)
			if err != nil {
				t.Fatal("acknowledgement made relocation conflict forever", err)
			}
			if count, err := reopened.CountOwner("alice", "mail-private-writes-v1"); err != nil || count != 0 {
				t.Fatal("retained source resurrected acknowledged work", count, err)
			}
			if count, err := reopened.CountOwner("bob", "mail-private-writes-v1"); err != nil || count != 1 {
				t.Fatal("acknowledgement erased another account", count, err)
			}
			if retired, err := JournalRelocationRetired(t.Context(), sourceDir, targetDir); !retired || err != nil {
				t.Fatal("retained source permanently blocked acknowledgement", retired, err)
			}
			root, err := os.OpenRoot(filepath.Dir(targetDir))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = root.Close() }()
			state, _, err := readJournalRelocation(root)
			if err != nil {
				t.Fatal(err)
			}
			archived, err := OpenWithSecrets(filepath.Join(filepath.Dir(targetDir), "retained", state.Source.CacheID), keys)
			if err != nil {
				t.Fatal(err)
			}
			var original string
			if found, err := archived.Read("alice", "mail-private-writes-v1", "pending", &original); err != nil || !found || original != "alice 東京" {
				t.Fatal("acknowledgement destroyed retained original", err)
			}
			if err = archived.Write("alice", "mail-private-writes-v1", "tampered", "unexpected work"); err != nil {
				t.Fatal(err)
			}
			if retired, err := JournalRelocationRetired(t.Context(), sourceDir, targetDir); retired || !errors.Is(err, ErrPendingJournal) {
				t.Fatal("changed archive was treated as safely retired", retired, err)
			}
		})
	}
}

func TestJournalRelocationConflictingIntentNeverRetiresOrOverwritesSource(t *testing.T) {
	base := t.TempDir()
	sourceDir, targetDir := filepath.Join(base, "private-cache"), filepath.Join(base, "group", "mail", "offline")
	keys := new(authtest.Secrets)
	source, err := OpenWithSecrets(sourceDir, keys)
	if err != nil {
		t.Fatal(err)
	}
	target, err := OpenWithSecrets(targetDir, keys)
	if err != nil {
		t.Fatal(err)
	}
	if err = source.Write("alice", "drafts", "pending", "original"); err != nil {
		t.Fatal(err)
	}
	if err = target.Write("alice", "drafts", "pending", "different destination"); err != nil {
		t.Fatal(err)
	}
	if _, err = relocateJournal(t.Context(), sourceDir, targetDir, keys, nil); !errors.Is(err, ErrJournalConflict) {
		t.Fatal("conflicting destination overwritten", err)
	}
	var actual string
	if found, err := source.Read("alice", "drafts", "pending", &actual); err != nil || !found || actual != "original" {
		t.Fatal("failed cutover fenced or changed source", err)
	}
	if found, err := target.Read("alice", "drafts", "pending", &actual); err != nil || !found || actual != "different destination" {
		t.Fatal("failed cutover changed destination", err)
	}
}

func TestJournalRelocationRequiresLeaseAndHonorsCancellation(t *testing.T) {
	base := t.TempDir()
	sourceDir, targetDir := filepath.Join(base, "private-cache"), filepath.Join(base, "group", "mail", "offline")
	if _, err := RelocateJournalContext(t.Context(), sourceDir, targetDir, nil); !errors.Is(err, ErrEncryptedStore) {
		t.Fatal("relocation allowed missing witness", err)
	}
	missingLease := func(context.Context) (func(), error) {
		return nil, nil //nolint:nilnil // Deliberately malformed acquire result must fail closed, rather than panic or migrate.
	}
	if _, err := RelocateJournalContext(t.Context(), sourceDir, targetDir, missingLease); !errors.Is(err, ErrEncryptedStore) {
		t.Fatal("relocation accepted absent acquired lease", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := relocateJournal(ctx, sourceDir, targetDir, new(authtest.Secrets), nil); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled relocation continued", err)
	}
}

func TestJournalRelocationKeepsDisjointExistingConsumerWork(t *testing.T) {
	base := t.TempDir()
	sourceDir, targetDir := filepath.Join(base, "private-cache"), filepath.Join(base, "group", "pdf", "offline")
	t.Cleanup(func() { _ = os.Chmod(sourceDir, 0o700) })
	keys := new(authtest.Secrets)
	source, err := OpenWithSecrets(sourceDir, keys)
	if err != nil {
		t.Fatal(err)
	}
	target, err := OpenWithSecrets(targetDir, keys)
	if err != nil {
		t.Fatal(err)
	}
	if err = source.Write("acme\x00alice\x00pdf", "pdf-private-v1", "original", "retained PDF source"); err != nil {
		t.Fatal(err)
	}
	if err = target.Write("acme\x00alice\x00drive", "drive-private-v1", "consumer-pending", "existing Drive consumer intent"); err != nil {
		t.Fatal(err)
	}
	migrated, err := relocateJournal(t.Context(), sourceDir, targetDir, keys, nil)
	if err != nil {
		t.Fatal("disjoint existing consumer work prevented safe relocation", err)
	}
	for _, record := range []struct{ owner, namespace, id, want string }{
		{"acme\x00alice\x00pdf", "pdf-private-v1", "original", "retained PDF source"},
		{"acme\x00alice\x00drive", "drive-private-v1", "consumer-pending", "existing Drive consumer intent"},
	} {
		var actual string
		if found, err := migrated.Read(record.owner, record.namespace, record.id, &actual); err != nil || !found || actual != record.want {
			t.Fatal("relocation overwrote original or disjoint consumer work", record.namespace, err)
		}
	}
	if retired, err := JournalRelocationRetired(t.Context(), sourceDir, targetDir); err != nil || !retired {
		t.Fatal("disjoint merge did not retire exact source archive", retired, err)
	}
}

func TestJournalRelocationHoldsExistingHTTPCacheLeaseUntilRetirement(t *testing.T) {
	base := t.TempDir()
	sourceDir, targetDir := filepath.Join(base, "private-cache"), filepath.Join(base, "group", "mail", "offline")
	t.Cleanup(func() { _ = os.Chmod(sourceDir, 0o700) })
	keys := new(authtest.Secrets)
	source, err := OpenWithSecrets(sourceDir, keys)
	if err != nil {
		t.Fatal(err)
	}
	if err = source.Write("alice", "mail-private-v1", "pending", "original"); err != nil {
		t.Fatal(err)
	}
	checked := 0
	checkpoint := func(phase string) error {
		if phase != "copying" && phase != "copied" && phase != "verified" {
			return nil
		}
		checked++
		bounded, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		defer cancel()
		release, err := source.b.(ficheros).lockHTTP(bounded)
		if err == nil {
			release()
			t.Fatal("relocation allowed a pre-existing HTTP cache writer during " + phase)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("unexpected cache lease failure", err)
		}
		return nil
	}
	if _, err = relocateJournal(t.Context(), sourceDir, targetDir, keys, checkpoint); err != nil {
		t.Fatal(err)
	}
	if checked != 3 {
		t.Fatal("cache-copy windows were not all guarded", checked)
	}
}
