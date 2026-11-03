// SPDX-License-Identifier: Apache-2.0
package pqc

import (
	"bytes"
	"sync"
)

// StorageKeyDecoder extends the existing serialized keyring cache with ONE
// parsed identity per client lifetime. It is not authority or a freshness cache:
// callers still obtain current owner/recovery bindings and secure-store bytes.
// The complete bytes and binding must match before a decode can be reused.
// Do not copy after first use; the owner resets it when retiring its keys.
type StorageKeyDecoder struct {
	mu       sync.Mutex
	binding  StorageKeyBinding
	raw      []byte
	secret   StorageKeySecret
	identity *Identity
}

func (d *StorageKeyDecoder) Decode(raw []byte, binding StorageKeyBinding) (StorageKeySecret, *Identity, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !binding.Valid() || len(raw) == 0 || len(raw) > 16<<10 {
		d.reset()
		return StorageKeySecret{}, nil, ErrIdentity
	}
	if d.identity != nil && d.binding == binding && bytes.Equal(d.raw, raw) {
		return d.secret, d.identity, nil
	}
	d.reset()
	secret, identity, err := DecodeStorageKeySecret(raw, binding)
	if err != nil {
		return StorageKeySecret{}, nil, err
	}
	d.binding, d.raw, d.secret, d.identity = binding, bytes.Clone(raw), secret, identity
	return secret, identity, nil
}

// Reset clears owned serialized bytes and releases parsed-key references.
// It does not cancel work or revoke a device; the owning lifecycle does that.
func (d *StorageKeyDecoder) Reset() { d.mu.Lock(); defer d.mu.Unlock(); d.reset() }

func (d *StorageKeyDecoder) reset() {
	clear(d.raw)
	d.raw = nil
	d.binding = StorageKeyBinding{}
	d.secret = StorageKeySecret{}
	d.identity = nil
}
