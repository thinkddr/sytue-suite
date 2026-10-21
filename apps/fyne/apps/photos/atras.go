// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

// atras conserva la misma salida que los mandos visibles. Las capas se descartan
// primero en ui; estas ramas también sirven si aún no se ha montado su capa.
func (a *App) atras() bool {
	switch {
	case a.ed != nil:
		a.salirDelEditor() // recortando, cancela el recorte antes de salir
	case a.vis != nil:
		a.cerrarVisor()
	case a.pub.capa != nil:
		a.cerrarPublica()
	case a.g.seleccion != nil && (a.ruta == RutaFotos || a.ruta == RutaFavoritas):
		a.salirDeSeleccion()
	case a.ruta == RutaAlbum:
		a.Ir(RutaAlbumes)
	case a.ruta == RutaCompartido:
		a.Ir(RutaCompartidos)
	case a.ruta != RutaFotos && a.ruta != RutaPublica:
		a.Ir(RutaFotos)
	default:
		return false
	}
	return true
}
