// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import "git.sytue.eu/sytue/one/apps/fyne/tema"

// AnchoHoja es el ancho útil del armazón, sin el menú ni el aire del marco.
// menu y carril son sus anchos naturales; cero indica que esa navegación no existe.
// En ventana estrecha (< 768) el menú pasa a cajón superpuesto y la hoja ocupa todo el
// ancho disponible. Las apps que deben medir sus párrafos antes de Layout usan la misma
// cuenta que el propio armazón.
func AnchoHoja(ventana, menu, carril float32) float32 {
	estrecha := ventana < Estrecho
	separa := float32(0)
	if tema.EsMarco() && !estrecha {
		separa = tema.Forma("sheet-gap")
	}
	ocupado := separa
	switch {
	case carril > 0 && !estrecha:
		ocupado = carril
	case menu > 0 && !estrecha:
		ocupado = menu
	}
	return max(ventana-ocupado-separa, 0)
}

// AcotarAnchoPanel reserva el mínimo del detalle sin perder la preferencia de ancho
// de la lista. El armazón y las mediciones previas de sus apps comparten el límite.
func AcotarAnchoPanel(ancho, hoja float32) float32 {
	return max(AnchoPanelMinimo, min(ancho, hoja-AnchoContenidoMinimo))
}
