// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Obligaciones: JSX real de2026-10-04, Chromium/Figtree, raíz16.
// Estados independientes cerrados, sólo guía+cálculo y sanción a626.89/288px.
// Incluye el pipeline de cuatro pasos y el botón del proceso persistido.

func dia(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func diaP(s string) *time.Time { t := dia(s); return &t }

// obligacionesArnes son las cuatro filas del arnés, con hoy el 23 de septiembre.
func obligacionesArnes() *api.ERPObligaciones {
	return &api.ERPObligaciones{Anio: 2026, Pais: "ES", Verificado: true, Moneda: "EUR", Obligaciones: []api.ERPObligacion{
		{
			Clave: "303", Nombre: "Modelo 303", ConceptoRotulo: "IVA trimestral", Ambito: "fiscal", AmbitoRotulo: "Fiscal",
			Periodo: "2026-T3", Limite: dia("2026-10-21"), Recomendada: dia("2026-10-14"), LimiteDomiciliacion: diaP("2026-10-16"),
			Portal: api.ERPPortal{Nombre: "AEAT", URL: "https://sede.agenciatributaria.gob.es"}, Calculable: true,
			Sancion: &api.ERPSancion{
				Tipo: "recargo", RecargoBasePct: 1, RecargoPctMes: 1, RecargoMaxPct: 15, ConRequerimientoPct: []float64{50, 150},
				Nota: "Recargo del artículo 27 de la Ley General Tributaria; con requerimiento previo, sanción del 50 al 150 %.",
			},
			Guia: &api.ERPGuia{
				Que:       "Declaras el IVA que cobraste menos el que pagaste en el trimestre.",
				Necesitas: []string{"Las facturas emitidas del trimestre", "Las facturas recibidas del trimestre"},
				Pasos:     []string{"Entra en la sede con tu certificado", "Rellena el modelo con las cifras de «Preparar»", "Presenta y guarda el justificante"},
				SiNo:      "Pagas un recargo del 1 % más un 1 % por mes.",
			},
			Pendientes: map[string]int{"banco": 0, "facturas": 0},
			Documentos: []api.ERPDocumentoObligacion{{Clave: "justificante", Nombre: "Justificante de presentación", Requisitos: []string{"Con el CSV de la AEAT"}}},
		},
		{
			Clave: "111", Nombre: "Modelo 111", ConceptoRotulo: "Retenciones de IRPF", Ambito: "fiscal", AmbitoRotulo: "Fiscal",
			Periodo: "2026-T3", Limite: dia("2026-10-21"), Recomendada: dia("2026-09-27"),
			Portal: api.ERPPortal{Nombre: "AEAT", URL: "https://sede.agenciatributaria.gob.es"}, Pendientes: map[string]int{"banco": 3, "nominas": 2},
		},
		{
			Clave: "tc2", Nombre: "Cotizaciones sociales", ConceptoRotulo: "Seguros sociales", Ambito: "laboral", AmbitoRotulo: "Laboral",
			Periodo: "2026-09", Limite: dia("2026-10-31"), Recomendada: dia("2026-10-28"),
			Portal: api.ERPPortal{Nombre: "RED", URL: "https://portal.seg-social.gob.es"}, Pendientes: map[string]int{}, Sancion: &api.ERPSancion{Tipo: "fija", FijaCentimos: 25000},
		},
		{
			Clave: "303", Nombre: "Modelo 303", ConceptoRotulo: "IVA trimestral", Ambito: "fiscal", AmbitoRotulo: "Fiscal",
			Periodo: "2026-T2", Limite: dia("2026-07-20"), Recomendada: dia("2026-07-13"),
			Portal: api.ERPPortal{Nombre: "AEAT", URL: "https://sede.agenciatributaria.gob.es"}, Calculable: true,
		},
	}}
}

func obligacionesDePrueba(t *testing.T, ventana float32) *Obligaciones {
	o := NuevaObligaciones(traductor(t))
	o.Datos, o.Anio, o.Pais, o.Ventana = obligacionesArnes(), 2026, "ES", ventana
	o.Hoy = time.Date(2026, 9, 23, 10, 0, 0, 0, time.Local)
	return o
}

func cerca(t *testing.T, que string, got, quiero float32) {
	t.Helper()
	if d := got - quiero; d < -0.3 || d > 0.3 {
		t.Errorf("%s mide %.2f; la web %.2f", que, got, quiero)
	}
}

// TestLasCuentasDeObligacionesSonLasDeLaWeb: las funciones exportadas de
// `Obligaciones.tsx`, con los números del arnés.
//
// VISTO EN ROJO (frontal, 2026-09-23): truncando en vez de redondear como `Math.round`,
// la estimación sobre 172,26 € da 3,44 y no 3,45.
func TestLasCuentasDeObligacionesSonLasDeLaWeb(t *testing.T) {
	tr := traductor(t)
	hoy := time.Date(2026, 9, 23, 23, 30, 0, 0, time.Local)
	for _, c := range []struct {
		fecha string
		dias  int
	}{{"2026-09-27", 4}, {"2026-09-23", 0}, {"2026-07-20", -65}} {
		if d := DiasHasta(dia(c.fecha), hoy); d != c.dias {
			t.Errorf("días hasta %s: %d; la web %d", c.fecha, d, c.dias)
		}
	}
	for periodo, quiero := range map[string]string{
		"2026-T3": "3T 2026", "2026-09": "septiembre de 2026", "2026-A": "Ejercicio 2026", "2026-2P": "Pago 2, 2026", "raro": "raro",
	} {
		if got := RotuloDePeriodo(periodo, "es", tr); got != quiero {
			t.Errorf("periodo %s: %q; la web %q", periodo, got, quiero)
		}
	}
	d := obligacionesArnes().Obligaciones
	if got, quiero := DescribirSancion(d[0].Sancion, "EUR", "es", tr),
		"recargo del 1 % · 1 % por mes de retraso · hasta el 15 % · del 50 al 150 % si reclama antes la administración"; got != quiero {
		t.Errorf("sanción: %q\nla web: %q", got, quiero)
	}
	if got := DescribirSancion(d[2].Sancion, "EUR", "es", tr); got != "importe fijo de 250,00 €" {
		t.Errorf("sanción fija: %q", got)
	}
	if n, ok := EstimarUnMes(d[0].Sancion, 17226); !ok || n != 345 {
		t.Errorf("un mes de retraso sobre 172,26: %d", n)
	}
	if f := Faltas(d[1], tr); len(f) != 2 || f[0].Texto != "3 movimientos del banco sin factura" || f[1].Clave != "nominas" {
		t.Errorf("faltas: %+v", f)
	}
	if f := Faltas(d[0], tr); len(f) != 0 {
		t.Errorf("con todo a cero está lista: %+v", f)
	}
	if n := NombreDePais("ES", "es"); n != "España" {
		t.Errorf("país: %q", n)
	}
	// Fuera del euro, el patrón de Intl para ESA moneda: en inglés el código va delante
	// y la corona islandesa no lleva decimales. Antes salía «250.00 SEK» en inglés.
	for _, c := range []struct {
		l      i18n.Locale
		moneda string
		quiero string
	}{
		{"es", "SEK", "250,00\u00a0SEK"}, {"en", "SEK", "SEK\u00a0250.00"}, {"en", "ISK", "ISK\u00a0250"}, {"es", "EUR", "250,00\u00a0€"},
	} {
		if got := Dinero(c.l, 25000, c.moneda); got != c.quiero {
			t.Errorf("Dinero(%s, 250, %s) = %q; Intl dice %q", c.l, c.moneda, got, c.quiero)
		}
	}
}

// TestObligacionesMideLoDeLaWeb: la cabecera, las filas y los meses a 1280 y a 360.
//
// VISTO EN ROJO (frontal, 2026-09-23): sin el `mt-1` de los mandos de la derecha, la
// fila del 111 mide 80,5 y no 84.
func TestObligacionesMideLoDeLaWeb(t *testing.T) {
	o := obligacionesDePrueba(t, 1280)
	_, altoCab := o.cabecera(tema.Claro, 854)
	cerca(t, "la cabecera", altoCab, 65)
	_, anio := o.pasoAnio(tema.Claro)
	cerca(t, "el año", anio.Width, 102.5625)
	cerca(t, "el año (alto)", anio.Height, 24)
	todas, fiscal := NuevaPildora("Todas", true, nil), NuevaPildora("Fiscal", false, nil)
	todas.Ligera, fiscal.Ligera = true, true
	cerca(t, "«Todas»", MedidaPildora(todas).Width, 52.046875)
	cerca(t, "«Fiscal»", MedidaPildora(fiscal).Width, 50.15625)
	proximos, vencidos := o.Meses()
	if len(proximos) != 2 || len(vencidos) != 1 || len(proximos[1].filas) != 2 {
		t.Fatalf("meses: %d próximos y %d vencidos", len(proximos), len(vencidos))
	}
	d := obligacionesArnes().Obligaciones
	_, alto111 := o.fila(tema.Claro, d[1], 852)
	cerca(t, "la fila del 111", alto111, 146)
	_, alto303 := o.fila(tema.Claro, d[0], 852)
	cerca(t, "la fila del 303, incluido divide-y", alto303+1, 225)
	_, altoTC := o.fila(tema.Claro, d[2], 852)
	cerca(t, "la fila de las cotizaciones", altoTC, 166)
	_, altoMes := o.mes(tema.Claro, proximos[1], 854)
	cerca(t, "octubre", altoMes, 417)
	_, altoVenc := o.vencidas(tema.Claro, vencidos, 854)
	cerca(t, "las vencidas plegadas", altoVenc, 44)
	// Desplegadas la guía y las vencidas, y con la cifra preparada.
	o.Abiertos["guia|303|2026-T3"], o.Abiertos["vencidas"] = true, true
	o.Calculos["303|2026-T3"] = &CalculoObl{R: api.ERPCalculoObligacion{Desde: "2026-07-01", Hasta: "2026-09-30", ResultadoCentimos: 17226, Asentado: true}}
	_, alto303 = o.fila(tema.Claro, d[0], 852)
	cerca(t, "la fila del 303 desplegada, incluido divide-y", alto303+1, 501)
	_, altoVenc = o.vencidas(tema.Claro, vencidos, 854)
	cerca(t, "las vencidas desplegadas", altoVenc, 212)

	m := obligacionesDePrueba(t, 360)
	m.AlEditarPerfil = func() {} // la web lo pinta; aquí, cuando llegue el asistente
	_, altoCab = m.cabecera(tema.Claro, 318)
	cerca(t, "la cabecera en el móvil", altoCab, 199)
	_, alto111 = m.fila(tema.Claro, d[1], 316)
	cerca(t, "la fila del 111 en el móvil", alto111, 266)
}

// TestObligacionesSePinta guarda la pantalla a los dos anchos para mirarla, y comprueba
// que la entrada del menú lleva a ella.
func TestObligacionesSePinta(t *testing.T) {
	p, w := pantallaArnes(t, fyne.NewSize(1280, 800))
	defer w.Close()
	p.IrA(VistaObligaciones)
	o := p.oblig
	o.Datos, o.Anio, o.Cargando, o.Hoy = obligacionesArnes(), 2026, false, time.Date(2026, 9, 23, 10, 0, 0, 0, time.Local)
	p.Organizacion = &api.ERPOrganizacion{Pais: "ES"}
	p.Repintar()
	guardarPNG(t, w.Canvas().Capture(), "erp-obligaciones.png")
	o.Abiertos["guia|303|2026-T3"], o.Abiertos["vencidas"] = true, true
	o.Calculos["303|2026-T3"] = &CalculoObl{R: api.ERPCalculoObligacion{Desde: "2026-07-01", Hasta: "2026-09-30", ResultadoCentimos: 17226, Asentado: true}}
	w.Resize(fyne.NewSize(1280, 1400))
	p.Ventana = fyne.NewSize(1280, 1400)
	p.Repintar()
	guardarPNG(t, w.Canvas().Capture(), "erp-obligaciones-abierta.png")
	if !contiene(textosDe(o.Objeto(tema.Claro)), "Importe a presentar: 172,26") {
		t.Error("la cifra preparada no se ve")
	}
	p.rellenarMenu()
	var hay bool
	for _, d := range p.menu.Fijas {
		if d.Rotulo == p.t("nav.obligations") {
			hay = d.Activa
		}
	}
	if !hay {
		t.Error("la entrada de Obligaciones no está o no es la activa")
	}
	w.Resize(fyne.NewSize(360, 740))
	p.Ventana = fyne.NewSize(360, 740)
	o.Abiertos = map[string]bool{}
	p.Repintar()
	guardarPNG(t, w.Canvas().Capture(), "erp-obligaciones-movil.png")
}

// TestLoDesplegadoDeUnaObligacionMideLoDeLaWeb: los documentos y la letra pequeña con
// sus `details` abiertos, medidos en el arnés (2026-09-24) a 1280 y a 360.
//
// VISTO EN ROJO (frontal, 2026-09-24): sin el `mt-0.5` del `details` al abrirse, la
// sanción mide 56 y no 57,75 (84 y no 85,75 en el móvil).
func TestLoDesplegadoDeUnaObligacionMideLoDeLaWeb(t *testing.T) {
	o := obligacionesDePrueba(t, 1280)
	d := obligacionesArnes().Obligaciones
	cerca(t, "el ancho de los documentos", ProsaDocumentos, 666.640625)
	cerca(t, "el ancho de la letra pequeña", ProsaNota, 499.96875)
	frase := DescribirSancion(d[0].Sancion, "EUR", "es", o.tr)
	o.Abiertos["nota|"+IDObligacion(d[0])] = true
	_, alto := o.sancion(tema.Claro, d[0], frase, 626.89)
	cerca(t, "la sanción con la letra pequeña abierta", alto, 66)
	_, alto = o.sancion(tema.Claro, d[0], frase, 288)
	cerca(t, "la sanción abierta en el móvil", alto, 98)
	o.Adjuntos[RegistroDocumento(d[0], "justificante")] = []api.ERPAdjunto{{
		ID: "d1", Nombre: "factura-FV-2026-00041.pdf", Tipo: "application/pdf", Tamano: 184320,
	}}
	_, alto = o.documentos(tema.Claro, d[0], ProsaDocumentos)
	cerca(t, "los documentos abiertos", alto, 98)
}
