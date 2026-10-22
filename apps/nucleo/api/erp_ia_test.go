// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestERPIAPideLoQueLaWeb: cada llamada de Sytue AI va a la ruta y con el cuerpo de
// `SytueIA.tsx`, y la consulta de una parte vuelve al servicio BYTE A BYTE: la ejecuta
// él y el cliente no la entiende.
//
// VISTO EN ROJO (frontal, 2026-09-23): con `Consulta map[string]any` en vez de
// `json.RawMessage`, el orden de las claves cambia al volver («{"agrupar":…,"fuente":…}»)
// y la comparación de los cuerpos falla.
func TestERPIAPideLoQueLaWeb(t *testing.T) {
	var mu sync.Mutex
	var vistas []string
	const consulta = `{"fuente":"factura_venta","medida":{"op":"suma","campo":"total"},"agrupar":"cliente"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		mu.Lock()
		vistas = append(vistas, r.Method+" "+r.URL.Path+" "+r.Header.Get("Content-Type")+" "+string(cuerpo))
		mu.Unlock()
		switch r.URL.Path {
		case "/api/v1/ia/informe":
			_, _ = io.WriteString(w, `{"informe":{"titulo":"Ventas","partes":[{"titulo":"Por cliente","vista":"barras","consulta":`+consulta+`}]}}`)
		case "/api/v1/informes/datos":
			_, _ = io.WriteString(w, `{"datos":[{"columnas":[{"clave":"cliente","tipo":"texto","etiqueta":"Cliente"},{"clave":"total","tipo":"dinero","etiqueta":"Total"}],"filas":[["Lobo",1234.56]],"recortada":true}]}`)
		case "/api/v1/informes/excel":
			_, _ = io.WriteString(w, "PK")
		case "/api/v1/tableros":
			if r.Method == http.MethodGet {
				_, _ = io.WriteString(w, `{"tableros":[{"id":"t1","titulo":"Ventas","informe":{"titulo":"Ventas","partes":[]}}]}`)
				return
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"tablero":{"id":"t2","titulo":"Ventas","informe":{"titulo":"Ventas","partes":[]}}}`)
		case "/api/v1/ia/acciones":
			_, _ = io.WriteString(w, `{"resumen":"Un gasto","pasos":[{"ref":"p1","modelo":"gasto","data":{"importe":18000},"faltan":["fecha"]}]}`)
		case "/api/v1/ia/acciones/crear":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"creados":[{"id":"g1","modelo":"gasto","etiqueta":"Claude Code","data":{}}],"actividades":[]}`)
		case "/api/v1/gasto/ia/rellenar":
			_, _ = io.WriteString(w, `{"data":{"importe":18000,"proveedor":"c1"},"etiquetas":{"c1":"Anthropic"}}`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	c, ctx := NewERP(srv.URL, nil, nil, "es"), context.Background()

	inf, err := c.ProponerInforme(ctx, "ventas por cliente", ERPParaTablero, "factura_venta")
	if err != nil || len(inf.Partes) != 1 || inf.Partes[0].Vista != ERPVistaBarras {
		t.Fatalf("informe: %+v, %v", inf, err)
	}
	datos, err := c.DatosInforme(ctx, inf)
	if err != nil || len(datos) != 1 || !datos[0].Recortada || datos[0].Columnas[1].Tipo != ERPColDinero || datos[0].Filas[0][1] != 1234.56 {
		t.Errorf("datos: %+v, %v", datos, err)
	}
	if x, err := c.ExcelInforme(ctx, inf); err != nil {
		t.Errorf("excel: %v", err)
	} else {
		_ = x.Close()
	}
	if tb, err := c.GuardarTablero(ctx, inf); err != nil || tb.ID != "t2" {
		t.Errorf("guardar: %+v, %v", tb, err)
	}
	if ts, err := c.Tableros(ctx); err != nil || len(ts) != 1 || ts[0].Informe.Titulo != "Ventas" {
		t.Errorf("tableros: %+v, %v", ts, err)
	}
	if err := c.BorrarTablero(ctx, "t1"); err != nil {
		t.Errorf("borrar: %v", err)
	}
	plan, err := c.ProponerAcciones(ctx, "un gasto de 180 €")
	if err != nil || !plan.Incompleto() || plan.Pasos[0].Faltan[0] != "fecha" {
		t.Errorf("plan: %+v, %v", plan, err)
	}
	if cr, err := c.CrearAcciones(ctx, plan.Pasos); err != nil || len(cr.Creados) != 1 {
		t.Errorf("crear: %+v, %v", cr, err)
	}
	if rl, err := c.RellenarFicha(ctx, "gasto", "Factura de Anthropic por 180 €"); err != nil || rl.Etiquetas["c1"] != "Anthropic" {
		t.Errorf("rellenar: %+v, %v", rl, err)
	}
	if _, err := c.RellenarFichaPDF(ctx, "gasto", []byte("%PDF")); err != nil {
		t.Errorf("rellenar con PDF: %v", err)
	}

	informe := `{"informe":{"titulo":"Ventas","partes":[{"titulo":"Por cliente","vista":"barras","consulta":` + consulta + `}]}}`
	quiero := []string{
		`POST /api/v1/ia/informe application/json {"modelo":"factura_venta","para":"tablero","peticion":"ventas por cliente"}`,
		`POST /api/v1/informes/datos application/json ` + informe,
		`POST /api/v1/informes/excel application/json ` + informe,
		`POST /api/v1/tableros application/json ` + informe,
		`GET /api/v1/tableros  `,
		`DELETE /api/v1/tableros/t1  `,
		`POST /api/v1/ia/acciones application/json {"peticion":"un gasto de 180 €"}`,
		`POST /api/v1/ia/acciones/crear application/json {"pasos":[{"ref":"p1","modelo":"gasto","data":{"importe":18000},"faltan":["fecha"]}]}`,
		`POST /api/v1/gasto/ia/rellenar application/json {"texto":"Factura de Anthropic por 180 €"}`,
		`POST /api/v1/gasto/ia/rellenar application/pdf %PDF`,
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(vistas, "\n") != strings.Join(quiero, "\n") {
		t.Errorf("peticiones:\n%s\nquiero:\n%s", strings.Join(vistas, "\n"), strings.Join(quiero, "\n"))
	}
}

// TestERPIAHablaElIdiomaDelServicio lee el código del servicio: si un nombre de campo o
// una ruta de Sytue AI cambian allí, esto se pone en rojo aquí y no en producción.
func TestERPIAHablaElIdiomaDelServicio(t *testing.T) {
	for fichero, literales := range map[string][]string{
		"services/syerp/internal/http/server.go": {
			`largas.Post("/ia/informe"`, `largas.Post("/ia/acciones"`, `api.Post("/ia/acciones/crear"`,
			`api.Post("/informes/datos"`, `api.Post("/informes/excel"`, `api.Get("/tableros"`,
			`api.Post("/tableros"`, `api.Delete("/tableros/{id}"`, `mod.Post("/ia/rellenar"`,
		},
		"services/syerp/internal/http/handlers_ia.go": {
			`json:"peticion"`, `json:"para"`, `json:"modelo"`, `"informe": inf`, `"datos": h.tablas(`,
			`"tablero": t`, `json:"texto"`, `"application/pdf"`, `"data": data, "etiquetas": etiquetas`,
		},
		"services/syerp/internal/http/ia_acciones.go": {
			`json:"existente,omitempty"`, `json:"enlaces,omitempty"`, `json:"faltan,omitempty"`,
			`json:"tarea,omitempty"`, `"creados": creados, "actividades": actividades`,
		},
		"services/syerp/internal/ia/informe.go": {
			`json:"columnas"`, `json:"filas"`, `json:"recortada,omitempty"`, `json:"error,omitempty"`,
			`json:"titulo"`, `json:"vista"`, `json:"consulta"`,
		},
		"services/syerp/internal/ia/acciones.go": {`json:"resumen"`, `json:"avisos,omitempty"`},
	} {
		codigo := webref.Read(t, fichero)
		for _, l := range literales {
			if !strings.Contains(codigo, l) {
				t.Errorf("%s ya no dice %s", fichero, l)
			}
		}
	}
}
