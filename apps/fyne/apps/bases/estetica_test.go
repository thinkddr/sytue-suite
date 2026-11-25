// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestMisBasesVaciaLlevaSuDibujo (BASE-05): «Todavía no tienes ninguna base» lleva el
// dibujo de base de datos del `EmptyState` de la web; era solo texto.
func TestMisBasesVaciaLlevaSuDibujo(t *testing.T) {
	p, _, w := montar(t, map[string]string{"GET /bases": "[]"})
	var vacio *ui.Vacio
	recorrer(w.Canvas().Content(), func(o fyne.CanvasObject) {
		if v, ok := o.(*ui.Vacio); ok && v.Titulo == p.t("bases.empty.title") {
			vacio = v
		}
	})
	if vacio == nil {
		t.Fatal("sin bases no se ve el estado vacío")
	}
	if vacio.Glifo == nil && vacio.Ilustracion == nil {
		t.Error("el estado vacío de «Mis bases» no lleva dibujo")
	}
}

// TestLosDesplegablesDeLaBarraVanConLaMismaLetra: el selector de vista y «Ordenar por»
// están uno encima del otro; el de vista iba en `text-xs` (`CampoSM`) y el de orden en la
// talla de los campos.
func TestLosDesplegablesDeLaBarraVanConLaMismaLetra(t *testing.T) {
	p, w := abrir(t, nuevoFalso(3))
	var vista, orden *ui.Selector
	recorrer(w.Canvas().Content(), func(o fyne.CanvasObject) {
		if s, ok := o.(*ui.Selector); ok {
			switch s.Rotulo {
			case p.t("view.list"):
				vista = s
			case p.t("grid.sortBy"):
				orden = s
			}
		}
	})
	if vista == nil || orden == nil {
		t.Fatalf("no se ven los dos desplegables (vista %v, orden %v)", vista != nil, orden != nil)
	}
	if vista.Talla != orden.Talla {
		t.Errorf("el selector de vista va en la talla %v y el de orden en la %v", vista.Talla, orden.Talla)
	}
}

// TestElMenuMasDeLaBaseSeVe: el «⋯» de la base lleva el glifo entero de la rejilla y la
// tinta del texto; a 16 en la del enlace eran tres motas.
func TestElMenuMasDeLaBaseSeVe(t *testing.T) {
	p, w := abrir(t, nuevoFalso(3))
	mas := menuMasDe(w, p.abierta.Datos.Name)
	if mas == nil {
		t.Fatal("no se ve el «⋯» de la base")
	}
	if mas.TamIcono < iconos.TallaXL || mas.Trazo != "text-muted" {
		t.Errorf("el «⋯» lleva el glifo de %v en %q; quiero %v en text-muted", mas.TamIcono, mas.Trazo, iconos.TallaXL)
	}
}
