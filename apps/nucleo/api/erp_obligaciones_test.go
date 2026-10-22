// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestERPObligacionesPideLoQueLaWeb: el calendario, «Preparar cifras» y el iCalendar van
// a las rutas de `Obligaciones.tsx`, y la respuesta se lee con sus fechas y sus faltas.
//
// VISTO EN ROJO (frontal, 2026-09-23): con `json:"pendiente"` en vez de
// `json:"pendientes"`, la fila no sabría qué le falta y diría «Todo listo». Y guardando
// el perfil con POST en vez de PUT, el servicio no tiene esa ruta.
func TestERPObligacionesPideLoQueLaWeb(t *testing.T) {
	var vistas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vistas = append(vistas, r.URL.Path+"?"+r.URL.RawQuery)
		switch r.URL.Path {
		case "/api/v1/obligaciones":
			_, _ = io.WriteString(w, `{"anio":2026,"pais":"ES","verificado":true,"moneda":"EUR","territorio":{"nombre":"Navarra","nota":"Foral"},
				"obligaciones":[{"clave":"303","nombre":"Modelo 303","ambito":"fiscal","ambito_rotulo":"Fiscal","periodo":"2026-T3",
				"limite":"2026-10-20T00:00:00Z","recomendada":"2026-10-14T00:00:00Z","limite_domiciliacion":"2026-10-15T00:00:00Z",
				"portal":{"nombre":"AEAT","url":"https://sede"},"calculable":true,"pendientes":{"banco":3},
				"sancion":{"tipo":"recargo","recargo_base_pct":1,"con_requerimiento_pct":[50,150],"fija_centimos":2500},
				"guia":{"que":"IVA","necesitas":["Facturas"],"pasos":["Entra"],"si_no":"Recargo"},
				"documentos":[{"clave":"justificante","nombre":"Justificante","requisitos":["CSV"]}]}]}`)
		case "/api/v1/obligaciones/calculo":
			_, _ = io.WriteString(w, `{"clave":"303","periodo":"2026-T3","desde":"2026-07-01","hasta":"2026-09-30","resultado_centimos":17226,"asentado":true}`)
		case "/api/v1/organizacion/paises":
			_, _ = io.WriteString(w, `{"paises":[{"pais":"ES","verificado":true,"moneda":"EUR","formas":[{"clave":"sl","nombre":"SL","tipo":"sociedad"}],"obligaciones":14}],"condiciones":["iva"]}`)
		case "/api/v1/organizacion":
			cuerpo, _ := io.ReadAll(r.Body)
			vistas = append(vistas, r.Method+" "+string(cuerpo))
			_, _ = w.Write(cuerpo)
		case "/api/v1/obligaciones/ics":
			w.Header().Set("Content-Disposition", `attachment; filename="obligaciones-2026.ics"`)
			_, _ = io.WriteString(w, "BEGIN:VCALENDAR")
		}
	}))
	defer srv.Close()
	c, ctx := NewERP(srv.URL, nil, nil, "es"), context.Background()

	o, err := c.Obligaciones(ctx, 2026)
	if err != nil || len(o.Obligaciones) != 1 || o.Territorio == nil {
		t.Fatalf("obligaciones: %+v, %v", o, err)
	}
	ob := o.Obligaciones[0]
	if ob.Pendientes["banco"] != 3 || ob.Recomendada.Day() != 14 || ob.LimiteDomiciliacion == nil ||
		ob.Sancion.FijaCentimos != 2500 || ob.Sancion.ConRequerimientoPct[1] != 150 ||
		ob.Guia.SiNo != "Recargo" || ob.Documentos[0].Requisitos[0] != "CSV" {
		t.Errorf("obligación mal leída: %+v", ob)
	}
	calc, err := c.CalculoObligacion(ctx, "303", "2026-T3")
	if err != nil || calc.ResultadoCentimos != 17226 || !calc.Asentado {
		t.Errorf("cálculo: %+v, %v", calc, err)
	}
	ics, nombre, err := c.ObligacionesICS(ctx, 2026)
	if err != nil || nombre != "obligaciones-2026.ics" {
		t.Fatalf("ics: %q, %v", nombre, err)
	}
	_ = ics.Close()
	ps, err := c.Paises(ctx)
	if err != nil || len(ps) != 1 || ps[0].Formas[0].Tipo != "sociedad" || ps[0].Obligaciones != 14 {
		t.Errorf("países: %+v, %v", ps, err)
	}
	org, err := c.GuardarOrganizacion(ctx, ERPOrganizacion{Nombre: "Acme", Pais: "ES", Forma: "sl", Condiciones: map[string]bool{"iva": true}})
	if err != nil || org.Nombre != "Acme" || !org.Condiciones["iva"] {
		t.Errorf("guardar: %+v, %v", org, err)
	}

	quiero := []string{
		"/api/v1/obligaciones?anio=2026",
		"/api/v1/obligaciones/calculo?clave=303&periodo=2026-T3",
		"/api/v1/obligaciones/ics?anio=2026",
		"/api/v1/organizacion/paises?",
		"/api/v1/organizacion?",
		`PUT {"nombre":"Acme","nif":"","direccion":"","cp":"","ciudad":"","pais":"ES","forma":"sl","condiciones":{"iva":true}}`,
	}
	if strings.Join(vistas, "\n") != strings.Join(quiero, "\n") {
		t.Errorf("peticiones:\n%s\nquiero:\n%s", strings.Join(vistas, "\n"), strings.Join(quiero, "\n"))
	}
}

// TestERPObligacionesHablaElIdiomaDelServicio lee el servicio: si un campo o una ruta
// cambian allí, esto se pone en rojo aquí y no en producción.
func TestERPObligacionesHablaElIdiomaDelServicio(t *testing.T) {
	for fichero, literales := range map[string][]string{
		"services/syerp/internal/obligaciones/obligaciones.go": {
			`json:"recomendada"`, `json:"limite_domiciliacion,omitempty"`, `json:"calendario_provisional,omitempty"`,
			`json:"con_requerimiento_pct,omitempty"`, `json:"fija_centimos,omitempty"`, `json:"si_no"`, `json:"requisitos"`,
		},
		"services/syerp/internal/http/handlers_organizacion.go": {
			`json:"concepto_rotulo"`, `json:"ambito_rotulo"`, `json:"pendientes,omitempty"`, `"territorio"`,
			`"resultado_centimos"`, `"asentado"`, `Get("anio")`, `json:"formas"`, `json:"obligaciones"`,
		},
		"services/syerp/internal/store/organizacion.go": {
			`json:"direccion"`, `json:"cp"`, `json:"ciudad"`, `json:"condiciones"`,
		},
		"services/syerp/internal/http/server.go": {
			`api.Get("/organizacion/paises"`, `Put("/organizacion"`,
			`obl.Get("/obligaciones"`, `obl.Get("/obligaciones/ics"`, `obl.Get("/obligaciones/calculo"`,
		},
	} {
		codigo := webref.Read(t, fichero)
		for _, l := range literales {
			if !strings.Contains(codigo, l) {
				t.Errorf("%s ya no dice %s", fichero, l)
			}
		}
	}
}
