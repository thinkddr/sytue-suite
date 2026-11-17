// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestERPAprobacionesPideLoQueLaWeb: cada llamada va a la ruta, con el método y el
// cuerpo que usa la web (Aprobaciones.tsx y la ficha de ErpApp.tsx).
func TestERPAprobacionesPideLoQueLaWeb(t *testing.T) {
	var mu sync.Mutex
	var vistas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		mu.Lock()
		vistas = append(vistas, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery+" "+string(cuerpo))
		mu.Unlock()
		switch {
		case r.URL.Path == "/api/v1/aprobaciones":
			_, _ = io.WriteString(w, `{"aprobaciones":[{"id":"ap1","modelo":"factura_compra","registro_id":"g1",`+
				`"importe":1250000,"moneda":"EUR","estado":"pendiente","solicitada_por":"ana","solicitada_en":"2026-11-10T09:30:00Z",`+
				`"creada_por":"luis"}]}`)
		case r.URL.Path == "/api/v1/factura_compra/g1/aprobacion":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"ap2","estado":"pendiente","importe":1250000,"moneda":"EUR"}`)
		case r.URL.Path == "/api/v1/admin/umbrales" && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"umbrales":[{"modulo":"compras","importe":500000,"moneda":"EUR"}],`+
				`"moneda_por_defecto":"EUR","modulos":[{"clave":"compras","rotulo":"Compras"}]}`)
		case r.URL.Path == "/api/v1/aprobaciones/ap1/aprobar":
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":{"code":"cuatro_ojos","message":"no"}}`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	c, ctx := NewERP(srv.URL, nil, nil, "es"), context.Background()

	as, err := c.Aprobaciones(ctx, "pendiente")
	if err != nil || len(as) != 1 || as[0].Importe != 1250000 || as[0].RegistroID != "g1" || as[0].CreadaPor != "luis" {
		t.Fatalf("aprobaciones: %+v, %v", as, err)
	}
	// Cuatro ojos: ni quien la pidió ni quien creó el documento.
	for yo, puede := range map[string]bool{"ana": false, "luis": false, "eva": true} {
		if as[0].PuedeResolverla(yo) != puede {
			t.Errorf("%s ¿puede resolverla? %v", yo, !puede)
		}
	}
	if a, err := c.PedirAprobacion(ctx, "factura_compra", "g1"); err != nil || a.ID != "ap2" {
		t.Errorf("pedir: %+v, %v", a, err)
	}
	var e *Error
	if err := c.ResolverAprobacion(ctx, "ap1", true, ""); !errors.As(err, &e) || e.Code() != "cuatro_ojos" {
		t.Errorf("el 403 tiene que llegar con su código: %v", err)
	}
	if err := c.ResolverAprobacion(ctx, "ap1", false, "Falta el pedido"); err != nil {
		t.Errorf("rechazar: %v", err)
	}
	u, err := c.Umbrales(ctx)
	if err != nil || len(u.Umbrales) != 1 || u.Umbrales[0].Importe != 500000 || u.Modulos[0].Rotulo != "Compras" {
		t.Errorf("umbrales: %+v, %v", u, err)
	}
	if err := c.GuardarUmbral(ctx, "compras", -1, "EUR"); err != nil {
		t.Errorf("quitar el umbral: %v", err)
	}

	quiere := []string{
		"GET /api/v1/aprobaciones?estado=pendiente ",
		"POST /api/v1/factura_compra/g1/aprobacion? {}",
		`POST /api/v1/aprobaciones/ap1/aprobar? {"motivo":""}`,
		`POST /api/v1/aprobaciones/ap1/rechazar? {"motivo":"Falta el pedido"}`,
		"GET /api/v1/admin/umbrales? ",
		`PUT /api/v1/admin/umbrales/compras? {"importe":-1,"moneda":"EUR"}`,
	}
	if !slices.Equal(vistas, quiere) {
		t.Errorf("peticiones:\n%s\nquiere:\n%s", strings.Join(vistas, "\n"), strings.Join(quiere, "\n"))
	}
}

// TestERPAprobacionesHablaElIdiomaDelServicio lee el código del servicio: si cambia un
// campo o una ruta, esto se pone en rojo aquí y no en producción.
func TestERPAprobacionesHablaElIdiomaDelServicio(t *testing.T) {
	for fichero, literales := range map[string][]string{
		"services/syerp/internal/store/aprobaciones.go": {
			`json:"registro_id"`, `json:"solicitada_por"`, `json:"solicitada_en"`, `json:"creada_por,omitempty"`,
			`json:"motivo,omitempty"`, `json:"importe"`, `json:"moneda"`, `json:"modulo"`,
		},
		"services/syerp/internal/http/aprobaciones.go": {
			`"moneda_por_defecto"`, `"modulos"`, `"umbrales"`, `"aprobaciones"`, `json:"motivo"`,
			`"requiere_aprobacion"`, `"cuatro_ojos"`,
		},
		// La lista de módulos con su nombre es la de los permisos.
		"services/syerp/internal/http/permisos.go": {`"clave": m.Clave, "rotulo": h.cat.Rotulo(`},
		"services/syerp/internal/http/server.go": {
			`api.Get("/aprobaciones"`, `"/aprobaciones/{id}/aprobar"`, `"/aprobaciones/{id}/rechazar"`,
			`Get("/admin/umbrales"`, `Put("/admin/umbrales/{modulo}"`, `Post("/{id}/aprobacion"`,
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
