// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build !js

package offline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
)

// JournalSnapshot identifies opaque sealed records, without exposing their
// owners or contents. The digest covers stable paths and ciphertext, not keys.
type JournalSnapshot struct {
	CacheID string `json:"cache_id"`
	Records int    `json:"records"`
	Digest  string `json:"digest"`
}

// Snapshot requires the caller's exclusive source work lease. Unknown files,
// symlinks and interrupted atomic writes remain errors, never an empty journal.
func (s *Store) Snapshot(ctx context.Context) (JournalSnapshot, error) {
	if !s.Encrypted() {
		return JournalSnapshot{}, ErrEncryptedStore
	}
	files, ok := s.b.(ficheros)
	if !ok {
		return JournalSnapshot{}, ErrEncryptedStore
	}
	return snapshotJournal(ctx, files)
}

func snapshotJournal(ctx context.Context, files ficheros) (JournalSnapshot, error) {
	result := JournalSnapshot{CacheID: files.cacheID}
	digest := sha256.New()
	err := files.walkRecords(ctx, func(key string) error {
		file, err := files.raiz.Open(key + ".json")
		if err != nil {
			return err
		}
		sum := sha256.New()
		_, readErr := io.Copy(sum, file)
		closeErr := file.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		_, _ = digest.Write([]byte(key))
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write(sum.Sum(nil))
		result.Records++
		return nil
	})
	if err != nil {
		return JournalSnapshot{}, err
	}
	result.Digest = hex.EncodeToString(digest.Sum(nil))
	return result, ctx.Err()
}

// CopySnapshotTo preserves every account and namespace of this app's store.
// Callers hold the source EX lease across copy, archive verification and cutover.
// The source must stay identical; destination conflicts cannot be overwritten.
func (s *Store) CopySnapshotTo(ctx context.Context, target *Store) (JournalSnapshot, error) {
	if s == nil || target == nil || s == target || !s.Encrypted() || !target.Encrypted() {
		return JournalSnapshot{}, ErrEncryptedStore
	}
	before, err := s.Snapshot(ctx)
	if err != nil {
		return JournalSnapshot{}, err
	}
	files, ok := s.b.(ficheros)
	if !ok {
		return JournalSnapshot{}, ErrEncryptedStore
	}
	if err := files.walkRecords(ctx, func(key string) error { return s.copyRecordKey(ctx, target, key) }); err != nil {
		return JournalSnapshot{}, err
	}
	after, err := s.Snapshot(ctx)
	if err != nil {
		return JournalSnapshot{}, err
	}
	if before != after {
		return JournalSnapshot{}, ErrJournalConflict
	}
	return before, nil
}

func canonicalCachePart(part string) bool {
	if len(part) != 32 {
		return false
	}
	value, err := hex.DecodeString(part)
	return err == nil && hex.EncodeToString(value) == part
}

func (f ficheros) walkRecords(ctx context.Context, visit func(string) error) error {
	return fs.WalkDir(f.raiz.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		key, record, err := journalRecordPath(name, entry)
		if err != nil {
			return err
		}
		if !record {
			return nil
		}
		return visit(key)
	})
}

// fenceRetired is called only after the controller has durably verified both
// destination and retained archive. A permission fence reaches existing Root
// descriptors too; a cache-open marker alone cannot stop an older Store.
func (s *Store) fenceRetired(ctx context.Context, expected JournalSnapshot) error {
	current, err := s.Snapshot(ctx)
	if err != nil {
		return err
	}
	if current != expected {
		return ErrJournalConflict
	}
	files, ok := s.b.(ficheros)
	if !ok {
		return ErrEncryptedStore
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	directory, err := files.raiz.Open(".")
	if err != nil {
		return err
	}
	defer func() { _ = directory.Close() }()
	if err := files.raiz.Chmod(".", 0); err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		return err
	}
	info, err := os.Stat(files.raiz.Name())
	if err != nil {
		return err
	}
	if info.Mode().Perm() != 0 {
		return ErrPendingJournal
	}
	return ctx.Err()
}

func journalRecordPath(name string, entry fs.DirEntry) (string, bool, error) {
	if name == "." {
		return "", false, nil
	}
	if entry.Type()&fs.ModeSymlink != 0 {
		return "", false, ErrPendingJournal
	}
	if !strings.Contains(name, "/") && !entry.IsDir() && (name == cacheMarker || name == cacheKeyLock || name == ".http-cache-lock" || strings.HasPrefix(name, recordLockPrefix)) {
		return "", false, nil
	}
	parts := strings.Split(name, "/")
	if entry.IsDir() {
		if len(parts) > 2 {
			return "", false, ErrPendingJournal
		}
		for _, part := range parts {
			if !canonicalCachePart(part) {
				return "", false, ErrPendingJournal
			}
		}
		return "", false, nil
	}
	if len(parts) != 3 || path.Ext(name) != ".json" {
		return "", false, ErrPendingJournal
	}
	for index, part := range parts {
		if index == 2 {
			part = strings.TrimSuffix(part, ".json")
		}
		if !canonicalCachePart(part) {
			return "", false, ErrPendingJournal
		}
	}
	return strings.TrimSuffix(name, ".json"), true, nil
}
