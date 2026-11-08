// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"log/slog"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// avisarPapelera es el `avisarPapelera` de la web: el aviso de éxito de un envío a la
// papelera, uno o en lote, con «Deshacer», que los devuelve a su sitio. tirados es lo
// que el servicio dijo que tiró (no la lista de UID): con eso se compara lo que vuelve.
// Si no tiró nada, no hay nada que deshacer y el aviso va sin acción.
func (a *App) avisarPapelera(titulo, descripcion string, uids []string, tirados int) {
	av := ui.NuevoAviso(ui.AvisoExito, titulo, descripcion)
	if tirados > 0 {
		av.Duracion = ui.DuracionDeshacer
		av.RotuloAccion = a.Pantalla.t("common.undo", nil)
		av.AlPulsarAccion = func() {
			av.Cerrar()
			a.deshacerPapelera(uids, tirados)
		}
	}
	a.avisos.Notificar(av)
}

// deshacerPapelera es `deshacerPapelera`: UNA petición al lote de restaurar del
// servicio, que revive lo último que se tiró de cada UID en una sola operación, y el
// aviso dice si volvieron todos. Lo que no vuelve (ya purgado, un UID vivo que choca)
// se cuenta contra lo tirado, en singular si era uno.
func (a *App) deshacerPapelera(uids []string, tirados int) {
	if a.svc == nil || len(uids) == 0 {
		return
	}
	a.enFondo(func() {
		ctx, fin := a.ctx()
		restaurados, err := a.svc.BatchRestore(ctx, uids)
		fin()
		if err != nil {
			slog.Warn("contactos: no se pudo deshacer", "error", err)
		}
		fyne.Do(func() {
			switch {
			case err == nil && restaurados >= tirados:
				a.avisar(ui.AvisoExito, "trash.undone", "")
			case tirados == 1:
				a.avisar(ui.AvisoPeligro, "trash.undoFailedOne", "trash.undoFailedOneHint")
			default:
				a.avisar(ui.AvisoPeligro, "trash.undoFailed", "trash.undoFailedHint")
			}
			a.recargarTodo()
			a.enFondo(a.cargarEtiquetas)
		})
	})
}
