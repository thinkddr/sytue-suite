// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
)

// CampoFecha es `<Input type="date">` y CampoHora es `<Input type="time">`.
//
// En la web el selector lo pone el NAVEGADOR: `type="date"` pinta la caja con la hoja
// de la suite y el calendario con el del sistema, que es distinto en cada uno y no
// tiene CSS que copiar. Aquí la fecha se TECLEA en una Entrada con su icono —la misma
// caja, el mismo borde, el mismo aro— y LeerFecha dice si lo escrito vale.
//
// ponytail: sin rejilla de mes desplegable. Techo: elegir una fecha lejana a base de
// teclear. Se releva colgando una rejilla del mismo emergente que usa el Selector; lo
// tecleado seguiría valiendo, que es como se mete un cumpleaños de 1985.
func CampoFecha(t TallaCampo) *Entrada {
	// El ejemplo es una fecha y no «dd/mm/aaaa»: los números se leen igual en los 26
	// idiomas de la suite y las letras no.
	e := NuevaEntrada("31/12/2026")
	e.Icono = iconos.Calendario
	return e.Con(t)
}

// CampoHora es el campo de hora: la misma caja con el icono del reloj.
func CampoHora(t TallaCampo) *Entrada {
	e := NuevaEntrada("09:30")
	e.Icono = iconos.Reloj
	return e.Con(t)
}

// EscribirFecha escribe una fecha como LeerFecha la lee: «05/10/2026».
func EscribirFecha(t time.Time) string { return t.Format("02/01/2006") }

// EscribirHora escribe una hora como LeerHora la lee: «09:30».
func EscribirHora(t time.Time) string { return t.Format("15:04") }

// separadores son los que la gente teclea de verdad: «12/10/2026», «12.10.2026»,
// «12-10-2026» y «12 10 2026» son la misma fecha.
var separadores = strings.NewReplacer(".", "/", "-", "/", " ", "/")

// LeerFecha lee una fecha tecleada, a las 00:00 de `zona`.
//
// El día va DELANTE, como en los 26 idiomas de la suite, salvo que lo primero sean los
// cuatro dígitos del año («2026-10-05», que es como se escribe en Suecia, Hungría o
// Lituania). Sin separadores valen los ocho dígitos seguidos, que es lo que deja
// teclear el teclado numérico de un móvil.
//
// El año va ENTERO: «85» no dice de qué siglo es un cumpleaños, y `time.Parse` con
// «06» lo resolvería solo y mal.
//
// `vacia` distingue «no ha escrito nada» de «ha escrito algo que no es una fecha»: un
// campo opcional en blanco es válido y uno con basura no.
func LeerFecha(s string, zona *time.Location) (t time.Time, vacia, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, true, true
	}
	if len(s) == 8 && soloDigitos(s) { // 05102026
		s = s[:2] + "/" + s[2:4] + "/" + s[4:]
	}
	s = separadores.Replace(s)
	// `time.Parse` NO normaliza: el 31 de febrero es un error, no el 3 de marzo, que es
	// justo lo que hay que decirle a quien lo teclea.
	for _, formato := range []string{"2/1/2006", "2006/1/2"} {
		if t, err := time.ParseInLocation(formato, s, zona); err == nil {
			return t, false, true
		}
	}
	return time.Time{}, false, false
}

// LeerHora lee una hora tecleada: «9», «9:30», «09.30» o, sin separador, «930» y
// «0930», que es lo que sale de un teclado numérico.
func LeerHora(s string) (hora, minuto int, vacia, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, true, true
	}
	if soloDigitos(s) {
		switch len(s) {
		case 1, 2: // «9», «09»
			s += ":00"
		case 3: // «930»
			s = s[:1] + ":" + s[1:]
		case 4: // «0930»
			s = s[:2] + ":" + s[2:]
		}
	}
	s = strings.NewReplacer(".", ":", "-", ":", " ", ":").Replace(s)
	for _, formato := range []string{"15:04", "3:04"} {
		if t, err := time.Parse(formato, s); err == nil {
			return t.Hour(), t.Minute(), false, true
		}
	}
	return 0, 0, false, false
}

func soloDigitos(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
