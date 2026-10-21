// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"slices"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
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
}

// ProximosCumpleanos son los de `contactos` en los próximos `ventana` días desde
// `hoy`, del más cercano al más lejano. Un 29 de febrero en
// año no bisiesto cae el 1 de marzo, que es lo que hace `time.Date`, igual que `Date`
// en la web.
//
// ponytail: un cumpleaños SIN año («--0215», vCard 4) no llega. La web lo lee de la
// vCard cruda y la API REST solo da `birthday` como fecha completa
// (`services/contacts/internal/domain/vcard.go`, `parseBirthday`): el servicio lo
// descarta al proyectar. Se arregla en el servicio, no aquí.
func ProximosCumpleanos(contactos []api.Contact, hoy time.Time, ventana int) []Cumpleanos {
	inicio := time.Date(hoy.Year(), hoy.Month(), hoy.Day(), 0, 0, 0, 0, hoy.Location())
	var fuera []Cumpleanos
	for _, c := range contactos {
		if c.Birthday == nil {
			continue
		}
		b := *c.Birthday
		f := time.Date(inicio.Year(), b.Month(), b.Day(), 0, 0, 0, 0, inicio.Location())
		if f.Before(inicio) {
			f = time.Date(inicio.Year()+1, b.Month(), b.Day(), 0, 0, 0, 0, inicio.Location())
		}
		// Por fechas de calendario y no por horas: un cambio de hora no puede
		// convertir «mañana» en «hoy».
		dias := int(f.Sub(inicio).Round(24*time.Hour) / (24 * time.Hour))
		if dias > ventana {
			continue
		}
		cu := Cumpleanos{Contacto: c, Fecha: f, Dias: dias}
		if b.Year() > 1000 {
			cu.Edad = f.Year() - b.Year()
		}
		fuera = append(fuera, cu)
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
