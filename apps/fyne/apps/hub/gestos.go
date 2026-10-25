// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Desplazable keeps Hub's actual scroll and list lifecycle together. The account,
// onboarding and data-export forms deliberately keep their own input behaviour.
func (v *Vista) Desplazable() fyne.CanvasObject {
	scroll := container.NewVScroll(v.Objeto())
	return ui.RefrescarAlTirar(scroll, &ui.RefrescoLista{
		Arriba: func() bool { return scroll.Offset.Y <= 0.5 },
		Puede: func() bool {
			v.mu.Lock()
			defer v.mu.Unlock()
			return v.puedeRefrescarLista()
		},
		Revision: func() uint64 {
			v.mu.Lock()
			defer v.mu.Unlock()
			return v.revisionRefresco
		},
		Actualizar: v.refrescarLista,
	})
}

func (v *Vista) puedeRefrescarLista() bool {
	switch v.destino {
	case AInicio:
		return v.hub != nil && v.lanzador.Reciente.Estado != Cargando && v.lanzador.Agenda.Estado != Cargando
	case AActividad:
		return v.hub != nil && v.actividad.Estado != Cargando
	case AAvisos:
		return v.buzon != nil && v.avisos.Estado != Cargando && !v.cargandoMas
	case ADescargas:
		return v.instaladores != nil && v.bajadas.Estado != Cargando
	default:
		return false
	}
}

func (v *Vista) refrescarLista() {
	v.mu.Lock()
	if !v.puedeRefrescarLista() {
		v.mu.Unlock()
		return
	}
	destino, origen := v.destino, v.actividad.Origen
	switch destino {
	case AInicio:
		v.lanzador.Reciente.Estado = Cargando
		if v.agenda != nil {
			v.lanzador.Agenda.Estado = Cargando
		}
	case AActividad:
		v.actividad.Estado = Cargando
	case AAvisos:
		v.avisos.Estado = Cargando
	case ADescargas:
		v.bajadas.Estado = Cargando
	default:
		v.mu.Unlock()
		return
	}
	v.mu.Unlock()
	switch destino {
	case AInicio:
		go v.cargarReciente()
		go v.cargarAgenda()
	case AActividad:
		go v.cargarActividad(origen)
	case AAvisos:
		v.avisos.AlReintentar()
	case ADescargas:
		go v.cargarDescargas()
	default:
		return
	}
}
