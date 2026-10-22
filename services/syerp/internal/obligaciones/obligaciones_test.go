// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package obligaciones

import (
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
)

func cargar(t *testing.T) *Catalogo {
	t.Helper()
	cat, err := catalogo.Cargar()
	if err != nil {
		t.Fatalf("catálogo: %v", err)
	}
	c, err := Cargar(func(clave string) bool { return cat.Rotulo(clave, "en") != "" })
	if err != nil {
		t.Fatalf("obligaciones: %v", err)
	}
	return c
}

// Los treinta y uno —UE, EEE y Suiza—, cada concepto con sus cinco idiomas,
// cada plazo generable.
// Que esto pase es lo que permite que un fichero de país nuevo entre sin que
// nadie lo revise línea a línea: lo revisa el arranque.
func TestCargaTodosLosPaises(t *testing.T) {
	c := cargar(t)
	if n := len(c.Paises()); n != 31 {
		t.Fatalf("países cargados = %d, quiero 31", n)
	}
	es, ok := c.Pais("ES")
	if !ok || !es.Verificado {
		t.Fatalf("España debe existir y estar verificada")
	}
	// Los que alguien contrastó con la fuente oficial (ley o portal de la
	// administración). Marcar `verificado` en un fichero sin pasar por aquí es
	// exactamente lo que este bucle impide.
	revisados := map[string]bool{
		"ES": true, "DE": true, "FR": true, "IT": true, "PT": true, "NL": true, "AT": true,
		"IE": true, "PL": true, "SE": true, "DK": true, "FI": true, "CZ": true, "BE": true,
		"RO": true, "HU": true, "GR": true, "SK": true, "HR": true, "SI": true, "BG": true,
		"LT": true, "LV": true, "EE": true, "LU": true, "MT": true, "CY": true,
		"NO": true, "IS": true,
	}
	for _, p := range c.Paises() {
		if p.Verificado && !revisados[p.Pais] {
			t.Errorf("%s: marcado como verificado sin haberlo revisado", p.Pais)
		}
		if len(p.Obligaciones) < 5 {
			t.Errorf("%s: solo %d obligaciones", p.Pais, len(p.Obligaciones))
		}
	}
}

func fecha(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestReglasDePlazo(t *testing.T) {
	fijas := []struct{ regla, quiero string }{
		{"04-20", "2026-04-20"},
		{"01-30+1", "2027-01-30"},
		{"02-ultimo+1", "2027-02-28"},
		{"03-31+2", "2028-03-31"},
	}
	for _, c := range fijas {
		got, err := fechaFija(c.regla, 2026)
		if err != nil || !got.Equal(fecha(c.quiero)) {
			t.Errorf("fija %q = %v, %v; quiero %s", c.regla, got, err, c.quiero)
		}
	}
	relativas := []struct {
		regla  string
		mesFin int
		quiero string
	}{
		{"20", 3, "2026-04-20"},
		{"ultimo", 12, "2027-01-31"},
		// Enero de 2027 acaba en domingo: el último hábil es el viernes 29.
		{"ultimo_habil", 12, "2027-01-29"},
		{"20+2", 12, "2027-02-20"},
		{"ultimo+0", 2, "2026-02-28"},
		{"30", 1, "2026-02-28"}, // el 30 de febrero se recorta al 28
		{"10+0", 5, "2026-05-10"},
	}
	for _, c := range relativas {
		got, err := fechaRelativa(c.regla, 2026, c.mesFin)
		if err != nil || !got.Equal(fecha(c.quiero)) {
			t.Errorf("relativa %q (mes %d) = %v, %v; quiero %s", c.regla, c.mesFin, got, err, c.quiero)
		}
	}
	for _, mala := range []string{"", "13-01", "04-32", "x", "20+99", "04-20+a"} {
		if _, err := fechaFija(mala, 2026); err == nil {
			t.Errorf("fija %q: se esperaba error", mala)
		}
	}
}

func buscar(inst []Instancia, clave, periodo string) *Instancia {
	for i := range inst {
		if inst[i].Clave == clave && inst[i].Periodo == periodo {
			return &inst[i]
		}
	}
	return nil
}

func TestCalendarioAutonomoEspana(t *testing.T) {
	c := cargar(t)
	perfil := Perfil{Pais: "ES", Forma: "autonomo", Condiciones: map[string]bool{"iva": true}}
	inst, err := c.Calendario(perfil, 2026)
	if err != nil {
		t.Fatal(err)
	}
	// 303 trimestral: 20 de abril; el cuarto trimestre vence el 30 de enero,
	// que en 2027 es sábado y pasa al lunes 1 de febrero.
	if x := buscar(inst, "es_303", "2026-T1"); x == nil || !x.Limite.Equal(fecha("2026-04-20")) {
		t.Errorf("303 T1 = %+v", x)
	}
	if x := buscar(inst, "es_303", "2026-T4"); x == nil || !x.Limite.Equal(fecha("2027-02-01")) {
		t.Errorf("303 T4 = %+v (30 de enero de 2027 es sábado)", x)
	}
	// Domiciliando, el 303 acaba el 15 de abril; «tu fecha» son cuatro hábiles antes.
	if x := buscar(inst, "es_303", "2026-T1"); x == nil || x.LimiteDomiciliacion == nil ||
		!x.LimiteDomiciliacion.Equal(fecha("2026-04-15")) || !x.Recomendada.Equal(fecha("2026-04-09")) {
		t.Errorf("303 T1 domiciliación y tu fecha = %+v", x)
	}
	// Autónomo: pagos fraccionados y renta sí; sociedades y cuentas no.
	if buscar(inst, "es_130", "2026-T2") == nil || buscar(inst, "es_100", "2026-A") == nil {
		t.Error("faltan el 130 o la renta")
	}
	if buscar(inst, "es_200", "2026-A") != nil || buscar(inst, "es_cuentas", "2026-A") != nil {
		t.Error("un autónomo no presenta sociedades ni deposita cuentas")
	}
	// Sin empleados ni intracomunitarias: ni 111 ni 349. Sin IVA mensual: no 303m.
	for _, clave := range []string{"es_111", "es_349", "es_303m", "es_115"} {
		for _, i := range inst {
			if i.Clave == clave {
				t.Errorf("%s no aplica a este perfil", clave)
				break
			}
		}
	}
	// La cuota de autónomos, doce veces y el último día del propio mes.
	n := 0
	for _, i := range inst {
		if i.Clave == "es_cuota_autonomos" {
			n++
		}
	}
	if n != 12 {
		t.Errorf("cuota de autónomos: %d instancias, quiero 12", n)
	}
	if x := buscar(inst, "es_cuota_autonomos", "2026-02"); x == nil || !x.Limite.Equal(fecha("2026-03-02")) {
		t.Errorf("cuota febrero = %+v (28 de febrero de 2026 es sábado → lunes 2 de marzo)", x)
	}
	// Ordenado por límite.
	for i := 1; i < len(inst); i++ {
		if inst[i].Limite.Before(inst[i-1].Limite) {
			t.Fatalf("desordenado en %d: %v tras %v", i, inst[i].Limite, inst[i-1].Limite)
		}
	}
}

func TestCalendarioSociedadEspana(t *testing.T) {
	c := cargar(t)
	perfil := Perfil{Pais: "ES", Forma: "sl", Condiciones: map[string]bool{"iva": true, "empleados": true, "intracomunitario": true}}
	inst, err := c.Calendario(perfil, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if x := buscar(inst, "es_111", "2026-T4"); x == nil || !x.Limite.Equal(fecha("2027-01-20")) {
		t.Errorf("111 T4 = %+v (el 20 de enero, no el 30)", x)
	}
	// 25 de julio de 2027 es domingo → lunes 26.
	if x := buscar(inst, "es_200", "2026-A"); x == nil || !x.Limite.Equal(fecha("2027-07-26")) {
		t.Errorf("200 = %+v", x)
	}
	if buscar(inst, "es_349", "2026-T3") == nil || buscar(inst, "es_202", "2026-2P") == nil {
		t.Error("faltan el 349 o el segundo pago fraccionado")
	}
	if buscar(inst, "es_130", "2026-T1") != nil {
		t.Error("una sociedad no presenta el 130")
	}
}

// Los ejemplos que publica la Agencia Tributaria para 2026 (ejercicio 2025): el
// 303 del cuarto trimestre vence el 30 de enero y se domicilia hasta el 27; el 200
// vence el 27 de julio (el 25 es sábado) y se domicilia hasta el 22.
func TestDomiciliacionYFestivosEspana(t *testing.T) {
	c := cargar(t)
	sl := Perfil{Pais: "ES", Forma: "sl", Condiciones: map[string]bool{"iva": true}}
	inst, err := c.Calendario(sl, 2025)
	if err != nil {
		t.Fatal(err)
	}
	for _, caso := range []struct{ clave, periodo, limite, dom string }{
		{"es_303", "2025-T4", "2026-01-30", "2026-01-27"},
		{"es_200", "2025-A", "2026-07-27", "2026-07-22"},
	} {
		x := buscar(inst, caso.clave, caso.periodo)
		if x == nil || !x.Limite.Equal(fecha(caso.limite)) || x.LimiteDomiciliacion == nil || !x.LimiteDomiciliacion.Equal(fecha(caso.dom)) {
			t.Errorf("%s %s = %+v; quiero límite %s y domiciliación %s", caso.clave, caso.periodo, x, caso.limite, caso.dom)
		}
	}

	// 303 del 3T 2026: domicilia hasta el jueves 15 de octubre. Cuatro hábiles antes
	// se salta el 12 (fiesta nacional) y, en la Comunitat Valenciana, también el 9.
	for region, quiero := range map[string]string{"": "2026-10-08", "VC": "2026-10-07"} {
		sl.Region = region
		inst, err := c.Calendario(sl, 2026)
		if err != nil {
			t.Fatal(err)
		}
		if x := buscar(inst, "es_303", "2026-T3"); x == nil || !x.Recomendada.Equal(fecha(quiero)) || x.CalendarioProvisional {
			t.Errorf("303 3T en %q: tu fecha = %+v, quiero %s", region, x, quiero)
		}
		// El 200 del ejercicio 2026 vence en 2027, sin resolución publicada aún.
		if x := buscar(inst, "es_200", "2026-A"); x == nil || !x.CalendarioProvisional {
			t.Errorf("200 de 2026: el calendario de 2027 debe salir provisional: %+v", x)
		}
	}

	// En el País Vasco el IVA y Sociedades van a la hacienda foral; la Seguridad
	// Social, no. En Canarias no hay IVA, pero Sociedades sí va a la Agencia.
	sl.Region = "PV"
	if inst, _ := c.Calendario(sl, 2026); buscar(inst, "es_303", "2026-T1") != nil || buscar(inst, "es_200", "2026-A") != nil {
		t.Error("una sociedad vasca no presenta IVA ni Sociedades en la Agencia Tributaria")
	}
	sl.Region = "CN"
	if inst, _ := c.Calendario(sl, 2026); buscar(inst, "es_303", "2026-T1") != nil || buscar(inst, "es_200", "2026-A") == nil {
		t.Error("en Canarias no hay 303 pero sí 200")
	}
	if es, _ := c.Pais("ES"); es.Calendario.Region("46001") != "VC" || es.Territorios["CN"].Nota == "" {
		t.Error("el código postal 46 es Valencia y Canarias tiene su aviso")
	}
}

func TestPagoFraccionado(t *testing.T) {
	// Resultados acumulados desde enero al cierre de cada trimestre: pierde en el
	// primero y gana después. El 20 % se paga sobre lo acumulado menos lo ya pagado.
	for _, c := range []struct {
		acumulados []int64
		quiero     int64
	}{
		{[]int64{100_000}, 20_000},
		{[]int64{100_000, 250_000}, 30_000},
		{[]int64{-50_000, 100_000}, 20_000},
		{[]int64{-50_000, 30_000}, 6_000},
		{[]int64{100_000, 50_000}, 0},
		{[]int64{100_000, 50_000, 300_000}, 40_000},
	} {
		if got := PagoFraccionado(c.acumulados, 20); got != c.quiero {
			t.Errorf("PagoFraccionado(%v) = %d, quiero %d", c.acumulados, got, c.quiero)
		}
	}
}

func TestPerfilDesconocido(t *testing.T) {
	c := cargar(t)
	if _, err := c.Calendario(Perfil{Pais: "XX", Forma: "sl"}, 2026); err == nil {
		t.Error("país desconocido debe fallar")
	}
	if _, err := c.Calendario(Perfil{Pais: "ES", Forma: "gmbh"}, 2026); err == nil {
		t.Error("forma de otro país debe fallar")
	}
}

func TestEstimarSancion(t *testing.T) {
	recargo := &Sancion{Tipo: "recargo", RecargoPctMes: 1, RecargoMaxPct: 15}
	if got, ok := Estimar(recargo, 100_000, 45); !ok || got != 2_000 {
		t.Errorf("45 días = dos meses = 2 %%: %d %v", got, ok)
	}
	if got, _ := Estimar(recargo, 100_000, 500); got != 15_000 {
		t.Errorf("tope del 15 %% (17 meses): %d", got)
	}
	if got, _ := Estimar(&Sancion{Tipo: "fija", FijaCentimos: 30_000}, 1, 1); got != 30_000 {
		t.Errorf("fija: %d", got)
	}
	if _, ok := Estimar(&Sancion{Tipo: "nota"}, 100, 10); ok {
		t.Error("una nota no se estima")
	}
	if _, ok := Estimar(recargo, 100, 0); ok {
		t.Error("sin retraso no hay sanción")
	}
}

func TestRangoDePeriodo(t *testing.T) {
	casos := []struct{ periodo, desde, hasta string }{
		{"2026-T3", "2026-07-01", "2026-09-30"},
		{"2026-02", "2026-02-01", "2026-02-28"},
		{"2026-A", "2026-01-01", "2026-12-31"},
		{"2026-1P", "2026-01-01", "2026-12-31"},
	}
	for _, c := range casos {
		d, h, ok := RangoDePeriodo(c.periodo)
		if !ok || !d.Equal(fecha(c.desde)) || !h.Equal(fecha(c.hasta)) {
			t.Errorf("%s = %v..%v %v", c.periodo, d, h, ok)
		}
	}
	if _, _, ok := RangoDePeriodo("garbage"); ok {
		t.Error("periodo inválido aceptado")
	}
}

func TestICS(t *testing.T) {
	ev := []Evento{{
		UID: "acme-es_303-2026-T1@erp.sytue.eu", Fecha: fecha("2026-04-20"),
		Resumen:     "Modelo 303 · 1T 2026",
		Descripcion: "Declaración periódica de IVA; portal: https://sede.agenciatributaria.gob.es/\nSanción: 1 %, 2 %, …",
		URL:         "https://sede.agenciatributaria.gob.es/", Categoria: "Fiscal", AvisoDias: 7,
	}, {
		UID: "acme-semanal@erp.sytue.eu", Fecha: fecha("2026-01-05"), Resumen: "Revisión semanal", Repetir: "FREQ=WEEKLY;BYDAY=MO",
	}}
	out := string(ICS("Obligaciones · ACME", ev, fecha("2026-01-01")))
	for _, quiero := range []string{
		"BEGIN:VCALENDAR\r\n", "X-WR-CALNAME:Obligaciones · ACME\r\n",
		"DTSTART;VALUE=DATE:20260420\r\n", "DTEND;VALUE=DATE:20260421\r\n",
		"TRIGGER:-P7D\r\n", "TRIGGER:-P1D\r\n", `\;`, `\n`, "END:VCALENDAR\r\n",
		"RRULE:FREQ=WEEKLY;BYDAY=MO\r\n", "TRIGGER:PT9H\r\n",
	} {
		if !strings.Contains(out, quiero) {
			t.Errorf("falta %q en:\n%s", quiero, out)
		}
	}
	for _, l := range strings.Split(out, "\r\n") {
		if len(l) > 75 {
			t.Errorf("línea de %d octetos sin plegar: %q", len(l), l)
		}
	}
	// El plegado no parte una secuencia UTF-8.
	if !strings.Contains(out, "\r\n ") {
		t.Error("la descripción larga debería haberse plegado")
	}
	if strings.Count(out, "�") > 0 {
		t.Error("plegado sobre un carácter multibyte")
	}
}
