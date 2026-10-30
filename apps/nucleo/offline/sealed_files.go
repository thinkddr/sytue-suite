// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build !js

package offline

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

const (
	cacheMarker  = ".encrypted-cache-v1"
	cacheKeyLock = ".cache-key-lock"
	cacheMagic   = "sytue-cache-v1\x00"
)

var (
	ErrCacheKey        = errors.New("offline: secure cache key is missing, locked or inconsistent")
	ErrCacheCiphertext = errors.New("offline: cache record is unauthenticated")
)

// CacheSecrets uses the same OS keychain as the suite session. This key is never
// sent to Sytue or an external storage provider, or derived from an OIDC token.
type CacheSecrets interface {
	Read(string) ([]byte, error)
	Write(string, []byte) error
}

type systemSecrets struct{}

func (systemSecrets) Read(k string) ([]byte, error)  { return auth.LeerSecreto(k) }
func (systemSecrets) Write(k string, v []byte) error { return auth.GuardarSecreto(k, v) }

type cacheKey struct {
	Version   int    `json:"v"`
	Key       []byte `json:"key"`
	Migrating bool   `json:"migrating"`
}

// sealCache serializes key creation/migration across suite processes. The keychain
// holds the migration state too: once sealed, injecting a legacy JSON record is an
// error, not an invitation to downgrade. Atomic replacement leaves old data intact
// on a failed write; interrupted migrations resume with the same key.
func sealCache(ctx context.Context, root *os.Root, secrets CacheSecrets) (ficheros, error) {
	if secrets == nil {
		return ficheros{}, ErrCacheKey
	}
	lock, err := root.OpenFile(cacheKeyLock, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return ficheros{}, err
	}
	unlock, err := ajustes.CerrojoFichero(ctx, lock)
	if err != nil {
		return ficheros{}, err
	}
	defer unlock()
	// The cache lock follows the key lock; ordinary writes/clear never take the
	// key lock, so a second app cannot race the migration or deadlock logout.
	plain := ficheros{raiz: root}
	release, err := plain.lockHTTP(ctx)
	if err != nil {
		return ficheros{}, err
	}
	defer release()
	cacheID, newCache, err := cacheIdentity(root)
	if err != nil {
		return ficheros{}, err
	}
	name := "offline-cache-" + cacheID
	key, err := loadCacheKey(secrets, name, newCache)
	if err != nil {
		return ficheros{}, err
	}
	defer clear(key.Key)
	block, err := aes.NewCipher(key.Key)
	if err != nil {
		return ficheros{}, ErrCacheKey
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return ficheros{}, err
	}
	f := ficheros{raiz: root, sealed: aead, cacheID: cacheID}
	if newCache {
		if err := ajustes.EscribirEnRaiz(root, cacheMarker, []byte(cacheMagic+cacheID)); err != nil {
			return ficheros{}, err
		}
	}
	if key.Migrating {
		if err := f.migrateCache(ctx); err != nil {
			return ficheros{}, err
		}
		key.Migrating = false
		if err := saveCacheKey(secrets, name, key); err != nil {
			return ficheros{}, err
		}
	}
	return f, nil
}

// cacheIdentity survives an OS moving the app container during an update.
func cacheIdentity(root *os.Root) (string, bool, error) {
	marker, err := root.ReadFile(cacheMarker)
	if errors.Is(err, fs.ErrNotExist) {
		if err := rejectOrphanedCiphertext(root); err != nil {
			return "", false, err
		}
		return rand.Text(), true, nil
	}
	if err != nil {
		return "", false, err
	}
	id := strings.TrimPrefix(string(marker), cacheMagic)
	invalid := func(c rune) bool { return (c < 'A' || c > 'Z') && (c < '2' || c > '7') }
	if !bytes.HasPrefix(marker, []byte(cacheMagic)) || len(id) < 26 || len(id) > 64 || strings.IndexFunc(id, invalid) >= 0 {
		return "", false, ErrCacheKey
	}
	return id, false, nil
}

func rejectOrphanedCiphertext(root *os.Root) error {
	return fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		_, temporary := cacheTemporary(entry)
		if entry.IsDir() || !strings.HasSuffix(name, ".json") && !temporary {
			return nil
		}
		data, err := root.ReadFile(name)
		defer clear(data)
		if err != nil {
			return err
		}
		if bytes.HasPrefix(data, []byte(cacheMagic)) {
			return ErrCacheKey
		}
		return nil
	})
}

func loadCacheKey(secrets CacheSecrets, name string, newCache bool) (cacheKey, error) {
	raw, err := secrets.Read(name)
	if err != nil {
		return cacheKey{}, ErrCacheKey
	}
	defer clear(raw)
	if len(raw) != 0 {
		var key cacheKey
		if json.Unmarshal(raw, &key) != nil || key.Version != 1 || len(key.Key) != 32 {
			clear(key.Key)
			return cacheKey{}, ErrCacheKey
		}
		return key, nil
	}
	if !newCache {
		return cacheKey{}, ErrCacheKey
	}
	key := cacheKey{Version: 1, Key: make([]byte, 32), Migrating: true}
	if _, err := rand.Read(key.Key); err != nil {
		clear(key.Key)
		return cacheKey{}, err
	}
	if err := saveCacheKey(secrets, name, key); err != nil {
		clear(key.Key)
		return cacheKey{}, err
	}
	return key, nil
}

func saveCacheKey(secrets CacheSecrets, name string, key cacheKey) error {
	raw, err := json.Marshal(key)
	if err != nil {
		return err
	}
	defer clear(raw)
	if err := secrets.Write(name, raw); err != nil {
		return ErrCacheKey
	}
	return nil
}

func (f ficheros) sealRecord(key string, data []byte) ([]byte, error) {
	if f.sealed == nil {
		return nil, ErrCacheKey
	}
	return f.sealed.Seal([]byte(cacheMagic), nil, data, []byte(cacheMagic+f.cacheID+"\x00"+key)), nil //nolint:gosec // G407: NewGCMWithRandomNonce generates and prepends a fresh nonce internally; nil is mandatory.
}

func (f ficheros) openRecord(key string, data []byte) ([]byte, error) {
	if f.sealed == nil {
		return nil, ErrCacheKey
	}
	n := len(cacheMagic) + f.sealed.NonceSize()
	if len(data) < n+f.sealed.Overhead() || !bytes.HasPrefix(data, []byte(cacheMagic)) {
		return nil, ErrCacheCiphertext
	}
	plain, err := f.sealed.Open(nil, data[len(cacheMagic):n], data[n:], []byte(cacheMagic+f.cacheID+"\x00"+key))
	if err != nil {
		return nil, ErrCacheCiphertext
	}
	return plain, nil
}

func (f ficheros) storedSize(size int64) int64 {
	return size + int64(len(cacheMagic)+f.sealed.NonceSize()+f.sealed.Overhead())
}

func (f ficheros) migrateCache(ctx context.Context) error {
	return fs.WalkDir(f.raiz.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return f.migrateCacheFile(name, entry)
	})
}

func cacheTemporary(entry fs.DirEntry) (string, bool) {
	if entry.IsDir() {
		return "", false
	}
	base, _, found := strings.Cut(entry.Name(), ".tmp-")
	base = strings.TrimSuffix(base, ".json")
	if !found || len(base) != 32 {
		return "", false
	}
	_, err := hex.DecodeString(base)
	return base, err == nil
}

func (f ficheros) migrateCacheFile(name string, entry fs.DirEntry) error {
	base, temporary := cacheTemporary(entry)
	if entry.IsDir() || !strings.HasSuffix(name, ".json") && !temporary {
		return nil
	}
	key := strings.TrimSuffix(name, ".json")
	if temporary {
		key = path.Join(path.Dir(name), base)
	}
	data, err := f.raiz.ReadFile(name)
	if err != nil {
		return err
	}
	defer clear(data)
	if bytes.HasPrefix(data, []byte(cacheMagic)) {
		plain, err := f.openRecord(key, data)
		clear(plain)
		return err
	}
	// Preserve even a damaged legacy record byte-for-byte. Orphan temporaries can
	// be the only copy of an interrupted draft and must not be deleted as garbage.
	sealed, err := f.sealRecord(key, data)
	if err != nil {
		return err
	}
	verified, err := f.openRecord(key, sealed)
	if err != nil || !bytes.Equal(verified, data) {
		clear(verified)
		return ErrCacheCiphertext
	}
	clear(verified)
	if err := ajustes.EscribirEnRaiz(f.raiz, name, sealed); err != nil {
		return err
	}
	// Confirm the authenticated replacement before advancing the secure checkpoint.
	stored, err := f.raiz.ReadFile(name)
	if err != nil {
		return err
	}
	verified, err = f.openRecord(key, stored)
	defer clear(verified)
	if err != nil || !bytes.Equal(verified, data) {
		return ErrCacheCiphertext
	}
	return nil
}
