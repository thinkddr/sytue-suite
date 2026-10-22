// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package analisis

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
)

// El RELLENO CON IA de una columna: `sheets/relleno.ts` de la web, con los mismos
// vectores (testdata/relleno.json). Qué se le pide a Sytue AI a partir de los ejemplos
// de la columna y cómo se lee lo que contesta; la persona ve la propuesta antes de que
// se escriba nada.

// TopeRelleno es `FILL_MAX_CHARS`, en unidades UTF-16 como `length` en JavaScript.
const TopeRelleno = 12000

// PeticionRelleno es `FillPrompt`.
type PeticionRelleno struct {
	Instruccion, Contexto string
	// Pendientes son las filas de la hoja (1-based) que se piden.
	Pendientes []int
}

var reSaltos = regexp.MustCompile(`[\t\r\n]+`)

// limpiarCelda es `clean`: sin tabuladores ni saltos, que parten la tabla.
func limpiarCelda(s string) string { return strings.TrimSpace(reSaltos.ReplaceAllString(s, " ")) }

func largoJS(s string) int { return len(utf16.Encode([]rune(s))) }

func celdaDe(f []string, c int) string {
	if c < len(f) {
		return f[c]
	}
	return ""
}

func filaLlena(f []string) bool {
	for _, v := range f {
		if strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

func lineaRelleno(n string, cs []string) string {
	partes := make([]string, 0, 1+len(cs))
	partes = append(partes, n)
	for _, c := range cs {
		partes = append(partes, limpiarCelda(c))
	}
	return strings.Join(partes, "\t")
}

func instruccionRelleno(columna, pista string) string {
	instr := "Rellena la columna «" + columna + "» de la tabla del contexto. " +
		"Las filas que ya tienen valor en esa columna son ejemplos: sigue su patrón y su formato. "
	if pista != "" {
		instr += "Criterio de la persona: «" + limpiarCelda(pista) + "». "
	}
	return instr + "Devuelve SOLO una línea por cada fila marcada con «?», con el número de la fila, un tabulador " +
		"y el valor. Sin encabezados, sin comillas y sin explicaciones."
}

// PedirRelleno es `buildFillPrompt`. false si no hay nada que pedir.
func PedirRelleno(cabeceras []string, filas [][]string, primera, objetivo int, pista string) (PeticionRelleno, bool) {
	pista = strings.TrimSpace(pista)
	if objetivo < 0 || objetivo >= len(cabeceras) {
		return PeticionRelleno{}, false
	}
	ejemplos, faltan := 0, 0
	for _, f := range filas {
		switch {
		case !filaLlena(f):
		case strings.TrimSpace(celdaDe(f, objetivo)) != "":
			ejemplos++
		default:
			faltan++
		}
	}
	if faltan == 0 || (ejemplos == 0 && pista == "") {
		return PeticionRelleno{}, false
	}
	columna := limpiarCelda(cabeceras[objetivo])
	if columna == "" {
		columna = "#" + strconv.Itoa(objetivo+1)
	}
	cab := lineaRelleno("fila", cabeceras)
	out, tam := []string{cab}, largoJS(cab)
	var pendientes []int
	for i, f := range filas {
		if !filaLlena(f) {
			continue
		}
		pendiente := strings.TrimSpace(celdaDe(f, objetivo)) == ""
		cs := make([]string, len(cabeceras))
		for c := range cabeceras {
			cs[c] = celdaDe(f, c)
		}
		if pendiente {
			cs[objetivo] = "?"
		}
		l := lineaRelleno(strconv.Itoa(primera+i), cs)
		if tam+1+largoJS(l) > TopeRelleno {
			continue
		}
		tam += 1 + largoJS(l)
		out = append(out, l)
		if pendiente {
			pendientes = append(pendientes, primera+i)
		}
	}
	if len(pendientes) == 0 {
		return PeticionRelleno{}, false
	}
	return PeticionRelleno{Instruccion: instruccionRelleno(columna, pista), Contexto: strings.Join(out, "\n"), Pendientes: pendientes}, true
}

var (
	reVallas = regexp.MustCompile("```[a-zA-Z0-9_-]*[ \t\r\n]*|```")
	reLinea  = []*regexp.Regexp{
		regexp.MustCompile(`(?i)^[ \t]*(?:fila[ \t]*)?(\d+)\t(.*)$`),
		regexp.MustCompile(`(?i)^[ \t]*(?:fila[ \t]*)?(\d+)[ \t]*[|:.)][ \t]*(.*)$`),
		regexp.MustCompile(`(?i)^[ \t]*(?:fila[ \t]*)?(\d+)[ \t]+-[ \t]+(.*)$`),
	}
	comillas = "`'\"“”‘’«»"
)

// LeerRelleno es `parseFillResponse`: fila → valor, solo de las pedidas y la primera
// vez que salen con un valor; nunca una fórmula.
func LeerRelleno(texto string, pendientes []int) map[int]string {
	quiero := map[int]bool{}
	for _, n := range pendientes {
		quiero[n] = true
	}
	out := map[int]string{}
	cuerpo := reVallas.ReplaceAllString(texto, "\n")
	for _, bruta := range strings.Split(cuerpo, "\n") {
		bruta = strings.TrimSuffix(bruta, "\r")
		var m []string
		for _, re := range reLinea {
			if m = re.FindStringSubmatch(bruta); m != nil {
				break
			}
		}
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		valor := strings.Trim(strings.Trim(strings.Trim(m[2], " \t"), comillas), " \t")
		if err != nil || !quiero[n] || valor == "" || valor == "?" || strings.HasPrefix(valor, "=") {
			continue
		}
		if _, ya := out[n]; ya {
			continue
		}
		out[n] = valor
	}
	return out
}
