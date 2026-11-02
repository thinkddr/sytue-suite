// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

// Referencia de tipos y tonos: apps/frontend/apps/bases/src/lib/aspecto.ts.
// Es un helper, no una pantalla ni una pieza del inventario de paridad.

import (
	"slices"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Iconos y tonos de Bases: los mismos tipos, opciones y recursos que en la web,
// usando el catálogo y los colores del tema compartido.

// iconosDeTipo es `ICONO_DE_TIPO`: el glifo del catálogo común que encabeza cada columna.
var iconosDeTipo = map[string]iconos.Icono{
	api.BasesKindText:        iconos.Texto,
	api.BasesKindNumber:      iconos.Grafico,
	api.BasesKindDate:        iconos.Calendario,
	api.BasesKindCheckbox:    iconos.Tic,
	api.BasesKindSelect:      iconos.Etiqueta,
	api.BasesKindMultiselect: iconos.Lista,
	api.BasesKindLink:        iconos.Enlace,
	api.BasesKindFormula:     iconos.Codigo,
	api.BasesKindAttachment:  iconos.Copias,
}

// iconoDeTipo es el glifo de un tipo de campo; uno que no se conoce lleva el de texto.
func iconoDeTipo(kind string) iconos.Icono {
	if i, ok := iconosDeTipo[kind]; ok {
		return i
	}
	return iconos.Texto
}

// tonosDeOpcion es `TONOS_DE_OPCION`: el color de cada opción, por su posición en la
// lista de opciones del campo. Va por turno y no por el texto para que dos opciones
// seguidas nunca compartan color.
var tonosDeOpcion = [...]ui.Tono{
	ui.InsigniaPrimaria, ui.InsigniaExito, ui.InsigniaAviso,
	ui.InsigniaAcento, ui.InsigniaInfo, ui.InsigniaPeligro,
}

// tonoDeOpcion es el color del chip de un valor de elección. Un valor que ya no es una
// opción (se borró) va en neutro: se sigue viendo, pero no finge ser una opción viva.
func tonoDeOpcion(campo api.BasesField, valor string) ui.Tono {
	i := slices.Index(campo.Choices, valor)
	if i < 0 {
		return ui.InsigniaNeutra
	}
	return tonosDeOpcion[i%len(tonosDeOpcion)]
}

// tonoDeBase mantiene el color de una base al cambiar el orden de la lista.
// El desbordamiento uint32 es el mismo que `Math.imul(..., 31) >>> 0` en la web.
func tonoDeBase(id string) ui.TonoTarjeta {
	var hash uint32
	for _, c := range []byte(id) { // los identificadores son UUID, en ASCII
		hash = hash*31 + uint32(c)
	}
	tonos := ui.TonosTarjeta()
	return tonos[hash%uint32(len(tonos))]
}

// alineaALaDerecha dice si la columna es numérica (`text-right tabular-nums`): un número
// o una fórmula que da un número.
func alineaALaDerecha(campo api.BasesField) bool {
	return efectivo(campo).Kind == api.BasesKindNumber
}
