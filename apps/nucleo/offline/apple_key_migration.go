// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build !js

package offline

import (
	"context"
	"errors"
	"os"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

// A system reader can claim only the cache whose immutable marker was read from
// this already-open root. The existing codec authenticates every sealed record
// before the OS atomically moves the key; no plaintext fallback or new key.
type boundCacheSecrets struct {
	CacheSecrets
	root    *os.Root
	cacheID string
	ctx     context.Context
}

func (s boundCacheSecrets) Read(name string) ([]byte, error) {
	raw, err := s.CacheSecrets.Read(name)
	if !errors.Is(err, auth.ErrSecretoLegadoCompartido) {
		return raw, err
	}
	if name != "offline-cache-"+s.cacheID {
		return nil, ErrCacheKey
	}
	scheme, err := auth.EsquemaAplicacionPrivada()
	if err != nil {
		return nil, err
	}
	return auth.MigrarSecretoPrivado(name, scheme, s.validate)
}

func (s boundCacheSecrets) validate(data []byte) error {
	key, err := decodeCacheKey(data)
	if err != nil {
		return err
	}
	defer clear(key.Key)
	sealed, err := cacheCipher(key.Key)
	if err != nil {
		return err
	}
	return (ficheros{raiz: s.root, sealed: sealed, cacheID: s.cacheID}).checkCache(s.ctx, key.Migrating)
}
