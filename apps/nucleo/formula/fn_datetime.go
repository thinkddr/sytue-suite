// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Fecha y hora (functions/datetime.ts).
//
// SISTEMA DE FECHA SERIE (ECMA-376 §18.17.4.1, «sistema 1900»): la parte entera cuenta
// días y la fraccional la hora; el serial 1 es 1900-01-01. Se REPRODUCE el bug histórico
// de Lotus 1-2-3 —1900 tratado como bisiesto, con su día fantasma 1900-02-29 = 60—, así
// que todo serial ≥ 61 coincide EXACTAMENTE con Excel. Desviaciones deliberadas del
// original, sin valor práctico: DATE(1900,2,29) normaliza a 61 y daysInMonth(1900,2)=28.
//
// Las cuentas van en float64, como en el original: un serial absurdo da NaN y no un
// pánico, y NaN acaba en #NUM! al mostrarse.

const secsPorDia = 86_400

// jsS es lo que `\s` significa en una regex de JavaScript (la de Go es solo ASCII).
const jsS = `[\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}]`

var diasDelMes = [12]float64{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}

// diasCivil son los días desde 1970-01-01 de (año, mes 1-based, día), con el mes y el
// día desbordables como en Date.setUTCFullYear. NaN fuera del rango de un Date.
func diasCivil(y, m, d float64) float64 {
	if math.IsNaN(y+m+d) || math.Abs(y) > 400_000 || math.Abs(m) > 4_000_000 || math.Abs(d) > 2e8 {
		return math.NaN()
	}
	mes := int64(m) - 1
	año := int64(y) + mes/12
	mes %= 12
	if mes < 0 {
		mes += 12
		año--
	}
	// Algoritmo de días desde la fecha civil (H. Hinnant), con marzo como mes 0.
	if mes < 2 {
		año--
	}
	era := año / 400
	if año < 0 {
		era = (año - 399) / 400
	}
	yoe := año - era*400
	mp := (mes + 10) % 12
	doy := (153*mp + 2) / 5 // el día 1 de ese mes
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	dias := float64(era*146097+doe-719468) + d - 1
	if math.Abs(dias) > 1e8 {
		return math.NaN()
	}
	return dias
}

// epoca es el serial 0: 1899-12-31.
var epoca = diasCivil(1899, 12, 31)

type ymd struct{ y, m, d float64 }

// civilDe es la fecha civil de un número de días desde 1970-01-01.
func civilDe(dias float64) ymd {
	if math.IsNaN(dias) || math.Abs(dias) > 1e8 {
		return ymd{math.NaN(), math.NaN(), math.NaN()}
	}
	z := int64(math.Floor(dias)) + 719468
	era := z / 146097
	if z < 0 {
		era = (z - 146096) / 146097
	}
	doe := z - era*146097
	yoe := (doe - doe/1460 + doe/36524 - doe/146096) / 365
	doy := doe - (365*yoe + yoe/4 - yoe/100)
	mp := (5*doy + 2) / 153
	d := doy - (153*mp+2)/5 + 1
	m := mp + 3
	if mp >= 10 {
		m = mp - 9
	}
	y := yoe + era*400
	if m <= 2 {
		y++
	}
	return ymd{float64(y), float64(m), float64(d)}
}

func isLeap(y float64) bool {
	return (math.Mod(y, 4) == 0 && math.Mod(y, 100) != 0) || math.Mod(y, 400) == 0
}

func daysInMonth(y, m float64) float64 {
	if m == 2 {
		if isLeap(y) {
			return 29
		}
		return 28
	}
	if m >= 1 && m <= 12 && m == math.Trunc(m) {
		return diasDelMes[int(m)-1]
	}
	return 30
}

// serialFromYMD es (año, mes, día) → serial entero, con el día fantasma 1900-02-29 = 60.
func serialFromYMD(y, m, d float64) float64 {
	s := diasCivil(y, m, d) - epoca
	if s > 59 {
		s++ // añade el inexistente 29-feb-1900
	}
	return s
}

// maxSerial cierra el rango válido [0, 2958465] = 1900-01-00 … 9999-12-31.
var maxSerial = serialFromYMD(9999, 12, 31)

// ymdFromSerial es serial entero → fecha, con las rarezas de Excel en los dos seriales
// especiales del sistema 1900.
func ymdFromSerial(dia float64) ymd {
	switch {
	case dia == 0:
		return ymd{1900, 1, 0} // el «1900-01-00» de Excel
	case dia == 60:
		return ymd{1900, 2, 29} // día fantasma
	case dia > 60:
		dia--
	}
	return civilDe(epoca + dia)
}

// dowSun es el día de la semana, 0=domingo … 6=sábado (el serial 1 es domingo).
func dowSun(dia float64) float64 { return math.Mod(math.Mod(dia-1, 7)+7, 7) }

func isWeekend(dia float64) bool {
	w := dowSun(dia)
	return w == 0 || w == 6
}

/* ---------- Texto de fecha y hora ---------- */

// checkedDate valida una fecha real y da su serial.
func checkedDate(y, m, d float64) (float64, bool) {
	if m < 1 || m > 12 || d < 1 || d > daysInMonth(y, m) {
		return 0, false
	}
	s := serialFromYMD(y, m, d)
	return s, !(s < 0)
}

// expandYear expande un año de 2 cifras a la ventana de Excel (0-29 → 2000+).
func expandYear(y float64) float64 {
	switch {
	case y >= 100:
		return y
	case y < 30:
		return y + 2000
	}
	return y + 1900
}

var (
	reFechaISO  = regexp.MustCompile(`^(\d{4})-(\d{1,2})-(\d{1,2})$`)
	reFechaBar  = regexp.MustCompile(`^(\d{1,4})/(\d{1,2})/(\d{1,4})$`)
	reHora      = regexp.MustCompile(`^(\d{1,2}):(\d{1,2})(?::(\d{1,2}))?` + jsS + `*([APap][Mm])?$`)
	reSieteBits = regexp.MustCompile(`^[01]{7}$`)
)

func atof(s string) float64 {
	n, _ := strconv.ParseFloat(s, 64)
	return n
}

// parseDate acepta ISO «AAAA-M-D», «AAAA/M/D» y US «M/D/AAAA».
func parseDate(s string) (float64, bool) {
	if m := reFechaISO.FindStringSubmatch(s); m != nil {
		return checkedDate(atof(m[1]), atof(m[2]), atof(m[3]))
	}
	m := reFechaBar.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	a, b, c := atof(m[1]), atof(m[2]), atof(m[3])
	if len(m[1]) == 4 || a > 31 {
		return checkedDate(expandYear(a), b, c) // año primero
	}
	mes, dia := a, b
	if mes > 12 && dia <= 12 {
		mes, dia = dia, mes // «D/M/AAAA» si el primero no es mes
	}
	return checkedDate(expandYear(c), mes, dia)
}

// parseTime es «H:M» o «H:M:S», con AM/PM opcional → fracción de día [0, 1).
func parseTime(s string) (float64, bool) {
	m := reHora.FindStringSubmatch(jsTrim(s))
	if m == nil {
		return 0, false
	}
	h, mi, se := atof(m[1]), atof(m[2]), atof(m[3])
	if mi > 59 || se > 59 {
		return 0, false
	}
	switch strings.ToUpper(m[4]) {
	case "":
		if h > 23 {
			return 0, false
		}
	case "PM":
		if h < 1 || h > 12 {
			return 0, false
		}
		if h != 12 {
			h += 12
		}
	case "AM":
		if h < 1 || h > 12 {
			return 0, false
		}
		if h == 12 {
			h = 0
		}
	}
	return (h*3600 + mi*60 + se) / secsPorDia, true
}

// palabras es `s.split(/\s+/)` sobre un texto ya recortado.
func palabras(s string) []string {
	if p := strings.FieldsFunc(s, jsEspacio); len(p) > 0 {
		return p
	}
	return []string{""}
}

// parseDateTimeText es fecha con hora opcional (o solo hora) → serial.
func parseDateTimeText(crudo string) (float64, bool) {
	s := jsTrim(crudo)
	if t, ok := parseTime(s); ok {
		return t, true
	}
	p := palabras(s)
	d, ok := parseDate(p[0])
	if !ok || len(p) == 1 {
		return d, ok
	}
	t, ok := parseTime(strings.Join(p[1:], " "))
	return d + t, ok
}

/* ---------- Coacción de argumentos ---------- */

// toSerial coacciona a serial: número directo o texto de fecha/hora. Los errores se
// PROPAGAN, no se enmascaran como #VALUE!.
func toSerial(v Value) (float64, Error) {
	n, err := CoerceNumber(v)
	if err == "" {
		return n, ""
	}
	if s, ok := v.(string); ok {
		if n, ok := parseDateTimeText(s); ok {
			return n, ""
		}
		return 0, ErrValue
	}
	return 0, err
}

func serialArg(args []*Arg, i int) (float64, Error) {
	if i >= len(args) {
		return 0, ErrValue
	}
	return toSerial(args[i].Value())
}

// gatherHolidays reúne los seriales de un rango de festivos (blancos ignorados).
func gatherHolidays(args []*Arg, i int) (map[float64]bool, Error) {
	set := map[float64]bool{}
	if i >= len(args) {
		return set, ""
	}
	for _, fila := range args[i].Matrix() {
		for _, v := range fila {
			if v == "" {
				continue
			}
			s, err := toSerial(v)
			if err != "" {
				return nil, err
			}
			set[math.Floor(s)] = true
		}
	}
	return set, ""
}

// countWeekdays cuenta lunes-viernes entre a y b inclusive, con a ≤ b.
func countWeekdays(a, b float64) float64 {
	dias := b - a + 1
	cuenta := math.Floor(dias/7) * 5
	inicio := dowSun(a)
	for i := 0.0; i < math.Mod(dias, 7); i++ {
		if w := math.Mod(inicio+i, 7); w >= 1 && w <= 5 {
			cuenta++
		}
	}
	return cuenta
}

// codigosFinde: códigos de WORKDAY.INTL → días dowSun. 1-7 son dos días; 11-17, uno.
var codigosFinde = map[float64][]float64{
	1: {6, 0}, 2: {0, 1}, 3: {1, 2}, 4: {2, 3}, 5: {3, 4}, 6: {4, 5}, 7: {5, 6},
	11: {0}, 12: {1}, 13: {2}, 14: {3}, 15: {4}, 16: {5}, 17: {6},
}

// weekendSet resuelve el argumento `fin_de_semana`. Ausente → sábado y domingo. Código
// inválido → #NUM!; cadena que no sea «[01]{7}» o toda de unos → #VALUE!.
func weekendSet(args []*Arg, i int) (map[float64]bool, Error) {
	if i >= len(args) {
		return map[float64]bool{0: true, 6: true}, ""
	}
	v := args[i].Value()
	set := map[float64]bool{}
	if s, ok := v.(string); ok {
		if !reSieteBits.MatchString(s) || s == "1111111" {
			return nil, ErrValue
		}
		for pos, dow := range [7]float64{1, 2, 3, 4, 5, 6, 0} { // «LMXJVSD» → dowSun
			if s[pos] == '1' {
				set[dow] = true
			}
		}
		return set, ""
	}
	n, err := CoerceNumber(v)
	if err != "" {
		return nil, err
	}
	dias, ok := codigosFinde[math.Trunc(n)]
	if !ok {
		return nil, ErrNum
	}
	for _, d := range dias {
		set[d] = true
	}
	return set, ""
}

// dateParts son los componentes de un argumento serial; #NUM! si es negativo.
func dateParts(args []*Arg, i int) (ymd, Error) {
	s, err := serialArg(args, i)
	if err != "" {
		return ymd{}, err
	}
	if s < 0 {
		return ymd{}, ErrNum
	}
	return ymdFromSerial(math.Floor(s)), ""
}

// daySeconds son los segundos del día [0, 86400) de un argumento serial.
func daySeconds(args []*Arg, i int) (float64, Error) {
	s, err := serialArg(args, i)
	if err != "" {
		return 0, err
	}
	if s < 0 {
		return 0, ErrNum
	}
	return math.Mod(jsRound((s-math.Floor(s))*secsPorDia), secsPorDia), ""
}

// shiftMonth desplaza `months` meses: mismo día (recortado) o fin de mes. Lo usa también
// el calendario de cupones, que retrocede meses desde el vencimiento.
func shiftMonth(serial, months float64, finDeMes bool) float64 {
	p := ymdFromSerial(math.Floor(serial))
	idx := p.y*12 + (p.m - 1) + months
	ny := math.Floor(idx / 12)
	nm := math.Mod(math.Mod(idx, 12)+12, 12) + 1
	ultimo := daysInMonth(ny, nm)
	if finDeMes {
		return serialFromYMD(ny, nm, ultimo)
	}
	return serialFromYMD(ny, nm, math.Min(p.d, ultimo))
}

/* ---------- Fracción de año ---------- */

func daysInYear(y float64) float64 {
	if isLeap(y) {
		return 366
	}
	return 365
}

func isEndOfFeb(p ymd) bool { return p.m == 2 && p.d == daysInMonth(p.y, 2) }

// days360US son los días por la base 30/360 US (NASD).
func days360US(a, b ymd) float64 {
	d1, d2 := a.d, b.d
	if isEndOfFeb(a) && isEndOfFeb(b) {
		d2 = 30
	}
	if isEndOfFeb(a) {
		d1 = 30
	}
	if d1 == 31 {
		d1 = 30
	}
	if d2 == 31 && d1 == 30 {
		d2 = 30
	}
	return (b.y-a.y)*360 + (b.m-a.m)*30 + (d2 - d1)
}

// days360EU son los días por la base 30E/360 (europea).
func days360EU(a, b ymd) float64 {
	d1, d2 := a.d, b.d
	if d1 == 31 {
		d1 = 30
	}
	if d2 == 31 {
		d2 = 30
	}
	return (b.y-a.y)*360 + (b.m-a.m)*30 + (d2 - d1)
}

// actualActual es real/real: días reales sobre la longitud media de los años del tramo.
func actualActual(aSer, bSer float64, a, b ymd) float64 {
	dias := bSer - aSer
	if a.y == b.y {
		return dias / daysInYear(a.y)
	}
	total := 0.0
	for y := a.y; y <= b.y; y++ {
		total += daysInYear(y)
	}
	return dias / (total / (b.y - a.y + 1))
}

// isMonthEnd: ¿el serial cae en el último día de su mes? Si el vencimiento de un bono es
// fin de mes, TODOS sus cupones lo son, y eso mueve el devengado un día entero.
func isMonthEnd(serial float64) bool {
	p := ymdFromSerial(math.Floor(serial))
	return p.d == daysInMonth(p.y, p.m)
}

// dayCount son los DÍAS entre dos seriales por la base 0-4 (COUPDAYBS y familia).
func dayCount(a, b, base float64) (float64, Error) {
	pa, pb := ymdFromSerial(math.Floor(a)), ymdFromSerial(math.Floor(b))
	switch base {
	case 0:
		return days360US(pa, pb), ""
	case 1, 2, 3:
		return math.Floor(b) - math.Floor(a), ""
	case 4:
		return days360EU(pa, pb), ""
	}
	return 0, ErrNum
}

// yearFrac es la fracción de año entre dos seriales ya truncados, por la base 0-4.
// Simétrica. Núcleo de YEARFRAC y de las financieras de descuento.
func yearFrac(a, b, base float64) (float64, Error) {
	if a < 0 || b < 0 {
		return 0, ErrNum
	}
	if a == b {
		return 0, ""
	}
	if a > b {
		a, b = b, a
	}
	pa, pb := ymdFromSerial(a), ymdFromSerial(b)
	switch base {
	case 0:
		return days360US(pa, pb) / 360, ""
	case 1:
		return actualActual(a, b, pa, pb), ""
	case 2:
		return (b - a) / 360, ""
	case 3:
		return (b - a) / 365, ""
	case 4:
		return days360EU(pa, pb) / 360, ""
	}
	return 0, ErrNum
}

func valorOError(v float64, err Error) Value {
	if err != "" {
		return err
	}
	return v
}

/* ---------- Registro ---------- */

// parteFecha es YEAR, MONTH y DAY.
func parteFecha(parte func(ymd) float64) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 {
			return ErrValue
		}
		p, err := dateParts(args, 0)
		if err != "" {
			return err
		}
		return parte(p)
	}
}

// parteHora es HOUR, MINUTE y SECOND.
func parteHora(parte func(segundos float64) float64) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 {
			return ErrValue
		}
		t, err := daySeconds(args, 0)
		if err != "" {
			return err
		}
		return parte(t)
	}
}

// hoy es el serial de la fecha LOCAL del reloj inyectable, como Excel.
func hoy(c *FnCtx) (float64, float64) {
	d := c.Now()
	segundos := float64(d.Hour()*3600+d.Minute()*60+d.Second()) + float64(d.Nanosecond()/1e6)/1000
	return serialFromYMD(float64(d.Year()), float64(d.Month()), float64(d.Day())), segundos
}

// tipoSemana lee el segundo argumento opcional de WEEKDAY y WEEKNUM.
func tipoSemana(args []*Arg) (serial, tipo float64, err Error) {
	if len(args) < 1 || len(args) > 2 {
		return 0, 0, ErrValue
	}
	if serial, err = serialArg(args, 0); err != "" {
		return 0, 0, err
	}
	if serial < 0 {
		return 0, 0, ErrNum
	}
	if tipo, err = numOpc(args, 1, 1); err != "" {
		return 0, 0, err
	}
	return serial, math.Trunc(tipo), ""
}

// semanaISO es la semana ISO 8601: semanas de lunes; la 1 contiene el primer jueves.
func semanaISO(dia float64) float64 {
	isoDow := math.Mod(dowSun(dia)+6, 7) + 1 // 1=lunes … 7=domingo
	jueves := dia + (4 - isoDow)
	return math.Floor((jueves-serialFromYMD(ymdFromSerial(jueves).y, 1, 1))/7) + 1
}

// mesesDespues es EDATE y EOMONTH.
func mesesDespues(finDeMes bool) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		s, err := serialArg(args, 0)
		if err != "" {
			return err
		}
		if s < 0 {
			return ErrNum
		}
		mo, err := numArg(args, 1)
		if err != "" {
			return err
		}
		return enRango(shiftMonth(s, math.Trunc(mo), finDeMes))
	}
}

// enRango da #NUM! si una fecha PRODUCIDA se sale de [0, 9999-12-31].
func enRango(s float64) Value {
	if s < 0 || s > maxSerial {
		return ErrNum
	}
	return s
}

// topeLaborables corta un WORKDAY absurdo. ponytail: el original avanza día a día sin
// tope y cuelga la pestaña con 1e12 días; aquí eso es #NUM!, que es donde acabaría: el
// calendario entero (1900-01-00 … 9999-12-31) tiene 2.958.465 días.
const topeLaborables = 3e6

// laborable avanza `dias` laborables desde `cur` saltando lo que diga `libre`.
func laborable(cur, dias float64, libre func(dia float64) bool) Value {
	if cur < 0 {
		return ErrNum
	}
	resto, dir := math.Abs(math.Trunc(dias)), 1.0
	if dias < 0 {
		dir = -1
	}
	if resto > topeLaborables || math.IsNaN(resto) || cur > 1e15 {
		return ErrNum // por encima de 2^53 sumar un día ya no mueve la fecha
	}
	for resto > 0 {
		cur += dir
		if !libre(cur) {
			resto--
		}
	}
	return enRango(cur)
}

// dosFechas lee inicio y fin de NETWORKDAYS y los ordena; signo -1 si venían al revés.
func dosFechas(args []*Arg) (a, b, signo float64, err Error) {
	if a, err = serialArg(args, 0); err != "" {
		return a, 0, 0, err
	}
	if b, err = serialArg(args, 1); err != "" {
		return a, b, 0, err
	}
	a, b, signo = math.Floor(a), math.Floor(b), 1
	if a > b {
		a, b, signo = b, a, -1
	}
	return a, b, signo, err
}

// funcionesFecha es el registro de funciones de este fichero. Es un MAPA y no un init():
// el cuerpo de cada función vive dentro del literal, y medido como UNA función
// lo que sale es la suma de todos ellos, que no es una medida de nada.
var funcionesFecha = map[string]fn{
	"TODAY": func(c *FnCtx, args []*Arg) any {
		if len(args) != 0 {
			return ErrValue
		}
		dia, _ := hoy(c)
		return dia
	},
	"NOW": func(c *FnCtx, args []*Arg) any {
		if len(args) != 0 {
			return ErrValue
		}
		dia, segundos := hoy(c)
		return dia + segundos/secsPorDia
	},
	// Mes y día desbordan; año 0-1899 → +1900.
	"DATE": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 3 {
			return ErrValue
		}
		v, err := numArgs(args, 3)
		if err != "" {
			return err
		}
		y := math.Trunc(v[0])
		if y < 0 || y > 9999 {
			return ErrNum
		}
		if y < 1900 {
			y += 1900
		}
		return enRango(serialFromYMD(y, math.Trunc(v[1]), math.Trunc(v[2])))
	},
	// Envuelve módulo 24 h; negativo → #NUM!.
	"TIME": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 3 {
			return ErrValue
		}
		v, err := numArgs(args, 3)
		if err != "" {
			return err
		}
		total := math.Trunc(v[0])*3600 + math.Trunc(v[1])*60 + math.Trunc(v[2])
		if total < 0 {
			return ErrNum
		}
		return math.Mod(total, secsPorDia) / secsPorDia
	},
	"YEAR":   parteFecha(func(p ymd) float64 { return p.y }),
	"MONTH":  parteFecha(func(p ymd) float64 { return p.m }),
	"DAY":    parteFecha(func(p ymd) float64 { return p.d }),
	"HOUR":   parteHora(func(t float64) float64 { return math.Floor(t / 3600) }),
	"MINUTE": parteHora(func(t float64) float64 { return math.Mod(math.Floor(t/60), 60) }),
	"SECOND": parteHora(func(t float64) float64 { return math.Mod(t, 60) }),
	// Tipo 1 (domingo=1) por defecto; 2, 3 y 11-17 soportados.
	"WEEKDAY": func(_ *FnCtx, args []*Arg) any {
		s, tipo, err := tipoSemana(args)
		if err != "" {
			return err
		}
		w := dowSun(math.Floor(s))
		switch {
		case tipo == 1:
			return w + 1
		case tipo == 2:
			return math.Mod(w+6, 7) + 1
		case tipo == 3:
			return math.Mod(w+6, 7)
		case tipo >= 11 && tipo <= 17:
			return math.Mod(w-math.Mod(tipo-10, 7)+7, 7) + 1
		}
		return ErrNum
	},
	// Tipo 1 por defecto; 2, 11-17 (día de inicio) y 21 (ISO 8601).
	"WEEKNUM": func(_ *FnCtx, args []*Arg) any {
		s, tipo, err := tipoSemana(args)
		if err != "" {
			return err
		}
		dia, inicio := math.Floor(s), 0.0
		switch {
		case tipo == 21:
			return semanaISO(dia)
		case tipo == 1:
		case tipo == 2:
			inicio = 1
		case tipo >= 11 && tipo <= 17:
			inicio = math.Mod(tipo-10, 7)
		default:
			return ErrNum
		}
		ene1 := serialFromYMD(ymdFromSerial(dia).y, 1, 1)
		return math.Floor((dia-(ene1-math.Mod(dowSun(ene1)-inicio+7, 7)))/7) + 1
	},
	"EDATE":   mesesDespues(false),
	"EOMONTH": mesesDespues(true),
	"DATEDIF": datedif,
	// Laborables entre dos fechas, inclusive, menos festivos. Al revés → negativo.
	"NETWORKDAYS": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 || len(args) > 3 {
			return ErrValue
		}
		a, b, signo, err := dosFechas(args)
		if err != "" {
			return err
		}
		festivos, err := gatherHolidays(args, 2)
		if err != "" {
			return err
		}
		cuenta := countWeekdays(a, b)
		for h := range festivos {
			if h >= a && h <= b && !isWeekend(h) {
				cuenta--
			}
		}
		return signo * cuenta
	},
	"WORKDAY": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 || len(args) > 3 {
			return ErrValue
		}
		s, err := serialArg(args, 0)
		if err != "" {
			return err
		}
		dias, err := numArg(args, 1)
		if err != "" {
			return err
		}
		festivos, err := gatherHolidays(args, 2)
		if err != "" {
			return err
		}
		return laborable(math.Floor(s), dias, func(d float64) bool { return isWeekend(d) || festivos[d] })
	},
	"DAYS": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		fin, err := serialArg(args, 0)
		if err != "" {
			return err
		}
		ini, err := serialArg(args, 1)
		if err != "" {
			return err
		}
		return math.Floor(fin) - math.Floor(ini)
	},
	// Texto de fecha → serial entero (ignora la hora).
	"DATEVALUE": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 {
			return ErrValue
		}
		t, err := CoerceText(args[0].Value())
		if err != "" {
			return err
		}
		if d, ok := parseDate(palabras(jsTrim(t))[0]); ok {
			return d
		}
		return ErrValue
	},
	// Texto de hora → fracción de día (ignora la fecha).
	"TIMEVALUE": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 {
			return ErrValue
		}
		t, err := CoerceText(args[0].Value())
		if err != "" {
			return err
		}
		s := jsTrim(t)
		if f, ok := parseTime(s); ok {
			return f
		}
		if p := palabras(s); len(p) >= 2 {
			if f, ok := parseTime(strings.Join(p[1:], " ")); ok {
				return f
			}
		}
		return ErrValue
	},
	"ISOWEEKNUM": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 {
			return ErrValue
		}
		s, err := serialArg(args, 0)
		if err != "" {
			return err
		}
		if s < 0 {
			return ErrNum
		}
		return semanaISO(math.Floor(s))
	},
	// Bases: 0 US 30/360, 1 real/real, 2 real/360, 3 real/365, 4 europea 30E/360.
	"YEARFRAC": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 || len(args) > 3 {
			return ErrValue
		}
		s0, err := serialArg(args, 0)
		if err != "" {
			return err
		}
		e0, err := serialArg(args, 1)
		if err != "" {
			return err
		}
		base, err := numOpc(args, 2, 0)
		if err != "" {
			return err
		}
		return valorOError(yearFrac(math.Floor(s0), math.Floor(e0), math.Trunc(base)))
	},
	// DATESTR(fecha): serial → «AAAA-MM-DD» (el inverso de DATEVALUE).
	"DATESTR": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 {
			return ErrValue
		}
		p, err := dateParts(args, 0)
		if err != "" {
			return err
		}
		return padStart(jsNumero(p.y), 4) + "-" + padStart(jsNumero(p.m), 2) + "-" + padStart(jsNumero(p.d), 2)
	},
	// DAYS360(inicio, fin, [método]): FALSE = US (NASD); TRUE = europeo. No simétrica.
	"DAYS360": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 || len(args) > 3 {
			return ErrValue
		}
		s0, err := serialArg(args, 0)
		if err != "" {
			return err
		}
		e0, err := serialArg(args, 1)
		if err != "" {
			return err
		}
		euro := false
		if len(args) == 3 {
			if euro, err = CoerceBoolean(args[2].Value()); err != "" {
				return err
			}
		}
		a, b := math.Floor(s0), math.Floor(e0)
		if a < 0 || b < 0 {
			return ErrNum
		}
		if euro {
			return days360EU(ymdFromSerial(a), ymdFromSerial(b))
		}
		return days360US(ymdFromSerial(a), ymdFromSerial(b))
	},
	"NETWORKDAYS.INTL": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 || len(args) > 4 {
			return ErrValue
		}
		a, b, signo, err := dosFechas(args)
		if err != "" {
			return err
		}
		finde, err := weekendSet(args, 2)
		if err != "" {
			return err
		}
		festivos, err := gatherHolidays(args, 3)
		if err != "" {
			return err
		}
		if b-a > topeLaborables || math.Abs(b) > 1e15 {
			// ponytail: como en WORKDAY. Y por encima de 2^53 `d++` ya no avanza: el
			// original se queda girando para siempre con =NETWORKDAYS.INTL(1E+300;1E+300).
			return ErrNum
		}
		cuenta := 0.0
		// ponytail: recorrido día a día; basta para el rango real de uso.
		for d := a; d <= b; d++ {
			if !finde[dowSun(d)] && !festivos[d] {
				cuenta++
			}
		}
		return signo * cuenta
	},
	"WORKDAY.INTL": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 || len(args) > 4 {
			return ErrValue
		}
		s, err := serialArg(args, 0)
		if err != "" {
			return err
		}
		dias, err := numArg(args, 1)
		if err != "" {
			return err
		}
		finde, err := weekendSet(args, 2)
		if err != "" {
			return err
		}
		festivos, err := gatherHolidays(args, 3)
		if err != "" {
			return err
		}
		return laborable(math.Floor(s), dias, func(d float64) bool { return finde[dowSun(d)] || festivos[d] })
	},
}

func init() { registrar(funcionesFecha) }

// padStart es `String.prototype.padStart(n, '0')`.
func padStart(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return strings.Repeat("0", n-len(s)) + s
}

// datedif es la diferencia entre fechas por unidad (heredada; inicio ≤ fin).
func datedif(_ *FnCtx, args []*Arg) any {
	if len(args) != 3 {
		return ErrValue
	}
	sS, err := serialArg(args, 0)
	if err != "" {
		return err
	}
	eS, err := serialArg(args, 1)
	if err != "" {
		return err
	}
	unidad, err := CoerceText(args[2].Value())
	if err != "" {
		return err
	}
	ini, fin := math.Floor(sS), math.Floor(eS)
	if ini < 0 || fin < ini {
		return ErrNum
	}
	s, e := ymdFromSerial(ini), ymdFromSerial(fin)
	meses := (e.y-s.y)*12 + (e.m - s.m)
	if e.d < s.d {
		meses--
	}
	switch jsUpper(jsTrim(unidad)) {
	case "D":
		return fin - ini
	case "Y":
		y := e.y - s.y
		if e.m < s.m || (e.m == s.m && e.d < s.d) {
			y--
		}
		return y
	case "M":
		return meses
	case "YM":
		return math.Mod(math.Mod(meses, 12)+12, 12)
	case "MD":
		// ponytail: «MD» es notoriamente inestable en Excel cerca de fin de mes; se
		// implementa la definición estándar, bug incluido.
		dd := e.d - s.d
		if dd < 0 {
			pm, py := e.m-1, e.y
			if e.m == 1 {
				pm, py = 12, e.y-1
			}
			dd += daysInMonth(py, pm)
		}
		return dd
	case "YD":
		a := serialFromYMD(e.y, s.m, s.d)
		if a > fin {
			a = serialFromYMD(e.y-1, s.m, s.d)
		}
		return fin - a
	}
	return ErrNum
}
