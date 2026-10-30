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
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
)

func TestEncryptedCacheSurvivesAppRestartAndRejectsCopyTamperingAndDowngrade(t *testing.T) {
	dir := t.TempDir()
	keys := new(authtest.Secrets)
	store, err := OpenWithSecrets(dir, keys)
	if err != nil {
		t.Fatal(err)
	}
	const secret = "private title and message never stored in plaintext"
	if err := store.Write("alice", "mail-drafts", "message", secret); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, clave("alice", "mail-drafts", "message")+".json")
	first, err := os.ReadFile(file)
	if err != nil || bytes.Contains(first, []byte(secret)) || bytes.Contains(first, []byte("message")) {
		t.Fatal("plaintext cache", err)
	}
	// Encrypt exactly the same record, including its timestamp, a second time.
	// Changing UpdatedAt would hide nonce reuse by also changing the plaintext.
	key := clave("alice", "mail-drafts", "message")
	exact, err := store.b.get(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.b.put(key, exact); err != nil {
		t.Fatal(err)
	}
	clear(exact)
	second, _ := os.ReadFile(file)
	if bytes.Equal(first, second) {
		t.Fatal("nonce reused")
	}
	again, err := OpenWithSecrets(dir, keys)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	if ok, err := again.Read("alice", "mail-drafts", "message", &got); err != nil || !ok || got != secret {
		t.Fatal("restart lost cache", ok, err)
	}
	if err := again.Write("bob", "mail-drafts", "message", "bob"); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, clave("bob", "mail-drafts", "message")+".json")
	if err := os.WriteFile(other, second, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := again.Read("bob", "mail-drafts", "message", &got); !errors.Is(err, ErrCacheCiphertext) {
		t.Fatal("record copied across owner", err)
	}
	for _, bad := range [][]byte{append(bytes.Clone(second[:len(second)-1]), second[len(second)-1]^1), second[:len(second)-1], []byte(`{"id":"message","value":"injected"}`)} {
		if err := os.WriteFile(file, bad, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := again.Read("alice", "mail-drafts", "message", &got); !errors.Is(err, ErrCacheCiphertext) {
			t.Fatal("unauthenticated record accepted", err)
		}
	}
	if err := os.WriteFile(file, second, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenWithSecrets(dir, new(authtest.Secrets)); !errors.Is(err, ErrCacheKey) {
		t.Fatal("lost key was silently replaced", err)
	}
	if err := again.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenWithSecrets(dir, keys); err != nil {
		t.Fatal("clear removed key marker", err)
	}
}

func TestCacheMigratesLegacyRecordBeforeAnyAppCanUseIt(t *testing.T) {
	dir := t.TempDir()
	key := clave("alice", "notes-drafts", "n1")
	file := filepath.Join(dir, key+".json")
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	raw, err := encodeRecord("n1", map[string]string{"title": "legacy-private-title"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	temp := filepath.Join(dir, key+".tmp-interrupted")
	if err := os.WriteFile(temp, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	keys := new(authtest.Secrets)
	store, err := OpenWithSecrets(dir, keys)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := store.b.get(key)
	if err != nil || !bytes.Equal(restored, raw) {
		t.Fatal("migration changed record/timestamp", err)
	}
	sealed, _ := os.ReadFile(file)
	if bytes.Contains(sealed, []byte("legacy-private-title")) {
		t.Fatal("legacy plaintext retained")
	}
	orphan, err := os.ReadFile(temp)
	if err != nil || bytes.Contains(orphan, []byte("legacy-private-title")) {
		t.Fatal("orphan was lost or remained plaintext", err)
	}
	recovered, err := store.b.(ficheros).openRecord(key, orphan)
	if err != nil || !bytes.Equal(recovered, raw) {
		t.Fatal("only copy of an interrupted draft was discarded", err)
	}
	if n, err := CountDirectory(dir, "notes-drafts"); err != nil || n != 2 {
		t.Fatal("orphan not counted before logout", n, err)
	}
	if err := os.WriteFile(file, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err = OpenWithSecrets(dir, keys)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.b.get(key); !errors.Is(err, ErrCacheCiphertext) {
		t.Fatal("sealed cache reaccepted plaintext", err)
	}
}

type failingCacheSecrets struct {
	keys   *authtest.Secrets
	writes int
	failAt int
}

func (s *failingCacheSecrets) Read(k string) ([]byte, error) { return s.keys.Read(k) }
func (s *failingCacheSecrets) Write(k string, v []byte) error {
	s.writes++
	if s.writes == s.failAt {
		return errors.New("locked")
	}
	return s.keys.Write(k, v)
}

func TestCacheMigrationResumesAfterKeychainRelocks(t *testing.T) {
	dir := t.TempDir()
	key := clave("alice", "notes", "one")
	file := filepath.Join(dir, key+".json")
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	raw, _ := encodeRecord("one", "preserve me")
	if err := os.WriteFile(file, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	keys := &failingCacheSecrets{keys: new(authtest.Secrets), failAt: 2}
	if _, err := OpenWithSecrets(dir, keys); !errors.Is(err, ErrCacheKey) {
		t.Fatal("failed checkpoint accepted", err)
	}
	store, err := OpenWithSecrets(dir, keys)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.b.get(key)
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatal("resume lost data", err)
	}
}

func TestConcurrentAppOpenKeepsOneCacheKey(t *testing.T) {
	dir := t.TempDir()
	keys := new(authtest.Secrets)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			store, err := OpenWithSecrets(dir, keys)
			if err != nil {
				t.Error(err)
				return
			}
			if err := store.Write("alice", "drafts", "one", "private"); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	store, err := OpenWithSecrets(dir, keys)
	if err != nil {
		t.Fatal(err)
	}
	var value string
	if ok, err := store.Read("alice", "drafts", "one", &value); err != nil || !ok || value != "private" {
		t.Fatal("concurrent key replacement", ok, err)
	}
}

func TestCacheKeySurvivesOSMovingTheAppContainerDuringUpdate(t *testing.T) {
	parent := t.TempDir()
	before, after := filepath.Join(parent, "old-container"), filepath.Join(parent, "new-container")
	keys := new(authtest.Secrets)
	store, err := OpenWithSecrets(before, keys)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Write("alice", "mail-drafts", "one", "draft survives update"); err != nil {
		t.Fatal(err)
	}
	if err := store.b.(ficheros).raiz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(before, after); err != nil {
		t.Fatal(err)
	}
	store, err = OpenWithSecrets(after, keys)
	if err != nil {
		t.Fatal("update changed cache-key identity", err)
	}
	var value string
	if ok, err := store.Read("alice", "mail-drafts", "one", &value); err != nil || !ok || value != "draft survives update" {
		t.Fatal("cache lost after app update", ok, err)
	}
}

func TestLogoutErasesCacheEvenWhenItsKeyIsUnavailable(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenWithSecrets(dir, new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Write("alice", "notes-drafts", "one", "private"); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenWithSecrets(dir, new(authtest.Secrets)); !errors.Is(err, ErrCacheKey) {
		t.Fatal("missing key accepted", err)
	}
	if n, err := CountDirectory(dir, "notes-drafts"); err != nil || n != 1 {
		t.Fatal("lost key hid unsent drafts", n, err)
	}
	if err := ClearDirectory(dir); err != nil {
		t.Fatal("logout needed private key", err)
	}
	n, err := store.Count("notes-drafts")
	if err != nil || n != 0 {
		t.Fatal("logout retained private data", n, err)
	}
}

func TestCancelledOpenNeverMigratesOrReplacesData(t *testing.T) {
	dir := t.TempDir()
	key := clave("alice", "notes", "one")
	file := filepath.Join(dir, key+".json")
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	raw, _ := encodeRecord("one", "preserve cancelled draft")
	if err := os.WriteFile(file, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := OpenWithSecretsContext(ctx, dir, new(authtest.Secrets)); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled cache open proceeded", err)
	}
	got, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatal("cancelled operation changed data", err)
	}
}

func TestDamagedLegacyRecordIsPreservedEncryptedInsteadOfBlockingMigration(t *testing.T) {
	dir := t.TempDir()
	key := clave("alice", "notes", "one")
	file := filepath.Join(dir, key+".json")
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"id":"one","value":"private interrupted record`)
	if err := os.WriteFile(file, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenWithSecrets(dir, new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.b.get(key)
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatal("migration discarded damaged data", err)
	}
	sealed, err := os.ReadFile(file)
	if err != nil || bytes.Contains(sealed, []byte("private interrupted")) {
		t.Fatal("damaged record remained plaintext", err)
	}
	var value any
	if _, err := store.Read("alice", "notes", "one", &value); err == nil {
		t.Fatal("damaged record silently accepted as valid")
	}
}
