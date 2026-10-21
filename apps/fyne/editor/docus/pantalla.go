// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Pantalla monta Sytue Docus entero: el cromo compartido con el folio dentro.
//
// Es la ÚNICA función que el binario necesita, y es tres líneas de verdad: crea el
// contexto, registra la pestaña propia y le da el lienzo al marco. Lo que Calcus y
// Diapos tienen que escribir es exactamente esto con su lienzo y su pestaña.
//
// `t` traduce; con nil se devuelven las claves, que es lo que quiere un guardián.
func Pantalla(t func(string, i18n.Vars) string, alVolver func()) fyne.CanvasObject {
	p, _ := PantallaConCarga(t, alVolver)
	return p
}

// PantallaConCarga es Pantalla con la función que le mete un documento ya leído: lo que
// necesita el binario cuando le llega un fichero de Drive (`sesion.AlAbrirFichero`).
//
// `cargar` pone el título, carga el HTML y deja el documento en SOLO LECTURA: lo que se
// enseña es una conversión del fichero, no un documento de Sytue Docs, y lo que se
// escribiera encima no tendría dónde guardarse. Se llama en el hilo de la interfaz.
func PantallaConCarga(t func(string, i18n.Vars) string, alVolver func()) (fyne.CanvasObject, func(titulo, html string)) {
	return pantallaConCarga(t, "es", alVolver, nil)
}

// pantallaConCarga es PantallaConCarga con el idioma de los números de la cinta y lo que
// la app añade al contexto (el menú Archivo).
func pantallaConCarga(t func(string, i18n.Vars) string, loc i18n.Locale, alVolver func(),
	decorar func(*editor.Contexto),
) (fyne.CanvasObject, func(titulo, html string)) {
	ctx := &editor.Contexto{
		Tipo:   editor.Documento,
		Titulo: "",
		Estado: editor.Listo,
		Zoom:   100,
		Regla:  true,
		T:      t,
	}
	folio := NuevoFolio(ctx)
	ctx.Lienzo = folio
	ctx.Paneles = panelesDe(folio, t, loc)
	ctx.AjustarAlAncho = folio.AjustarAlAncho
	if decorar != nil {
		decorar(ctx)
	}
	marco := editor.NuevoMarco(ctx, registroDocus(loc), alVolver)
	cargar := func(titulo, html string) {
		folio.Cargar(html)
		ctx.Titulo, ctx.Estado, ctx.SoloLectura = titulo, editor.Listo, true
		ctx.Refrescar()
	}
	return marco, cargar
}
