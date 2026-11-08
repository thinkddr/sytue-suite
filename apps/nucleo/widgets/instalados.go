// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package widgets

import "sync/atomic"

// instaladosPlataforma cuenta los widgets de ESTA app que la persona tiene puestos, y
// dice si lo ha podido saber. Lo fija la plataforma en su init: en iOS y macOS lo
// pregunta el puente de WidgetKit (`sytue_widgets_instalados`); en el resto no se sabe.
var instaladosPlataforma = func() (int, bool) { return 0, false }

// instaladosSimulados es lo que fija SimularInstalados: -1, nada (manda la plataforma).
var instaladosSimulados atomic.Int64

func init() { instaladosSimulados.Store(-1) }

// Instalados dice cuántos widgets de esta app hay puestos y si se ha podido saber. Lo
// usa el despertar de iOS: una app sin widget puesto no se registra para que el Hub la
// despierte, y se olvida al quitar el último (ADR 0039, enmienda del 2026-11-06).
// Puede tardar (WidgetKit contesta en otra cola): no se llama desde el hilo principal.
func Instalados() (int, bool) {
	if n := instaladosSimulados.Load(); n >= 0 {
		return int(n), true
	}
	return instaladosPlataforma()
}

// SimularInstalados es SOLO para tests: fija lo que contesta Instalados y devuelve
// cómo deshacerlo.
func SimularInstalados(n int) (restaurar func()) {
	antes := instaladosSimulados.Swap(int64(n))
	return func() { instaladosSimulados.Store(antes) }
}
