// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package voz lee texto en voz alta con la voz del SISTEMA: lo que en la web es
// `speechSynthesis` del navegador («Leer en voz alta» de CertiPDF).
//
//   - macOS: `say`, con la primera voz instalada del idioma pedido (`say -v '?'`).
//   - Windows: `System.Speech` (SAPI) desde PowerShell, con la voz de esa cultura.
//   - Linux: Speech Dispatcher (`spd-say`), el mismo servicio que usa el lector de
//     pantalla Orca, con el idioma pedido.
//
// Donde no hay voz —Android, iOS, un Linux sin Speech Dispatcher— [Leer] devuelve
// [ErrNoDisponible] y quien llama lo DICE (`pdf.speak.unsupported`), igual que la web
// en un navegador sin `speechSynthesis`: callarse se leería como un botón roto.
//
// ponytail: Android e iOS tienen voz (TextToSpeech, AVSpeechSynthesizer), pero piden
// JNI y Objective-C que desde el frontal no se pueden ver funcionar. Se relevan cuando
// haya un móvil donde probarlos.
package voz

import (
	"errors"
	"regexp"
	"strings"
)

// ErrNoDisponible es que este sistema no tiene voz que usar.
var ErrNoDisponible = errors.New("voz: el sistema no lee en voz alta")

// Leer empieza a leer `texto` en `idioma` (BCP 47, «es-ES») y vuelve enseguida.
// `alAcabar` se llama UNA vez, en su propia rutina, cuando termina: porque acabó o
// porque se calló. Devuelve con qué callarla.
func Leer(texto, idioma string, alAcabar func()) (callar func(), err error) {
	// Un salto de línea parte el texto en mensajes en Speech Dispatcher: se lee seguido.
	texto = strings.Join(strings.Fields(texto), " ")
	if texto == "" {
		return func() {}, nil
	}
	return leer(texto, idioma, alAcabar)
}

// lengua es la parte del idioma que entienden las voces: «es» de «es-ES».
func lengua(idioma string) string {
	l, _, _ := strings.Cut(strings.ReplaceAll(idioma, "_", "-"), "-")
	return strings.ToLower(l)
}

// lineaDeVoz es una línea de `say -v '?'`: el nombre (que puede llevar espacios y
// paréntesis), su idioma como `es_ES` y, detrás de «#», una frase de muestra.
var lineaDeVoz = regexp.MustCompile(`^(.+?)\s+([a-z]{2,3})[_-]([A-Za-z0-9]{2,4})\s+#`)

// vozDe elige, de la lista de `say -v '?'`, la voz para `idioma`: la de su región si la
// hay y si no, la primera de su lengua. Vacío es que no hay ninguna y se lee con la del
// sistema.
func vozDe(lista, idioma string) string {
	l := lengua(idioma)
	_, region, _ := strings.Cut(strings.ReplaceAll(idioma, "_", "-"), "-")
	primera := ""
	for _, linea := range strings.Split(lista, "\n") {
		m := lineaDeVoz.FindStringSubmatch(linea)
		if len(m) < 4 || m[2] != l {
			continue
		}
		if region != "" && strings.EqualFold(m[3], region) {
			return m[1]
		}
		if primera == "" {
			primera = m[1]
		}
	}
	return primera
}
