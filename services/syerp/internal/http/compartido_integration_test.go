// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package syerphttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// comoPersona ejecuta un manejador con la cadena de middlewares que importa aquí
// (permisos de la petición y, si se da, el de un módulo o el del CRUD genérico) y
// con los claims de una persona de la organización.
func comoPersona(t *testing.T, sub string, roles []string, metodo, ruta, cuerpo string, params map[string]string,
	h http.Handler,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(metodo, ruta, bytes.NewReader([]byte(cuerpo)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", "es")
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	ctx := tenancy.Into(req.Context(), tenancy.Tenant{Slug: "acme"})
	ctx = authn.Into(ctx, authn.Claims{Subject: sub, Email: sub + "@acme.es", Name: strings.ToUpper(sub), Roles: roles, Tenant: "acme"})
	req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	conPermisos(h).ServeHTTP(w, req)
	return w
}

func TestPermisosPorModuloYAccionesDeIA(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(harness.Pool)
	hs := manejadoresCompletos(t, st)
	miembro := []string{"member"}

	// Sin nivel puesto, un miembro ve contactos y no ve facturas recibidas
	// (facturación empieza cerrada).
	listar := func(modelo string) int {
		return comoPersona(t, "luis", miembro, http.MethodGet, "/api/v1/"+modelo, "", map[string]string{"modelo": modelo},
			hs.exigirModelo(http.HandlerFunc(hs.Listar))).Code
	}
	if listar("contacto") != http.StatusOK || listar("factura_compra") != http.StatusForbidden {
		t.Fatal("contactos abierto y facturación cerrada por defecto")
	}
	w := comoPersona(t, "luis", miembro, http.MethodGet, "/api/v1/obligaciones", "", nil,
		hs.exigir("obligaciones", "lectura")(http.HandlerFunc(hs.Obligaciones)))
	if w.Code != http.StatusForbidden {
		t.Fatalf("obligaciones cerrado para un miembro: %d", w.Code)
	}

	// La administración le da lectura en facturación: lee, pero no crea.
	w = comoPersona(t, "jefa", []string{"owner"}, http.MethodPut, "/api/v1/admin/permisos/luis",
		`{"email":"luis@acme.es","nombre":"Luis","modulos":{"facturacion":"lectura","crm":"usuario","inventado":"x"}}`,
		map[string]string{"sub": "luis"}, http.HandlerFunc(hs.GuardarPermisos))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("un módulo inventado no se guarda: %d %s", w.Code, w.Body.String())
	}
	w = comoPersona(t, "jefa", []string{"owner"}, http.MethodPut, "/api/v1/admin/permisos/luis",
		`{"email":"luis@acme.es","nombre":"Luis","modulos":{"facturacion":"lectura","crm":"usuario"}}`,
		map[string]string{"sub": "luis"}, http.HandlerFunc(hs.GuardarPermisos))
	if w.Code != http.StatusNoContent {
		t.Fatalf("guardar permisos: %d %s", w.Code, w.Body.String())
	}
	if p, _, _ := st.PermisosDe(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"}), "luis"); len(p) != 1 || p["facturacion"] != "lectura" {
		t.Fatalf("solo se guarda lo distinto del defecto (crm ya era usuario): %v", p)
	}
	if listar("factura_compra") != http.StatusOK {
		t.Fatal("con lectura, lista")
	}
	w = comoPersona(t, "luis", miembro, http.MethodPost, "/api/v1/factura_compra", `{"data":{}}`,
		map[string]string{"modelo": "factura_compra"}, hs.exigirModelo(http.HandlerFunc(hs.Crear)))
	if w.Code != http.StatusForbidden {
		t.Fatalf("con lectura no crea: %d", w.Code)
	}
	w = comoPersona(t, "luis", miembro, http.MethodGet, "/api/v1/mis-permisos", "", nil, http.HandlerFunc(hs.MisPermisos))
	var mios struct {
		Administrador bool              `json:"administrador"`
		Modulos       map[string]string `json:"modulos"`
	}
	if json.Unmarshal(w.Body.Bytes(), &mios) != nil || mios.Administrador || mios.Modulos["facturacion"] != "lectura" ||
		mios.Modulos["nominas"] != "ninguno" || mios.Modulos["obligaciones"] != "ninguno" {
		t.Fatalf("mis permisos: %s", w.Body.String())
	}

	// La dueña crea con Sytue AI un gasto de Anthropic: proveedor nuevo, factura y
	// la tarea de subir el PDF, para hoy. (El plan es el que la persona ha visto.)
	plan := `{"pasos":[
		{"ref":"p1","modelo":"contacto","data":{"nombre":"Anthropic Ireland, Limited","tipo":"organizacion"}},
		{"ref":"p2","modelo":"factura_compra","data":{"fecha":"2026-09-15","base":18000,"iva":0,"total":18000,"notas":"Claude Code"},"enlaces":{"proveedor":"p1"}}
	]}`
	w = comoPersona(t, "jefa", []string{"owner"}, http.MethodPost, "/api/v1/ia/acciones/crear", plan, nil, http.HandlerFunc(hs.CrearAcciones))
	var creado struct {
		Creados     []store.Registro  `json:"creados"`
		Actividades []store.Actividad `json:"actividades"`
	}
	if w.Code != http.StatusCreated || json.Unmarshal(w.Body.Bytes(), &creado) != nil || len(creado.Creados) != 2 ||
		len(creado.Actividades) != 1 || creado.Actividades[0].Tipo != "subir_documento" || creado.Actividades[0].ResponsableSub != "jefa" {
		t.Fatalf("crear con IA: %d %s", w.Code, w.Body.String())
	}
	factura := creado.Creados[1]
	if factura.Data["proveedor"] != creado.Creados[0].ID || !strings.HasPrefix(factura.Ref, "FC-") {
		t.Fatalf("la factura apunta al proveedor recién creado: %+v", factura)
	}

	// Un segundo gasto con el proveedor ya existente no lo duplica.
	plan = `{"pasos":[
		{"ref":"p1","modelo":"contacto","existente":{"id":"` + creado.Creados[0].ID + `","etiqueta":"Anthropic"},"data":{}},
		{"ref":"p2","modelo":"factura_compra","data":{"fecha":"2026-10-15","base":18000,"iva":0,"total":18000},"enlaces":{"proveedor":"p1"}}
	]}`
	w = comoPersona(t, "jefa", []string{"owner"}, http.MethodPost, "/api/v1/ia/acciones/crear", plan, nil, http.HandlerFunc(hs.CrearAcciones))
	if w.Code != http.StatusCreated || strings.Count(w.Body.String(), `"modelo":"contacto"`) != 0 {
		t.Fatalf("con el proveedor existente solo se crea la factura: %d %s", w.Code, w.Body.String())
	}
	// Y Luis, con solo lectura en facturación, no puede crear gastos con la IA.
	w = comoPersona(t, "luis", miembro, http.MethodPost, "/api/v1/ia/acciones/crear", plan, nil, http.HandlerFunc(hs.CrearAcciones))
	if w.Code != http.StatusForbidden {
		t.Fatalf("la IA no crea lo que la persona no puede crear: %d", w.Code)
	}

	// Subir el PDF cierra la tarea; un HTML con nombre de PDF no entra.
	subir := func(nombre string, contenido []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/adjuntos?modelo=factura_compra&registro="+factura.ID+"&nombre="+nombre, bytes.NewReader(contenido))
		ctx := tenancy.Into(req.Context(), tenancy.Tenant{Slug: "acme"})
		ctx = authn.Into(ctx, authn.Claims{Subject: "jefa", Roles: []string{"owner"}, Tenant: "acme"})
		rec := httptest.NewRecorder()
		conPermisos(http.HandlerFunc(hs.SubirAdjunto)).ServeHTTP(rec, req.WithContext(ctx))
		return rec
	}
	if w := subir("factura.pdf", []byte("<html><script>alert(1)</script></html>")); w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("un HTML disfrazado de PDF: %d %s", w.Code, w.Body.String())
	}
	if w := subir("Factura%20Anthropic", []byte("%PDF-1.7\n%%EOF")); w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"tareas_hechas":1`) ||
		!strings.Contains(w.Body.String(), `"nombre":"Factura Anthropic.pdf"`) {
		t.Fatalf("subir la factura: %d %s", w.Code, w.Body.String())
	}
}
