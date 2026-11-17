// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"encoding/json"
	"io/fs"
	"regexp"
	"strings"
	"sync"
	"unicode"
	"unicode/utf16"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
)

// separarCita separa lo nuevo de una respuesta del historial que arrastra, como el
// «•••» de Gmail. Es `separarCita` de `apps/frontend/apps/mail/src/components/utils.ts`,
// regla a regla; los casos que pasan las dos, en `testdata/cita.json`.
//
// La cita es lo que va desde el separador de Outlook, o el BLOQUE FINAL de líneas `>`
// con su línea de atribución delante (que puede venir partida en dos, como la parte
// Gmail al pasar de 78 columnas). Solo el bloque final: las respuestas intercaladas
// entre trozos citados son la respuesta, y plegarlas escondería lo que se contesta.
//
// Si no queda nada nuevo —un mensaje que es solo cita—, no se pliega: esconderlo todo
// tras un botón dejaría la tarjeta vacía. Entonces `cita` es "" y `cuerpo` el texto.
func separarCita(texto string) (cuerpo, cita string) {
	lineas := strings.Split(strings.ReplaceAll(texto, "\r\n", "\n"), "\n")
	corte := len(lineas)
	for i, l := range lineas {
		if separadorOriginal.MatchString(strings.TrimSpace(l)) {
			corte = i
			break
		}
	}

	blanca := func(l string) bool { return strings.TrimSpace(l) == "" }
	i := len(lineas) - 1
	for i >= 0 && blanca(lineas[i]) {
		i--
	}
	if i >= 0 && esLineaCitada(lineas[i]) {
		inicio := i
		for j := i; j >= 0 && (esLineaCitada(lineas[j]) || blanca(lineas[j])); j-- {
			if esLineaCitada(lineas[j]) {
				inicio = j
			}
		}
		a := inicio - 1
		for a >= 0 && blanca(lineas[a]) {
			a--
		}
		if a >= 0 && esAtribucion(lineas[a]) {
			inicio = a
		} else if a >= 1 && esAtribucion(lineas[a-1]+" "+lineas[a]) {
			inicio = a - 1
		}
		corte = min(corte, inicio)
	}

	cuerpo = strings.TrimRightFunc(strings.Join(lineas[:corte], "\n"), unicode.IsSpace)
	if strings.TrimSpace(cuerpo) == "" {
		return texto, ""
	}
	return cuerpo, strings.Join(lineas[corte:], "\n")
}

// separadorOriginal es el que pone Outlook encima del mensaje respondido.
var separadorOriginal = regexp.MustCompile(`(?i)^-{2,}[\s\p{Zs}]*original message[\s\p{Zs}]*-{2,}$`)

func esLineaCitada(l string) bool {
	return strings.HasPrefix(strings.TrimLeftFunc(l, unicode.IsSpace), ">")
}

// largoMaxAtribucion es el tope de `LARGO_MAX_ATRIBUCION` en la web: una atribución
// de verdad cabe en un par de líneas, y allí, sin él, una línea larga encima de la cita
// colgaba la lectura (aquí RE2 no retrocede, pero la regla es la misma en las dos).
const largoMaxAtribucion = 256

func esAtribucion(l string) bool {
	l = strings.TrimSpace(l)
	// Contado en unidades UTF-16, que es lo que mide `length` en la web.
	n := 0
	for _, r := range l {
		n += utf16.RuneLen(r)
	}
	if n > largoMaxAtribucion {
		return false
	}
	// Los espacios se juntan en uno y la plantilla los pide como « ?», igual que en la
	// web, donde con `\s*` entre dos huecos `.+` una racha de espacios aún tardaba.
	junta := espaciosDeCita.ReplaceAllString(l, " ")
	for _, e := range atribuciones() {
		if e.MatchString(junta) {
			return true
		}
	}
	return false
}

// atribuciones son las cabeceras de `reader.quoteHeader` de TODOS los idiomas del
// catálogo, no solo del de quien lee: un correo citado en sueco llega igual a quien
// tiene la app en castellano. Aquí sí se leen del catálogo, que va entero en el
// binario; la web, que baja uno solo, las lleva en `ATRIBUCIONES_DE_CITA`.
var atribuciones = sync.OnceValue(func() []*regexp.Regexp {
	var out []*regexp.Regexp
	vistas := map[string]bool{}
	cat := catalogos.Mail()
	nombres, _ := fs.Glob(cat, "*.json")
	for _, n := range nombres {
		crudo, err := fs.ReadFile(cat, n)
		if err != nil {
			continue
		}
		var claves map[string]json.RawMessage
		var plantilla string
		if json.Unmarshal(crudo, &claves) != nil || json.Unmarshal(claves["reader.quoteHeader"], &plantilla) != nil {
			continue
		}
		if plantilla == "" || vistas[plantilla] {
			continue
		}
		vistas[plantilla] = true
		out = append(out, expresionDeAtribucion(plantilla))
	}
	return out
})

// espaciosDeCita son los blancos que se juntan en uno, contando el de no separación.
var espaciosDeCita = regexp.MustCompile(`[\s\p{Zs}]+`)

var huecoDePlantilla = regexp.MustCompile(`\{(?:date|name)\}`)

// expresionDeAtribucion vuelve una plantilla una expresión que acepta lo que de verdad
// escriben los clientes: los huecos son «cualquier cosa», los espacios pueden faltar o
// sobrar (también el de no separación que Apple Mail pone en francés antes de los dos
// puntos) y la coma es opcional —Gmail escribe «On Mon, Jan 5, 2026 at 10:00 Ana
// <a@b.eu> wrote:», sin la coma de la plantilla—.
func expresionDeAtribucion(plantilla string) *regexp.Regexp {
	partes := huecoDePlantilla.Split(plantilla, -1)
	for i, p := range partes {
		var b strings.Builder
		for _, c := range espaciosDeCita.ReplaceAllString(p, " ") {
			switch c {
			case ' ':
				b.WriteString(` ?`)
			case ',':
				b.WriteString(",?")
			default:
				b.WriteString(regexp.QuoteMeta(string(c)))
			}
		}
		partes[i] = b.String()
	}
	return regexp.MustCompile(`(?i)^` + strings.Join(partes, ".+") + `$`)
}
