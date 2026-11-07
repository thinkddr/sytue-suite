// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image/color"
	"sort"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La pareja en la web es apps/frontend/apps/calendar/src/components/modeloMesCompacto.ts
// (y, para TituloCompacto, `compactTitle` de dates.ts).
//
// EL MES COMPACTO DEL TELÉFONO, como datos. Sin widgets: lo pinta vistamescompacta.go.
// Los vectores de test (V1–V14) son los mismos, con el mismo nombre, a los dos lados.
//
//   - La semana empieza en lunes (`PrimerDia`) y su número es el ISO 8601 de ese lunes
//     (`time.ISOWeek`).
//   - Solo las filas que tocan el mes (4, 5 o 6); los días de otro mes son HUECO.
//     Plegado, la semana del cursor va COMPLETA: los días del mes vecino llevan fecha,
//     puntos y se pueden elegir (`OtroMes`).
//   - Un evento cubre un día si [inicio, fin) lo solapa: fin exclusivo; sin fin, o con
//     fin ≤ inicio, solo el día de inicio. Es `Tramar`, una pasada por evento.
//   - Orden de un día: primero los de día completo, luego por instante de inicio; si
//     empatan, el de entrada. Los puntos son los colores de los MaxPuntos primeros.

// MaxPuntos es `MAX_PUNTOS`: puntos por día como mucho. El resto lo dice el nombre
// accesible (`Eventos`).
const MaxPuntos = 3

// DiaCompacto es un día de la rejilla compacta.
type DiaCompacto struct {
	// Fecha es la medianoche del día en la zona de visualización; cero si Hueco.
	Fecha time.Time
	// Hueco es un día de otro mes en el mes desplegado: no se pinta ni se elige.
	Hueco bool
	Hoy   bool
	// Elegido es el cursor de la pantalla: no hay otro estado de selección.
	Elegido bool
	// OtroMes es un día del mes vecino en la semana plegada.
	OtroMes bool
	// Puntos son los colores de los primeros MaxPuntos eventos del día, en su orden.
	Puntos []color.NRGBA
	// Eventos son todos los que cubren el día.
	Eventos int
}

// SemanaCompacta es una fila: su número ISO y sus siete días, de lunes a domingo.
type SemanaCompacta struct {
	Numero int
	Dias   [7]DiaCompacto
}

// FilaDia es un evento del día, como lo pinta la lista bajo la rejilla.
type FilaDia struct {
	O           Ocurrencia
	Inicio, Fin time.Time
	TodoElDia   bool
	// Cruza: con hora y acaba otro día que el de su inicio; la fila enseña las dos fechas.
	Cruza bool
}

// colorPunto es `event.color ?? accent`: `Fundir` ya resuelve el del calendario, y una
// ocurrencia sin color (alfa cero) lleva el acento.
func colorPunto(o Ocurrencia) color.NRGBA {
	if o.Color.A == 0 {
		return AcentoPorDefecto
	}
	return o.Color
}

// porDia reparte las ocurrencias entre los días de `dias`, ya ordenadas. Una pasada por
// evento: `Tramar` da el tramo y se apunta en cada día del tramo.
func porDia(dias []time.Time, loc *time.Location, eventos []Ocurrencia) [][]FilaDia {
	cubos := make([][]FilaDia, len(dias))
	for _, o := range eventos {
		ini, fin := ocurrenciaInstantes(o, loc)
		desde, hasta, ok := Tramar(dias, ini, fin, loc)
		if !ok {
			continue
		}
		f := FilaDia{O: o, Inicio: ini, Fin: fin, TodoElDia: o.Event.AllDay}
		for i := desde; i <= hasta; i++ {
			cubos[i] = append(cubos[i], f)
		}
	}
	for _, c := range cubos {
		sort.SliceStable(c, func(i, j int) bool {
			if c[i].TodoElDia != c[j].TodoElDia {
				return c[i].TodoElDia
			}
			return c[i].Inicio.Before(c[j].Inicio)
		})
	}
	return cubos
}

func nuevoDiaDelModelo(fecha, hoy, cursor time.Time, filas []FilaDia, otroMes bool) DiaCompacto {
	d := DiaCompacto{
		Fecha:   fecha,
		Hoy:     mismoDia(fecha, hoy),
		Elegido: mismoDia(fecha, cursor),
		OtroMes: otroMes,
		Eventos: len(filas),
	}
	for _, f := range filas[:min(len(filas), MaxPuntos)] {
		d.Puntos = append(d.Puntos, colorPunto(f.O))
	}
	return d
}

// DiasDelMes son los días (lunes → domingo) de las semanas que tocan el mes del cursor:
// 28, 35 o 42.
func DiasDelMes(cursor time.Time, loc *time.Location) []time.Time {
	c := cursor.In(loc)
	inicio := PrimerDia(c, loc)
	ultimo := time.Date(c.Year(), c.Month()+1, 0, 0, 0, 0, 0, loc)
	semanas := deltaDias(inicio, ultimo, loc)/7 + 1
	dias := make([]time.Time, 7*semanas)
	for i := range dias {
		dias[i] = inicio.AddDate(0, 0, i)
	}
	return dias
}

// SemanasCompactas es `semanasCompactas`: el mes del cursor desplegado, con el número
// ISO de cada semana, sus huecos y sus puntos.
func SemanasCompactas(cursor, hoy time.Time, loc *time.Location, eventos []Ocurrencia) []SemanaCompacta {
	cursor, hoy = cursor.In(loc), hoy.In(loc)
	dias := DiasDelMes(cursor, loc)
	cubos := porDia(dias, loc, eventos)
	semanas := make([]SemanaCompacta, len(dias)/7)
	for w := range semanas {
		_, semanas[w].Numero = dias[w*7].ISOWeek()
		for c := range 7 {
			i := w*7 + c
			if dias[i].Month() != cursor.Month() {
				semanas[w].Dias[c] = DiaCompacto{Hueco: true}
				continue
			}
			semanas[w].Dias[c] = nuevoDiaDelModelo(dias[i], hoy, cursor, cubos[i], false)
		}
	}
	return semanas
}

// SemanaPlegada es `semanaPlegada`: la semana del cursor completa, con los días del mes
// vecino elegibles.
func SemanaPlegada(cursor, hoy time.Time, loc *time.Location, eventos []Ocurrencia) SemanaCompacta {
	cursor, hoy = cursor.In(loc), hoy.In(loc)
	dia := inicioDelDia(cursor, loc)
	lunes := LunesDe(dia)
	dias := make([]time.Time, 7)
	for i := range dias {
		dias[i] = lunes.AddDate(0, 0, i)
	}
	cubos := porDia(dias, loc, eventos)
	var s SemanaCompacta
	_, s.Numero = lunes.ISOWeek()
	for i, f := range dias {
		s.Dias[i] = nuevoDiaDelModelo(f, hoy, cursor, cubos[i], f.Month() != cursor.Month())
	}
	return s
}

// EventosDelDia es `eventosDelDia`: los que cubren el día, en el orden de la lista.
func EventosDelDia(dia time.Time, loc *time.Location, eventos []Ocurrencia) []FilaDia {
	filas := porDia([]time.Time{inicioDelDia(dia, loc)}, loc, eventos)[0]
	for i, f := range filas {
		ultimo := f.Inicio
		if !f.Fin.IsZero() && f.Fin.After(f.Inicio) {
			ultimo = f.Fin.Add(-time.Millisecond)
		}
		filas[i].Cruza = !f.TodoElDia && !mismoDia(f.Inicio, ultimo)
	}
	return filas
}

// TituloCompacto es `compactTitle`: el mes suelto si es del año de hoy y, si no, con su
// año («noviembre», «enero de 2027»). Sin capitalizar, como la captura del teléfono. Los
// dos instantes se leen en la zona de la pantalla, como la rejilla: el cursor de
// cmd/calendar nace en la hora LOCAL y, con otra zona elegida, cerca de la medianoche
// del cambio de mes el título decía otro mes que la rejilla.
func TituloCompacto(l i18n.Locale, cursor, hoy time.Time, loc *time.Location) string {
	cursor, hoy = cursor.In(loc), hoy.In(loc)
	if cursor.Year() == hoy.Year() {
		return i18n.FormatMonth(l, cursor)
	}
	return i18n.FormatMonthYear(l, cursor)
}
