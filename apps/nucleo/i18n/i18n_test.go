// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package i18n

import (
	"encoding/json"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// testdata/intl.json lo escribe gen-i18n.ts con el motor Intl de verdad: son las
// respuestas de la web, no las de este puerto.
type vectores struct {
	Cantidades []float64 `json:"cantidades"`
	Numeros    []float64 `json:"numeros"`
	Intl       map[Locale]struct {
		Plural []string `json:"plural"`
		Numero []string `json:"numero"`
	} `json:"intl"`
	Instantes  []time.Time `json:"instantes"`
	IntlFechas map[Locale]struct {
		Fecha          []string `json:"fecha"`
		Hora           []string `json:"hora"`
		HoraDos        []string `json:"hora_dos"`
		MesAno         []string `json:"mes_ano"`
		MesCortoAno    []string `json:"mes_corto_ano"`
		DiaMes         []string `json:"dia_mes"`
		FechaDia2      []string `json:"fecha_dia2"`
		FechaLarga     []string `json:"fecha_larga"`
		DiaMesLargo    []string `json:"dia_mes_largo"`
		FechaLargaHora []string `json:"fecha_larga_hora"`
		FechaCompleta  []string `json:"fecha_completa"`
		MesCorto       []string `json:"mes_corto"`
		DiaCorto       []string `json:"dia_corto"`
		RangoSemana    []string `json:"rango_semana"`
		RangoLargo     []string `json:"rango_largo"`
	} `json:"intl_fechas"`
	Tamanos        []float64                      `json:"tamanos"`
	IntlBytes      map[Locale][]string            `json:"intl_bytes"`
	Porcentajes    []float64                      `json:"porcentajes"`
	IntlPorcentaje map[Locale][]string            `json:"intl_porcentaje"`
	Importes       []int64                        `json:"importes"`
	IntlMoneda     map[Locale][]string            `json:"intl_moneda"`
	IntlMonedas    map[Locale]map[string][]string `json:"intl_monedas"`
	Relativos      [][2]any                       `json:"relativos"`
	IntlRelativo   map[Locale][]string            `json:"intl_relativo"`
	Listas         [][]string                     `json:"listas"`
	IntlLista      map[Locale][]string            `json:"intl_lista"`
}

// TestFormatDateShortMonthEsElDeIntl: la fecha de una lista de ficheros, con el día a
// dos cifras y el mes por su nombre (`{ day: '2-digit', month: 'short', year:
// 'numeric' }`, el `formatDate` de FileBrowser.tsx).
//
// Tiene su propio vector porque NO es la fecha media con el día rellenado: medido el
// 2026-09-22 en los 26 idiomas, en alemán el mes pasa de «09» a «Sept.» y en letón
// cambia el literal del año. Se compara contra lo que devolvió Intl de verdad, no
// contra la tabla que lee la implementación.
func TestFormatDateShortMonthEsElDeIntl(t *testing.T) {
	v := leerVectores(t)
	for _, l := range Locales {
		for i, inst := range v.Instantes {
			if got, quiero := FormatDateShortMonth(l, inst.UTC()), v.IntlFechas[l].FechaDia2[i]; got != quiero {
				t.Errorf("FormatDateShortMonth(%s, %s) = %q, Intl dice %q", l, inst, got, quiero)
			}
		}
	}
}

// TestLasFechasLargasSonLasDeIntl: el cumpleaños de una ficha de contacto, con año y
// sin él. Contra lo que devolvió Intl de verdad en los 26 idiomas.
func TestLasFechasLargasSonLasDeIntl(t *testing.T) {
	v := leerVectores(t)
	for _, l := range Locales {
		if len(v.IntlFechas[l].FechaLarga) == 0 || len(v.IntlFechas[l].DiaMesLargo) == 0 {
			t.Fatalf("%s: sin vectores de fecha larga: regenerar testdata/intl.json EN LINUX", l)
		}
		for i, inst := range v.Instantes {
			if got, quiero := FormatDateLong(l, inst.UTC()), v.IntlFechas[l].FechaLarga[i]; got != quiero {
				t.Errorf("FormatDateLong(%s, %s) = %q, Intl dice %q", l, inst, got, quiero)
			}
			if got, quiero := FormatDayMonthLong(l, inst.UTC()), v.IntlFechas[l].DiaMesLargo[i]; got != quiero {
				t.Errorf("FormatDayMonthLong(%s, %s) = %q, Intl dice %q", l, inst, got, quiero)
			}
			if got, quiero := FormatDateTimeLong(l, inst.UTC()), v.IntlFechas[l].FechaLargaHora[i]; got != quiero {
				t.Errorf("FormatDateTimeLong(%s, %s) = %q, Intl dice %q", l, inst, got, quiero)
			}
		}
	}
}

// TestLasFechasDeCalendarSonLasDeIntl: `dayMonthLabel`, el título de la vista de día y
// `monthShortLabel` de Sytue Calendar, contra lo que devolvió Intl de verdad. Hasta el
// 2026-09-22 la app nativa pintaba la fecha CORTA («22 sept») donde la web pone «22 de
// septiembre», y el título del día sin año.
func TestLasFechasDeCalendarSonLasDeIntl(t *testing.T) {
	v := leerVectores(t)
	for _, l := range Locales {
		if len(v.IntlFechas[l].FechaCompleta) != len(v.Instantes) || len(v.IntlFechas[l].MesCorto) != len(v.Instantes) ||
			len(v.IntlFechas[l].HoraDos) != len(v.Instantes) || len(v.IntlFechas[l].DiaCorto) != len(v.Instantes) {
			t.Fatalf("%s: faltan vectores de Calendar en testdata/intl.json (regenerar con gen-i18n.ts)", l)
		}
		for i, inst := range v.Instantes {
			if got, quiero := FormatFullDate(l, inst.UTC()), v.IntlFechas[l].FechaCompleta[i]; got != quiero {
				t.Errorf("FormatFullDate(%s, %s) = %q, Intl dice %q", l, inst, got, quiero)
			}
			if got, quiero := FormatTimeDigits(l, inst.UTC()), v.IntlFechas[l].HoraDos[i]; got != quiero {
				t.Errorf("FormatTimeDigits(%s, %s) = %q, Intl dice %q", l, inst, got, quiero)
			}
			if got, quiero := FormatWeekdayDayMonthShort(l, inst.UTC()), v.IntlFechas[l].DiaCorto[i]; got != quiero {
				t.Errorf("FormatWeekdayDayMonthShort(%s, %s) = %q, Intl dice %q", l, inst, got, quiero)
			}
			if got, quiero := FormatMonthShort(l, inst.UTC()), v.IntlFechas[l].MesCorto[i]; got != quiero {
				t.Errorf("FormatMonthShort(%s, %s) = %q, Intl dice %q", l, inst, got, quiero)
			}
			for dias, quiero := range map[int]string{6: v.IntlFechas[l].RangoSemana[i], 10: v.IntlFechas[l].RangoLargo[i]} {
				fin := inst.UTC().AddDate(0, 0, dias)
				if got := FormatDateRange(l, inst.UTC(), fin); got != quiero {
					t.Errorf("FormatDateRange(%s, %s, %s) = %q, Intl dice %q", l, inst, fin, got, quiero)
				}
			}
		}
	}
}

// TestFormatPercentEsElDeIntl: el sitio del signo y lo que va delante son del idioma
// —en inglés pegado, en los otros veinticinco con espacio duro—, así que un «%» a mano
// sería correcto en uno de veintiséis. `ui.Medidor` escribía «0» donde la web escribe
// «0 %» justo por no tener esto.
func TestFormatListEsElDeIntl(t *testing.T) {
	v := leerVectores(t)
	if len(v.Listas) == 0 {
		t.Fatal("sin vectores de enumeración: regenerar testdata/intl.json EN LINUX")
	}
	for _, l := range Locales {
		for i, xs := range v.Listas {
			if got, quiero := FormatList(l, xs), v.IntlLista[l][i]; got != quiero {
				t.Errorf("FormatList(%s, %v) = %q, Intl dice %q", l, xs, got, quiero)
			}
		}
	}
}

func TestFormatPercentEsElDeIntl(t *testing.T) {
	v := leerVectores(t)
	if len(v.Porcentajes) == 0 {
		t.Fatal("sin vectores de porcentaje: regenerar testdata/intl.json EN LINUX")
	}
	for _, l := range Locales {
		for i, f := range v.Porcentajes {
			if got, quiero := FormatPercent(l, f), v.IntlPorcentaje[l][i]; got != quiero {
				t.Errorf("FormatPercent(%s, %v) = %q, Intl dice %q", l, f, got, quiero)
			}
		}
	}
}

// TestFormatCurrencyEsElDeIntl: un importe en EUR, en los 26 idiomas, contra lo que
// devolvió Intl de verdad.
//
// Lo que se vigila de verdad son las tres cosas que se escriben mal solas: el SITIO del
// símbolo (detrás en español, delante en inglés e irlandés), dónde cae el menos de un
// abono —delante del símbolo, no delante del número— y que los dos decimales lleven su
// cero de relleno («0,05 €», no «0,5 €»).
func TestFormatCurrencyEsElDeIntl(t *testing.T) {
	v := leerVectores(t)
	if len(v.Importes) == 0 {
		t.Fatal("sin vectores de moneda: regenerar testdata/intl.json EN LINUX")
	}
	for _, l := range Locales {
		for i, c := range v.Importes {
			if got, quiero := FormatCurrency(l, c), v.IntlMoneda[l][i]; got != quiero {
				t.Errorf("FormatCurrency(%s, %d) = %q, Intl dice %q", l, c, got, quiero)
			}
		}
	}
}

// TestFormatCurrencyInEsElDeIntl: los mismos importes en las diez monedas del EEE y de
// Suiza que no son el euro, contra Intl. Lo que vigila es lo que un «código detrás»
// escrito a mano hacía mal: en inglés el código va DELANTE («SEK 1,234.56»), y la
// corona islandesa no lleva decimales y se redondea («1235 ISK»).
//
// VISTO EN ROJO (frontal, 2026-09-23): con `abs/100` en vez de `(abs+50)/100` en las
// monedas sin decimales, 1234,56 ISK sale «1234 ISK» y el guardián lo dice en los 26.
func TestFormatCurrencyInEsElDeIntl(t *testing.T) {
	v := leerVectores(t)
	if len(v.IntlMonedas) == 0 {
		t.Fatal("sin vectores de monedas: regenerar testdata/intl.json EN LINUX")
	}
	for _, l := range Locales {
		for codigo, esperados := range v.IntlMonedas[l] {
			for i, c := range v.Importes {
				if got, quiero := FormatCurrencyIn(l, c, codigo), esperados[i]; got != quiero {
					t.Errorf("FormatCurrencyIn(%s, %d, %s) = %q, Intl dice %q", l, c, codigo, got, quiero)
				}
			}
		}
		if got, quiero := FormatCurrencyIn(l, 123456, "EUR"), FormatCurrency(l, 123456); got != quiero {
			t.Errorf("%s: el euro por FormatCurrencyIn da %q y por FormatCurrency %q", l, got, quiero)
		}
	}
}

// TestFormatMonthShortYearEsElDeIntl: el mes agrupado de un informe de Sytue AI.
func TestFormatMonthShortYearEsElDeIntl(t *testing.T) {
	v := leerVectores(t)
	for _, l := range Locales {
		if len(v.IntlFechas[l].MesCortoAno) == 0 {
			t.Fatalf("%s: sin vectores de mes corto: regenerar testdata/intl.json EN LINUX", l)
		}
		for i, inst := range v.Instantes {
			if got, quiero := FormatMonthShortYear(l, inst.UTC()), v.IntlFechas[l].MesCortoAno[i]; got != quiero {
				t.Errorf("FormatMonthShortYear(%s, %s) = %q, Intl dice %q", l, inst, got, quiero)
			}
		}
	}
}

// TestLosPaisesEstanEnLosVeintiseisIdiomas: los 31 países del ERP con nombre propio en
// cada idioma. Un nombre que sale igual que su código es que Intl no lo conocía, o que
// el generador no lo pidió.
func TestLosPaisesEstanEnLosVeintiseisIdiomas(t *testing.T) {
	codigos := []string{
		"AT", "BE", "BG", "CH", "CY", "CZ", "DE", "DK", "EE", "ES", "FI", "FR", "GR", "HR", "HU", "IE",
		"IS", "IT", "LI", "LT", "LU", "LV", "MT", "NL", "NO", "PL", "PT", "RO", "SE", "SI", "SK",
	}
	for _, l := range Locales {
		for _, c := range codigos {
			if n := CountryName(l, c); n == c || n == "" {
				t.Errorf("CountryName(%s, %s) = %q", l, c, n)
			}
		}
	}
	for _, c := range []struct {
		l              Locale
		codigo, quiero string
	}{
		{"es", "ES", "España"}, {"en", "DE", "Germany"}, {"de", "CH", "Schweiz"}, {"es", "XX", "XX"},
	} {
		if got := CountryName(c.l, c.codigo); got != c.quiero {
			t.Errorf("CountryName(%s, %s) = %q, quiero %q", c.l, c.codigo, got, c.quiero)
		}
	}
}

// TestFormatRelativeTimeEsElDeIntl es el guardián del puerto de
// `Intl.RelativeTimeFormat`: las formas literales de `numeric: 'auto'` («ayer»,
// «ahora»), el singular, y las cantidades que separan las categorías de plural
// eslavas y bálticas, que es donde un patrón único se rompe.
func TestFormatRelativeTimeEsElDeIntl(t *testing.T) {
	v := leerVectores(t)
	if len(v.Relativos) == 0 {
		t.Fatal("sin vectores de tiempo relativo: regenerar testdata/intl.json EN LINUX")
	}
	for _, l := range Locales {
		for i, par := range v.Relativos {
			n, ok := par[0].(float64)
			if !ok {
				t.Fatalf("vector %d mal formado: %v", i, par)
			}
			u, ok := par[1].(string)
			if !ok {
				t.Fatalf("vector %d mal formado: %v", i, par)
			}
			if got, quiero := FormatRelativeTime(l, int(n), Unidad(u)), v.IntlRelativo[l][i]; got != quiero {
				t.Errorf("FormatRelativeTime(%s, %v, %s) = %q, Intl dice %q", l, n, u, got, quiero)
			}
		}
	}
}

// TestFormatTimeAgoCortaDondeLaWeb: los cortes de `formatTimeAgo` —minuto, hora, día y
// la semana, a partir de la cual es fecha corta— y el redondeo, que es el de
// `Math.round` de JavaScript y no el de Go (que redondea al par).
func TestFormatTimeAgoCortaDondeLaWeb(t *testing.T) {
	ahora := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		hace   time.Duration
		quiero string
	}{
		{0, "now"},
		{59 * time.Second, "now"},
		{89 * time.Second, "1 min ago"}, // redondeo de JS: 89 segundos es un minuto
		{91 * time.Second, "2 min ago"}, // y 91 ya son dos
		{30 * time.Minute, "30 min ago"},
		{89 * time.Minute, "1 hr ago"},
		{23 * time.Hour, "23 hr ago"},
		{25 * time.Hour, "yesterday"}, // `numeric: 'auto'`: nunca «hace 1 día»
		{47 * time.Hour, "2 days ago"},
		{6 * 24 * time.Hour, "6 days ago"},
		{8 * 24 * time.Hour, "14 Sept"}, // a partir de la semana, fecha corta
	} {
		if got := FormatTimeAgo("en", ahora.Add(-c.hace), ahora); got != c.quiero {
			t.Errorf("FormatTimeAgo(en, hace %v) = %q, quiero %q", c.hace, got, c.quiero)
		}
	}
	// Un instante en el FUTURO cae en «ahora», igual que en la web: `seconds` sale
	// negativo y entra por el primer corte. Decirlo aquí porque es fácil «arreglarlo»
	// y dejar de parecerse a la referencia.
	if got := FormatTimeAgo("en", ahora.Add(time.Hour), ahora); got != "now" {
		t.Errorf("un instante futuro da %q y la web dice «now»", got)
	}
}

func leerVectores(t *testing.T) vectores {
	t.Helper()
	crudo, err := os.ReadFile("testdata/intl.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vectores
	if err := json.Unmarshal(crudo, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Intl) != len(Locales) {
		t.Fatalf("vectores para %d idiomas, quiero %d", len(v.Intl), len(Locales))
	}
	return v
}

func TestCategoryEsLaDeIntl(t *testing.T) {
	v := leerVectores(t)
	for _, l := range Locales {
		for i, n := range v.Cantidades {
			if got, quiero := Category(l, n), v.Intl[l].Plural[i]; got != quiero {
				t.Errorf("%s: Category(%v) = %s, Intl dice %s", l, n, got, quiero)
			}
		}
	}
}

func TestFechasSonLasDeIntl(t *testing.T) {
	v := leerVectores(t)
	if len(v.Instantes) < 8 {
		t.Fatalf("solo %d instantes en los vectores", len(v.Instantes))
	}
	for _, l := range Locales {
		for i, instante := range v.Instantes {
			if got, quiero := FormatDate(l, instante.UTC()), v.IntlFechas[l].Fecha[i]; got != quiero {
				t.Errorf("%s: FormatDate(%s) = %q, Intl dice %q", l, instante.Format(time.RFC3339), got, quiero)
			}
			if got, quiero := FormatTime(l, instante.UTC()), v.IntlFechas[l].Hora[i]; got != quiero {
				t.Errorf("%s: FormatTime(%s) = %q, Intl dice %q", l, instante.Format(time.RFC3339), got, quiero)
			}
			if got, quiero := FormatMonthYear(l, instante.UTC()), v.IntlFechas[l].MesAno[i]; got != quiero {
				t.Errorf("%s: FormatMonthYear(%s) = %q, Intl dice %q", l, instante.Format(time.RFC3339), got, quiero)
			}
			// El sobretítulo de Sytue Hub. Es el que lleva el día de la SEMANA, y el
			// giro de time.Weekday (domingo = 0) frente a lunes = 0 solo sale aquí.
			if got, quiero := FormatWeekdayDayMonth(l, instante.UTC()), v.IntlFechas[l].DiaMes[i]; got != quiero {
				t.Errorf("%s: FormatWeekdayDayMonth(%s) = %q, Intl dice %q", l, instante.Format(time.RFC3339), got, quiero)
			}
		}
		for i, dia := range WeekdaysShort(l) {
			if dia == "" {
				t.Errorf("%s: falta el día %d de la semana", l, i)
			}
		}
	}
	// De lunes a domingo, como `weekdayLabels` de la web; no de domingo a sábado.
	if d := WeekdaysShort("es"); d[0] != "lun" || d[6] != "dom" {
		t.Errorf("la semana en español va de %q a %q", d[0], d[6])
	}
}

func TestFormatBytesEsElDeLaWeb(t *testing.T) {
	v := leerVectores(t)
	for _, l := range Locales {
		for i, n := range v.Tamanos {
			if got, quiero := FormatBytes(l, n), v.IntlBytes[l][i]; got != quiero {
				t.Errorf("%s: FormatBytes(%v) = %q, la web dice %q", l, n, got, quiero)
			}
		}
	}
	// El cálculo de los vectores es una copia del de @sytue/ui: si aquel cambia, se ve.
	fuente := webref.Read(t, "apps/frontend/packages/ui/src/lib/formatBytes.ts")
	for _, literal := range []string{"['KB', 'MB', 'GB', 'TB']", "if (n < 1024)", "v < 10 ? 1 : 0", "return '—';"} {
		if !strings.Contains(fuente, literal) {
			t.Errorf("formatBytes.ts ya no dice %q: revisar FormatBytes y gen-i18n.ts", literal)
		}
	}
}

func TestFormatNumberEsElDeIntl(t *testing.T) {
	v := leerVectores(t)
	for _, l := range Locales {
		for i, n := range v.Numeros {
			if got, quiero := FormatNumber(l, n), v.Intl[l].Numero[i]; got != quiero {
				t.Errorf("%s: FormatNumber(%v) = %q, Intl dice %q", l, n, got, quiero)
			}
		}
	}
}

// Los mismos casos que i18n.test.ts: el puerto tiene que contestar lo mismo que el
// original a las mismas preguntas.
func TestNegotiateComoLaWeb(t *testing.T) {
	for _, c := range []struct {
		aceptados, guardada string
		quiero              Locale
	}{
		{"fr-CA,fr;q=0.9,en;q=0.8", "", "fr"},
		{"en;q=0.5, de", "", "de"},
		{"es;q=0.3,it;q=0.9", "", "it"},
		{"it;q=0.8,de;q=0.8", "", "it"}, // a igual calidad, el orden
		{"ja-JP,ja;q=0.9,fr;q=0.4", "", "fr"},
		{"tr-TR,ja;q=0.9", "", Default},
		{"de;q=0, es;q=0.1", "", "es"}, // q=0 es «no lo quiero»
		{"", "", Default},
		{";;;,,,", "", Default},
		{"de;q=abc", "", Default},
		{"*", "", Default},
		{"FR-CA", "", "fr"},
		{"fr-CA,fr;q=0.9", "it", "it"}, // la preferencia guardada manda
		{"de", "ja", "de"},             // …salvo que no sea un idioma de la suite
	} {
		if got := Negotiate(c.aceptados, c.guardada); got != c.quiero {
			t.Errorf("Negotiate(%q, %q) = %s, quiero %s", c.aceptados, c.guardada, got, c.quiero)
		}
	}
	for tag, quiero := range map[string]Locale{"es-ES": "es", "pt_BR.UTF-8": "pt", "nb": "nb", "EL": "el"} {
		if got, ok := FromTag(tag); !ok || got != quiero {
			t.Errorf("FromTag(%q) = %s, %v", tag, got, ok)
		}
	}
	if _, ok := FromTag("ja-JP"); ok {
		t.Error("FromTag da por bueno el japonés")
	}
}

func TestTComoLaWeb(t *testing.T) {
	for _, c := range []struct {
		l      Locale
		clave  string
		vars   Vars
		quiero string
	}{
		{"es", "account.menuFor", Vars{"name": "Ada"}, "Cuenta de Ada"},
		{"de", "account.organization", Vars{"tenant": "ACME"}, "Organisation: ACME"},
		{"de", "search.resultCount", Vars{"count": 1234}, "1.234 Ergebnisse"},
		{"en", "search.resultCount", Vars{"count": 1234}, "1,234 results"},
		{"en", "search.resultCount", Vars{"count": 1}, "1 result"},
		{"en", "search.resultCount", Vars{"count": 0}, "0 results"},
		{"fr", "search.resultCount", Vars{"count": 0}, "0 résultat"},
		{"fr", "search.resultCount", Vars{"count": 2}, "2 résultats"},
		{"it", "notifications.unread", Vars{"count": 1}, "1 non letta"},
		{"it", "notifications.unread", Vars{"count": 3}, "3 non lette"},
		// Maltés: Intl dice `two`, el catálogo no la tiene y cae a `few`.
		{"mt", "search.resultCount", Vars{"count": 2}, "2 riżultati"},
		{"mt", "search.resultCount", Vars{"count": 20}, "20 riżultat"},
		{"en", "search.resultCount", Vars{"count": "7"}, "7 results"},
		{"en", "search.resultCount", nil, "{count} results"},
		{"es", "clave.que.no.existe", nil, "clave.que.no.existe"},
	} {
		tr, err := New(c.l, Common())
		if err != nil {
			t.Fatal(err)
		}
		if got := tr.T(c.clave, c.vars); got != c.quiero {
			t.Errorf("%s T(%q, %v) = %q, quiero %q", c.l, c.clave, c.vars, got, c.quiero)
		}
	}
	if _, err := New("ja", Common()); err == nil {
		t.Error("New acepta un idioma que no es de la suite")
	}
}

// El catálogo de una app puede ir por detrás: el idioma que le falte, o la clave que
// le falte, sale en inglés — no en blanco.
func TestCatalogoDeAppParcial(t *testing.T) {
	app := fstest.MapFS{
		"en.json": {Data: []byte(`{"app.title": "Notes", "app.count": {"one": "{count} note", "other": "{count} notes"}}`)},
		"es.json": {Data: []byte(`{"app.title": "Notas"}`)},
	}
	for _, c := range []struct {
		l             Locale
		clave, quiero string
	}{
		{"es", "app.title", "Notas"},
		{"es", "app.count", "1234 notes"}, // la clave falta en es: inglés, con el número en español
		{"sv", "app.title", "Notes"},      // el idioma entero falta
		{"es", "ui.save", "Guardar"},      // y el común sigue ahí
	} {
		tr, err := New(c.l, Common(), app)
		if err != nil {
			t.Fatal(err)
		}
		if got := tr.T(c.clave, Vars{"count": 1234}); got != c.quiero {
			t.Errorf("%s T(%q) = %q, quiero %q", c.l, c.clave, got, c.quiero)
		}
	}
	if _, err := New("es", fstest.MapFS{"es.json": {Data: []byte(`{}`)}}); err == nil {
		t.Error("un catálogo sin inglés se ha dado por bueno")
	}
}

// locales/*.json es una copia generada. Si la web gana o pierde una clave y nadie
// regenera, esto lo dice. Compara CLAVES, no textos: leer valores de un .ts con una
// regex sería un parser de TypeScript a medias; los textos los vigila el CI, que
// regenera y exige que no haya diff.
func TestLosCatalogosTienenLasClavesDeLaWeb(t *testing.T) {
	reClave := regexp.MustCompile(`(?m)^ {2}'([\w.-]+)':`)
	for _, l := range Locales {
		fuente := webref.Read(t, "apps/frontend/packages/i18n/src/messages/"+string(l)+".ts")
		var web []string
		for _, m := range reClave.FindAllStringSubmatch(fuente, -1) {
			web = append(web, m[1])
		}
		slices.Sort(web)
		var m map[string]entrada
		crudo, err := os.ReadFile("locales/" + string(l) + ".json")
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(crudo, &m); err != nil {
			t.Fatal(err)
		}
		if aqui := slices.Sorted(maps.Keys(m)); !slices.Equal(web, aqui) {
			t.Errorf("%s: %d claves en la web y %d aquí — ejecutar `go generate ./internal/i18n`", l, len(web), len(aqui))
		}
	}
}
