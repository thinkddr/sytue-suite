// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package publicar

import (
	"sync"
)

var (
	islasMu        sync.RWMutex
	islasEmbebidas = map[string][]byte{}
)

// RegistrarIsla registra un bundle de islas bajo su hash de 16 caracteres hex.
func RegistrarIsla(hash16 string, contenido []byte) {
	islasMu.Lock()
	defer islasMu.Unlock()
	islasEmbebidas[hash16] = contenido
}

// BuscarIsla busca el código JS compilado de las islas por su hash de 16 caracteres.
// Devuelve nil, false si el hash no es conocido.
func BuscarIsla(hash16 string) ([]byte, bool) {
	islasMu.RLock()
	defer islasMu.RUnlock()
	b, ok := islasEmbebidas[hash16]
	return b, ok
}
