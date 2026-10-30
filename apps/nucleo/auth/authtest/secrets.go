// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package authtest provides an in-memory OS-secret-store boundary for tests. It
// deliberately has no disk/network fallback and must not be used by applications.
package authtest

import (
	"bytes"
	"sync"
)

type Secrets struct {
	mu     sync.Mutex
	values map[string][]byte
}

func (s *Secrets) Read(key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return bytes.Clone(s.values[key]), nil
}

func (s *Secrets) Write(key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil {
		s.values = map[string][]byte{}
	}
	clear(s.values[key])
	s.values[key] = bytes.Clone(value)
	return nil
}

func (s *Secrets) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.values[key])
	delete(s.values, key)
	return nil
}
