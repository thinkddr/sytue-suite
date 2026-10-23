// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build !js

package offline

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

func sparseCache(t *testing.T, f ficheros, key string, size int64) {
	t.Helper()
	if err := f.raiz.MkdirAll(path.Dir(key), 0o700); err != nil {
		t.Fatal(err)
	}
	file, err := f.raiz.OpenFile(key, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	err = file.Truncate(size)
	closed := file.Close()
	if err != nil || closed != nil {
		t.Fatal(err, closed)
	}
}

func TestHTTPBudgetIsSharedAcrossProductsAndOwnersNeverDrafts(t *testing.T) {
	store, _ := abrir(t)
	files := store.b.(ficheros)
	for i := range 3 {
		sparseCache(t, files, clave(fmt.Sprint(i), espacioHTTP([]string{"mail", "drive", "mail"}[i]), "old")+".json", 100<<20)
	}
	draft := clave("0", "drafts", "never-evict") + ".json"
	sparseCache(t, files, draft, 1<<30)
	if err := store.writeHTTP(context.Background(), "new", "drive", "current", []string{"drive", "mail"}, respuestaGuardada{Status: 200, Header: make(http.Header), Body: []byte("artificial")}, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	entries, size, err := files.httpFiles([]string{espacioHTTP("mail"), espacioHTTP("drive")}, "")
	if err != nil || size > MaxHTTPCacheBytes || len(entries) != 3 {
		t.Fatal(len(entries), size, err)
	}
	if _, err := files.raiz.Stat(draft); err != nil {
		t.Fatal("evicted draft", err)
	}
}

func TestHTTPBudgetBoundsEntryCountAndReplacement(t *testing.T) {
	store, _ := abrir(t)
	files := store.b.(ficheros)
	for i := range MaxHTTPCacheEntries {
		if err := files.put(clave("owner", espacioHTTP("mail"), fmt.Sprint(i)), []byte("{}")); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := store.writeHTTP(context.Background(), "owner", "mail", "new", nil, respuestaGuardada{Status: 200, Header: make(http.Header), Body: []byte("artificial")}, func() bool { return true }); err != nil {
			t.Fatal(err)
		}
		entries, _, err := files.httpFiles([]string{espacioHTTP("mail")}, "")
		if err != nil || len(entries) != MaxHTTPCacheEntries {
			t.Fatal(len(entries), err)
		}
	}
}

func TestCacheLockIsSeparateFromSessionAndCancellationReleasesFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	releaseSession, err := ajustes.Cerrojo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSession()
	store, _ := abrir(t)
	files := store.b.(ficheros)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	release, err := files.lockHTTP(ctx)
	if err != nil {
		t.Fatal("cache coupled to session lock", err)
	}
	wait, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	if unlock, err := files.lockHTTP(wait); !errors.Is(err, context.DeadlineExceeded) {
		if unlock != nil {
			unlock()
		}
		t.Fatal("cache lock did not exclude writer", err)
	}
	release()
	unlock, err := files.lockHTTP(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	if err := files.raiz.Remove(".http-cache-lock"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestClearKeepsTheSharedCacheLockInode(t *testing.T) {
	store, dir := abrir(t)
	file := filepath.Join(dir, ".http-cache-lock")
	planner := store.b.(httpBudgetPlanner)
	unlock, err := planner.lockHTTP(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- store.Clear() }()
	select {
	case err := <-finished:
		t.Fatal("Clear escaped the cache writer lock", err)
	case <-time.After(20 * time.Millisecond):
	}
	unlock()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(file)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("Clear replaced/removed the shared lock inode", err)
	}
}
