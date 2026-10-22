// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # Onboarding real, Chromium/Figtree, rem16, 2026-10-04.
// Oráculo independiente: medidas-dominios-final.json, ancho de paso504/móvil312.

func paisesArnes() []api.ERPPais {
	return []api.ERPPais{
		{Pais: "ES", Verificado: true, Moneda: "EUR", Formas: []api.ERPFormaJuridica{
			{Clave: "autonomo", Nombre: "Autónomo", Tipo: "persona_fisica"},
			{Clave: "sl", Nombre: "Sociedad limitada (SL)", Tipo: "sociedad"},
			{Clave: "sa", Nombre: "Sociedad anónima (SA)", Tipo: "sociedad"},
		}},
		{Pais: "FR", Verificado: true, Moneda: "EUR", Formas: []api.ERPFormaJuridica{{Clave: "sas", Nombre: "SAS", Tipo: "sociedad"}}},
	}
}

// TestElAsistenteDecideComoLaWeb: cambiar de país olvida la forma, un tipo con una sola
// forma la elige, sin razón social no se sigue, sin IVA no hay IVA mensual, y las áreas
// recomendadas son las de `modulosRecomendados`.
func TestElAsistenteDecideComoLaWeb(t *testing.T) {
	a := NuevoAsistente(traductor(t), nil, 2026)
	a.Cargados(paisesArnes())
	a.Modulos = []Modulo{{Clave: "contactos"}, {Clave: "facturacion"}, {Clave: "ventas"}, {Clave: "rrhh"}}
	if a.Pais != "ES" || !a.Cond["iva"] {
		t.Fatalf("de partida: país %q, IVA %v", a.Pais, a.Cond["iva"])
	}
	a.ElegirTipo("persona_fisica")
	if a.Forma != "autonomo" {
		t.Errorf("con una sola forma del tipo se elige sola: %q", a.Forma)
	}
	a.ElegirPais("FR")
	if a.Forma != "" {
		t.Errorf("cambiar de país tiene que olvidar la forma: %q", a.Forma)
	}
	a.Paso = PasoDatos
	a.Seguir()
	if a.Paso != PasoDatos || !a.Tocado {
		t.Errorf("sin razón social no se sigue: paso %d, tocado %v", a.Paso, a.Tocado)
	}
	a.campos["nombre"].PonerTexto("  Acme S.L.  ")
	a.Seguir()
	if a.Paso != PasoSituacion || a.Perfil().Nombre != "Acme S.L." {
		t.Errorf("con razón social se sigue y se guarda recortada: paso %d, %q", a.Paso, a.Perfil().Nombre)
	}
	a.Cond["iva_mensual"] = true
	a.conmutadores["iva"].AlCambiar(false)
	if a.Cond["iva_mensual"] {
		t.Error("sin IVA no hay periodicidad de IVA")
	}
	a.Tipo, a.Cond["empleados"] = "sociedad", true
	if got := a.Recomendados(); len(got) != 4 || got[1] != "ventas" || got[3] != "rrhh" {
		t.Errorf("recomendadas: %v", got)
	}
}

// TestElAsistenteMideLoDeLaWeb: el contenido de cada paso y el pie.
//
// El dd que contiene Editar hereda la caja de línea de22px de la página. Usar
// 19.25px conserva el resumen antiguo de211px en lugar de los216px del DOM real.
func TestElAsistenteMideLoDeLaWeb(t *testing.T) {
	a := NuevoAsistente(traductor(t), nil, 2026)
	a.Cargados(paisesArnes())
	a.Ventana = 1280
	v := tema.Claro
	_, alto := a.pasoPais(v, 504)
	cerca(t, "el paso del país", alto, 96)
	a.ElegirTipo("sociedad")
	_, alto = a.pasoForma(504)
	cerca(t, "el paso de la forma", alto, 187.625)
	_, alto = a.pasoDatos(504)
	cerca(t, "el paso de los datos", alto, 288)
	_, alto = a.pasoSituacion(504)
	cerca(t, "el paso de la situación", alto, 158)
	a.Forma = "sl"
	a.campos["nombre"].PonerTexto("Acme S.L.")
	a.campos["nif"].PonerTexto("B12345678")
	_, alto = a.pasoRevisar(v, 504)
	cerca(t, "el resumen", alto, 216)
	_, alto = a.pie(v, 504)
	cerca(t, "el pie", alto, 53)
}

// TestElAsistenteSePinta guarda sus pasos para mirarlos y comprueba que Obligaciones sin
// perfil lo abre y que «Editar perfil» lleva a él.
func TestElAsistenteSePinta(t *testing.T) {
	p, w := pantallaArnes(t, fyne.NewSize(1280, 800))
	defer w.Close()
	p.Organizacion, p.SabeOrg = nil, true
	p.IrA(VistaObligaciones)
	if p.asistente == nil {
		t.Fatal("Obligaciones sin perfil no abre el asistente")
	}
	a := p.asistente
	a.Cargados(paisesArnes())
	p.Repintar()
	guardarPNG(t, w.Canvas().Capture(), "erp-asistente-1.png")
	a.Paso = PasoForma
	a.ElegirTipo("sociedad")
	p.Repintar()
	guardarPNG(t, w.Canvas().Capture(), "erp-asistente-2.png")
	a.Forma, a.Paso = "sl", PasoRevisar
	a.campos["nombre"].PonerTexto("Acme S.L.")
	p.Repintar()
	guardarPNG(t, w.Canvas().Capture(), "erp-asistente-5.png")
	a.Paso, a.Total = PasoListo, 14
	a.Modulos = []Modulo{{Clave: "contactos", Etiqueta: "Contactos"}, {Clave: "facturacion", Etiqueta: "Facturación"}}
	p.Repintar()
	guardarPNG(t, w.Canvas().Capture(), "erp-asistente-6.png")
	if !contiene(textosDe(a.Objeto(tema.Claro)), "Áreas recomendadas") {
		t.Error("el último paso no propone las áreas")
	}
	p.Organizacion = &api.ERPOrganizacion{Nombre: "Acme S.L.", Pais: "ES", Forma: "sl"}
	p.oblig.AlEditarPerfil()
	if p.Vista != VistaAsistente || p.asistente.campos["nombre"].Texto() != "Acme S.L." {
		t.Errorf("«Editar perfil» no abre el asistente con el perfil: vista %q", p.Vista)
	}
}

// El JSX real a360 tiene dos etiquetas de situación a44px, y el pie de revisión
// en tres filas. Las otras pantallas mantienen su footer53px sin desbordar.
func TestElAsistenteCabeEnElMovilComoElDOM(t *testing.T) {
	a := NuevoAsistente(traductor(t), nil, 2026)
	a.Cargados(paisesArnes())
	a.Ventana = 360
	a.ElegirTipo("sociedad")
	a.Forma = "sl"
	a.campos["nombre"].PonerTexto("Acme S.L.")
	a.campos["nif"].PonerTexto("B12345678")
	v := tema.Claro
	_, alto := a.pasoPais(v, 312)
	cerca(t, "país móvil", alto, 112)
	_, alto = a.pasoForma(312)
	cerca(t, "forma móvil", alto, 275.5)
	_, alto = a.pasoDatos(312)
	cerca(t, "datos móvil", alto, 364)
	_, alto = a.pasoSituacion(312)
	cerca(t, "situación móvil", alto, 202)
	for _, clave := range []string{"iva_mensual", "intracomunitario"} {
		m := a.conmutadores[clave].MinSize()
		if m.Width > 312 || m.Height != 44 {
			t.Errorf("%s: %v, DOM label312x44", clave, m)
		}
	}
	_, alto = a.pasoRevisar(v, 312)
	cerca(t, "resumen móvil", alto, 216)
	for paso := PasoPais; paso <= PasoRevisar; paso++ {
		a.Paso = paso
		o, h := a.pie(v, 312)
		quiero := float32(53)
		if paso == PasoRevisar {
			quiero = 141
		}
		cerca(t, "pie móvil", h, quiero)
		l := o.(*fyne.Container).Layout.(*lienzo)
		for _, p := range l.puestos {
			if p.x < 0 || p.x+p.w > 312.3 {
				t.Errorf("mando del paso%d sale del pie: x%.2f ancho%.2f", paso, p.x, p.w)
			}
		}
	}
}
