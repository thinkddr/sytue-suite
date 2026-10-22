// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

// atras cierra primero el hilo o panel derecho. En un teléfono vuelve después a los
// canales; el borrador sigue asociado al canal y reaparece al elegirlo de nuevo.
func (sa *Sala) atras() bool {
	if sa.P.Canal.Derecho != nil {
		sa.cerrarDerecho()
		return true
	}
	if sa.abierto == "" {
		return false
	}
	sa.vista++ // invalida una carga de mensajes que aún estuviera en vuelo
	sa.abierto = ""
	sa.pintarCanales()
	sa.P.SinCanal()
	return true
}
