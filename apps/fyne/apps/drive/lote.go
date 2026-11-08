// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"sync"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Lo que el servicio no hace en lote
//
// `lib/lote.ts` de la web: deshacer un envío a la papelera de varios elementos y vaciar
// la papelera. El servicio restaura y purga POR ID, y cada llamada pasa por su propia
// autorización, así que lo que se niega se queda donde estaba y se cuenta aparte.

// EnParalelo es cuántas llamadas van a la vez (`EN_PARALELO` de la web): las justas para
// no esperar en fila ni saturar el servicio con cien peticiones de golpe.
const EnParalelo = 4

// EnTandas hace `hacer` con cada elemento, de `enParalelo` en `enParalelo`, y separa lo que
// salió de lo que no. Un fallo no corta el resto: lo que se pudo, se hizo; lo que no, se
// dice. Bloquea hasta que terminan todas.
func EnTandas[T any](items []T, hacer func(T) error, enParalelo int) (hechos, fallidos []T) {
	if enParalelo < 1 {
		enParalelo = 1
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	cupo := make(chan struct{}, enParalelo)
	for _, x := range items {
		cupo <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-cupo; wg.Done() }()
			err := hacer(x)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				fallidos = append(fallidos, x)
			} else {
				hechos = append(hechos, x)
			}
		}()
	}
	wg.Wait()
	return hechos, fallidos
}

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
	hechos, fallidos := EnTandas(els, func(el Elemento) error {
		ctx, cancelar := p.ctx()
		defer cancelar()
		return p.svc.Restore(ctx, claseDe(el), el.ID)
	}, EnParalelo)
	if len(fallidos) == 0 {
		p.avisar(ui.AvisoExito, p.tr.T("files.batch.restored", i18n.Vars{"count": len(hechos)}), "")
	} else {
		p.avisar(ui.AvisoPeligro, p.tr.T("files.batch.restorePartial", i18n.Vars{"count": len(fallidos)}), "")
	}
	p.recargarVista()
}
