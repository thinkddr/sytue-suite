// SPDX-License-Identifier: Apache-2.0
package offline

import (
	"context"
	"encoding/json"
	"io/fs"
	"path"
	"strings"
)

// SealedStorePort delegates to a scope-bound encrypted host store with durable
// async record locks. It is an explicit adapter; OpenBrowser never selects it
// and a plaintext/localStorage backend remains ineligible for private journals.
type SealedStorePort interface {
	Read(context.Context, string) ([]byte, bool, error)
	Write(context.Context, string, []byte) error
	Delete(context.Context, string) error
	Keys(context.Context, string, string, int) ([]string, string, error)
	Lock(context.Context, string) (func(), error)
	Revalidate(context.Context) error
}
type sealedPortBackend struct {
	ctx         context.Context
	ownerPrefix string
	port        SealedStorePort
}

func OpenSealedPort(ctx context.Context, owner string, port SealedStorePort) (*Store, error) {
	if ctx == nil || owner == "" || port == nil {
		return nil, ErrEncryptedStore
	}
	if err := port.Revalidate(ctx); err != nil {
		return nil, err
	}
	return &Store{b: &sealedPortBackend{ctx: ctx, ownerPrefix: clave(owner) + "/", port: port}}, nil
}
func (b *sealedPortBackend) encrypted() bool { return true }
func (b *sealedPortBackend) check(key string) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	if !strings.HasPrefix(key, b.ownerPrefix) || strings.Contains(key, "..") {
		return ErrEncryptedStore
	}
	for _, part := range strings.Split(strings.TrimSuffix(key, "/"), "/") {
		if len(part) != 32 || strings.Trim(part, "0123456789abcdef") != "" {
			return ErrEncryptedStore
		}
	}
	return nil
}

func (b *sealedPortBackend) get(key string) ([]byte, error) {
	if err := b.check(key); err != nil {
		return nil, err
	}
	raw, found, err := b.port.Read(b.ctx, key)
	if err == nil && !found {
		clear(raw)
		return nil, fs.ErrNotExist
	}
	return raw, err
}

func (b *sealedPortBackend) put(key string, raw []byte) error {
	if err := b.check(key); err != nil {
		return err
	}
	return b.port.Write(b.ctx, key, raw)
}

func (b *sealedPortBackend) del(key string) error {
	if err := b.check(key); err != nil {
		return err
	}
	return b.port.Delete(b.ctx, key)
}

func (b *sealedPortBackend) keys(ctx context.Context, prefix, after string, limit int) ([]string, string, error) {
	prefix = strings.TrimSuffix(prefix, "/") + "/"
	if err := b.check(prefix); err != nil {
		return nil, "", err
	}
	keys, next, err := b.port.Keys(ctx, prefix, after, limit)
	if err != nil {
		return nil, "", err
	}
	if len(keys) > limit {
		return nil, "", ErrPendingJournal
	}
	previous := after
	for _, key := range keys {
		if b.check(key) != nil || !strings.HasPrefix(key, prefix) || key <= previous {
			return nil, "", ErrPendingJournal
		}
		previous = key
	}
	if next != "" && (len(keys) == 0 || next != keys[len(keys)-1]) {
		return nil, "", ErrPendingJournal
	}
	return keys, next, nil
}

func (b *sealedPortBackend) allKeys(prefix string) ([]string, error) {
	var out []string
	after := ""
	for {
		keys, next, err := b.keys(b.ctx, prefix, after, 128)
		if err != nil {
			return nil, err
		}
		out = append(out, keys...)
		if next == "" {
			return out, nil
		}
		if next == after {
			return nil, ErrPendingJournal
		}
		after = next
	}
}

func (b *sealedPortBackend) list(prefix string) ([][]byte, error) {
	keys, err := b.allKeys(prefix)
	if err != nil {
		return nil, err
	}
	var out [][]byte
	for _, key := range keys {
		raw, err := b.get(key)
		if err != nil {
			for _, prior := range out {
				clear(prior)
			}
			return nil, err
		}
		out = append(out, raw)
	}
	return out, nil
}

func (b *sealedPortBackend) countPrefix(prefix string) (int, error) {
	keys, err := b.allKeys(prefix)
	return len(keys), err
}
func (b *sealedPortBackend) contar(string) (int, error) { return 0, ErrEncryptedStore }
func (b *sealedPortBackend) clear() error               { return ErrEncryptedStore }
func (b *sealedPortBackend) lockRecord(ctx context.Context, key string) (func(), error) {
	if err := b.check(key); err != nil {
		return nil, err
	}
	return b.port.Lock(ctx, key)
}

func (b *sealedPortBackend) recordIDsPage(ctx context.Context, prefix, after string, limit int) ([]string, string, error) {
	if after != "" {
		if len(after) != 32 || strings.Trim(after, "0123456789abcdef") != "" {
			return nil, "", ErrPendingJournal
		}
		after = prefix + "/" + after
	}
	keys, next, err := b.keys(ctx, prefix, after, limit)
	if err != nil {
		return nil, "", err
	}
	ids := make([]string, 0, len(keys))
	for _, key := range keys {
		raw, err := b.get(key)
		if err != nil {
			return nil, "", err
		}
		var record Record
		err = json.Unmarshal(raw, &record)
		clear(raw)
		if err != nil || record.ID == "" || path.Base(key) != clave(record.ID) {
			clear(record.Value)
			return nil, "", ErrPendingJournal
		}
		clear(record.Value)
		ids = append(ids, record.ID)
	}
	if next != "" {
		next = path.Base(next)
	}
	return ids, next, nil
}
