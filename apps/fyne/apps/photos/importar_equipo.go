// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import "git.sytue.eu/sytue/one/apps/fyne/ui"

// # Importar en Sytue Office
//
// En la edición local no hay copia de seguridad a Drive: lo que la conectada sube a
// «/Fotos», aquí se IMPORTA a la fototeca del equipo —del carrete en el móvil, de una
// carpeta o de las carpetas vigiladas en el escritorio— con el mismo motor de copia.go
// (cada foto una vez, por su huella), que monta `cmd-local/photos`. Esto es lo poco que
// ese `main` necesita de la pantalla: avisar y que lo importado aparezca.

// Avisar enseña un aviso de la casa. Va en el hilo de la interfaz.
func (a *App) Avisar(peligro bool, titulo, detalle string) {
	tono := ui.AvisoExito
	if peligro {
		tono = ui.AvisoPeligro
	}
	a.avisar(tono, titulo, detalle)
}

// RecargarGaleria vuelve a pedir la línea de tiempo para que lo importado caiga en su
// fecha, solo si no se está haciendo otra cosa con la galería (como al acabar una ronda
// de la copia: no se le quita a nadie lo que mira). Va en el hilo de la interfaz.
func (a *App) RecargarGaleria() {
	if a.ruta == RutaFotos && a.g.subida == nil && a.g.seleccion == nil && a.vis == nil && a.ed == nil && a.g.texto == "" {
		a.gen++
		a.g = estadoGaleria{}
		a.cargarMas()
	}
}
