// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

// Only list routes participate: a studio, upload, chat or playing video keeps its
// own drag controls and in-progress work. Existing readers retain their contexts.
func (a *App) puedeRefrescarLista() bool {
	if a.cli == nil {
		return false
	}
	switch a.ruta {
	case RutaInicio:
		return a.dentro() && !a.ini.cargando && !a.ini.masCargando
	case RutaExplorar:
		return !a.exp.cargando
	case RutaCanal:
		return !a.can.cargando
	default:
		return false
	}
}

func (a *App) refrescarLista() {
	if !a.puedeRefrescarLista() {
		return
	}
	switch a.ruta {
	case RutaInicio:
		a.pedirInicio()
	case RutaExplorar:
		a.pedirExplorar(false)
	case RutaCanal:
		a.pedirCanal(false)
	default:
		return
	}
}
