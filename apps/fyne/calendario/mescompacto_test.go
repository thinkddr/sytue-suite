// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image/color"
	"slices"
	"testing"
	"time"
	_ "time/tzdata" // V10 y V11 piden Europe/Madrid aunque el sistema no traiga zonas

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// VECTORES ESPEJO del mes compacto del teléfono. Los mismos, con el mismo nombre, están en
// la web (`modeloMesCompacto.test.ts`, y V12/V13 en `dates.test.ts`): si uno cambia allí,
// cambia aquí. V12 es TestV12MoverMesRecorta, en app_test.go, con `Desplazar`.
//
// Las fechas van en hora local de la zona del test: UTC salvo V10 y V11, que se prueban
// en Europe/Madrid porque son sus cambios de hora los que vigilan.
//
// Vistos en ROJO: con `DiasDelMes` devolviendo siempre seis semanas (V2–V4, V10), con la
// semana contada con `YearDay()/7+1` en vez de `ISOWeek` (V1, V3, V4), sin recortar los
// puntos a MaxPuntos (V8), sin ordenar (V9, V10), con el fin INCLUSIVO en el reparto (V6,
// V7: el que acaba a las 00:00 del 1-dic salía en el 1), con los días de otro mes
// pintados en vez de hueco (V1, V3, V6) y con `OtroMes` siempre falso en la semana
// plegada (V14).

var (
	colA = color.NRGBA{R: 0xa0, B: 0x01, A: 0xff}
	colB = color.NRGBA{R: 0xa0, B: 0x02, A: 0xff}
	colC = color.NRGBA{R: 0xa0, B: 0x03, A: 0xff}
	colD = color.NRGBA{R: 0xa0, B: 0x04, A: 0xff}
	colE = color.NRGBA{R: 0xa0, B: 0x05, A: 0xff}
)

// conHora es un evento con hora de [ini, fin); fin cero es sin fin.
func conHora(titulo string, ini, fin time.Time, c color.NRGBA) Ocurrencia {
	return Ocurrencia{Color: c, Instance: api.Instance{
		Event: api.Event{ID: titulo, Summary: titulo, DTStart: ini, DTEnd: fin},
		Start: ini, End: fin,
	}}
}

// todoElDia es un evento de día completo de `desde` a `hasta` (DTEND exclusivo), como lo
// trae el servicio: la fecha flotante en UTC.
func todoElDia(titulo string, desde, hasta time.Time, c color.NRGBA) Ocurrencia {
	a := time.Date(desde.Year(), desde.Month(), desde.Day(), 0, 0, 0, 0, time.UTC)
	b := time.Date(hasta.Year(), hasta.Month(), hasta.Day(), 0, 0, 0, 0, time.UTC)
	return Ocurrencia{Color: c, Instance: api.Instance{
		Event: api.Event{ID: titulo, Summary: titulo, AllDay: true, DTStart: a, DTEnd: b},
		Start: a, End: b,
	}}
}

func fecha(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func numeros(s []SemanaCompacta) []int {
	out := make([]int, len(s))
	for i, w := range s {
		out[i] = w.Numero
	}
	return out
}

// delMes es el día `d` del mes `m` en las semanas, o nil.
func delMes(s []SemanaCompacta, d int, m time.Month) *DiaCompacto {
	for i := range s {
		for j := range s[i].Dias {
			x := &s[i].Dias[j]
			if !x.Hueco && x.Fecha.Day() == d && x.Fecha.Month() == m {
				return x
			}
		}
	}
	return nil
}

func huecos(w SemanaCompacta) int {
	n := 0
	for _, d := range w.Dias {
		if d.Hueco {
			n++
		}
	}
	return n
}

func TestV1Nov2026EmpiezaEnDomingo(t *testing.T) {
	s := SemanasCompactas(fecha(2026, 11, 6), fecha(2026, 11, 6), time.UTC, nil)
	if got := numeros(s); !slices.Equal(got, []int{44, 45, 46, 47, 48, 49}) {
		t.Fatalf("semanas %v", got)
	}
	if huecos(s[0]) != 6 || s[0].Dias[6].Fecha.Day() != 1 {
		t.Errorf("fila 0: %d huecos y el día 1 en la col 6 (%v)", huecos(s[0]), s[0].Dias[6].Fecha)
	}
	if s[5].Dias[0].Fecha.Day() != 30 || huecos(s[5]) != 6 {
		t.Errorf("fila 5: el 30 en la col 0 y 6 huecos (%v, %d)", s[5].Dias[0].Fecha, huecos(s[5]))
	}
	seis := delMes(s, 6, 11)
	if !seis.Hoy || !seis.Elegido {
		t.Errorf("el 6 es hoy (%v) y elegido (%v)", seis.Hoy, seis.Elegido)
	}
	marcados := 0
	for _, w := range s {
		for _, d := range w.Dias {
			if d.Hoy || d.Elegido {
				marcados++
			}
		}
	}
	if marcados != 1 {
		t.Errorf("%d días marcados como hoy o elegido y solo es el 6", marcados)
	}
}

func TestV2Feb2027CuatroSemanas(t *testing.T) {
	s := SemanasCompactas(fecha(2027, 2, 10), fecha(2026, 11, 6), time.UTC, nil)
	if got := numeros(s); !slices.Equal(got, []int{5, 6, 7, 8}) {
		t.Fatalf("semanas %v", got)
	}
	for _, w := range s {
		if huecos(w) != 0 {
			t.Errorf("la semana %d tiene %d huecos", w.Numero, huecos(w))
		}
	}
}

func TestV3Oct2026CincoSemanas(t *testing.T) {
	s := SemanasCompactas(fecha(2026, 10, 15), fecha(2026, 11, 6), time.UTC, nil)
	if got := numeros(s); !slices.Equal(got, []int{40, 41, 42, 43, 44}) {
		t.Fatalf("semanas %v", got)
	}
	if huecos(s[0]) != 3 {
		t.Errorf("fila 0 con %d huecos y son 3", huecos(s[0]))
	}
}

func TestV4Ene2027Semana53Y1(t *testing.T) {
	s := SemanasCompactas(fecha(2027, 1, 15), fecha(2026, 11, 6), time.UTC, nil)
	if got := numeros(s); !slices.Equal(got, []int{53, 1, 2, 3, 4}) {
		t.Errorf("semanas %v", got)
	}
}

func TestV5Dic2024Semana1DelSiguiente(t *testing.T) {
	s := SemanasCompactas(fecha(2024, 12, 10), fecha(2026, 11, 6), time.UTC, nil)
	if got := numeros(s); !slices.Equal(got, []int{48, 49, 50, 51, 52, 1}) {
		t.Errorf("semanas %v", got)
	}
}

func TestV6DiaCompletoCruzaMeses(t *testing.T) {
	e := []Ocurrencia{todoElDia("Viaje", fecha(2026, 10, 30), fecha(2026, 11, 3), colA)}
	hoy := fecha(2026, 11, 6)
	nov := SemanasCompactas(fecha(2026, 11, 6), hoy, time.UTC, e)
	for d, quiero := range map[int]int{1: 1, 2: 1, 3: 0} {
		if got := len(delMes(nov, d, 11).Puntos); got != quiero {
			t.Errorf("nov: el %d con %d puntos y son %d", d, got, quiero)
		}
	}
	for _, d := range nov[0].Dias[:6] {
		if !d.Hueco || len(d.Puntos) != 0 {
			t.Errorf("el 30 y el 31 de octubre son huecos sin puntos en noviembre: %+v", d)
		}
	}
	oct := SemanasCompactas(fecha(2026, 10, 6), hoy, time.UTC, e)
	for d, quiero := range map[int]int{29: 0, 30: 1, 31: 1} {
		if got := len(delMes(oct, d, 10).Puntos); got != quiero {
			t.Errorf("oct: el %d con %d puntos y son %d", d, got, quiero)
		}
	}
	filas := EventosDelDia(fecha(2026, 11, 2), time.UTC, e)
	if len(filas) != 1 || !filas[0].TodoElDia || filas[0].Cruza {
		t.Errorf("el 2-nov: %+v", filas)
	}
}

func TestV7ConHoraCruzaMeses(t *testing.T) {
	e := []Ocurrencia{
		conHora("Guardia", time.Date(2026, 11, 30, 22, 0, 0, 0, time.UTC), time.Date(2026, 12, 1, 2, 0, 0, 0, time.UTC), colA),
		conHora("Cierre", time.Date(2026, 11, 30, 23, 0, 0, 0, time.UTC), time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), colB),
	}
	hoy := fecha(2026, 11, 6)
	nov := SemanasCompactas(fecha(2026, 11, 6), hoy, time.UTC, e)
	if got := delMes(nov, 30, 11).Puntos; !slices.Equal(got, []color.NRGBA{colA, colB}) {
		t.Errorf("el 30-nov con %v", got)
	}
	dic := SemanasCompactas(fecha(2026, 12, 6), hoy, time.UTC, e)
	if got := delMes(dic, 1, 12).Puntos; !slices.Equal(got, []color.NRGBA{colA}) {
		t.Errorf("el 1-dic con %v: el que acaba a las 00:00 no lo cubre", got)
	}
	filas := EventosDelDia(fecha(2026, 12, 1), time.UTC, e)
	if len(filas) != 1 || filas[0].O.Event.Summary != "Guardia" || !filas[0].Cruza {
		t.Errorf("el 1-dic: %+v", filas)
	}
}

func TestV8MasDeTresTresPuntos(t *testing.T) {
	e := make([]Ocurrencia, 0, 5)
	for i, c := range []color.NRGBA{colA, colB, colC, colD, colE} {
		ini := time.Date(2026, 11, 9, 8+i, 0, 0, 0, time.UTC)
		e = append(e, conHora(string(rune('A'+i)), ini, time.Time{}, c))
	}
	nueve := delMes(SemanasCompactas(fecha(2026, 11, 6), fecha(2026, 11, 6), time.UTC, e), 9, 11)
	if MaxPuntos != 3 || !slices.Equal(nueve.Puntos, []color.NRGBA{colA, colB, colC}) || nueve.Eventos != 5 {
		t.Errorf("el 9: puntos %v, eventos %d", nueve.Puntos, nueve.Eventos)
	}
}

func TestV9TodoElDiaPrimero(t *testing.T) {
	e := []Ocurrencia{
		conHora("A", time.Date(2026, 11, 16, 8, 0, 0, 0, time.UTC), time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC), colA),
		todoElDia("B", fecha(2026, 11, 16), fecha(2026, 11, 17), colB),
		conHora("C", time.Date(2026, 11, 15, 23, 0, 0, 0, time.UTC), time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC), colC),
	}
	d := delMes(SemanasCompactas(fecha(2026, 11, 6), fecha(2026, 11, 6), time.UTC, e), 16, 11)
	if !slices.Equal(d.Puntos, []color.NRGBA{colB, colC, colA}) {
		t.Errorf("puntos %v: B, C, A", d.Puntos)
	}
	orden := make([]string, 0, 3)
	for _, f := range EventosDelDia(fecha(2026, 11, 16), time.UTC, e) {
		orden = append(orden, f.O.Event.Summary)
	}
	if !slices.Equal(orden, []string{"B", "C", "A"}) {
		t.Errorf("orden %v", orden)
	}
}

// TestSinColorPropioElPuntoEsElAcento: `event.color ?? accent`.
func TestSinColorPropioElPuntoEsElAcento(t *testing.T) {
	e := []Ocurrencia{conHora("Tenis", time.Date(2026, 11, 6, 14, 0, 0, 0, time.UTC), time.Date(2026, 11, 6, 15, 0, 0, 0, time.UTC), color.NRGBA{})}
	d := delMes(SemanasCompactas(fecha(2026, 11, 6), fecha(2026, 11, 6), time.UTC, e), 6, 11)
	if !slices.Equal(d.Puntos, []color.NRGBA{AcentoPorDefecto}) {
		t.Errorf("puntos %v y es el acento", d.Puntos)
	}
}

func madrid(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Fatalf("sin la zona Europe/Madrid V10 y V11 no probarían nada: %v", err)
	}
	return loc
}

func TestV10CambioHoraOctubre(t *testing.T) {
	loc := madrid(t)
	e := []Ocurrencia{
		conHora("Tarde", time.Date(2026, 10, 25, 22, 30, 0, 0, time.UTC), time.Time{}, colA),
		todoElDia("Fiesta", fecha(2026, 10, 25), fecha(2026, 10, 26), colB),
	}
	oct := SemanasCompactas(time.Date(2026, 10, 6, 0, 0, 0, 0, loc), time.Date(2026, 11, 6, 0, 0, 0, 0, loc), loc, e)
	if len(oct) != 5 {
		t.Fatalf("octubre en %d filas y son 5", len(oct))
	}
	if got := delMes(oct, 25, 10).Puntos; !slices.Equal(got, []color.NRGBA{colB, colA}) {
		t.Errorf("el 25 con %v", got)
	}
	for _, d := range []int{24, 26} {
		if got := delMes(oct, d, 10).Puntos; len(got) != 0 {
			t.Errorf("el %d con %v", d, got)
		}
	}
}

func TestV11CambioHoraMarzo(t *testing.T) {
	loc := madrid(t)
	// 00:30 del 29 de marzo en Madrid (CET, +1) = 23:30Z del 28.
	e := []Ocurrencia{conHora("Madrugada", time.Date(2026, 3, 28, 23, 30, 0, 0, time.UTC), time.Time{}, colA)}
	mar := SemanasCompactas(time.Date(2026, 3, 6, 0, 0, 0, 0, loc), time.Date(2026, 11, 6, 0, 0, 0, 0, loc), loc, e)
	for d, quiero := range map[int]int{28: 0, 29: 1, 30: 0} {
		if got := len(delMes(mar, d, 3).Puntos); got != quiero {
			t.Errorf("el %d-mar con %d puntos y son %d", d, got, quiero)
		}
	}
}

func TestV13Titulo(t *testing.T) {
	hoy := fecha(2026, 11, 6)
	for _, c := range []struct {
		l      i18n.Locale
		cursor time.Time
		quiero string
	}{
		{"es", fecha(2026, 11, 6), "noviembre"},
		{"es", fecha(2027, 1, 15), "enero de 2027"},
		{"en", fecha(2026, 11, 6), "November"},
		{"en", fecha(2027, 1, 15), "January 2027"},
		// El griego en nominativo, como el mini-mes: `{ month: 'long' }` sin año daba el
		// genitivo («Νοεμβρίου»).
		{"el", fecha(2026, 11, 6), "Νοέμβριος"},
		{"el", fecha(2027, 1, 15), "Ιανουάριος 2027"},
	} {
		if got := TituloCompacto(c.l, c.cursor, hoy, time.UTC); got != c.quiero {
			t.Errorf("%s %s: %q y la web da %q", c.l, c.cursor.Format(time.DateOnly), got, c.quiero)
		}
	}
}

// TestV13TituloEnLaZonaDeLaPantalla: el título lee el cursor en la zona de la pantalla,
// como la rejilla. 31-dic-2026 23:30Z es ya el 1 de enero de 2027 en Madrid. Solo Go: la
// web no tiene zona de visualización aparte de la del navegador. VISTO EN ROJO sin el
// `.In(loc)` (decía «diciembre», del año de hoy, sobre la rejilla de enero).
func TestV13TituloEnLaZonaDeLaPantalla(t *testing.T) {
	loc := madrid(t)
	cursor := time.Date(2026, 12, 31, 23, 30, 0, 0, time.UTC)
	if got := TituloCompacto("es", cursor, time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC), loc); got != "enero de 2027" {
		t.Errorf("en Madrid el título es %q y es «enero de 2027»", got)
	}
	// Y hoy también: el 31-dic a las 23:30Z ya es 2027 en Madrid, así que enero va sin año.
	if got := TituloCompacto("es", cursor, cursor, loc); got != "enero" {
		t.Errorf("con hoy en el mismo instante el título es %q y es «enero»", got)
	}
}

func TestV14PlegadaCruzaMes(t *testing.T) {
	e := []Ocurrencia{todoElDia("Viaje", fecha(2026, 10, 30), fecha(2026, 10, 31), colA)}
	w := SemanaPlegada(fecha(2026, 11, 1), fecha(2026, 11, 6), time.UTC, e)
	if w.Numero != 44 {
		t.Errorf("semana %d y es la 44", w.Numero)
	}
	for i, d := range w.Dias[:6] {
		if d.Hueco || !d.OtroMes || d.Fecha.Day() != 26+i {
			t.Errorf("col %d: %+v y es el %d-oct, de otro mes y con fecha", i, d, 26+i)
		}
	}
	if len(w.Dias[4].Puntos) != 1 {
		t.Errorf("el 30-oct con %v puntos", w.Dias[4].Puntos)
	}
	if d := w.Dias[6]; d.Fecha.Day() != 1 || !d.Elegido || d.OtroMes {
		t.Errorf("el 1-nov: %+v", d)
	}
}

// TestLunesDe: la regla única del primer día de la semana. El domingo es del lunes
// ANTERIOR (no del siguiente) y la hora se conserva. VISTO EN ROJO con `int(Weekday())`
// sin rotar (el domingo 8-nov daba el propio 8 y el lunes 9 daba el 8).
func TestLunesDe(t *testing.T) {
	for dia, quiero := range map[int]int{2: 2, 6: 2, 8: 2, 9: 9} {
		got := LunesDe(time.Date(2026, 11, dia, 19, 36, 0, 0, time.UTC))
		if got.Day() != quiero || got.Hour() != 19 || got.Weekday() != time.Monday {
			t.Errorf("LunesDe(%d-nov) = %v y es el %d a las 19:36", dia, got, quiero)
		}
	}
}
