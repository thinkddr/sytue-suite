// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

// atras usa la salida «Todas las bases», con su misma recarga de la lista.
func (p *Pantalla) atras() bool {
	if p.abierta == nil {
		return false
	}
	p.volverALista()
	return true
}
