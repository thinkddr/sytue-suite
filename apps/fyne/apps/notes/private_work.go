// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"context"
	"errors"
	"sync"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// NuevaPantallaPrivada receives a controller registered during Build/Prepare,
// before the host allows a user to create local work. Scope never reads mutable
// Suite callbacks. The controller borrows the same encrypted journal as records.
func NuevaPantallaPrivada(parent context.Context, work *notas.LocalWork, tr *i18n.Translator, l i18n.Locale, data api.NotesData, red *offline.Monitor, win fyne.Window) (*Pantalla, error) {
	if work == nil || data == nil || data.PrivateStorageScope() != work.PrivateStorageScope() || !work.PrivateStorageScope().Valid() || work.PrivateStorageScope().Product != "notes" {
		return nil, errors.New("notes: registered private local work is required")
	}
	svc, err := api.NewNotes("", nil, nil).WithPrivateData(data)
	if err != nil {
		return nil, err
	}
	p := nuevaPantalla(parent, work, tr, l, svc, red, win)
	p.Almacen = work.Journal()
	work.OnResume(func() { //nolint:contextcheck // Resume installs UI work on the newly created phase, after the old phase joined.
		p.enHilo(func() {
			if v := p.vida(); v.unsubscribe != nil {
				v.unsubscribe()
				v.unsubscribe = nil
			}
			p.cancelarMigracion()
			p.creando, p.guardandoCaptura, p.abriendoDia = false, false, false
			p.vigilando = sync.Once{}
			p.Arrancar()
			if p.edicion != nil && p.edicion.esperaDurable {
				p.guardarEdicion(p.edicion)
			}
		})
	})
	return p, nil
}

func (p *Pantalla) escribirLocal(ctx context.Context, namespace, id string, value any) error {
	if work := p.vida().work; work != nil {
		return work.Write(ctx, namespace, id, value)
	}
	if value == nil {
		return p.Almacen.Delete(p.claveAlmacen(), namespace, id)
	}
	return p.Almacen.Write(p.claveAlmacen(), namespace, id, value)
}
