// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build !js

package offline

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
)

func TestJournalCopyUsesIndependentKeysAndPreservesExactRecordsAcrossRetry(t *testing.T) {
	sourceDir, targetDir := t.TempDir(), t.TempDir()
	sourceKeys, targetKeys := new(authtest.Secrets), new(authtest.Secrets)
	source, err := OpenWithSecrets(sourceDir, sourceKeys)
	if err != nil {
		t.Fatal(err)
	}
	target, err := OpenWithSecrets(targetDir, targetKeys)
	if err != nil {
		t.Fatal(err)
	}
	owner := "acme\x00alice\x00mail"
	for _, actor := range []string{owner, "acme\x00bob\x00mail"} {
		for _, namespace := range []string{"mail-private-writes-v1", "mail-private-source-stage-v1", "http:mail"} {
			if err = source.Write(actor, namespace, "same-id", map[string]string{"original": "private unchanged source & 東京"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	namespaces := []string{"mail-private-writes-v1", "mail-private-source-stage-v1"}
	if count, err := source.CopyOwnerTo(t.Context(), target, owner, namespaces...); err != nil || count != 2 {
		t.Fatal("journal copy did not retain selected work", count, err)
	}
	if count, err := source.CopyOwnerTo(t.Context(), target, owner, namespaces...); err != nil || count != 2 {
		t.Fatal("journal copy could not resume exact intents", count, err)
	}
	for _, namespace := range namespaces {
		key := clave(owner, namespace, "same-id")
		original, err := source.b.get(key)
		if err != nil {
			t.Fatal(err)
		}
		copied, err := target.b.get(key)
		if err != nil || !bytes.Equal(original, copied) {
			t.Fatal("journal copy changed value or timestamp", err)
		}
		clear(copied)
		clear(original)
		ciphertext, err := os.ReadFile(filepath.Join(targetDir, key+".json"))
		if err != nil || bytes.Contains(ciphertext, []byte("private unchanged source")) {
			t.Fatal("copied journal reached disk as plaintext", err)
		}
		sourceCipher, err := os.ReadFile(filepath.Join(sourceDir, key+".json"))
		if err != nil || bytes.Equal(sourceCipher, ciphertext) {
			t.Fatal("journal copy reused source ciphertext/key", err)
		}
	}
	if count, err := target.CountOwner("acme\x00bob\x00mail", namespaces...); err != nil || count != 0 {
		t.Fatal("journal copy crossed owner", count, err)
	}
	if count, err := target.CountOwner(owner, "http:mail"); err != nil || count != 0 {
		t.Fatal("journal copy included unselected disposable cache", count, err)
	}
	if _, err = OpenWithSecrets(targetDir, sourceKeys); !errors.Is(err, ErrCacheKey) {
		t.Fatal("destination borrowed source key", err)
	}
	if err = target.Write(owner, namespaces[0], "same-id", "newer target intent"); err != nil {
		t.Fatal(err)
	}
	if _, err = source.CopyOwnerTo(t.Context(), target, owner, namespaces...); !errors.Is(err, ErrJournalConflict) {
		t.Fatal("replay overwrote a different destination intent", err)
	}
	var kept string
	if found, err := target.Read(owner, namespaces[0], "same-id", &kept); err != nil || !found || kept != "newer target intent" {
		t.Fatal("conflict destroyed destination work", err)
	}
	if count, err := source.CountOwner(owner, namespaces...); err != nil || count != 2 {
		t.Fatal("copy or conflict erased legacy source", count, err)
	}
}

type cancellingJournalBackend struct {
	backend
	cancel context.CancelFunc
}

func (b cancellingJournalBackend) encrypted() bool { return true }
func (b cancellingJournalBackend) lockRecord(ctx context.Context, key string) (func(), error) {
	return b.backend.(interface {
		lockRecord(context.Context, string) (func(), error)
	}).lockRecord(ctx, key)
}

func (b cancellingJournalBackend) put(key string, raw []byte) error {
	if err := b.backend.put(key, raw); err != nil {
		return err
	}
	b.cancel()
	return nil
}

func TestJournalCopyCancellationRetainsSourceAndResumesVerifiedRecord(t *testing.T) {
	source, err := OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	target, err := OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	owner, namespace := "acme\x00alice\x00drive", "drive-private-migration-v1"
	if err = source.Write(owner, namespace, "retained", "pending"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	originalBackend := target.b
	target.b = cancellingJournalBackend{backend: target.b, cancel: cancel}
	if _, err = source.CopyOwnerTo(ctx, target, owner, namespace); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled copy was acknowledged", err)
	}
	target.b = originalBackend
	if count, err := source.CopyOwnerTo(t.Context(), target, owner, namespace); err != nil || count != 1 {
		t.Fatal("verified copy did not resume after cancellation", count, err)
	}
	if count, err := source.CountOwner(owner, namespace); err != nil || count != 1 {
		t.Fatal("cancelled copy discarded source", count, err)
	}
}

func TestJournalSnapshotCopiesAllOwnedAppAccountsAndDetectsUnknownWork(t *testing.T) {
	sourceDir := t.TempDir()
	source, err := OpenWithSecrets(sourceDir, new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	target, err := OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{"account-a", "account-b"} {
		for _, namespace := range []string{"drafts", "uploads"} {
			if err = source.Write(owner, namespace, "same-id", owner+namespace); err != nil {
				t.Fatal(err)
			}
		}
	}
	before, err := source.Snapshot(t.Context())
	if err != nil || before.Records != 4 {
		t.Fatal("owned app snapshot omitted data", err)
	}
	copied, err := source.CopySnapshotTo(t.Context(), target)
	if err != nil || copied != before {
		t.Fatal("owned app snapshot changed during copy", err)
	}
	for _, owner := range []string{"account-a", "account-b"} {
		for _, namespace := range []string{"drafts", "uploads"} {
			original, err := source.b.get(clave(owner, namespace, "same-id"))
			if err != nil {
				t.Fatal(err)
			}
			actual, err := target.b.get(clave(owner, namespace, "same-id"))
			if err != nil || !bytes.Equal(original, actual) {
				t.Fatal("snapshot lost other account or timestamp", err)
			}
			clear(original)
			clear(actual)
		}
	}
	orphan := filepath.Join(sourceDir, clave("account-a", "drafts"), clave("orphan")+".json.tmp-interrupted")
	if err = os.WriteFile(orphan, []byte("unknown retained interrupted record"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = source.Snapshot(t.Context()); !errors.Is(err, ErrPendingJournal) {
		t.Fatal("unknown work became a clean snapshot", err)
	}
}
