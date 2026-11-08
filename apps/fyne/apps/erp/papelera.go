// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # «Deshacer» en «Movido a la papelera» (`lib/papelera.ts`)
//
// El borrado del servicio es SUAVE; el aviso lo devuelve en un clic, del registro de la
// ficha o del lote entero de la lista, con el `restaurar` del lote.

// avisarPapelera es `avisarPapelera`: el aviso de éxito con «Deshacer» durante
// ui.DuracionDeshacer. Al deshacer, lo dice —o dice por qué no— y llama a `alDeshacer`
// para repintar.
func (p *Pantalla) avisarPapelera(m Modelo, ids []string, alDeshacer func()) {
	av := ui.NuevoAviso(ui.AvisoExito, p.t("toast.deleted"), "")
	av.Duracion = ui.DuracionDeshacer
	av.RotuloAccion = p.t("action.undo")
	av.AlPulsarAccion = func() {
		av.Cerrar()
		p.sacarDePapelera(m, ids, alDeshacer)
	}
	p.avisos.Notificar(av)
}

// sacarDePapelera es `sacarDePapelera` con su aviso: el lote de `restaurar`, uno o muchos.
func (p *Pantalla) sacarDePapelera(m Modelo, ids []string, alDeshacer func()) {
	if p.svc == nil || len(ids) == 0 {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		_, err := p.svc.Lote(ctx, m.Clave, "restaurar", ids, "")
		cancelar()
		enHilo(func() {
			if err != nil {
				p.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, p.t("toast.undoFailed"), MensajeDeError(err, p.tr)))
			} else {
				p.avisar(ui.AvisoExito, p.t("toast.undone"))
			}
			alDeshacer()
		})
	})
}
