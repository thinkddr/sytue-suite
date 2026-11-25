// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Cumpleanos es uno de los próximos cumpleaños: `client/cumpleanos.ts` de la web.
type Cumpleanos struct {
	Contacto api.Contact
	// Fecha es la próxima vez que cae, a medianoche local.
	Fecha time.Time
	// Dias son los que faltan (0 = hoy).
	Dias int
	// Edad son los años que cumple. 0: el servicio no sabe el año.
	Edad int
	// Aniversario: es su ANNIVERSARY y no su cumpleaños; la lista lo dice.
	Aniversario bool
}

// ProximosCumpleanos son los de `contactos` en los próximos `ventana` días desde
// `hoy`, del más cercano al más lejano, con año o sin él (`BirthdayNoYear`, que el
// servicio proyecta desde la migración 0007). Un 29 de febrero en año no bisiesto cae
// el 1 de marzo, que es lo que hace `time.Date`, igual que `Date` en la web.
func ProximosCumpleanos(contactos []api.Contact, hoy time.Time, ventana int) []Cumpleanos {
	inicio := time.Date(hoy.Year(), hoy.Month(), hoy.Day(), 0, 0, 0, 0, hoy.Location())
	var fuera []Cumpleanos
	// El cumpleaños y, si lo hay, el aniversario: los dos son «el día de alguien».
	poner := func(c api.Contact, mes time.Month, dia, anyo int, aniversario bool) {
		f := time.Date(inicio.Year(), mes, dia, 0, 0, 0, 0, inicio.Location())
		if f.Before(inicio) {
			f = time.Date(inicio.Year()+1, mes, dia, 0, 0, 0, 0, inicio.Location())
		}
		// Por fechas de calendario y no por horas: un cambio de hora no puede
		// convertir «mañana» en «hoy».
		dias := int(f.Sub(inicio).Round(24*time.Hour) / (24 * time.Hour))
		if dias > ventana {
			return
		}
		cu := Cumpleanos{Contacto: c, Fecha: f, Dias: dias, Aniversario: aniversario}
		if anyo > 1000 && !aniversario {
			cu.Edad = f.Year() - anyo
		}
		fuera = append(fuera, cu)
	}
	for _, c := range contactos {
		if mes, dia, anyo, ok := cumpleDe(c); ok {
			poner(c, mes, dia, anyo, false)
		}
		if a := c.Anniversary; a != nil {
			poner(c, a.Month(), a.Day(), a.Year(), true)
		}
	}
	// ESTABLE y solo por días: a igualdad se queda el orden de entrada, que es el
	// alfabético del idioma con el que ya viene la libreta (`ordenar`).
	slices.SortStableFunc(fuera, func(x, y Cumpleanos) int { return x.Dias - y.Dias })
	return fuera
}

// cumpleanosEn es el contador de la fila «Cumpleaños» del menú: los que caen en los
// próximos `dias`.
func cumpleanosEn(contactos []api.Contact, hoy time.Time, dias int) int {
	return len(ProximosCumpleanos(contactos, hoy, dias))
}

// cumpleDe es el cumpleaños de un contacto: la fecha completa o, si no la hay, el
// «--MM-DD» sin año (anyo = 0). Es `cumpleanosDe` de la web, sobre lo que proyecta el
// servicio en vez de sobre la vCard cruda; un «--MM-DD» malformado se ignora sin ruido,
// como allí, porque un cumpleaños mal escrito no es un fallo de la libreta.
func cumpleDe(c api.Contact) (mes time.Month, dia, anyo int, ok bool) {
	if c.Birthday != nil {
		return c.Birthday.Month(), c.Birthday.Day(), c.Birthday.Year(), true
	}
	v := c.BirthdayNoYear
	if len(v) != len("--MM-DD") || !strings.HasPrefix(v, "--") || v[4] != '-' {
		return 0, 0, 0, false
	}
	m, err1 := strconv.Atoi(v[2:4])
	d, err2 := strconv.Atoi(v[5:7])
	// En un año bisiesto, para que el 29 de febrero valga y un 31 de abril no.
	t := time.Date(2000, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	if err1 != nil || err2 != nil || m < 1 || m > 12 || d < 1 || int(t.Month()) != m || t.Day() != d {
		return 0, 0, 0, false
	}
	return time.Month(m), d, 0, true
}

// cumpleDeLaFicha es el cumpleaños de la ficha en el formato largo del idioma: «10 de
// diciembre de 1815», o «10 de diciembre» si no se sabe el año. Es `formatearCumpleanos`
// de la web; sin año se pinta sobre uno bisiesto, para que un 29 de febrero no salga
// como 1 de marzo.
func (p *Pantalla) cumpleDeLaFicha(c api.Contact) string {
	mes, dia, _, ok := cumpleDe(c)
	switch {
	case !ok:
		return ""
	case c.Birthday != nil:
		return i18n.FormatDateLong(p.locale(), *c.Birthday)
	}
	return i18n.FormatDayMonthLong(p.locale(), time.Date(2000, mes, dia, 0, 0, 0, 0, time.UTC))
}
