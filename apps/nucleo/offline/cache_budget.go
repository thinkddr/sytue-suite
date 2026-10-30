// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build !js

package offline

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

// HTTP caches share these native-suite budgets, across owners and products.
// Drafts and mutation queues use different namespaces and are never evicted here.
const (
	MaxHTTPCacheBytes   = int64(256 << 20)
	MaxHTTPCacheEntries = 2000
)

type httpBudgetPlanner interface {
	pruneHTTP(namespaces []string, keep string, bytes int64) error
	lockHTTP(context.Context) (func(), error)
}

func (s *Store) writeHTTP(ctx context.Context, owner, product, id string, products []string, value respuestaGuardada, valid func() bool) error {
	if !valid() {
		return context.Canceled
	}
	encoded, err := encodeRecord(id, value)
	if err != nil {
		return err
	}
	planner, ok := s.b.(httpBudgetPlanner)
	if !ok {
		return errors.New("offline: HTTP cache backend has no budget planner")
	}
	// Reuse the suite's OS mechanism on a separate cache file, never the OIDC lock.
	unlock, err := planner.lockHTTP(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !valid() {
		return context.Canceled
	}
	namespaces := make([]string, 0, len(products)+1)
	for _, p := range products {
		namespaces = append(namespaces, espacioHTTP(p))
	}
	namespaces = append(namespaces, espacioHTTP(product))
	key := clave(owner, espacioHTTP(product), id)
	size := int64(len(encoded))
	if sealed, ok := s.b.(interface{ storedSize(int64) int64 }); ok {
		size = sealed.storedSize(size)
	}
	if err := planner.pruneHTTP(namespaces, key+".json", size); err != nil {
		return err
	}
	if !valid() {
		return context.Canceled
	}
	return s.b.put(key, encoded)
}

func (f ficheros) lockHTTP(ctx context.Context) (func(), error) {
	file, err := f.raiz.OpenFile(".http-cache-lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	return ajustes.CerrojoFichero(ctx, file)
}

func (f ficheros) lockClear() (func(), error) {
	return f.lockHTTP(context.Background())
}

type cacheFile struct {
	path     string
	size     int64
	modified time.Time
}

func (f ficheros) pruneHTTP(namespaces []string, keep string, incoming int64) error {
	if incoming > MaxHTTPCacheBytes {
		return errors.New("offline: HTTP cache entry exceeds total budget")
	}
	entries, total, err := f.httpFiles(namespaces, keep)
	if err != nil {
		return err
	}
	total += incoming
	slices.SortFunc(entries, func(a, b cacheFile) int {
		if c := a.modified.Compare(b.modified); c != 0 {
			return c
		}
		return strings.Compare(a.path, b.path)
	})
	count := len(entries) + 1
	for _, entry := range entries {
		if total <= MaxHTTPCacheBytes && count <= MaxHTTPCacheEntries {
			break
		}
		if err := f.raiz.Remove(entry.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		total -= entry.size
		count--
	}
	return nil
}

func (f ficheros) httpFiles(namespaces []string, keep string) ([]cacheFile, int64, error) {
	allowed := make(map[string]bool, len(namespaces))
	for _, namespace := range namespaces {
		allowed[clave(namespace)] = true
	}
	owners, err := fs.ReadDir(f.raiz.FS(), ".")
	if err != nil {
		return nil, 0, err
	}
	var entries []cacheFile
	var total int64
	for _, owner := range owners {
		if !owner.IsDir() {
			continue
		}
		for namespace := range allowed {
			files, size, err := f.httpFilesIn(path.Join(owner.Name(), namespace), keep)
			if err != nil {
				return nil, 0, err
			}
			entries = append(entries, files...)
			total += size
		}
	}
	return entries, total, nil
}

func (f ficheros) httpFilesIn(prefix, keep string) ([]cacheFile, int64, error) {
	files, err := fs.ReadDir(f.raiz.FS(), prefix)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var entries []cacheFile
	var total int64
	for _, file := range files {
		name := path.Join(prefix, file.Name())
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") || name == keep {
			continue
		}
		info, err := file.Info()
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, 0, err
		}
		total += info.Size()
		entries = append(entries, cacheFile{name, info.Size(), info.ModTime()})
	}
	return entries, total, nil
}
