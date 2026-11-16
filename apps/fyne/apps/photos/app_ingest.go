// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"context"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Activation resumes only work already owned by this private device. Reading a
// timeline, switching screens or requesting a thumbnail never starts ingestion.
func (a *App) reanudarImportaciones() {
	if a.cli == nil || !a.cli.CanImportPrivatePhotos() || a.reanudandoImportaciones {
		return
	}
	a.reanudandoImportaciones = true
	a.enSesion(func(vida context.Context) {
		ctx, cancel := context.WithTimeout(vida, 30*time.Minute)
		defer cancel()
		completed, err := a.cli.ReanudarImportaciones(ctx)
		fyne.Do(func() {
			if a.cerrada || vida.Err() != nil || vida != a.vida {
				return
			}
			a.reanudandoImportaciones = false
			if err != nil {
				a.g.error = a.textoDeFallo(err)
			}
			if len(completed) > 0 && (a.ruta == RutaFotos || a.ruta == RutaFavoritas) {
				aviso := a.v.T.T("gallery.uploaded", i18n.Vars{"count": len(completed)})
				a.gen++
				a.g = estadoGaleria{estadoCarga: estadoCarga{aviso: aviso, error: a.g.error}}
				a.cerrarVisor()
				a.cargarMas()
			}
			a.pintar()
		})
	})
}
