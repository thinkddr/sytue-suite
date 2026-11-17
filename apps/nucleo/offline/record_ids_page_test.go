// SPDX-License-Identifier: Apache-2.0
//go:build !js

package offline

import (
	"context"
	"crypto/cipher"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
)

type pageDecryptCounter struct {
	cipher.AEAD
	opens int
}

func (p *pageDecryptCounter) Open(dst, nonce, ciphertext, additional []byte) ([]byte, error) {
	p.opens++
	return p.AEAD.Open(dst, nonce, ciphertext, additional)
}

func TestRecordIDsPageDecryptsOnlyItsOwnFilesAndAdvancesAfterDeletion(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenWithSecrets(dir, &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	const owner, namespace = "owner", "ingest"
	names := make([]string, 0, 211)
	for i := range 211 {
		id := fmt.Sprintf("owned_%d", i)
		if err := store.Write(owner, namespace, id, map[string]int{"value": i}); err != nil {
			t.Fatal(err)
		}
		names = append(names, clave(id))
	}
	sort.Strings(names)
	files := store.b.(ficheros)
	decrypts := &pageDecryptCounter{AEAD: files.sealed}
	files.sealed = decrypts
	store.b = files
	// The second physical record is corrupt. The first page must neither read
	// nor decrypt it; the next page must fail, never masquerade as completion.
	corrupt := filepath.Join(dir, clave(owner, namespace), names[1]+".json")
	sealed, err := os.ReadFile(corrupt)
	if err != nil {
		t.Fatal(err)
	}
	broken := append([]byte(nil), sealed...)
	broken[len(broken)-1] ^= 1
	if err := os.WriteFile(corrupt, broken, 0o600); err != nil {
		t.Fatal(err)
	}
	ids, next, err := store.RecordIDsPage(t.Context(), owner, namespace, "", 1)
	if err != nil || len(ids) != 1 || next != names[0] || decrypts.opens != 1 {
		t.Fatal("page decrypted a later record", len(ids), decrypts.opens, err)
	}
	if _, _, err := store.RecordIDsPage(t.Context(), owner, namespace, next, 1); !errors.Is(err, ErrCacheCiphertext) {
		t.Fatal("corrupt next page was silently skipped", err)
	}
	if err := os.WriteFile(corrupt, sealed, 0o600); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{ids[0]: true}
	if err := store.Delete(owner, namespace, ids[0]); err != nil {
		t.Fatal(err)
	}
	for next != "" {
		before := decrypts.opens
		page, cursor, err := store.RecordIDsPage(t.Context(), owner, namespace, next, 7)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) > 7 || cursor == next || decrypts.opens-before > 7 {
			t.Fatal("page did not respect decryption bound or advance", decrypts.opens-before)
		}
		for _, id := range page {
			if seen[id] {
				t.Fatal("record repeated after preceding file was deleted")
			}
			seen[id] = true
		}
		next = cursor
	}
	if len(seen) != 211 {
		t.Fatal("pagination imposed a global work limit", len(seen))
	}
	if ids, next, err := store.RecordIDsPage(t.Context(), "other", namespace, "", 7); err != nil || len(ids) != 0 || next != "" {
		t.Fatal("page crossed owner", err)
	}
	if _, _, err := store.RecordIDsPage(t.Context(), owner, namespace, "../", 7); !errors.Is(err, ErrPendingJournal) {
		t.Fatal("unsafe cursor accepted", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := store.RecordIDsPage(ctx, owner, namespace, "", 7); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled page opened a file", err)
	}
}
