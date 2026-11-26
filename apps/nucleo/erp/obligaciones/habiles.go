// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package obligaciones

import (
	"fmt"
	"slices"
	"strconv"
	"time"
)

// CalendarioLaboral son los días inhábiles del país para contar plazos. Solo lo
// declara un país donde se ha contrastado con la fuente oficial: en España, la
// resolución anual de la Secretaría de Estado de Función Pública en el BOE.
type CalendarioLaboral struct {
	// Fuente es la referencia oficial de los años declarados.
	Fuente string `json:"fuente"`
	// MargenHabiles son los días hábiles de margen que se dan a quien presenta
	// sobre el último día que da la administración: su «fecha».
	MargenHabiles int `json:"margen_habiles"`
	// RegionesPorCP asigna la comunidad por los dos primeros dígitos del código postal.
	RegionesPorCP map[string]string `json:"regiones_por_cp"`
	// Inhabiles por año: "nacional" y cada región, en AAAA-MM-DD.
	Inhabiles map[string]map[string][]string `json:"inhabiles"`
	// FiestasFijas (MM-DD) y el Viernes Santo sirven para los años sin resolución
	// publicada todavía (la del año que viene sale en noviembre): el calendario
	// sale provisional y la interfaz lo dice.
	FiestasFijas []string `json:"fiestas_fijas"`
	ViernesSanto bool     `json:"viernes_santo"`
}

func (c *CalendarioLaboral) validar() error {
	if c.MargenHabiles < 0 || c.MargenHabiles > 20 {
		return fmt.Errorf("calendario: margen_habiles fuera de rango")
	}
	for anio, regiones := range c.Inhabiles {
		if _, err := strconv.Atoi(anio); err != nil {
			return fmt.Errorf("calendario: año %q", anio)
		}
		for region, dias := range regiones {
			for _, d := range dias {
				t, err := time.Parse(time.DateOnly, d)
				if err != nil || strconv.Itoa(t.Year()) != anio {
					return fmt.Errorf("calendario %s/%s: día %q", anio, region, d)
				}
			}
		}
	}
	for _, f := range c.FiestasFijas {
		if _, err := time.Parse("01-02", f); err != nil {
			return fmt.Errorf("calendario: fiesta fija %q", f)
		}
	}
	return nil
}

// Region devuelve la región de un código postal, o "" si no se sabe.
func (c *CalendarioLaboral) Region(cp string) string {
	if c == nil || len(cp) < 2 {
		return ""
	}
	return c.RegionesPorCP[cp[:2]]
}

// inhabil dice si un día no cuenta para un plazo: fin de semana, fiesta
// nacional o, si se da región, fiesta de esa región. El segundo valor es true si
// el año no tiene resolución publicada y se ha supuesto con las fiestas fijas.
func (c *CalendarioLaboral) inhabil(t time.Time, region string) (bool, bool) {
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		return true, false
	}
	dia := t.Format(time.DateOnly)
	if anio, ok := c.Inhabiles[strconv.Itoa(t.Year())]; ok {
		return slices.Contains(anio["nacional"], dia) || (region != "" && slices.Contains(anio[region], dia)), false
	}
	if slices.Contains(c.FiestasFijas, t.Format("01-02")) {
		return true, true
	}
	return c.ViernesSanto && t.Equal(pascua(t.Year()).AddDate(0, 0, -2)), true
}

// siguienteHabil corre un límite que cae en inhábil al primer hábil siguiente
// (art. 30.5 de la Ley 39/2015). Cuenta SOLO lo nacional: es lo que publica la
// Agencia Tributaria en su calendario, y correr el plazo por una fiesta regional
// que la administración no aplicara haría presentar tarde.
func (c *CalendarioLaboral) siguienteHabil(t time.Time) (time.Time, bool) {
	provisional := false
	for {
		no, prov := c.inhabil(t, "")
		provisional = provisional || prov
		if !no {
			return t, provisional
		}
		t = t.AddDate(0, 0, 1)
	}
}

// habilesAntes retrocede n días hábiles de la región. Aquí SÍ cuentan las fiestas
// regionales: el margen es para quien trabaja allí, y contarlas solo adelanta la
// fecha, que es el lado seguro.
func (c *CalendarioLaboral) habilesAntes(t time.Time, n int, region string) (time.Time, bool) {
	provisional := false
	for n > 0 {
		t = t.AddDate(0, 0, -1)
		no, prov := c.inhabil(t, region)
		provisional = provisional || prov
		if !no {
			n--
		}
	}
	return t, provisional
}

// limiteDomiciliacion es el último día para presentar domiciliando el pago: el
// más tardío entre cinco días naturales y tres hábiles antes del fin del plazo.
// Es la regla que aplica la Agencia Tributaria en su calendario del contribuyente
// (2026: el 303 del 4.º trimestre vence el 30 de enero y domicilia hasta el 27).
func (c *CalendarioLaboral) limiteDomiciliacion(limite time.Time) time.Time {
	naturales := limite.AddDate(0, 0, -5)
	habiles, _ := c.habilesAntes(limite, 3, "")
	if habiles.After(naturales) {
		return habiles
	}
	return naturales
}

// pascua es el domingo de Pascua del año (algoritmo gregoriano anónimo).
func pascua(anio int) time.Time {
	a, b, c := anio%19, anio/100, anio%100
	d, e := b/4, b%4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i, k := c/4, c%4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	mes := (h + l - 7*m + 114) / 31
	dia := (h+l-7*m+114)%31 + 1
	return time.Date(anio, time.Month(mes), dia, 0, 0, 0, 0, time.UTC)
}
