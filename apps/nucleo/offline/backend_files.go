// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package offline

import (
	"context"
	"crypto/cipher"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

// ponytail: un fichero por valor, no bbolt como pedía el encargo. bbolt bloquea su
// fichero para UN proceso, y la suite son varios binarios (Mail, Drive, el paraguas…)
// abiertos a la vez sobre los mismos datos: el segundo se quedaría esperando al
// arrancar. Techo: miles de valores por espacio, que es leer un directorio. Si se
// queda corto, SQLite en modo WAL, que sí admite varios procesos.
//
// Todo pasa por os.Root: aunque las claves ya son hashes, es el sistema —y no una
// comprobación nuestra— quien garantiza que nada se abre fuera del directorio.
type ficheros struct {
	raiz    *os.Root
	sealed  cipher.AEAD
	cacheID string
}

const recordLockPrefix = ".record-lock-"

func (f ficheros) encrypted() bool { return f.sealed != nil }

func (f ficheros) lockRecord(ctx context.Context, key string) (func(), error) {
	file, err := ajustes.AbrirCreandoEnRaiz(f.raiz, recordLockPrefix+clave(key), os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	return ajustes.CerrojoFichero(ctx, file)
}

// Open abre (y crea) el almacén bajo dir: lo normal, os.UserCacheDir()/sytue/offline.
func Open(dir string) (*Store, error) {
	return OpenContext(context.Background(), dir)
}

func OpenContext(ctx context.Context, dir string) (*Store, error) {
	return OpenWithSecretsContext(ctx, dir, systemSecrets{})
}

// ClearDirectory erases records even if the OS keychain is locked or unavailable.
// Logout must not need permission to decrypt the data it is removing.
func ClearDirectory(dir string) error {
	root, err := os.OpenRoot(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	f := ficheros{raiz: root}
	unlock, err := f.lockClear()
	if err != nil {
		return err
	}
	defer unlock()
	return f.clear()
}

// CountDirectory can warn about unsent drafts even when the keychain is locked.
// It reads only hashed record paths; it never decrypts or replaces private data.
func CountDirectory(dir string, namespaces ...string) (int, error) {
	root, err := os.OpenRoot(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer func() { _ = root.Close() }()
	store := Store{b: ficheros{raiz: root}}
	return store.Count(namespaces...)
}

// OpenWithSecrets is Open with an injectable OS-secret-store boundary. A missing
// or locked key never falls back to plaintext or replaces an existing cache key.
func OpenWithSecrets(dir string, secrets CacheSecrets) (*Store, error) {
	return OpenWithSecretsContext(context.Background(), dir, secrets)
}

func OpenWithSecretsContext(ctx context.Context, dir string, secrets CacheSecrets) (*Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil { //nolint:gosec // G703: native caller establishes the cache root; all records are confined with os.Root below.
		return nil, err
	}
	raiz, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	f, err := sealCache(ctx, raiz, secrets)
	if err != nil {
		_ = raiz.Close()
		return nil, err
	}
	return &Store{b: f}, nil
}

func (f ficheros) get(clave string) ([]byte, error) {
	data, err := f.raiz.ReadFile(clave + ".json")
	if err != nil {
		return nil, err
	}
	return f.openRecord(clave, data)
}

// put escribe a un temporal y lo renombra: un corte de luz deja el valor viejo o el
// nuevo, nunca medio.
func (f ficheros) put(clave string, datos []byte) error {
	sealed, err := f.sealRecord(clave, datos)
	if err != nil {
		return err
	}
	return f.putRaw(clave, sealed)
}

func (f ficheros) putRaw(clave string, datos []byte) error {
	return ajustes.EscribirEnRaiz(f.raiz, clave+".json", datos)
}

func (f ficheros) del(clave string) error {
	if err := f.raiz.Remove(clave + ".json"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (f ficheros) list(prefijo string) ([][]byte, error) {
	entradas, err := fs.ReadDir(f.raiz.FS(), prefijo)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out [][]byte
	for _, e := range entradas {
		if e.IsDir() || path.Ext(e.Name()) != ".json" {
			continue
		}
		datos, err := f.get(path.Join(prefijo, strings.TrimSuffix(e.Name(), ".json")))
		if errors.Is(err, fs.ErrNotExist) {
			continue // otro proceso lo ha borrado entre el ReadDir y ahora
		}
		if err != nil {
			return nil, err
		}
		out = append(out, datos)
	}
	return out, nil
}

func (f ficheros) contar(espacio string) (int, error) {
	duenos, err := fs.ReadDir(f.raiz.FS(), ".")
	if err != nil {
		return 0, err
	}
	n := 0
	for _, d := range duenos {
		if !d.IsDir() {
			continue
		}
		count, err := f.countPrefix(path.Join(d.Name(), espacio))
		if err != nil {
			return n, err
		}
		n += count
	}
	return n, nil
}

func (f ficheros) countPrefix(prefix string) (int, error) {
	entries, err := fs.ReadDir(f.raiz.FS(), prefix)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		_, temporary := cacheTemporary(entry)
		if entry.IsDir() || (path.Ext(entry.Name()) != ".json" && !temporary) {
			return 0, ErrPendingJournal
		}
		count++
	}
	return count, nil
}

// CountDirectoryOwner uses only hashed path metadata, even with a locked keychain.
func CountDirectoryOwner(dir, owner string, namespaces ...string) (int, error) {
	root, err := os.OpenRoot(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer func() { _ = root.Close() }()
	store := Store{b: ficheros{raiz: root}}
	return store.CountOwner(owner, namespaces...)
}

func (f ficheros) clear() error {
	entradas, err := fs.ReadDir(f.raiz.FS(), ".")
	if err != nil {
		return err
	}
	for _, e := range entradas {
		if e.Name() == ".http-cache-lock" || e.Name() == cacheMarker || e.Name() == cacheKeyLock || strings.HasPrefix(e.Name(), recordLockPrefix) {
			continue // stable inode, also while another app is waiting for the lock
		}
		if err := f.raiz.RemoveAll(e.Name()); err != nil {
			return err
		}
	}
	return nil
}

func (f ficheros) recordIDs(prefix string) ([]string, error) {
	entries, err := fs.ReadDir(f.raiz.FS(), prefix)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		id, err := f.recordID(prefix, entry)
		if err != nil {
			return nil, err
		}
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (f ficheros) recordID(prefix string, entry fs.DirEntry) (string, error) {
	if _, temporary := cacheTemporary(entry); temporary {
		return "", ErrPendingJournal
	}
	if entry.IsDir() || path.Ext(entry.Name()) != ".json" {
		return "", ErrPendingJournal
	}
	raw, err := f.get(path.Join(prefix, strings.TrimSuffix(entry.Name(), ".json")))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var record struct {
		ID string `json:"id"`
	}
	err = json.Unmarshal(raw, &record)
	clear(raw)
	if err != nil {
		return "", err
	}
	if record.ID == "" || entry.Name() != clave(record.ID)+".json" {
		return "", ErrPendingJournal
	}
	return record.ID, nil
}

func (f ficheros) recordIDsPage(ctx context.Context, prefix, after string, limit int) ([]string, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if after != "" {
		decoded, err := hex.DecodeString(after)
		if err != nil || len(decoded) != 16 || hex.EncodeToString(decoded) != after {
			return nil, "", ErrPendingJournal
		}
	}
	entries, err := fs.ReadDir(f.raiz.FS(), prefix)
	if errors.Is(err, fs.ErrNotExist) {
		return []string{}, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	first := 0
	if after != "" {
		first = sort.Search(len(entries), func(i int) bool { return entries[i].Name() > after+".json" })
	}
	last := min(first+limit, len(entries))
	ids := make([]string, 0, last-first)
	for _, entry := range entries[first:last] {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		id, err := f.recordID(prefix, entry)
		if err != nil {
			return nil, "", err
		}
		if id != "" {
			ids = append(ids, id)
		}
	}
	next := ""
	if last < len(entries) {
		next = strings.TrimSuffix(entries[last-1].Name(), ".json")
	}
	return ids, next, nil
}

// ClearDirectoryKeeping removes ephemeral state while retaining durable work of
// every account. Namespace names come from the suite's shared journal registry.
func ClearDirectoryKeeping(dir string, namespaces ...string) error {
	return ClearDirectoryKeepingContext(context.Background(), dir, namespaces...)
}

func ClearDirectoryKeepingContext(ctx context.Context, dir string, namespaces ...string) error {
	return withDirectoryForClear(ctx, dir, func(files ficheros) error {
		keep := make(map[string]bool, len(namespaces))
		for _, namespace := range namespaces {
			keep[clave(namespace)] = true
		}
		owners, err := fs.ReadDir(files.raiz.FS(), ".")
		if err != nil {
			return err
		}
		for _, owner := range owners {
			if !owner.IsDir() {
				continue
			}
			entries, err := fs.ReadDir(files.raiz.FS(), owner.Name())
			if err != nil {
				return err
			}
			for _, entry := range entries {
				if !keep[entry.Name()] {
					if err := files.raiz.RemoveAll(path.Join(owner.Name(), entry.Name())); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
}

// DiscardDirectoryOwner requires the caller's exclusive scope lease and explicit
// user choice. It never erases another account or replaces stable lock inodes.
func DiscardDirectoryOwner(dir, owner string, namespaces ...string) error {
	return DiscardDirectoryOwnerContext(context.Background(), dir, owner, namespaces...)
}

func DiscardDirectoryOwnerContext(ctx context.Context, dir, owner string, namespaces ...string) error {
	return withDirectoryForClear(ctx, dir, func(files ficheros) error {
		for _, namespace := range namespaces {
			if err := files.raiz.RemoveAll(clave(owner, namespace)); err != nil {
				return err
			}
		}
		return nil
	})
}

func withDirectoryForClear(ctx context.Context, dir string, fn func(ficheros) error) error {
	root, err := os.OpenRoot(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	files := ficheros{raiz: root}
	release, err := files.lockHTTP(ctx)
	if err != nil {
		return err
	}
	defer release()
	return fn(files)
}
