// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package i18n

import (
	_ "embed"
	"encoding/json"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

// fechas.json lo genera gen-i18n.ts preguntándole a Intl: de cada idioma, el patrón de
// `dateStyle: 'medium'` y el de `timeStyle: 'short'`, y los doce meses TAL Y COMO SALEN
// en ese patrón — «sept» en español, «09» en alemán, «9» en finés, «szept.» en húngaro.
// En golang.org/x/text no hay formato de fechas; y aunque lo hubiera, sería CLDR 32.
//
//go:embed fechas.json
var fechasJSON []byte

type parteFecha struct {
	Tipo    string `json:"t"`
	Literal string `json:"v"`
	// Dos: el campo va con cero delante («05»).
	Dos bool `json:"dos"`
}

// parteRango es una parte de `formatRangeToParts`: la de siempre y de qué extremo sale
// («startRange», «endRange» o «shared»).
type parteRango struct {
	parteFecha
	Src string `json:"src"`
}

// patronRango es un patrón de rango con los meses tal y como salen EN él: en checo y en
// eslovaco el rango escribe el mes en cifras y el formato suelto, con su nombre.
type patronRango struct {
	Partes []parteRango `json:"partes"`
	Meses  []string     `json:"meses"`
}

type formatoFecha struct {
	Fecha []parteFecha `json:"fecha"`
	Hora  []parteFecha `json:"hora"`
	// HoraDos es `{ hour: '2-digit', minute: '2-digit' }`, el `timeLabel` de Calendar.
	HoraDos []parteFecha `json:"hora_dos"`
	Meses   []string     `json:"meses"`
	// Periodos son «a. m.» y «p. m.» en el idioma, o vacíos si va a 24 horas.
	Periodos [2]string `json:"periodos"`
	// MesAno es el patrón de `{ month: 'long', year: 'numeric' }` y MesesLargos los doce
	// meses tal y como salen EN él: en los idiomas con casos no son los de la fecha corta.
	MesAno      []parteFecha `json:"mes_ano"`
	MesesLargos []string     `json:"meses_largos"`
	// MesCortoAno es `{ month: 'short', year: 'numeric' }` y MesesCortoAno sus meses: el
	// mes agrupado de un informe de Sytue AI («sept 2026»). No es MesAno con el mes
	// corto: en español el largo lleva «de» y el corto no.
	MesCortoAno   []parteFecha `json:"mes_corto_ano"`
	MesesCortoAno []string     `json:"meses_corto_ano"`
	// Dias son los siete días en corto, de lunes a domingo.
	Dias [7]string `json:"dias"`
	// DiasEstrechos son sus INICIALES, las del mini-calendario. No salen de recortar
	// las cortas: en español el miércoles es «X» y no «M».
	DiasEstrechos [7]string `json:"dias_estrechos"`
	// DiaMes es el patrón de `{ weekday: 'long', day: 'numeric', month: 'long' }` —el
	// sobretítulo de Sytue Hub—, con sus meses y sus días de la semana tal y como salen
	// EN él: en los idiomas con casos no son los de la fecha corta ni los de mes_ano.
	DiaMes      []parteFecha `json:"dia_mes"`
	MesesDiaMes []string     `json:"meses_dia_mes"`
	DiasLargos  [7]string    `json:"dias_largos"`
	// FechaDia2 es el patrón de `{ day: '2-digit', month: 'short', year: 'numeric' }`
	// y MesesDia2 sus meses. Tiene vector propio y no se deriva de Fecha rellenando el
	// día porque en alemán y en letón el patrón entero es otro (medido, ver
	// FormatDateShortMonth).
	FechaDia2 []parteFecha `json:"fecha_dia2"`
	MesesDia2 []string     `json:"meses_dia2"`
	// FechaLarga es `{ day: 'numeric', month: 'long', year: 'numeric' }` y DiaMesLargo
	// lo mismo sin año, cada una con sus meses tal y como salen EN ella.
	FechaLarga       []parteFecha `json:"fecha_larga"`
	MesesFechaLarga  []string     `json:"meses_fecha_larga"`
	DiaMesLargo      []parteFecha `json:"dia_mes_largo"`
	MesesDiaMesLargo []string     `json:"meses_dia_mes_largo"`
	// FechaLargaHora es `{ dateStyle: 'long', timeStyle: 'short' }`, con sus meses.
	FechaLargaHora      []parteFecha `json:"fecha_larga_hora"`
	MesesFechaLargaHora []string     `json:"meses_fecha_larga_hora"`
	// FechaCompleta es la larga CON el día de la semana (`{ weekday: 'long', day, month:
	// 'long', year }`), el título de la vista de día de Calendar, con sus meses y sus días
	// tal y como salen EN ella. MesesCortos es el mes SUELTO de `{ month: 'short' }`.
	FechaCompleta      []parteFecha `json:"fecha_completa"`
	MesesFechaCompleta []string     `json:"meses_fecha_completa"`
	DiasFechaCompleta  [7]string    `json:"dias_fecha_completa"`
	MesesCortos        []string     `json:"meses_cortos"`
	// DiaCorto es `{ weekday: 'short', day: 'numeric', month: 'short' }`, con sus meses
	// y sus días tal y como salen EN él.
	DiaCorto      []parteFecha `json:"dia_corto"`
	MesesDiaCorto []string     `json:"meses_dia_corto"`
	DiasDiaCorto  [7]string    `json:"dias_dia_corto"`
	// RangoSemana son los tres patrones de `formatRange` de `{ day, month: 'long', year
	// }` —mismo mes, mismo año, distinto año—, con el extremo del que sale cada parte.
	RangoSemana struct {
		MismoMes patronRango `json:"mismo_mes"`
		MismoAno patronRango `json:"mismo_ano"`
		Distinto patronRango `json:"distinto"`
	} `json:"rango_semana"`
	// Relativo son las tablas de `Intl.RelativeTimeFormat`, por unidad.
	Relativo map[string]formatoRelativo `json:"relativo"`
}

// formatoRelativo es una unidad de `Intl.RelativeTimeFormat(tag, { numeric: 'auto',
// style: 'short' })`: los patrones del PASADO por categoría de plural, con `{n}` donde
// va el número, y las formas literales que `numeric: 'auto'` da sin él («ayer»).
type formatoRelativo struct {
	Pasado map[string]string `json:"pasado"`
	Auto   map[string]string `json:"auto"`
}

var fechas = func() map[Locale]formatoFecha {
	var m map[Locale]formatoFecha
	if err := json.Unmarshal(fechasJSON, &m); err != nil {
		panic("i18n: fechas.json: " + err.Error())
	}
	return m
}()

// FormatDate es `toLocaleDateString(tag, { dateStyle: 'medium' })`: «20 sept 2026»,
// «20.09.2026», «2026. szept. 20.». La zona horaria es la de `t`: la pone quien llama.
func FormatDate(l Locale, t time.Time) string { return componer(fechas[l].Fecha, fechas[l], t) }

// FormatTime es `toLocaleTimeString(tag, { timeStyle: 'short' })`. Casi todos los
// idiomas de la suite van a 24 horas y cambian el separador y el cero de la hora; el
// griego va a 12, con «πμ» y «μμ». Lo dice el patrón de cada uno, no una lista aquí.
func FormatTime(l Locale, t time.Time) string { return componer(fechas[l].Hora, fechas[l], t) }

// FormatTimeDigits es `{ hour: '2-digit', minute: '2-digit' }`: «09:07», «09:07 πμ». Es
// `timeLabel` de Sytue Calendar —las horas de la rejilla, de la agenda y de la vista
// rápida—, que NO es la hora corta: en español `timeStyle: 'short'` da «9:07».
func FormatTimeDigits(l Locale, t time.Time) string {
	return componer(fechas[l].HoraDos, fechas[l], t)
}

// FormatDateTime es fecha y hora juntas, como `formatDateTime` de format.ts.
func FormatDateTime(l Locale, t time.Time) string { return FormatDate(l, t) + ", " + FormatTime(l, t) }

// FormatMonthYear es `toLocaleDateString(tag, { month: 'long', year: 'numeric' })`:
// «septiembre de 2026», «September 2026», «2026. szeptember». Es el título de un mes; la
// mayúscula inicial que le pone la web la pone quien lo pinta.
func FormatMonthYear(l Locale, t time.Time) string {
	f := fechas[l]
	f.Meses = f.MesesLargos
	return componer(f.MesAno, f, t)
}

// FormatMonthShortYear es `toLocaleDateString(tag, { month: 'short', year: 'numeric' })`:
// «sept 2026», «Sep 2026», «2026. szept.». Es el mes agrupado de un informe de Sytue AI
// (`valor` de SytueIA.tsx).
func FormatMonthShortYear(l Locale, t time.Time) string {
	f := fechas[l]
	f.Meses = f.MesesCortoAno
	return componer(f.MesCortoAno, f, t)
}

// FormatDayMonth es `toLocaleDateString(tag, { day: 'numeric', month: 'short' })`:
// «11 sept», «Sep 11», «szept. 11.». Es la fecha de una fila de lista —una nota del
// árbol de Sytue Notepads—, donde el año sobra: casi todo es de este año y repetirlo en
// cada renglón gasta el ancho que necesita el título.
//
// Sale de la fecha media QUITÁNDOLE el año, y con él el separador que lo pega al resto:
// el orden de los campos y el nombre del mes cambian por idioma —el húngaro empieza por
// el año y el griego declina el mes— y volver a escribirlos aquí sería una segunda tabla
// que se desincroniza de fechas.json.
//
// ponytail: el resto de la puntuación se deja tal cual. En búlgaro la fecha media es
// «11.09.2026 г.» y al quitarle el año queda «11.09.», con un punto de más respecto al
// «11.09» de Intl. Corregirlo pide una segunda tabla por idioma, que es justo lo que
// fechas.json existe para no tener; un punto en el renglón gris de una fila no lo paga.
func FormatDayMonth(l Locale, t time.Time) string {
	f := fechas[l]
	return strings.TrimSpace(componer(sinAno(f.Fecha), f, t))
}

// sinAno quita del patrón la parte del año y el literal que la acompaña: el de detrás
// si lo hay —«2026. szept. 11.» deja «szept. 11.»— y si no, el de delante.
func sinAno(patron []parteFecha) []parteFecha {
	i := slices.IndexFunc(patron, func(p parteFecha) bool { return p.Tipo == "year" })
	if i < 0 {
		return patron
	}
	desde, hasta := i, i+1
	switch {
	case hasta < len(patron) && patron[hasta].Tipo == "literal":
		hasta++
	case desde > 0 && patron[desde-1].Tipo == "literal":
		desde--
	}
	return slices.Delete(slices.Clone(patron), desde, hasta)
}

// WeekdaysShort son las cabeceras de una semana, de LUNES a domingo como en la web.
func WeekdaysShort(l Locale) [7]string { return fechas[l].Dias }

// FormatWeekdayDayMonth es `toLocaleDateString(tag, { weekday: 'long', day: 'numeric',
// month: 'long' })`: «lunes, 21 de septiembre», «Monday 21 September», «21. September».
// Es el `todayLabel` de Launcher.tsx, el sobretítulo de Sytue Hub. SIN año a propósito:
// la fecha de hoy no necesita decir el año. Las mayúsculas las pone quien lo pinta
// (`text-transform: uppercase` de `.sytue-overline`), que aquí no hay CSS.
func FormatWeekdayDayMonth(l Locale, t time.Time) string {
	f := fechas[l]
	f.Meses, f.Dias = f.MesesDiaMes, f.DiasLargos
	return componer(f.DiaMes, f, t)
}

// FormatDateShortMonth es `toLocaleDateString(tag, { day: '2-digit', month: 'short',
// year: 'numeric' })`: «06 sept 2026», «06 Sept. 2026», «2026. g. 06. sept.». Es la
// columna de fecha de una LISTA DE FICHEROS —el explorador de Sytue Drive
// (`FileBrowser.tsx:429`)—, donde el día a dos cifras es lo que alinea la columna.
//
// No es `FormatDate` con el día rellenado, y por eso tiene su propio vector: medido en
// los 26 idiomas el 2026-09-22, en alemán el mes pasa de «09» a «Sept.» y en letón el
// literal del año cambia («2026. gada» contra «2026. g.»). En los otros 24 coincide.
func FormatDateShortMonth(l Locale, t time.Time) string {
	f := fechas[l]
	f.Meses = f.MesesDia2
	return componer(f.FechaDia2, f, t)
}

// FormatDateLong es `toLocaleDateString(tag, { day: 'numeric', month: 'long', year:
// 'numeric' })`: «4 de mayo de 1990», «4 May 1990», «1990. május 4.». Es el cumpleaños
// de la ficha de Sytue Contacts (`formatBirthday` de ContactDetail.tsx).
func FormatDateLong(l Locale, t time.Time) string {
	f := fechas[l]
	f.Meses = f.MesesFechaLarga
	return componer(f.FechaLarga, f, t)
}

// FormatDayMonthLong es `{ day: 'numeric', month: 'long' }`: «22 de septiembre»,
// «22 September», «22. September». Es `dayMonthLabel` de Sytue Calendar —la fecha de la
// vista rápida de un evento y el título de la agenda— y la de la lista de próximos
// cumpleaños de Sytue Contacts (BirthdaysModal.tsx), con el mes ENTERO donde
// FormatDayMonth lo abrevia.
func FormatDayMonthLong(l Locale, t time.Time) string {
	f := fechas[l]
	f.Meses = f.MesesDiaMesLargo
	return componer(f.DiaMesLargo, f, t)
}

// FormatFullDate es `{ weekday: 'long', day: 'numeric', month: 'long', year: 'numeric'
// }`: «martes, 22 de septiembre de 2026». Es el título de la vista de DÍA de Sytue
// Calendar; la mayúscula inicial la pone quien lo pinta, como en la web.
func FormatFullDate(l Locale, t time.Time) string {
	f := fechas[l]
	f.Meses, f.Dias = f.MesesFechaCompleta, f.DiasFechaCompleta
	return componer(f.FechaCompleta, f, t)
}

// FormatWeekdayDayMonthShort es `{ weekday: 'short', day: 'numeric', month: 'short' }`:
// «lun, 21 sept», «Mon 21 Sept». Es el día de cada grupo de huecos de «buscar hueco» en
// Sytue Calendar (FindTimePanel.tsx).
func FormatWeekdayDayMonthShort(l Locale, t time.Time) string {
	f := fechas[l]
	f.Meses, f.Dias = f.MesesDiaCorto, f.DiasDiaCorto
	return componer(f.DiaCorto, f, t)
}

// FormatMonthShort es `{ month: 'short' }`, el mes SUELTO: «sept», «Sept.», «szept.». Es
// `monthShortLabel` de la columna de fecha de la agenda. No sale de la fecha media: en
// alemán ahí el mes es «09».
func FormatMonthShort(l Locale, t time.Time) string {
	m := fechas[l].MesesCortos
	if len(m) != 12 {
		return ""
	}
	return m[t.Month()-1]
}

// FormatDateRange es `formatRange(a, b)` de `{ day: 'numeric', month: 'long', year:
// 'numeric' }`: «1–7 de enero de 2026», «5 de enero – 3 de febrero de 2026». Es el título
// de la vista de SEMANA de Sytue Calendar. Lo que los dos extremos comparten se escribe
// una vez, y cada lengua decide qué es eso: por eso hay tres patrones y no uno.
func FormatDateRange(l Locale, a, b time.Time) string {
	f := fechas[l]
	patron := f.RangoSemana.Distinto
	switch {
	case a.Year() == b.Year() && a.Month() == b.Month():
		patron = f.RangoSemana.MismoMes
	case a.Year() == b.Year():
		patron = f.RangoSemana.MismoAno
	}
	f.Meses = patron.Meses
	var s strings.Builder
	for _, p := range patron.Partes {
		t := a
		if p.Src == "endRange" {
			t = b
		}
		s.WriteString(componer([]parteFecha{p.parteFecha}, f, t))
	}
	return s.String()
}

// Unidad es una de las cuatro unidades de tiempo relativo que la suite usa. Conjunto
// CERRADO: son las de `formatTimeAgo`, y las tablas de CLDR solo se generan para estas.
type Unidad string

// Las cuatro.
const (
	Segundo Unidad = "second"
	Minuto  Unidad = "minute"
	Hora    Unidad = "hour"
	Dia     Unidad = "day"
)

// FormatRelativeTime es `new Intl.RelativeTimeFormat(tag, { numeric: 'auto', style:
// 'short' }).format(n, unidad)`: «hace 3 h», «ayer», «ahora».
//
// ponytail: solo el PASADO (n <= 0). `formatTimeAgo`, que es quien lo usa, nunca pasa
// un valor positivo, y generar el futuro sería doblar la tabla de los 26 idiomas por
// nada. Un n positivo cae en la forma del pasado, que diría «hace»: si algún día hace
// falta «dentro de 3 h», se añade `futuro` al lado de `pasado` en gen-i18n.ts.
func FormatRelativeTime(l Locale, n int, u Unidad) string {
	r, ok := fechas[l].Relativo[string(u)]
	if !ok {
		return ""
	}
	// `numeric: 'auto'` primero: es lo que da «ayer» en vez de «hace 1 día», y sale de
	// CLDR en los 26 idiomas sin necesidad de una clave de catálogo.
	if forma, ok := r.Auto[strconv.Itoa(n)]; ok {
		return forma
	}
	absoluto := n
	if absoluto < 0 {
		absoluto = -absoluto
	}
	plantilla, ok := r.Pasado[Category(l, float64(absoluto))]
	if !ok {
		if plantilla, ok = r.Pasado["other"]; !ok {
			return ""
		}
	}
	return strings.ReplaceAll(plantilla, "{n}", FormatNumber(l, float64(absoluto)))
}

// FormatTimeAgo es `formatTimeAgo` de format.ts: la antigüedad legible de un instante
// —«ahora», «hace 5 min», «ayer»— y, a partir de una semana, la fecha corta, que es más
// útil que «hace 43 días».
//
// El redondeo y los cortes son los de la web, cifra a cifra: lo que allí es
// `Math.round((now - date) / 1000)` aquí es lo mismo, y un instante en el FUTURO cae en
// «ahora» en los dos sitios.
func FormatTimeAgo(l Locale, cuando, ahora time.Time) string {
	redondear := func(f float64) int { return int(math.Floor(f + 0.5)) } // Math.round de JS
	segundos := redondear(ahora.Sub(cuando).Seconds())
	if segundos < 60 {
		return FormatRelativeTime(l, 0, Segundo)
	}
	if minutos := redondear(float64(segundos) / 60); minutos < 60 {
		return FormatRelativeTime(l, -minutos, Minuto)
	} else if horas := redondear(float64(minutos) / 60); horas < 24 {
		return FormatRelativeTime(l, -horas, Hora)
	} else if dias := redondear(float64(horas) / 24); dias < 7 {
		return FormatRelativeTime(l, -dias, Dia)
	}
	return FormatDayMonth(l, cuando)
}

// WeekdaysNarrow son las INICIALES de esos siete días (`weekdayNarrowLabels`): lo que
// cabe en las siete columnas del mini-calendario del menú.
func WeekdaysNarrow(l Locale) [7]string { return fechas[l].DiasEstrechos }

func componer(patron []parteFecha, f formatoFecha, t time.Time) string {
	doceHoras := false
	for _, p := range patron {
		doceHoras = doceHoras || p.Tipo == "dayPeriod"
	}
	var b strings.Builder
	numero := func(n int, dos bool) {
		if dos && n < 10 {
			b.WriteByte('0')
		}
		b.WriteString(strconv.Itoa(n))
	}
	for _, p := range patron {
		switch p.Tipo {
		case "literal":
			b.WriteString(p.Literal)
		case "day":
			numero(t.Day(), p.Dos)
		case "month":
			b.WriteString(f.Meses[t.Month()-1]) // ya trae su forma: nombre, «09» o «9»
		case "weekday":
			// Dias va de LUNES a domingo, como `weekdayLabels` de la web; time.Weekday
			// empieza en domingo. Sin el giro, un lunes se anuncia como martes.
			b.WriteString(f.Dias[(int(t.Weekday())+6)%7])
		case "year":
			b.WriteString(strconv.Itoa(t.Year()))
		case "hour":
			h := t.Hour()
			if doceHoras {
				if h %= 12; h == 0 {
					h = 12 // medianoche y mediodía son las 12, no las 0
				}
			}
			numero(h, p.Dos)
		case "dayPeriod":
			b.WriteString(f.Periodos[t.Hour()/12])
		case "minute":
			numero(t.Minute(), true)
		}
	}
	return b.String()
}

// FormatDateTimeLong es `toLocaleString(tag, { dateStyle: 'long', timeStyle: 'short' })`:
// «14 de septiembre de 2026, 12:00», «September 14, 2026 at 12:00 PM». Es la fecha de
// la ficha del visor de Sytue Fotos (`formatDateTime` con esas opciones). El conector
// entre fecha y hora es del idioma, y por eso tiene patrón propio.
func FormatDateTimeLong(l Locale, t time.Time) string {
	f := fechas[l]
	f.Meses = f.MesesFechaLargaHora
	return componer(f.FechaLargaHora, f, t)
}
