// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

func (a *App) puedeRefrescarLista() bool {
	if a.cli == nil || a.vida == nil || a.vida.Err() != nil || a.vis != nil || a.ed != nil {
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
