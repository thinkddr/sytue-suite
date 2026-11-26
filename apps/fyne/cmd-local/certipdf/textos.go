// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Los textos PROPIOS de esta app, que no tienen clave en el catálogo de la ofimática
// (PLAN §5.14): el aviso de la IA, la nota del OCR y el título del selector de anclas.
// Español e inglés aquí, los otros veinticuatro idiomas en textos_idiomas.go. Los de la
// pantalla de firmas de la edición local están en `apps/certipdf` (edicionlocal.go).
var textos = map[string][2]string{ // clave: {es, en}
	"anclasTitulo": {"Certificados de confianza", "Trusted certificates"},
	"ocrTesseract": {
		"El texto se añade como capa invisible: no cambia ni un píxel del escaneo. Se reconoce con Tesseract en este equipo; el documento no sale de él.",
		"The text is added as an invisible layer: not a pixel of the scan changes. It is recognised with Tesseract on this device; the document does not leave it.",
	},
	"ocrWindows": {
		"El texto se añade como capa invisible: no cambia ni un píxel del escaneo. Se reconoce con el OCR de Windows en este equipo; el documento no sale de él.",
		"The text is added as an invisible layer: not a pixel of the scan changes. It is recognised with Windows OCR on this device; the document does not leave it.",
	},
	"ocrIA": {
		"El texto se añade como capa invisible. Lo transcribe el modelo de IA que configuraste en Ajustes: se le envía la imagen de cada página. Para reconocerlo sin enviar nada, instala Tesseract.",
		"The text is added as an invisible layer. The AI model you set up in Settings transcribes it: the image of each page is sent to it. To recognise it without sending anything, install Tesseract.",
	},
	"iaTitulo":   {"Enviar a la IA", "Send to AI"},
	"iaCancelar": {"Cancelar", "Cancel"},
	"iaEnviar":   {"Enviar", "Send"},
	"iaTexto": {
		"Para responder, el texto de este PDF (y, al leer un escaneo, la imagen de sus páginas) se enviará a %s, el proveedor de IA que configuraste en Ajustes. ¿Continuar?",
		"To answer, the text of this PDF (and, when reading a scan, the image of its pages) will be sent to %s, the AI provider you set up in Settings. Continue?",
	},
}

// propio es el texto propio de clave en un idioma; lo que falte cae al inglés.
func propio(idioma i18n.Locale, clave string) string {
	return local.Propios{Base: textos, Traducciones: traducciones}.En(idioma, clave)
}
