// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
)

// Las marcas de tiempo de la web (apps/frontend/apps/cast/src/lib/marcas.ts), con los
// mismos casos en marcas_test.go: los capítulos de una descripción, el «?t=» de
// «Empezar en…» y las teclas del reproductor que llevan a un sitio.

// Capitulo es una línea «1:30 Demo» de la descripción.
type Capitulo struct {
	Inicio time.Duration
	Titulo string
}

var lineaCapitulo = regexp.MustCompile(`^\s*(?:[-–—•*]\s*)?[(\[]?(?:(\d{1,2}):)?(\d{1,2}):(\d{2})[)\]]?\s*(?:[-–—:|]\s*)?(\S.*?)\s*$`)

// segundosDe valida «h:mm:ss» o «m:ss»: segundos por debajo de 60, y los minutos
// también si hay horas.
func segundosDe(h, m, s string) (time.Duration, bool) {
	hh, _ := strconv.Atoi(h)
	mm, _ := strconv.Atoi(m)
	ss, _ := strconv.Atoi(s)
	if ss >= 60 || (h != "" && mm >= 60) {
		return 0, false
	}
	return time.Duration(hh*3600+mm*60+ss) * time.Second, true
}

// CapitulosDe son los capítulos de una descripción: las líneas que EMPIEZAN por una
// marca. Solo cuentan si son al menos dos, el primero en 0:00 y cada uno después del
// anterior; si no, son horas sueltas en el texto, no un índice.
func CapitulosDe(desc string) []Capitulo {
	var out []Capitulo
	for _, linea := range strings.Split(desc, "\n") {
		m := lineaCapitulo.FindStringSubmatch(linea)
		if m == nil {
			continue
		}
		inicio, ok := segundosDe(m[1], m[2], m[3])
		if !ok {
			continue
		}
		out = append(out, Capitulo{Inicio: inicio, Titulo: m[4]})
	}
	if len(out) < 2 || out[0].Inicio != 0 {
		return nil
	}
	for i := 1; i < len(out); i++ {
		if out[i].Inicio <= out[i-1].Inicio {
			return nil
		}
	}
	return out
}

// EnlaceEn es el enlace con «?t=» en segundos enteros, o sin él en el segundo 0.
func EnlaceEn(enlace string, momento time.Duration) string {
	ruta, consulta, _ := strings.Cut(enlace, "?")
	q, err := url.ParseQuery(consulta)
	if err != nil {
		q = url.Values{}
	}
	if s := int(momento / time.Second); s > 0 {
		q.Set("t", strconv.Itoa(s))
	} else {
		q.Del("t")
	}
	if c := q.Encode(); c != "" {
		return ruta + "?" + c
	}
	return ruta
}

// PasoLargo es lo que mueven J y L: el doble que las flechas.
const PasoLargo = 10 * time.Second

// DestinoDeTecla es adónde lleva una tecla del reproductor: J y L 10 s atrás y
// adelante, Inicio y Fin a los extremos, y 0–9 al 0 %–90 %. false si no lleva a nada.
func DestinoDeTecla(k fyne.KeyName, pos, dur time.Duration) (time.Duration, bool) {
	if dur <= 0 {
		return 0, false
	}
	acotar := func(d time.Duration) time.Duration { return min(max(d, 0), dur) }
	switch k {
	case fyne.KeyJ:
		return acotar(pos - PasoLargo), true
	case fyne.KeyL:
		return acotar(pos + PasoLargo), true
	case fyne.KeyHome:
		return 0, true
	case fyne.KeyEnd:
		return dur, true
	default:
		if len(k) == 1 && k[0] >= '0' && k[0] <= '9' {
			return dur * time.Duration(k[0]-'0') / 10, true
		}
		return 0, false
	}
}
