// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// anchoFormularioClave es el `max-w-sm` del formulario menos su `px-6`: 24rem de 16: 384 menos 48 de relleno.
var anchoFormularioClave = 24*tema.Rem - 2*tema.Espacio(6)

// nuevoFormularioClave es `PideContrasena` de `PdfApp.tsx`: el título, la explicación,
// el campo, el error si lo hay y «Abrir», en una columna `gap-4` centrada.
//
// El campo es el que la web escribe a mano, `border-border rounded-md border px-3 py-2
// text-sm`, sin clase `placeholder:`: 36 px de alto con la caja de línea de 18.
func nuevoFormularioClave(t Traductor, errorClave string, enviar func(string)) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	campo := ui.NuevaEntrada(t("pdf.password.placeholder")).ConPropia(ui.CampoPropio{
		MargenX: tema.Espacio(3), MargenY: tema.Espacio(2), Texto: tema.TextoSM, Borde: "border", MarcaDelTexto: true,
	})
	campo.Contrasena(true)
	campo.Ancho = anchoFormularioClave
	abrir := NuevaAccion(t("pdf.password.open"), func() { enviar(campo.Texto()) })
	abrir.Escalon, abrir.PX, abrir.PY = tema.TextoSM, tema.Espacio(3), tema.Espacio(2)
	abrir.Fondo, abrir.FondoEncima, abrir.Tinta = "primary", "primary-hover", "on-primary"
	abrir.Ancho = anchoFormularioClave
	campo.AlTeclear = func(e *fyne.KeyEvent) bool {
		if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
			enviar(campo.Texto())
			return true
		}
		return false
	}
	partes := []fyne.CanvasObject{
		ui.NuevoRotulo(t("pdf.password.title"), tema.TextoLG, ui.SemiNegrita, "text"),
		parrafoCentrado(t("pdf.password.help"), tema.TextoSM, "text-muted", anchoFormularioClave),
		campo,
	}
	if errorClave != "" {
		partes = append(partes, ui.NuevoParrafo(errorClave, tema.TextoSM, ui.Normal, "danger-text", v).A(anchoFormularioClave))
	}
	partes = append(partes, abrir)
	// El foco va al campo al montar, como el `useEffect` de la web.
	fyne.Do(func() {
		if c := fyne.CurrentApp().Driver().CanvasForObject(campo); c != nil {
			c.Focus(campo)
		}
	})
	return container.New(centradoEnTodo{relleno: tema.Espacio(6)},
		container.New(ui.Columna{Hueco: tema.Espacio(4), Encaja: true}, partes...))
}

// parrafoCentrado es un `<p class="text-center">`: cada línea centrada en el ancho.
func parrafoCentrado(texto string, e tema.Escalon, token string, ancho float32) fyne.CanvasObject {
	lineas := ui.Repartir(texto, e, ui.Normal, ancho)
	objs := make([]fyne.CanvasObject, 0, len(lineas))
	for _, l := range lineas {
		objs = append(objs, ui.NuevoRotulo(l, e, ui.Normal, token))
	}
	return container.New(ui.Columna{Encaja: true}, objs...)
}
