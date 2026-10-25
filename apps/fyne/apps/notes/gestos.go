// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import "git.sytue.eu/sytue/one/apps/fyne/ui"

func (p *Pantalla) puedeRefrescarNotas() bool {
	if p.svc == nil || !p.hayRed() || p.vista != VistaCuaderno || p.activo.ID == "" || esProvisional(p.activo.ID) ||
		p.bloqueado || p.migrando || p.creando || p.guardandoCaptura || p.edicion != nil || p.vaciando.Load() || len(p.cola) != 0 {
		return false
	}
	return p.activo.Vista != Libre && p.lienzo.Seleccionada == "" && p.lienzo.Fase != Cargando &&
		(p.campoCaptura == nil || p.campoCaptura.Texto() == "") &&
		(p.activo.Crudo.DailyDate == "" || p.vistaDiario() == VistaDia)
}

func (p *Pantalla) refrescoNotas() ui.RefrescoLista {
	return ui.RefrescoLista{
		Puede:    p.puedeRefrescarNotas,
		Revision: func() uint64 { return p.cryptoEpoch },
		Actualizar: func() {
			if !p.puedeRefrescarNotas() {
				return
			}
			c := p.activo
			p.lienzo.Fase = Cargando
			p.Repintar()
			go p.cargarNotas(c)
		},
	}
}
