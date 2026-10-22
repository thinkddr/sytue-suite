// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

// atras usa las salidas de la paleta, la ficha y el menú Inicio, en ese orden.
// La ficha conserva su propia acción de cierre, igual que el botón visible.
func (p *Pantalla) atras() bool {
	switch {
	case p.abierta:
		p.CerrarPaleta()
	case p.ficha != nil:
		if p.ficha.AlCerrar == nil {
			return false
		}
		p.ficha.AlCerrar()
	case p.Vista != "" || p.lista.Modelo.Clave != "":
		p.Abrir(Modelo{})
	default:
		return false
	}
	return true
}
