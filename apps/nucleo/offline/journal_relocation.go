// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build !js

package offline

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

const relocationManifest = ".journal-relocation-v1.json"

type journalRelocation struct {
	Version int             `json:"version"`
	Phase   string          `json:"phase"`
	Source  JournalSnapshot `json:"source"`
	Target  JournalSnapshot `json:"target"`
	Archive JournalSnapshot `json:"archive"`
}

// RelocateJournalContext moves this app's entire private cache, including other
// accounts, to a suite-visible directory without sharing any OS key. acquire
// must drain existing work and obtain the persisted exclusive session witness.
// This must never receive another app/UID's cache or a shared desktop cache.
// The original remains sealed and kernel-fenced; it is never deleted or reopened.
func RelocateJournalContext(ctx context.Context, source, target string, acquire WorkLease) (*Store, error) {
	if acquire == nil {
		return nil, ErrEncryptedStore
	}
	release, err := acquire(ctx)
	if err != nil {
		return nil, err
	}
	if release == nil {
		return nil, ErrEncryptedStore
	}
	defer release()
	return relocateJournal(ctx, source, target, systemSecrets{}, nil)
}

func relocateJournal(ctx context.Context, sourceDir, targetDir string, secrets CacheSecrets, checkpoint func(string) error) (*Store, error) {
	if !filepath.IsAbs(sourceDir) || !filepath.IsAbs(targetDir) || filepath.Clean(sourceDir) != sourceDir || filepath.Clean(targetDir) != targetDir {
		return nil, ErrPendingJournal
	}
	if sourceDir == targetDir || withinJournal(sourceDir, targetDir) || withinJournal(targetDir, sourceDir) {
		return nil, ErrJournalConflict
	}
	if err := os.MkdirAll(filepath.Dir(targetDir), 0o700); err != nil { //nolint:gosec // G703: caller supplies an approved native App Group root, never a network/document path.
		return nil, err
	}
	root, err := os.OpenRoot(filepath.Dir(targetDir))
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	lock, err := ajustes.AbrirCreandoEnRaiz(root, ".journal-relocation-lock", os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	release, err := ajustes.CerrojoFichero(ctx, lock)
	if err != nil {
		return nil, err
	}
	defer release()
	state, exists, err := readJournalRelocation(root)
	if err != nil {
		return nil, err
	}
	source, releaseSource, err := leaseRelocationSource(ctx, sourceDir, state, secrets)
	if err != nil {
		return nil, err
	}
	defer releaseSource()
	if exists && (state.Phase == "verified" || state.Phase == "retired") {
		return resumeJournalRelocation(ctx, root, source, sourceDir, targetDir, state, secrets, checkpoint)
	}
	state, err = copyJournalRelocation(ctx, root, source, targetDir, state, exists, secrets, checkpoint)
	if err != nil {
		return nil, err
	}
	return resumeJournalRelocation(ctx, root, source, sourceDir, targetDir, state, secrets, checkpoint)
}

func copyJournalRelocation(ctx context.Context, root *os.Root, source *Store, targetDir string, state journalRelocation, exists bool, secrets CacheSecrets, checkpoint func(string) error) (journalRelocation, error) {
	snapshot, err := source.Snapshot(ctx)
	if err != nil {
		return state, err
	}
	if exists && state.Source != snapshot {
		return state, ErrJournalConflict
	}
	if !exists {
		state = journalRelocation{Version: 1, Phase: "copying", Source: snapshot}
		if err := writeJournalRelocation(root, state); err != nil {
			return state, err
		}
		if err := journalCheckpoint(checkpoint, "copying"); err != nil {
			return state, err
		}
	}
	target, err := OpenWithSecretsContext(ctx, targetDir, secrets)
	if err != nil {
		return state, err
	}
	defer closeJournalStore(target)
	archiveDir := filepath.Join(filepath.Dir(targetDir), "retained", snapshot.CacheID)
	archive, err := OpenWithSecretsContext(ctx, archiveDir, secrets)
	if err != nil {
		return state, err
	}
	defer closeJournalStore(archive)
	copied, err := source.CopySnapshotTo(ctx, target)
	if err != nil || copied != snapshot {
		return state, errors.Join(ErrJournalConflict, err)
	}
	if err := journalCheckpoint(checkpoint, "copied"); err != nil {
		return state, err
	}
	copied, err = source.CopySnapshotTo(ctx, archive)
	if err != nil || copied != snapshot {
		return state, errors.Join(ErrJournalConflict, err)
	}
	state, err = verifyRelocationCopies(ctx, target, archive, targetDir, archiveDir, state)
	if err != nil {
		return state, err
	}
	state.Phase = "verified"
	if err := writeJournalRelocation(root, state); err != nil {
		return state, err
	}
	if err := journalCheckpoint(checkpoint, "verified"); err != nil {
		return state, err
	}
	return state, nil
}

func withinJournal(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && rel != "" && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func journalCheckpoint(checkpoint func(string) error, phase string) error {
	if checkpoint != nil {
		return checkpoint(phase)
	}
	return nil
}

func resumeJournalRelocation(ctx context.Context, root *os.Root, source *Store, sourceDir, targetDir string, state journalRelocation, secrets CacheSecrets, checkpoint func(string) error) (*Store, error) {
	archiveDir := filepath.Join(filepath.Dir(targetDir), "retained", state.Source.CacheID)
	archive, err := snapshotJournalDirectory(ctx, archiveDir)
	if err != nil || archive != state.Archive {
		return nil, errors.Join(ErrPendingJournal, err)
	}
	targetSnapshot, err := snapshotJournalDirectory(ctx, targetDir)
	if err != nil || targetSnapshot.CacheID != state.Target.CacheID {
		return nil, errors.Join(ErrJournalConflict, err)
	}
	info, err := os.Lstat(sourceDir) //nolint:gosec // G703: inspect the caller-owned native cache root itself, never a record path.
	if err != nil || !info.IsDir() {
		return nil, errors.Join(ErrPendingJournal, err)
	}
	if state.Phase == "verified" {
		if targetSnapshot != state.Target {
			return nil, ErrJournalConflict
		}
		if info.Mode().Perm() != 0 {
			if source == nil {
				return nil, ErrPendingJournal
			}
			err = source.fenceRetired(ctx, state.Source)
			if err != nil {
				return nil, err
			}
		}
		if err := journalCheckpoint(checkpoint, "fenced"); err != nil {
			return nil, err
		}
		state.Phase = "retired"
		if err := writeJournalRelocation(root, state); err != nil {
			return nil, err
		}
	} else if info.Mode().Perm() != 0 {
		return nil, ErrPendingJournal
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return OpenWithSecretsContext(ctx, targetDir, secrets)
}

func closeJournalStore(store *Store) {
	if files, ok := store.b.(ficheros); ok {
		_ = files.raiz.Close()
	}
}

// JournalRelocationRetired checks only opaque metadata and the retained encrypted
// archive. A copying/verified, corrupt or uncertain state never hides old work.
// Once retired, destination acknowledgements do not reactivate the archive.
func JournalRelocationRetired(ctx context.Context, sourceDir, targetDir string) (bool, error) {
	visible, err := JournalRelocationVisible(ctx, targetDir)
	if err != nil || !visible {
		return false, err
	}
	info, err := os.Lstat(sourceDir) //nolint:gosec // G703: inspect the caller-owned native cache root itself, never a record path.
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0 {
		return false, errors.Join(ErrPendingJournal, err)
	}
	return true, ctx.Err()
}

// JournalRelocationVisible verifies a closed app's shared relocation metadata,
// without opening its private source cache or loading its OS key. Missing state
// returns false; that is not proof that a legacy app has no local drafts.
func JournalRelocationVisible(ctx context.Context, targetDir string) (bool, error) {
	root, err := os.OpenRoot(filepath.Dir(targetDir))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = root.Close() }()
	state, exists, err := readJournalRelocation(root)
	if err != nil || !exists {
		return false, err
	}
	if state.Phase != "retired" {
		return false, ErrPendingJournal
	}
	archive, err := snapshotJournalDirectory(ctx, filepath.Join(filepath.Dir(targetDir), "retained", state.Source.CacheID))
	if err != nil || archive != state.Archive {
		return false, errors.Join(ErrPendingJournal, err)
	}
	target, err := os.OpenRoot(targetDir)
	if err != nil {
		return false, err
	}
	defer func() { _ = target.Close() }()
	id, created, err := cacheIdentity(target)
	if err != nil || created || id != state.Target.CacheID {
		return false, errors.Join(ErrPendingJournal, err)
	}
	return true, ctx.Err()
}

func readJournalRelocation(root *os.Root) (journalRelocation, bool, error) {
	raw, err := root.ReadFile(relocationManifest)
	if errors.Is(err, fs.ErrNotExist) {
		return journalRelocation{}, false, nil
	}
	if err != nil {
		return journalRelocation{}, false, err
	}
	var state journalRelocation
	if json.Unmarshal(raw, &state) != nil || state.Version != 1 || !validJournalSnapshot(state.Source) || (state.Phase != "copying" && state.Phase != "verified" && state.Phase != "retired") {
		return state, false, ErrPendingJournal
	}
	if state.Phase != "copying" && (!validJournalSnapshot(state.Target) || !validJournalSnapshot(state.Archive) || state.Target.CacheID == state.Source.CacheID || state.Archive.CacheID == state.Source.CacheID || state.Archive.CacheID == state.Target.CacheID) {
		return state, false, ErrPendingJournal
	}
	canonical, err := json.Marshal(state)
	if err != nil || !bytes.Equal(raw, canonical) {
		return state, false, ErrPendingJournal
	}
	return state, true, nil
}

func validJournalSnapshot(snapshot JournalSnapshot) bool {
	digest, err := hex.DecodeString(snapshot.Digest)
	return validCacheIdentity(snapshot.CacheID) && snapshot.Records >= 0 && err == nil && len(digest) == 32 && hex.EncodeToString(digest) == snapshot.Digest
}

func writeJournalRelocation(root *os.Root, state journalRelocation) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := ajustes.EscribirEnRaiz(root, relocationManifest, raw); err != nil {
		return err
	}
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer func() { _ = directory.Close() }()
	return directory.Sync()
}

func snapshotJournalDirectory(ctx context.Context, dir string) (JournalSnapshot, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return JournalSnapshot{}, err
	}
	defer func() { _ = root.Close() }()
	id, created, err := cacheIdentity(root)
	if err != nil || created {
		return JournalSnapshot{}, errors.Join(ErrPendingJournal, err)
	}
	return snapshotJournal(ctx, ficheros{raiz: root, cacheID: id})
}

func syncJournalTree(dir string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	return fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return err
		}
		directory, err := root.Open(name)
		if err != nil {
			return err
		}
		syncErr := directory.Sync()
		closeErr := directory.Close()
		return errors.Join(syncErr, closeErr)
	})
}

func verifyRelocationCopies(ctx context.Context, target, archive *Store, targetDir, archiveDir string, state journalRelocation) (journalRelocation, error) {
	var err error
	state.Target, err = target.Snapshot(ctx)
	if err != nil {
		return state, err
	}
	state.Archive, err = archive.Snapshot(ctx)
	if err != nil {
		return state, err
	}
	if state.Target.Records < state.Source.Records || state.Archive.Records != state.Source.Records {
		return state, ErrJournalConflict
	}
	if err := syncJournalTree(targetDir); err != nil {
		return state, err
	}
	if err := syncJournalTree(filepath.Dir(archiveDir)); err != nil {
		return state, err
	}
	return state, nil
}

// HTTP responses have their own maintained cache lease. The private EX witness
// drains repository writes, while this lease also excludes older HTTP writers
// until the source is kernel-fenced. Keep one source open: sealCache takes this
// lease too, so reopening it inside the transaction would deadlock.
func leaseRelocationSource(ctx context.Context, sourceDir string, state journalRelocation, secrets CacheSecrets) (*Store, func(), error) {
	if state.Phase == "retired" {
		return nil, func() {}, nil
	}
	if state.Phase == "verified" {
		info, err := os.Lstat(sourceDir) //nolint:gosec // G703: caller-owned native source root, never a document or record path.
		if err != nil {
			return nil, nil, err
		}
		if info.IsDir() && info.Mode().Perm() == 0 {
			return nil, func() {}, nil
		}
	}
	source, err := OpenWithSecretsContext(ctx, sourceDir, secrets)
	if err != nil {
		return nil, nil, err
	}
	files, ok := source.b.(ficheros)
	if !ok {
		closeJournalStore(source)
		return nil, nil, ErrEncryptedStore
	}
	release, err := files.lockHTTP(ctx)
	if err != nil {
		closeJournalStore(source)
		return nil, nil, err
	}
	return source, func() { release(); closeJournalStore(source) }, nil
}
