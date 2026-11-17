// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Lo que el servicio no hace en lote
//
// `lib/lote.ts` de la web: deshacer un envío a la papelera de varios elementos y vaciar
// la papelera, con `ui.EnTandas` (el servicio restaura y purga POR ID).

// conID es lo de una selección que la papelera sabe devolver: lo que tiene id.
func conID(sel []Elemento) []Elemento {
	out := make([]Elemento, 0, len(sel))
	for _, el := range sel {
		if el.ID != "" {
			out = append(out, el)
		}
	}
	return out
}

// avisoLoteEnPapelera es el aviso de «N elementos enviados a la papelera», con el mismo
// «Deshacer» de 10 s que el de un solo elemento: confirmar antes no quita que se haya
// marcado uno de más. Va en la rutina de la interfaz.
func (p *Pantalla) avisoLoteEnPapelera(total int, devolver []Elemento) *ui.Aviso {
	a := ui.NuevoAviso(ui.AvisoExito, p.tr.T("files.batch.trashDone", i18n.Vars{"count": total}), "")
	a.Duracion = ui.DuracionDeshacer
	if len(devolver) > 0 {
		a.RotuloAccion = p.tr.T("files.delete.undo", nil)
		a.AlPulsarAccion = func() { go p.deshacerLote(devolver) }
	}
	a.RotuloCerrar = p.tr.T("ui.close", nil)
	return a
}

// deshacerLote devuelve a su sitio lo que acaba de ir a la papelera en lote. Corre en su
// propia rutina.
func (p *Pantalla) deshacerLote(els []Elemento) {
	hechos, fallidos := ui.EnTandas(els, func(el Elemento) error {
		ctx, cancelar := p.ctx()
		defer cancelar()
		return p.svc.Restore(ctx, claseDe(el), el.ID)
	}, ui.EnParalelo)
	if len(fallidos) == 0 {
		p.avisar(ui.AvisoExito, p.tr.T("files.batch.restored", i18n.Vars{"count": len(hechos)}), "")
	} else {
		p.avisar(ui.AvisoPeligro, p.tr.T("files.batch.restorePartial", i18n.Vars{"count": len(fallidos)}), "")
	}
	p.recargarVista()
}
