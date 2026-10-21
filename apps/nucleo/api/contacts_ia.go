// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"regexp"
	"slices"
	"strings"
)

// El asistente de la ficha de Contacts (`lib/ai.ts` de la web): se pega una firma de
// correo y el formulario sale relleno. PRERRELLENA Y NO GUARDA: la persona revisa antes
// de pulsar, porque un teléfono mal copiado no se nota hasta que alguien llama.

// InstruccionFicha es `instruccionFicha`, literal: pide JSON plano y prohíbe rellenar
// huecos. Es una instrucción para el modelo, no un texto de la interfaz.
const InstruccionFicha = "Extrae los datos de contacto del texto (una firma de correo, una tarjeta o unas notas). " +
	"Devuelve SOLO un objeto JSON con las claves " +
	"{nombre, organizacion, correos, telefonos, nota}, donde correos y telefonos son listas de cadenas. " +
	"Copia los valores TAL CUAL aparecen; no inventes correos ni teléfonos ni completes los que estén a medias. " +
	"Omite las claves que el texto no diga."

// FichaPropuesta es lo que el asistente propone. Todo opcional salvo el nombre; vacío
// es que el texto no lo decía. Sin puesto: el formulario no tiene ese campo.
type FichaPropuesta struct {
	Nombre, Organizacion, Nota string
	Correos, Telefonos         []string
}

var (
	// reCorreo descarta lo que no parece un correo: el modelo a veces cuela la web.
	reCorreo = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]{2,}$`)
	// reTelefono es un número utilizable: seis cifras o más y nada raro fuera de los
	// separadores habituales. No se normaliza: quitar el prefijo es decidir por quien marca.
	reTelefono = regexp.MustCompile(`^\+?[\d\s().-]{6,}$`)
)

// LeerFicha es `fichaDe`: valida lo que respondió el modelo. Sin nombre no hay ficha.
func LeerFicha(texto string) (FichaPropuesta, bool) {
	obj, ok := primerObjeto(texto)
	if !ok {
		return FichaPropuesta{}, false
	}
	f := FichaPropuesta{
		Nombre: cadenaDe(obj, "nombre", 200), Organizacion: cadenaDe(obj, "organizacion", 200),
		Nota: cadenaDe(obj, "nota", 2000),
	}
	if f.Nombre == "" {
		return FichaPropuesta{}, false
	}
	for _, c := range listaDe(obj, "correos", 320) {
		if reCorreo.MatchString(c) {
			f.Correos = append(f.Correos, c)
		}
	}
	for _, n := range listaDe(obj, "telefonos", 40) {
		if reTelefono.MatchString(n) && len(strings.Map(soloCifras, n)) >= 6 {
			f.Telefonos = append(f.Telefonos, n)
		}
	}
	return f, true
}

// soloCifras deja las cifras y quita lo demás (strings.Map).
func soloCifras(r rune) rune {
	if r >= '0' && r <= '9' {
		return r
	}
	return -1
}

// cadenaDe es `cadena` de @sytue/ai: una cadena no vacía, recortada a `tope`.
func cadenaDe(obj map[string]any, clave string, tope int) string {
	s, _ := obj[clave].(string)
	return primerasRunas(strings.TrimSpace(s), tope)
}

// listaDe es `lista` de @sytue/ai: sin vacíos ni repetidos y diez como mucho. Tolera
// una cadena sola en vez de una lista, que es lo que contesta el modelo cuando solo
// encuentra una: descartarla sería perder el dato por un corchete.
func listaDe(obj map[string]any, clave string, tope int) []string {
	var bruta []any
	switch v := obj[clave].(type) {
	case string:
		bruta = []any{v}
	case []any:
		bruta = v
	}
	var fuera []string
	for _, x := range bruta {
		s, _ := x.(string)
		if s = primerasRunas(strings.TrimSpace(s), tope); s != "" && !slices.Contains(fuera, s) {
			fuera = append(fuera, s)
		}
		if len(fuera) >= 10 {
			break
		}
	}
	return fuera
}
