// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"strings"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Pregunta es usePrompt.tsx: `window.prompt` con el diálogo de la suite y un campo de
// verdad. El nombre de una hoja, la etiqueta de una versión, el de una categoría.
//
// Es una Confirmacion con un FieldGroup dentro —la etiqueta en `text-sm font-semibold`
// con el asterisco de obligatorio— y el campo con `valor` ya escrito. Lo que añade
// sobre la confirmación es lo que el diálogo del navegador no sabía hacer: el botón de
// aceptar se APAGA mientras el campo esté vacío, e Intro acepta como allí.
//
// `mensaje` hace de título y de etiqueta, como en la web cuando no se da `titulo`.
// `obligatorio` es «ui.required» YA traducido: el asterisco se pinta y no se lee, y lo
// que se anuncia es la palabra.
func Pregunta(mensaje, valor, rotuloCancelar, rotuloAceptar, obligatorio string, alAceptar func(string)) *Dialogo {
	return pregunta(mensaje, valor, rotuloCancelar, rotuloAceptar, obligatorio, false, alAceptar)
}

// PreguntaOpcional es la Pregunta con `permitirVacio`: el campo vacío también se acepta
// —el botón no se apaga e Intro acepta—, y la etiqueta va sin asterisco, como FieldGroup
// sin `required`. La etiqueta de una versión.
func PreguntaOpcional(mensaje, valor, rotuloCancelar, rotuloAceptar string, alAceptar func(string)) *Dialogo {
	return pregunta(mensaje, valor, rotuloCancelar, rotuloAceptar, "", true, alAceptar)
}

func pregunta(mensaje, valor, rotuloCancelar, rotuloAceptar, obligatorio string, permitirVacio bool,
	alAceptar func(string),
) *Dialogo {
	mensaje = SinDosPuntos(mensaje)
	campo := NuevaEntrada("")
	campo.PonerTexto(valor)
	etiqueta := container.New(Fila{Hueco: tema.Espacio(0.5)}, // `ml-0.5`
		NuevoRotulo(mensaje, tema.TextoSM, SemiNegrita, "text"))
	if !permitirVacio {
		asterisco := &asteriscoObligatorio{
			Rotulo: Rotulo{Texto: "*", Escalon: tema.TextoSM, Peso: SemiNegrita, Token: "danger-text"},
			leido:  obligatorio,
		}
		asterisco.ExtendBaseWidget(asterisco)
		etiqueta.Add(asterisco)
	}
	cuerpo := container.New(Columna{Hueco: tema.Espacio(1.5)}, etiqueta, campo) // `gap-1.5`

	var d *Dialogo
	var aceptar *Boton
	falta := func() bool { return !permitirVacio && strings.TrimSpace(campo.Texto()) == "" }
	enviar := func() {
		if falta() {
			return
		}
		d.Cerrar()
		alAceptar(campo.Texto())
	}
	d, aceptar = confirmacion(mensaje, rotuloCancelar, rotuloAceptar, false, cuerpo,
		func() { d.Cerrar() }, enviar)
	aceptar.Inerte = falta()
	campo.AlCambiar(func(string) {
		aceptar.Inerte = falta()
		aceptar.Refresh()
	})
	campo.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name != fyne.KeyReturn && k.Name != fyne.KeyEnter {
			return false
		}
		enviar()
		return true
	}
	d.Foco = campo
	d.clase = clasePreguntar // la nueva cancela la pendiente, como usePrompt
	return d
}

// asteriscoObligatorio es el `<span aria-hidden>*</span><span class="sr-only">` de
// FieldGroup: se ve el asterisco y se anuncia la palabra.
type asteriscoObligatorio struct {
	Rotulo
	leido string
}

func (a *asteriscoObligatorio) AccessibilityLabel() string { return a.leido }

// SinDosPuntos quita los dos puntos del final: las preguntas heredadas de
// `window.prompt` («Nombre de la macro:», «Etiqueta de la versión (opcional):») hacen
// de título y de etiqueta del campo, y ahí no llevan. Lo mismo hace usePrompt.tsx.
func SinDosPuntos(s string) string {
	s = strings.TrimRight(strings.TrimRightFunc(s, unicode.IsSpace), ":：")
	return strings.TrimRightFunc(s, unicode.IsSpace) // «Nom de la macro :», con su espacio fino
}
