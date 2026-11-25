// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// Las rutas de «Importar desde otro programa» y las dos nuevas del TPV, con los cuerpos
// que manda la web (`ImportarDesde.tsx`, `PuntoVenta.tsx`): una instancia de la pasarela
// para probar y guardar, los objetos elegidos para importar, el presupuesto con su
// operación y las imágenes de los productos en tandas de 200.
func TestERPImportarDesdeYTPV(t *testing.T) {
	var cuerpos []string
	var consultas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		cuerpos = append(cuerpos, r.Method+" "+r.URL.Path+" "+string(b))
		switch r.URL.Path {
		case "/api/v1/conexiones":
			_, _ = io.WriteString(w, `{"conexiones":[{"proveedor":"odoo","instancia":"6f1c2a4e-1b2c-4d5e-8f90-123456789abc","activo":true,"tiene_firma":false}]}`)
		case "/api/v1/conexiones/holded/probar":
			var c map[string]string
			if json.Unmarshal(b, &c) != nil || c["instancia"] == "mal" {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = io.WriteString(w, `{"error":{"code":"unprocessable","message":"gateway (401)"}}`)
				return
			}
			_, _ = io.WriteString(w, `{"ok":true,"objeto":"contacts","filas":1}`)
		case "/api/v1/conexiones/holded":
			w.WriteHeader(http.StatusNoContent)
		case "/api/v1/importar/holded":
			_, _ = io.WriteString(w, `{"importado":[{"objeto":"contacts","modelo":"contacto","creados":12,"actualizados":3},{"objeto":"invoices","modelo":"factura_venta","creados":4,"actualizados":0,"fallos":["F-1: sin cliente"]}]}`)
		case "/api/v1/tpv/presupuesto":
			_, _ = io.WriteString(w, `{"id":"pr1","modelo":"presupuesto","ref":"P-2026-00001","data":{}}`)
		case "/api/v1/productos/imagenes":
			consultas = append(consultas, r.URL.Query().Get("registros"))
			ids := strings.Split(r.URL.Query().Get("registros"), ",")
			_, _ = io.WriteString(w, `{"imagenes":{"`+ids[0]+`":"adj-`+ids[0]+`"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c, ctx := NewERP(srv.URL, nil, nil, "es"), context.Background()

	cx, err := c.Conexiones(ctx)
	if err != nil || len(cx) != 1 || cx[0].Proveedor != "odoo" || cx[0].Instancia == "" {
		t.Fatalf("conexiones: %+v, %v", cx, err)
	}
	if err := c.ProbarConexion(ctx, "holded", " 6f1c2a4e-1b2c-4d5e-8f90-123456789abc "); err != nil {
		t.Fatalf("probar: %v", err)
	}
	var e *Error
	if err := c.ProbarConexion(ctx, "holded", "mal"); !errors.As(err, &e) || e.Status != http.StatusUnprocessableEntity {
		t.Fatalf("una instancia que no conecta tiene que dar 422: %v", err)
	}
	if err := c.GuardarConexion(ctx, "holded", "6f1c2a4e-1b2c-4d5e-8f90-123456789abc"); err != nil {
		t.Fatalf("guardar: %v", err)
	}
	r, err := c.ImportarDesde(ctx, "holded", []string{"contacts", "invoices"})
	if err != nil || len(r) != 2 || r[0].Creados != 12 || len(r[1].Fallos) != 1 {
		t.Fatalf("importar: %+v, %v", r, err)
	}
	p, err := c.PresupuestoTPV(ctx, ERPPresupuestoTPV{
		Operacion: "op-1", Cliente: "c1",
		Lineas: []ERPLineaTPV{{Producto: "p1", Cantidad: 2, Precio: 1210, TipoIVA: "21"}},
	})
	if err != nil || p.ID != "pr1" || p.Modelo != "presupuesto" {
		t.Fatalf("presupuesto: %+v, %v", p, err)
	}

	ids := make([]string, 250)
	for i := range ids {
		ids[i] = "p" + string(rune('a'+i%26)) + strings.Repeat("x", i/26)
	}
	imagenes, err := c.ImagenesProductos(ctx, ids)
	if err != nil || len(consultas) != 2 || len(strings.Split(consultas[0], ",")) != MaxImagenesProductos {
		t.Fatalf("imágenes: %d consultas (%v), %v", len(consultas), err, imagenes)
	}
	if imagenes[ids[0]] != "adj-"+ids[0] || imagenes[ids[MaxImagenesProductos]] != "adj-"+ids[MaxImagenesProductos] {
		t.Errorf("imágenes de las dos tandas: %v", imagenes)
	}

	esperados := []string{
		`POST /api/v1/conexiones/holded/probar {"instancia":"6f1c2a4e-1b2c-4d5e-8f90-123456789abc"}`,
		`POST /api/v1/conexiones/holded {"activo":true,"instancia":"6f1c2a4e-1b2c-4d5e-8f90-123456789abc"}`,
		`POST /api/v1/importar/holded {"objetos":["contacts","invoices"]}`,
		`POST /api/v1/tpv/presupuesto {"operacion":"op-1","cliente":"c1","lineas":[{"producto":"p1","cantidad":2,"precio":1210,"tipo_iva":"21"}]}`,
	}
	for _, q := range esperados {
		if !slices.Contains(cuerpos, q) {
			t.Errorf("no se pidió %s\n%s", q, strings.Join(cuerpos, "\n"))
		}
	}
}
