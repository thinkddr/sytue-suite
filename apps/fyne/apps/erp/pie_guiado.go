// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// pieGuiado es el footer de Wizard: flex-wrap con Cancelar separado del grupo
// max-w-full de Atrás/Continuar. El grupo también se reparte y alinea cada fila
// a la derecha. Lo comparten el alta del perfil y el proceso de una obligación.
func pieGuiado(v tema.Variante, cancelar *ui.Boton, der []pieza, ancho float32) (fyne.CanvasObject, float32) {
	a, altos := medidasDe(der)
	wd := float32(0)
	for i, w := range a {
		wd += w
		if i > 0 {
			wd += HuecoDos
		}
	}
	wgrupo := min(wd, ancho)
	pos, tam := Envolver(a, altos, HuecoDos, HuecoDos, wgrupo)
	// justify-end alinea cada fila del grupo, también cuando sus dos botones
	// se reparten a 320 px. El grupo conserva max-w-full antes de flex-wrap.
	for inicio := 0; inicio < len(pos); {
		fin := inicio + 1
		for fin < len(pos) && pos[fin].Y == pos[inicio].Y {
			fin++
		}
		desplazar := wgrupo - pos[fin-1].X - a[fin-1]
		for j := inicio; j < fin; j++ {
			pos[j].X += desplazar
		}
		inicio = fin
	}
	l := &lienzo{}
	l.poner(fileteHoja(v), 0, 0, ancho, tema.Borde)
	y := tema.Borde + tema.Espacio(4)
	h := max(cancelar.MinSize().Height, tam.Height)
	if cancelar.MinSize().Width+HuecoDos+wgrupo > ancho {
		l.poner(cancelar, 0, y, cancelar.MinSize().Width, cancelar.MinSize().Height)
		y += cancelar.MinSize().Height + HuecoDos
		l.piezas(der, pos, ancho-wgrupo, y)
		return l.objeto(y + tam.Height)
	}
	l.poner(cancelar, 0, y+(h-cancelar.MinSize().Height)/2, cancelar.MinSize().Width, cancelar.MinSize().Height)
	l.piezas(der, pos, ancho-wgrupo, y+(h-tam.Height)/2)
	return l.objeto(y + h)
}
