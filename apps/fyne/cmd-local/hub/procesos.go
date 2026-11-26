// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"runtime"
	"slices"

	"git.sytue.eu/sytue/one/apps/fyne/local"
)

// # Qué apps de la edición están abiertas
//
// Lo preguntan dos cosas del Hub: los recordatorios con las apps cerradas (HU-06: si
// Calendar está abierta, avisa ella) y «Traer desde Sytue» (HU-07: los almacenes del
// motor no admiten dos escritores, así que Calendar, Notepads… tienen que estar
// cerradas mientras se traen sus datos). Las apps de la edición no tienen un enchufe de
// instancia que se pueda tocar, así que se mira la lista de procesos de la persona por
// el nombre del ejecutable (`sytue-office-<app>`), sin hablar con nadie (`local.Abiertas`).

// abiertas son las apps del catálogo (por su carpeta) que están corriendo ahora, en el
// orden del catálogo. sabido es falso si el sistema no deja saberlo. Lo mira la cáscara
// (`local.Abiertas`), que lo usa también Ajustes → Datos para mover la carpeta de datos.
func abiertas(lista []string, goos string) (abiertas []string, sabido bool) {
	var cs []string
	for _, a := range catalogo {
		if slices.Contains(lista, a.Carpeta) {
			cs = append(cs, a.Carpeta)
		}
	}
	return local.Abiertas(cs, goos)
}

// carpetasDe son las carpetas del catálogo que abren esos productos.
func carpetasDe(productos []string) []string {
	var out []string
	for _, a := range catalogo {
		if slices.Contains(productos, a.Producto) {
			out = append(out, a.Carpeta)
		}
	}
	return out
}

// abierta dice si la app de ese producto está corriendo; sin saberlo, que no.
func abierta(producto string) bool {
	as, _ := abiertas(carpetasDe([]string{producto}), runtime.GOOS)
	return len(as) > 0
}
