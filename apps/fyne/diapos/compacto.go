// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

var _ editor.ComandosCompactos = (*Editor)(nil)

// ComandosCompactos usa los mismos mandos que la mini-barra, en la única fila
// desplazable del cromo. El selector sustituye al carril visible sólo en horizontal;
// cambiar, añadir, duplicar, borrar y reordenar mantienen los callbacks del editor.
func (e *Editor) ComandosCompactos() []fyne.CanvasObject {
	var elegir *editor.BotonCinta
	elegir = editor.NuevoBotonCinta(iconos.EdDiapoNueva,
		e.ctx.Traducir("slides.number", i18n.Vars{"n": e.activa + 1}), func() { e.abrirDiapos(elegir) })
	return append([]fyne.CanvasObject{elegir, editor.Divisor()}, e.controlesMiniBarra()...)
}

func (e *Editor) abrirDiapos(disparador fyne.CanvasObject) {
	if e.menuDiapos != nil {
		e.menuDiapos.Cerrar()
	}
	objetos := make([]fyne.CanvasObject, 0, len(e.m.Diapos)+6)
	for i := range e.m.Diapos {
		opcion := ui.NuevoElementoMenu(e.ctx.Traducir("slides.number", i18n.Vars{"n": i + 1}), iconos.Ninguno, func() { e.Elegir(i) })
		opcion.Marcada = i == e.activa
		objetos = append(objetos, opcion)
	}
	objetos = append(objetos, ui.NuevoSeparadorMenu())
	for _, accion := range []struct {
		clave  string
		inerte bool
		hacer  func()
	}{
		{"slides.new", e.ctx.SoloLectura, e.NuevaDiapositiva},
		{"slides.duplicate", e.ctx.SoloLectura, e.Duplicar},
		{"slides.delete", e.ctx.SoloLectura || len(e.m.Diapos) <= 1, e.Eliminar},
		{"common.moveUp", e.ctx.SoloLectura || e.activa == 0, func() { e.Mover(e.activa, e.activa-1) }},
		{"common.moveDown", e.ctx.SoloLectura || e.activa == len(e.m.Diapos)-1, func() { e.Mover(e.activa, e.activa+1) }},
	} {
		opcion := ui.NuevoElementoMenu(e.t(accion.clave), iconos.Ninguno, accion.hacer)
		opcion.Inerte = accion.inerte
		objetos = append(objetos, opcion)
	}
	e.menuDiapos = ui.NuevoMenu(objetos...)
	e.menuDiapos.Rotulo = e.t("group.slide")
	e.menuDiapos.Mostrar(disparador)
}
