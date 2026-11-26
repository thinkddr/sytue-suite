// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build !js && edicionlocal

package offline

import (
	"errors"
	"testing"
)

// TestSinLlaveroLaCacheCifradaNoSeAbre: en Sytue Office la caché cifrada de la suite no
// tiene llavero que la abra, y falla como con uno bloqueado; nunca se abre en claro.
func TestSinLlaveroLaCacheCifradaNoSeAbre(t *testing.T) {
	if _, err := OpenContext(t.Context(), t.TempDir()); !errors.Is(err, ErrCacheKey) {
		t.Fatalf("OpenContext sin llavero = %v; quiero ErrCacheKey", err)
	}
}
