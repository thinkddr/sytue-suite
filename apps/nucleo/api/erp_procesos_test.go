// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

func TestERPProcesoObligacionConservaElEstadoDelServicio(t *testing.T) {
	lecturas, escrituras := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/obligaciones/proceso" || r.URL.Query().Get("clave") != "303" || r.URL.Query().Get("periodo") != "2026-T3" {
			t.Errorf("ruta del proceso: %s", r.URL.String())
		}
		if r.Header.Get("Accept-Language") != "es" {
			t.Error("la petición debe conservar el idioma de la sesión")
		}
		switch r.Method {
		case http.MethodGet:
			lecturas++
		case http.MethodPost:
			escrituras++
			var p ERPProcesoObligacion
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil || p.Fase != 2 || p.Referencia != "CSV-123" || r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("avance: %+v, %v", p, err)
			}
		default:
			t.Errorf("método: %s", r.Method)
		}
		// Otra persona pudo terminarlo: el cliente no sustituye esta fase por la solicitada.
		_ = json.NewEncoder(w).Encode(ERPProcesoObligacion{Fase: 4, Referencia: "CSV-123"})
	}))
	defer srv.Close()
	c := NewERP(srv.URL, nil, nil, "es")
	p, err := c.ProcesoObligacion(context.Background(), "303", "2026-T3")
	if err != nil || p.Fase != 4 || lecturas != 1 || escrituras != 0 {
		t.Fatalf("leer no debe escribir: %+v, %v; lecturas=%d escrituras=%d", p, err, lecturas, escrituras)
	}
	p, err = c.AvanzarProcesoObligacion(context.Background(), "303", "2026-T3", 2, "CSV-123")
	if err != nil || p.Fase != 4 || p.Referencia != "CSV-123" || escrituras != 1 {
		t.Fatalf("estado compartido: %+v, %v", p, err)
	}
}

func TestERPProcesoPaginaPendientesYPropagaLaDenegacion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/obligaciones/proceso" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"Sin permiso"}}`))
			return
		}
		q := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/obligaciones/proceso/pendientes" || q.Get("clave") != "303" || q.Get("periodo") != "2026-T3" || q.Get("cursor") != "marca+firma" || q.Get("limite") != "30" {
			t.Errorf("página de pendientes: %s %s", r.Method, r.URL.String())
		}
		_, _ = w.Write([]byte(`{"registros":[{"id":"f1","modelo":"factura_venta","etiqueta":"Factura"}],"siguiente":"segunda"}`))
	}))
	defer srv.Close()
	c := NewERP(srv.URL, nil, nil, "es")
	p, err := c.FacturasPendientesObligacion(context.Background(), "303", "2026-T3", "marca+firma", 30)
	if err != nil || len(p.Registros) != 1 || p.Registros[0].ID != "f1" || p.Registros[0].Modelo != "factura_venta" || p.Siguiente != "segunda" {
		t.Fatalf("página: %+v, %v", p, err)
	}
	if _, err := c.AvanzarProcesoObligacion(context.Background(), "303", "2026-T3", 1, ""); err == nil {
		t.Fatal("una denegación de permisos no puede parecer un paso completado")
	}
}

func TestERPProcesoUsaLasRutasDelServicio(t *testing.T) {
	servidor := webref.Read(t, "services/syerp/internal/http/server.go")
	for _, ruta := range []string{
		`obl.Get("/obligaciones/proceso",`,
		`obl.Post("/obligaciones/proceso",`,
		`obl.Get("/obligaciones/proceso/pendientes",`,
	} {
		if !strings.Contains(servidor, ruta) {
			t.Errorf("la ruta del cliente ya no existe: %s", ruta)
		}
	}
	var calendario ERPObligaciones
	if err := json.Unmarshal([]byte(`{"obligaciones":[{"clave":"303","proceso":{"fase":3,"referencia":"CSV-321"}}]}`), &calendario); err != nil {
		t.Fatal(err)
	}
	if len(calendario.Obligaciones) != 1 || calendario.Obligaciones[0].Proceso == nil || calendario.Obligaciones[0].Proceso.Fase != 3 || calendario.Obligaciones[0].Proceso.Referencia != "CSV-321" {
		t.Fatalf("el calendario debe mostrar el progreso compartido: %+v", calendario)
	}
}
