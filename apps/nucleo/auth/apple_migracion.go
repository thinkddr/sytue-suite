// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"bytes"
	"strings"
)

// PrivateSecretMigration is implemented by the native keychain boundary. Move
// must change the access group atomically; it must never overwrite a target.
type privateSecretMigration interface {
	Read(string, string) ([]byte, error)
	Move(string, string, string) error
}

func migrarSecretoPrivado(store privateSecretMigration, account, own, shared string, validate func([]byte) error) ([]byte, error) {
	if store == nil || own == "" || shared == "" || own == shared || validate == nil {
		return nil, ErrSecretoLegadoCompartido
	}
	current, err := store.Read(account, own)
	if err != nil {
		return nil, err
	}
	if len(current) != 0 {
		if err = validate(current); err != nil {
			clear(current)
			return nil, err
		}
		return current, nil
	}
	original, err := store.Read(account, shared)
	if err != nil {
		return nil, err
	}
	defer clear(original)
	if len(original) == 0 {
		return nil, ErrSecretoLegadoCompartido
	}
	if err := validate(original); err != nil {
		return nil, err
	}
	if err := store.Move(account, shared, own); err != nil {
		return nil, err
	}
	copied, err := store.Read(account, own)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(original, copied) {
		clear(copied)
		return nil, ErrSecretoLegadoCompartido
	}
	if err = validate(copied); err != nil {
		clear(copied)
		return nil, err
	}
	return copied, nil
}

func cuentaPrivadaMigrable(account string) bool {
	return strings.HasPrefix(account, "offline-cache-") || strings.HasPrefix(account, "rooms-matrix-session-") || strings.HasPrefix(account, "rooms-matrix-master-")
}
