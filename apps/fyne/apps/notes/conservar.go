// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"reflect"
	"slices"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # El menú y la barra se conservan entre repintados
//
// Repintar volvía a hacer el carril entero —sus destinos, el árbol de cuadernos con su
// «…» y su contador— y la barra del cuaderno, aunque solo hubiera cambiado una nota.
// Ahora cada uno se rehace solo si cambia lo que lo decide (su clave), como el menú y la
// cabecera de Calendar; con la misma, se queda el de antes, con su foco, el ratón encima y
// sus medidas. Lo que sus filas y botones hacen al pulsarse lo leen al pulsarse, así que
// no cuenta.

// claveLateral es todo lo que decide el carril (Lateral.Objeto). Los cuadernos van
// enteros porque sus filas se los pasan tal cual a quien abre uno o sus ajustes.
type claveLateral struct {
	fija      claveLateralFija
	cuadernos []Cuaderno
	arbol     []string
	cerrados  []string
	notas     []filaNotaClave
}

type claveLateralFija struct {
	v                               tema.Variante
	tr                              *i18n.Translator
	hoy, activo, notaActiva         string
	vista                           Vista
	fase                            Fase
	bloqueado, vinculados, conNotas bool
	nNotas                          int
}

// filaNotaClave es lo que pinta la fila de una nota en el árbol (filaNota).
type filaNotaClave struct {
	id, rotulo, icono, tipo string
	terminada, fijada       bool
}

func (l *Lateral) clave(v tema.Variante) claveLateral {
	k := claveLateral{
		fija: claveLateralFija{
			v: v, tr: l.tr, hoy: time.Now().Format(time.DateOnly), activo: l.Activo, notaActiva: l.NotaActiva,
			vista: l.Vista, fase: l.Fase, bloqueado: l.Bloqueado, vinculados: l.AlServiciosVinculados != nil,
			nNotas: len(l.Notas),
		},
		cuadernos: slices.Clone(l.Cuadernos),
	}
	for _, c := range cuadernosDelArbol(l.Cuadernos, l.EsMio) {
		k.arbol = append(k.arbol, c.ID)
		if c.ID == l.Activo && c.Vista == VistaArbol {
			k.fija.conNotas = true
		}
	}
	for id, cerrado := range l.Cerrados {
		if cerrado {
			k.cerrados = append(k.cerrados, id)
		}
	}
	slices.Sort(k.cerrados)
	if k.fija.conNotas {
		for _, n := range l.Notas {
			k.notas = append(k.notas, filaNotaClave{
				id: n.ID, rotulo: n.Rotulo(l.tr), icono: n.Icono, tipo: n.Tipo, terminada: n.Terminada(), fijada: n.Fijada,
			})
		}
	}
	return k
}

func (k claveLateral) igual(o claveLateral) bool {
	return k.fija == o.fija && slices.Equal(k.arbol, o.arbol) && slices.Equal(k.cerrados, o.cerrados) &&
		slices.Equal(k.notas, o.notas) && reflect.DeepEqual(k.cuadernos, o.cuadernos)
}

// lateralConservado es el carril del último repintado si su clave no ha cambiado.
func (p *Pantalla) lateralConservado(v tema.Variante) fyne.CanvasObject {
	k := p.lateral.clave(v)
	if p.lateralHecho == nil || !k.igual(p.lateralClave) {
		p.lateralHecho, p.lateralClave = p.lateral.Objeto(v), k
		p.filasLateral = p.lateral.filasCuaderno
	}
	// Las filas que el arrastre ilumina son las del carril que está en pantalla.
	p.lateral.filasCuaderno = p.filasLateral
	return p.lateralHecho
}

// claveBarra es lo que pinta la barra del cuaderno (Barra.Objeto): sus menús se arman al
// abrirse, con lo que haya entonces.
type claveBarra struct {
	v                                     tema.Variante
	tr                                    *i18n.Translator
	emoji, titulo, dia                    string
	cifrado, bloqueado, sinRed, conFiltro bool
	estrecha                              bool
}

// barraConservada es la barra del último repintado si su clave no ha cambiado.
func (p *Pantalla) barraConservada(v tema.Variante) fyne.CanvasObject {
	b := p.barra
	k := claveBarra{
		v: v, tr: b.tr, emoji: b.Cuaderno.Emoji, titulo: b.Cuaderno.Titulo, dia: b.Cuaderno.Crudo.DailyDate,
		cifrado: b.Cuaderno.Cifrado, bloqueado: b.Bloqueado, sinRed: b.SinRed,
		conFiltro: b.Filtrando || b.Filtro != "", estrecha: b.Estrecha,
	}
	if p.barraHecha == nil || k != p.barraClave {
		p.barraHecha, p.barraClave = b.Objeto(v), k
	}
	return p.barraHecha
}
