// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

// atras recoge la herramienta y el panel antes de usar la salida de la cinta. No
// cierra diálogos por su cuenta: ui los descarta antes, con su propia confirmación.
func (p *Pantalla) atras() bool {
	if p.Contenido == nil {
		if p.doc == nil || p.AlVolver == nil {
			return false
		}
		p.AlVolver() // también desde un PDF que pide clave o falló al abrirse
		return true
	}
	switch {
	case p.barra.Herramienta != Mano:
		p.barra.Herramienta = Mano
		p.Recomponer()
	case p.PanelAbierto:
		p.PanelAbierto = false
		p.Recomponer()
	case p.AlVolver != nil:
		p.AlVolver()
	default:
		return false
	}
	return true
}
