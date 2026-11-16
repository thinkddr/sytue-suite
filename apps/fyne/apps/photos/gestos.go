// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"

	"fyne.io/fyne/v2"
)

func (a *App) puedeRefrescarLista() bool {
	if a.refrescando || a.cli == nil || a.vida == nil || a.vida.Err() != nil || a.vis != nil || a.ed != nil {
		return false
	}
	switch a.ruta {
	case RutaFotos, RutaFavoritas:
		return !a.g.cargando && a.g.seleccion == nil && a.g.subida == nil && a.g.lote == nil
	case RutaAlbumes:
		return !a.alb.cargando && !a.alb.buscando && !a.alb.creando
	case RutaPapelera:
		return !a.pap.cargando
	case RutaRepetidas:
		return !a.rep.cargando && !a.rep.trabajando
	case RutaCompartidos:
		return !a.cmp.cargando
	default:
		return false
	}
}

func (a *App) refrescarLista() {
	if !a.puedeRefrescarLista() {
		return
	}
	if !a.cli.UsesPrivateData() {
		a.recargarLista()
		return
	}
	a.refrescando = true
	generation, lifetime := a.gen, a.vida
	a.enSesion(func(life context.Context) {
		operation, done := ctx(life)
		err := a.cli.RefreshPrivateData(operation)
		done()
		// The result updates only this presenter's retained lifecycle.
		fyne.Do(func() {
			if a.cerrada || lifetime.Err() != nil {
				return
			}
			a.refrescando = false
			if generation != a.gen {
				return
			}
			if err != nil {
				a.errorRefresco() //nolint:contextcheck // This UI result restores future gestures under the retained account life after the request ends.
				return
			}
			a.recargarLista() //nolint:contextcheck // A newly displayed list starts its own requests under the retained account life.
		})
	})
}

func (a *App) errorRefresco() {
	message := a.v.Traduce("error.connection")
	switch a.ruta {
	case RutaFotos, RutaFavoritas:
		a.g.error = message
	case RutaAlbumes:
		a.alb.error = message
	case RutaPapelera:
		a.pap.error = message
	case RutaRepetidas:
		a.rep.error = message
	case RutaCompartidos:
		a.cmp.error = message
	default:
		return
	}
	a.pintar()
}

func (a *App) recargarLista() {
	if !a.puedeRefrescarLista() {
		return
	}
	a.gen++
	switch a.ruta {
	case RutaFotos, RutaFavoritas:
		// Keep the applied search and its expansion while replacing page one.
		text, expanded := a.g.texto, a.g.tambien
		a.g = estadoGaleria{texto: text, tambien: expanded}
		a.cargarMas()
	case RutaAlbumes:
		a.alb.cargando = true
		a.pedirAlbumes()
	case RutaPapelera:
		a.pap.cargando = true
		a.pedirPapelera()
	case RutaRepetidas:
		a.rep.cargando = true
		a.pedirRepetidas()
	case RutaCompartidos:
		a.cmp.cargando = true
		a.pedirCompartidos()
	default:
		return
	}
	a.pintar()
}
