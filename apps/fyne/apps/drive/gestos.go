// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// The section owns its actual scroller and its existing retry action, including
// the current folder in shared/team/external storage and the applied document query.
type listaDrive struct {
	fase       *Fase
	scroll     *container.Scroll
	arriba     func() bool
	ocupado    func() bool
	actualizar func()
}

func (p *Pantalla) listaParaRefresco() listaDrive {
	switch p.seccion {
	case MiUnidad:
		e := p.explor
		return listaDrive{
			fase: &e.Fase, arriba: e.Refresco.Arriba, actualizar: p.reintentar,
			ocupado: func() bool { return e.Ocupado || e.CargandoMas || e.AlgoMarcado() },
		}
	case Papelera:
		x := p.papelera
		return listaDrive{
			fase: &x.Fase, scroll: x.desplazable, actualizar: x.AlReintentar,
			ocupado: func() bool { return x.Ocupado },
		}
	case Recientes, Favoritos:
		x := p.sec.recientes
		if p.seccion == Favoritos {
			x = p.sec.destacados
		}
		return listaDrive{
			fase: &x.Fase, scroll: x.desplazable, actualizar: x.AlReintentar,
			ocupado: func() bool { return x.CargandoMas },
		}
	case Documentos:
		x := p.sec.documentos
		return listaDrive{fase: &x.Fase, scroll: x.desplazable, actualizar: x.AlReintentar}
	case CompartidoConmigo:
		x := p.sec.compartido
		l := listaDrive{
			fase: &x.Fase, scroll: x.desplazable, actualizar: x.AlReintentar,
			ocupado: func() bool { return x.Subiendo },
		}
		if x.Raiz != nil {
			l.fase, l.actualizar = &x.FaseCarpeta, x.AlReintentarCarpeta
		}
		return l
	case UnidadesDeEquipo:
		x := p.sec.equipos
		l := listaDrive{
			fase: &x.Fase, scroll: x.desplazable, actualizar: x.AlReintentar,
			ocupado: func() bool { return x.Ocupado || x.Subiendo || x.CargandoMas },
		}
		if x.Abierta != nil {
			l.fase, l.scroll, l.actualizar = &x.FaseDetalle, x.desplazableDetalle, x.AlReintentarDetalle
		}
		return l
	case Nube:
		x := p.sec.nube
		return listaDrive{
			fase: &x.Fase, scroll: x.desplazable, actualizar: x.AlReintentar,
			ocupado: func() bool { return x.Ocupado || x.Subiendo || x.CargandoMas },
		}
	}
	return listaDrive{}
}

func (p *Pantalla) refrescoSeccion(o fyne.CanvasObject) fyne.CanvasObject {
	if p.seccion == MiUnidad && p.explor.Fase == Exito {
		return o // Its virtual list, grid and folder strip each know their own offset.
	}
	l := p.listaParaRefresco()
	if l.fase == nil || l.actualizar == nil {
		return o
	}
	puede := func() bool {
		return p.svc != nil && p.dentro && p.hayRed() && !p.ocupado.Load() &&
			*l.fase != Cargando && *l.fase != SinSesion && *l.fase != Prohibida && (l.ocupado == nil || !l.ocupado())
	}
	return ui.RefrescarAlTirar(o, &ui.RefrescoLista{
		Arriba: func() bool {
			if *l.fase != Exito {
				return true // Empty/error panels have no live scroll.
			}
			if l.arriba != nil {
				return l.arriba()
			}
			return l.scroll == nil || l.scroll.Offset.Y <= 0.5
		},
		Puede:    puede,
		Revision: p.epoca.Load,
		Actualizar: func() {
			if puede() {
				*l.fase = Cargando
				p.Repintar()
				l.actualizar()
			}
		},
	})
}
