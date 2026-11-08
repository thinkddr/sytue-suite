// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package provider

// nombresIdioma son los idiomas de producto (CONVENTIONS §13.1) tal como se
// nombran dentro del system prompt, que está en español. Lista cerrada: un
// código que no está aquí no añade directiva, en vez de colar en el prompt un
// texto que llega de fuera.
var nombresIdioma = map[string]string{
	"en": "inglés",
	"de": "alemán",
	"es": "español",
	"fr": "francés",
	"it": "italiano",
}

// ConIdioma añade al system prompt la orden de escribir en el idioma que la
// persona tiene configurado. Sin idioma conocido devuelve el prompt tal cual.
//
// LA EXCEPCIÓN DE TRANSFORMAR UN TEXTO ES A PROPÓSITO: quien tiene la interfaz
// en español y pide «corrige la gramática» sobre un párrafo en inglés quiere el
// párrafo corregido en inglés, no traducido. El idioma configurado manda en lo
// que el asistente REDACTA para la persona —respuestas, resúmenes, texto nuevo—.
func ConIdioma(system, idioma string) string {
	nombre, ok := nombresIdioma[idioma]
	if !ok {
		return system
	}
	return system + "\n\nIDIOMA DEL USUARIO: " + nombre + " (" + idioma + "). " +
		"Escribe en " + nombre + " todo lo que redactes para el usuario —respuestas, resúmenes, " +
		"explicaciones, sugerencias, etiquetas y texto nuevo, también los valores de texto de una respuesta JSON " +
		"(no sus claves)—, aunque la instrucción o el contexto estén en otro idioma. " +
		"Solo hay dos excepciones: si la instrucción pide expresamente otro idioma (por ejemplo, traducir), " +
		"usa ese; y si la acción transforma un texto existente (corregir, reescribir, acortar, ampliar, " +
		"continuar, cambiar el tono), conserva el idioma de ese texto."
}
