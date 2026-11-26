// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package almacen

import "testing"

// AntesDeBarrer pone, durante la prueba, lo que pasa entre renombrar el diario y barrer
// los objetos al compactar (nil: nada).
func AntesDeBarrer(t *testing.T, f func() error) {
	t.Helper()
	if f == nil {
		f = func() error { return nil }
	}
	antes := antesDeBarrer
	antesDeBarrer = f
	t.Cleanup(func() { antesDeBarrer = antes })
}

// CompactacionesYa son las compactaciones en el acto que han empezado (CompactarYa).
func CompactacionesYa(r *Recursos) int {
	r.ya.mu.Lock()
	defer r.ya.mu.Unlock()
	return r.ya.hechas
}
