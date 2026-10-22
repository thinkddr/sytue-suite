// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Las medidas de FieldGroup (Form.tsx):
//
//	grupo     `flex flex-col gap-1.5`                                hoja
//	rótulo    `text-sm font-semibold text-text`                      hoja
//	asterisco `text-danger-text ml-0.5`, en la letra del rótulo       hoja
//	ayuda     `text-xs text-text-muted`                              hoja
//	error     `flex items-center gap-1 text-xs text-danger-text`,    hoja
//	          con el `<Icon name="danger" size={14}>` delante
var (
	// HuecoGrupoCampo es el `gap-1.5` entre el rótulo, el control y la ayuda.
	HuecoGrupoCampo = tema.Espacio(1.5)
	// MargenAsterisco es el `ml-0.5` del asterisco de «obligatorio».
	MargenAsterisco = tema.Espacio(0.5)
	// HuecoLineaError es el `gap-1` entre el icono y el texto de un error.
	HuecoLineaError = tema.Espacio(1)
)

// LadoIconoError es el `size={14}` del icono de un error de campo.
const LadoIconoError float32 = 14

// GrupoCampo es FieldGroup de Form.tsx: el rótulo del campo —con el asterisco rojo si
// es obligatorio—, el control debajo y, si los hay, la ayuda o el error. El error gana
// a la ayuda, como en la web (`help && !error`).
//
// El «(obligatorio)» que la web añade en `sr-only` no se escribe: es para el lector de
// pantalla, y aquí lo lleva el nombre accesible del control.
func GrupoCampo(rotulo string, obligatorio bool, control fyne.CanvasObject, ayuda, err string) *fyne.Container {
	cabeza := fyne.CanvasObject(NuevoRotulo(rotulo, tema.TextoSM, SemiNegrita, "text"))
	if obligatorio {
		cabeza = container.New(Fila{Hueco: MargenAsterisco}, cabeza,
			NuevoRotulo("*", tema.TextoSM, SemiNegrita, "danger-text"))
	}
	hijos := []fyne.CanvasObject{cabeza, control}
	switch {
	case err != "":
		hijos = append(hijos, LineaError(err))
	case ayuda != "":
		hijos = append(hijos, NuevoRotulo(ayuda, tema.TextoXS, Normal, "text-muted"))
	}
	return container.New(Columna{Hueco: HuecoGrupoCampo}, hijos...)
}

// LineaError es el `<p role="alert">` de un campo con error: el icono de peligro de 14
// y el texto `text-xs` en `danger-text`, centrados en la línea.
func LineaError(texto string) *fyne.Container {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	icono := iconos.Trazo(iconos.Peligro, LadoIconoError, "danger-text", v).Objeto()
	return container.New(filaCentrada{Hueco: HuecoLineaError}, icono,
		NuevoRotulo(texto, tema.TextoXS, Normal, "danger-text"))
}

// filaCentrada es `flex items-center gap-*`: cada hijo a su tamaño, en fila y centrado
// en vertical en la más alta.
type filaCentrada struct{ Hueco float32 }

var _ fyne.Layout = filaCentrada{}

func (f filaCentrada) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return Fila(f).MinSize(objs)
}

func (f filaCentrada) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	var x float32
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		m := o.MinSize()
		o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		o.Resize(m)
		x += m.Width + f.Hueco
	}
}
