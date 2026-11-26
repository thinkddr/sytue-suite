// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package obligaciones

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Las reglas de plazo son una mini-gramática de DOS formas, y no más:
//
//   - Fecha fija: "MM-DD" o "MM-ultimo", con "+N" opcional = N años después
//     del ejercicio. Sirve para lo anual y para los trimestres con fecha propia
//     (en España el cuarto trimestre vence el 30 de enero y no el 20).
//   - Fecha relativa al fin del periodo: "DD" o "ultimo", con "+n" opcional =
//     n meses después del mes en que acaba el periodo (por defecto 1; "+0" es
//     el mismo mes). Es la regla de casi todo lo mensual: «el día 20 del mes
//     siguiente».
//
// "ultimo_habil" vale donde "ultimo": es el último día LABORABLE del mes, que
// retrocede al viernes si el mes acaba en fin de semana. Grecia y Eslovenia
// formulan así el IVA («τελευταία εργάσιμη ημέρα», «zadnji delovni dan»), y
// con "ultimo" el calendario habría corrido la fecha al lunes siguiente: tres
// días tarde. Los festivos van aparte, en el calendario laboral del país.
//
// Las claves del mapa de plazos dicen cuál aplica: "M" (mensual, relativa),
// "T" (trimestral, relativa a cada trimestre), "T1".."T4" (trimestres con fecha
// fija) y cualquier otra clave ("A", "1P") con fecha fija. Un día que no existe
// en el mes (30 de febrero) se recorta al último día: es lo que hacen las
// administraciones, no un error del dato.

type instancia struct {
	periodo string
	limite  time.Time
}

func (o *Obligacion) instancias(anio int) ([]instancia, error) {
	if len(o.Plazos) == 0 {
		return nil, fmt.Errorf("sin plazos")
	}
	switch o.Periodicidad {
	case "mensual":
		return o.mensuales(anio)
	case "trimestral":
		return o.trimestrales(anio)
	default:
		return o.anuales(anio)
	}
}

func (o *Obligacion) mensuales(anio int) ([]instancia, error) {
	regla, ok := o.Plazos["M"]
	if !ok {
		return nil, fmt.Errorf("mensual sin clave M")
	}
	out := make([]instancia, 0, 12)
	for mes := 1; mes <= 12; mes++ {
		limite, err := fechaRelativa(regla, anio, mes)
		if err != nil {
			return nil, err
		}
		out = append(out, instancia{fmt.Sprintf("%d-%02d", anio, mes), limite})
	}
	return out, nil
}

func (o *Obligacion) trimestrales(anio int) ([]instancia, error) {
	out := make([]instancia, 0, 4)
	regla, relativa := o.Plazos["T"]
	for t := 1; t <= 4; t++ {
		var limite time.Time
		var err error
		if relativa {
			limite, err = fechaRelativa(regla, anio, t*3)
		} else {
			fija, ok := o.Plazos[fmt.Sprintf("T%d", t)]
			if !ok {
				return nil, fmt.Errorf("falta el trimestre T%d", t)
			}
			limite, err = fechaFija(fija, anio)
		}
		if err != nil {
			return nil, err
		}
		out = append(out, instancia{fmt.Sprintf("%d-T%d", anio, t), limite})
	}
	return out, nil
}

// anuales: una instancia por clave, en orden estable de clave.
func (o *Obligacion) anuales(anio int) ([]instancia, error) {
	claves := make([]string, 0, len(o.Plazos))
	for k := range o.Plazos {
		claves = append(claves, k)
	}
	sort.Strings(claves)
	out := make([]instancia, 0, len(claves))
	for _, k := range claves {
		limite, err := fechaFija(o.Plazos[k], anio)
		if err != nil {
			return nil, err
		}
		out = append(out, instancia{fmt.Sprintf("%d-%s", anio, k), limite})
	}
	return out, nil
}

func partir(regla string, porDefecto int) (base string, desplazamiento int, err error) {
	base, resto, hay := strings.Cut(regla, "+")
	if !hay {
		return base, porDefecto, nil
	}
	n, err := strconv.Atoi(resto)
	if err != nil || n < 0 || n > 24 {
		return "", 0, fmt.Errorf("regla %q: desplazamiento inválido", regla)
	}
	return base, n, nil
}

func dia(s string, anio int, mes time.Month) (int, error) {
	switch s {
	case "ultimo":
		return ultimoDia(anio, mes), nil
	case "ultimo_habil":
		d := time.Date(anio, mes, ultimoDia(anio, mes), 0, 0, 0, 0, time.UTC)
		for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			d = d.AddDate(0, 0, -1)
		}
		return d.Day(), nil
	}
	d, err := strconv.Atoi(s)
	if err != nil || d < 1 || d > 31 {
		return 0, fmt.Errorf("día inválido %q", s)
	}
	return min(d, ultimoDia(anio, mes)), nil
}

// fechaFija interpreta "MM-DD[+N]" para el ejercicio dado.
func fechaFija(regla string, anio int) (time.Time, error) {
	base, anios, err := partir(regla, 0)
	if err != nil {
		return time.Time{}, err
	}
	mesS, diaS, hay := strings.Cut(base, "-")
	if !hay {
		return time.Time{}, fmt.Errorf("regla %q: se esperaba MM-DD", regla)
	}
	m, err := strconv.Atoi(mesS)
	if err != nil || m < 1 || m > 12 {
		return time.Time{}, fmt.Errorf("regla %q: mes inválido", regla)
	}
	y := anio + anios
	d, err := dia(diaS, y, time.Month(m))
	if err != nil {
		return time.Time{}, fmt.Errorf("regla %q: %w", regla, err)
	}
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC), nil
}

// fechaRelativa interpreta "DD[+n]" o "ultimo[+n]" respecto al mes en que acaba
// el periodo. `time.Date` normaliza el mes 13 al enero siguiente, así que el
// cambio de año sale solo.
func fechaRelativa(regla string, anio, mesFin int) (time.Time, error) {
	base, meses, err := partir(regla, 1)
	if err != nil {
		return time.Time{}, err
	}
	primero := time.Date(anio, time.Month(mesFin+meses), 1, 0, 0, 0, 0, time.UTC)
	d, err := dia(base, primero.Year(), primero.Month())
	if err != nil {
		return time.Time{}, fmt.Errorf("regla %q: %w", regla, err)
	}
	return time.Date(primero.Year(), primero.Month(), d, 0, 0, 0, 0, time.UTC), nil
}

func ultimoDia(anio int, mes time.Month) int {
	return time.Date(anio, mes+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// RangoDePeriodo devuelve el primer y último día del periodo de una instancia
// ("2026-T3", "2026-03" o "2026-A"): es lo que acota la consulta al libro
// cuando la obligación se calcula.
func RangoDePeriodo(periodo string) (desde, hasta time.Time, ok bool) {
	anioS, resto, hay := strings.Cut(periodo, "-")
	anio, err := strconv.Atoi(anioS)
	if err != nil || !hay || anio < 2000 || anio > 2100 {
		return time.Time{}, time.Time{}, false
	}
	switch {
	case len(resto) == 2 && resto[0] == 'T' && resto[1] >= '1' && resto[1] <= '4':
		t := int(resto[1] - '0')
		desde = time.Date(anio, time.Month(t*3-2), 1, 0, 0, 0, 0, time.UTC)
		return desde, desde.AddDate(0, 3, -1), true
	case len(resto) == 2 && resto[0] >= '0' && resto[0] <= '9' && resto[1] >= '0' && resto[1] <= '9':
		m, err := strconv.Atoi(resto)
		if err != nil || m < 1 || m > 12 {
			return time.Time{}, time.Time{}, false
		}
		desde = time.Date(anio, time.Month(m), 1, 0, 0, 0, 0, time.UTC)
		return desde, desde.AddDate(0, 1, -1), true
	default:
		desde = time.Date(anio, 1, 1, 0, 0, 0, 0, time.UTC)
		return desde, time.Date(anio, 12, 31, 0, 0, 0, 0, time.UTC), true
	}
}

// EtiquetaPeriodo deja el periodo legible sin traducirlo: «1T 2026», «2026-03»,
// «2026». Es lo que va en el título del evento; la interfaz lo traduce mejor.
func EtiquetaPeriodo(periodo string) string {
	anio, resto, _ := strings.Cut(periodo, "-")
	switch {
	case len(resto) == 2 && resto[0] == 'T':
		return string(resto[1]) + "T " + anio
	case resto == "A":
		return anio
	default:
		return periodo
	}
}
