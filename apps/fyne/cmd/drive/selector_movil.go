// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android || ios

package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	"git.sytue.eu/sytue/one/apps/fyne/apps/drive"
)

// montarSelector pone el selector del MÓVIL: el del sistema a través de Fyne, que en
// Android devuelve `content://` y no rutas. No se copia a ningún sitio: cada fichero se
// sube en FLUJO desde el proveedor de contenido (`URIReadCloser`), que la subida cierra
// al terminar, y vale igual para «Mi unidad», una unidad de equipo o una carpeta
// compartida. Su nombre de verdad lo da el proveedor (`URI.Name()`) y su tamaño
// `OpenableColumns.SIZE` (`Size()`, del fork desde `v2.8.1-sytue.6`): con él la barra va
// en porcentaje; sin él (-1, o en iOS, que elige uno solo) la subida va troceada.
func montarSelector(w fyne.Window, p *drive.Pantalla) {
	p.ElegirFicheros = func(listo func([]drive.Entrada)) {
		dialog.ShowFileOpenMultiple(func(rs []fyne.URIReadCloser, err error) {
			if err != nil || len(rs) == 0 {
				return
			}
			entradas := make([]drive.Entrada, 0, len(rs))
			for _, r := range rs {
				entradas = append(entradas, drive.Fluido(r.URI().Name(), tamano(r.URI()), r))
			}
			listo(entradas)
		}, w)
	}
}

// tamano es lo que dice medir el proveedor, o -1 si no lo dice.
func tamano(u fyne.URI) int64 {
	if s, ok := u.(interface{ Size() int64 }); ok {
		return s.Size()
	}
	return -1
}
