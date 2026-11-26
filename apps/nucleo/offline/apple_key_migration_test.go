// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build !js && !edicionlocal

package offline

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
)

func TestLegacyAppleKeyMigrationAuthenticatesOwnedCacheBeforeClaim(t *testing.T) {
	dir := t.TempDir()
	keys := new(authtest.Secrets)
	store, err := OpenWithSecrets(dir, keys)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Write("alice", "notes-drafts", "one", "retained original"); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	cacheID, _, err := cacheIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := keys.Read("offline-cache-" + cacheID)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(candidate)
	bound := boundCacheSecrets{root: root, cacheID: cacheID, ctx: t.Context()}
	if err = bound.validate(candidate); err != nil {
		t.Fatal("authentic owned records rejected", err)
	}
	wrong, err := decodeCacheKey(candidate)
	if err != nil {
		t.Fatal(err)
	}
	wrong.Key = bytes.Repeat([]byte{0x55}, 32)
	invalid, err := json.Marshal(wrong)
	if err != nil {
		t.Fatal(err)
	}
	if err = bound.validate(invalid); err == nil {
		t.Fatal("foreign key claimed encrypted records")
	}
	file := filepath.Join(dir, clave("alice", "notes-drafts", "one")+".json")
	original, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(file, []byte("unsealed replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = bound.validate(candidate); err == nil {
		t.Fatal("completed cache accepted plaintext downgrade")
	}
	after, err := os.ReadFile(file)
	if err != nil || string(after) != "unsealed replacement" {
		t.Fatal("rejected migration modified source", err)
	}
	if err = os.WriteFile(file, original, 0o600); err != nil {
		t.Fatal(err)
	}
	again, err := OpenWithSecrets(dir, keys)
	if err != nil {
		t.Fatal(err)
	}
	var restored string
	if ok, err := again.Read("alice", "notes-drafts", "one", &restored); err != nil || !ok || restored != "retained original" {
		t.Fatal("migration validation damaged source", err)
	}
}
