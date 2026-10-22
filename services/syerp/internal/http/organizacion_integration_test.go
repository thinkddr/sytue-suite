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

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
	"git.sytue.eu/sytue/one/services/syerp/internal/config"
	"git.sytue.eu/sytue/one/services/syerp/internal/obligaciones"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// LO QUE SE PRUEBA AQUÍ: el perfil se guarda y se lee por organización (RLS),
// el calendario sale del perfil guardado, el CSV entra por la misma puerta que
// sale y la factura sale como UBL con el emisor del perfil. Son cuatro
// manejadores encadenados sobre Postgres de verdad: probar cada uno con un
// store de mentira probaría el JSON, que es la parte que no falla.

func manejadoresCompletos(t *testing.T, st *store.Store) *Handlers {
	t.Helper()
	c, err := catalogo.Cargar()
	if err != nil {
		t.Fatalf("cargar catálogo: %v", err)
	}
	obl, err := obligaciones.Cargar(func(clave string) bool { return c.Rotulo(clave, "en") != "" })
	if err != nil {
		t.Fatalf("cargar obligaciones: %v", err)
	}
	cifra, err := keycrypto.New(bytes.Repeat([]byte("k"), keycrypto.KeyLen))
	if err != nil {
		t.Fatalf("cifra: %v", err)
	}
	return &Handlers{store: st, cat: c, cfg: config.Config{}, cifra: cifra, obligaciones: obl}
}

// llamar ejecuta un manejador como lo haría el router: tenant en el contexto,
// parámetros de ruta de chi y el cuerpo dado.
func llamar(t *testing.T, tenant, metodo, ruta, tipo, cuerpo string, params map[string]string,
	h func(http.ResponseWriter, *http.Request),
) *httptest.ResponseRecorder {
	t.Helper()
	var lector *strings.Reader
	if cuerpo == "" {
		lector = strings.NewReader("")
	} else {
		lector = strings.NewReader(cuerpo)
	}
	req := httptest.NewRequest(metodo, ruta, lector)
	req.Header.Set("Accept-Language", "es")
	if tipo != "" {
		req.Header.Set("Content-Type", tipo)
	}
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	ctx := tenancy.Into(req.Context(), tenancy.Tenant{Slug: tenant})
	req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

func TestPerfilObligacionesImportacionYUBL(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(harness.Pool)
	hs := manejadoresCompletos(t, st)

	t.Run("sin perfil, el calendario dice que falta el asistente", func(t *testing.T) {
		w := llamar(t, "acme", http.MethodGet, "/api/v1/obligaciones", "", "", nil, hs.Obligaciones)
		if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "organizacion_no_configurada") {
			t.Fatalf("sin perfil: %d %s", w.Code, w.Body.String())
		}
	})

	t.Run("un perfil incoherente no entra", func(t *testing.T) {
		w := llamar(t, "acme", http.MethodPut, "/api/v1/organizacion", "application/json",
			`{"nombre":"","pais":"ES","forma":"gmbh"}`, nil, hs.GuardarOrganizacion)
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("perfil inválido: %d %s", w.Code, w.Body.String())
		}
		for _, campo := range []string{`"nombre"`, `"forma"`} {
			if !strings.Contains(w.Body.String(), campo) {
				t.Errorf("el 422 no señala %s: %s", campo, w.Body.String())
			}
		}
	})

	t.Run("guardar, releer y calendario", func(t *testing.T) {
		w := llamar(t, "acme", http.MethodPut, "/api/v1/organizacion", "application/json",
			`{"nombre":"Acme Ibérica SL","nif":"B12345678","direccion":"Gran Vía 1","cp":"28013","ciudad":"Madrid",
			  "pais":"es","forma":"sl","condiciones":{"iva":true,"empleados":true,"intracomunitario":false}}`,
			nil, hs.GuardarOrganizacion)
		if w.Code != http.StatusOK {
			t.Fatalf("guardar: %d %s", w.Code, w.Body.String())
		}
		w = llamar(t, "acme", http.MethodGet, "/api/v1/organizacion", "", "", nil, hs.Organizacion)
		var o store.Organizacion
		if err := json.Unmarshal(w.Body.Bytes(), &o); err != nil || o.Pais != "ES" || o.Forma != "sl" || !o.Condiciones["empleados"] {
			t.Fatalf("releer: %d %s (%v)", w.Code, w.Body.String(), err)
		}
		if o.Condiciones["intracomunitario"] {
			t.Error("una condición apagada no se guarda como encendida")
		}
		// Otra organización no ve el perfil: la RLS aísla también esta tabla.
		w = llamar(t, "otra", http.MethodGet, "/api/v1/organizacion", "", "", nil, hs.Organizacion)
		if w.Code != http.StatusNotFound {
			t.Fatalf("otra organización ve el perfil de acme: %d", w.Code)
		}

		w = llamar(t, "acme", http.MethodGet, "/api/v1/obligaciones?anio=2026", "", "", nil, hs.Obligaciones)
		if w.Code != http.StatusOK {
			t.Fatalf("obligaciones: %d %s", w.Code, w.Body.String())
		}
		var cal struct {
			Verificado   bool `json:"verificado"`
			Obligaciones []struct {
				Clave          string `json:"clave"`
				Periodo        string `json:"periodo"`
				Limite         string `json:"limite"`
				ConceptoRotulo string `json:"concepto_rotulo"`
				AmbitoRotulo   string `json:"ambito_rotulo"`
				Calculable     bool   `json:"calculable"`
			} `json:"obligaciones"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &cal); err != nil {
			t.Fatal(err)
		}
		if !cal.Verificado {
			t.Error("España está verificada")
		}
		hay := map[string]bool{}
		for _, ob := range cal.Obligaciones {
			hay[ob.Clave] = true
			if ob.Clave == "es_303" && ob.Periodo == "2026-T1" {
				if !strings.HasPrefix(ob.Limite, "2026-04-20") || ob.ConceptoRotulo != "Declaración periódica de IVA" || ob.AmbitoRotulo != "Fiscal" || !ob.Calculable {
					t.Errorf("303 T1 = %+v", ob)
				}
			}
		}
		if !hay["es_111"] || !hay["es_200"] || hay["es_349"] || hay["es_130"] {
			t.Errorf("para una SL con empleados y sin intracomunitarias: 111 y 200 sí, 349 y 130 no; hay %v", hay)
		}
	})

	t.Run("el ICS es un calendario con un evento por vencimiento", func(t *testing.T) {
		w := llamar(t, "acme", http.MethodGet, "/api/v1/obligaciones/ics?anio=2026", "", "", nil, hs.ObligacionesICS)
		if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "text/calendar; charset=utf-8" {
			t.Fatalf("ics: %d %s", w.Code, w.Header().Get("Content-Type"))
		}
		if cd := w.Header().Get("Content-Disposition"); cd != `attachment; filename="obligaciones-es-2026.ics"` {
			t.Errorf("Content-Disposition = %q", cd)
		}
		cuerpo := w.Body.String()
		if !strings.Contains(cuerpo, "SUMMARY:Modelo 303 · 1T 2026") || !strings.Contains(cuerpo, "UID:acme-es_303-2026-T1@erp.sytue.eu") {
			t.Errorf("faltan el resumen o el UID del 303:\n%s", cuerpo[:min(len(cuerpo), 1500)])
		}
	})

	t.Run("el cálculo de una obligación sale del libro", func(t *testing.T) {
		w := llamar(t, "acme", http.MethodGet, "/api/v1/obligaciones/calculo?clave=es_303&periodo=2026-T1", "", "", nil, hs.ObligacionCalculo)
		if w.Code != http.StatusOK {
			t.Fatalf("calculo: %d %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"resultado_centimos":0`) || !strings.Contains(w.Body.String(), `"asentado":false`) {
			t.Errorf("sin asientos el resultado es cero y se dice: %s", w.Body.String())
		}
		w = llamar(t, "acme", http.MethodGet, "/api/v1/obligaciones/calculo?clave=es_cuentas&periodo=2026-A", "", "", nil, hs.ObligacionCalculo)
		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("una obligación sin cálculo declarado: %d", w.Code)
		}
	})

	t.Run("el CSV entra: rótulos en español, punto y coma, importes con coma y relación por nombre", func(t *testing.T) {
		csv := "Nombre;Tipo;Correo;Ciudad;externo_id\r\n" +
			"Acme Ibérica;Organización;hola@acme.es;Madrid;SF-1\r\n" +
			"Ada Lovelace;persona;ada@acme.es;;\r\n" +
			"Sin tipo;;x@y.z;;\r\n"
		w := llamar(t, "acme", http.MethodPost, "/api/v1/contacto/importar", "text/csv", csv,
			map[string]string{"modelo": "contacto"}, hs.Importar)
		if w.Code != http.StatusOK {
			t.Fatalf("importar contactos: %d %s", w.Code, w.Body.String())
		}
		var res struct {
			Filas, Creados, Actualizados int
			Fallos                       []falloDeFila
			Ignoradas                    []string
		}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		if res.Filas != 3 || res.Creados != 2 || len(res.Fallos) != 1 || res.Fallos[0].Fila != 4 {
			t.Fatalf("resultado = %+v", res)
		}
		// La misma fila con externo_id vuelve a entrar y ACTUALIZA, no duplica.
		w = llamar(t, "acme", http.MethodPost, "/api/v1/contacto/importar", "text/csv",
			"nombre,tipo,externo_id\nAcme Ibérica SL,organizacion,SF-1\n",
			map[string]string{"modelo": "contacto"}, hs.Importar)
		_ = json.Unmarshal(w.Body.Bytes(), &res)
		if res.Actualizados != 1 || res.Creados != 0 {
			t.Fatalf("segunda pasada = %+v", res)
		}

		// Una factura que apunta al cliente por su NOMBRE y trae el importe «a la
		// española».
		w = llamar(t, "acme", http.MethodPost, "/api/v1/factura_venta/importar", "text/csv",
			"Cliente;Fecha;Base;IVA;Total\nAcme Ibérica SL;20/04/2026;1.000,00;210,00;1.210,00\nNadie;2026-04-20;1;0;1\n",
			map[string]string{"modelo": "factura_venta"}, hs.Importar)
		if w.Code != http.StatusOK {
			t.Fatalf("importar facturas: %d %s", w.Code, w.Body.String())
		}
		_ = json.Unmarshal(w.Body.Bytes(), &res)
		if res.Creados != 1 || len(res.Fallos) != 1 || !strings.Contains(res.Fallos[0].Error, "Nadie") {
			t.Fatalf("facturas = %+v", res)
		}
	})

	t.Run("la factura sale como UBL con el emisor del perfil", func(t *testing.T) {
		ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
		factura, err := st.BuscarPorRef(ctx, "factura_venta", "FV-2026-00001")
		if err != nil {
			t.Fatalf("la factura importada no está: %v", err)
		}
		w := llamar(t, "acme", http.MethodGet, "/api/v1/factura_venta/"+factura.ID+"/ubl", "", "",
			map[string]string{"modelo": "factura_venta", "id": factura.ID}, hs.UBL)
		if w.Code != http.StatusOK {
			t.Fatalf("ubl: %d %s", w.Code, w.Body.String())
		}
		x := w.Body.String()
		for _, quiero := range []string{
			`<?xml version="1.0" encoding="UTF-8"?>`, `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"`,
			"<cbc:ID>FV-2026-00001</cbc:ID>", "<cbc:IssueDate>2026-04-20</cbc:IssueDate>",
			"<cbc:Name>Acme Ibérica SL</cbc:Name>", "<cbc:CompanyID>B12345678</cbc:CompanyID>",
			`<cbc:TaxAmount currencyID="EUR">210.00</cbc:TaxAmount>`, "<cbc:Percent>21.00</cbc:Percent>",
			`<cbc:PayableAmount currencyID="EUR">1210.00</cbc:PayableAmount>`, "<cbc:IdentificationCode>ES</cbc:IdentificationCode>",
		} {
			if !strings.Contains(x, quiero) {
				t.Errorf("falta %s en:\n%s", quiero, x)
			}
		}
		// Una factura de compra no se emite: no hay UBL de salida para ella.
		w = llamar(t, "acme", http.MethodGet, "/api/v1/factura_compra/x/ubl", "", "",
			map[string]string{"modelo": "factura_compra", "id": factura.ID}, hs.UBL)
		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("factura de compra como UBL: %d", w.Code)
		}
	})

	t.Run("la factura electrónica recibida entra como compra y la emitida sale como Facturae", func(t *testing.T) {
		ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
		compra := map[string]string{"modelo": "factura_compra"}
		var res struct {
			Filas, Creados, Actualizados int
			Fallos                       []falloDeFila
		}
		for pasada, quiero := range []struct{ creados, actualizados int }{{1, 0}, {0, 1}} {
			w := llamar(t, "acme", http.MethodPost, "/api/v1/factura_compra/importar", "application/xml", ublRecibida, compra, hs.Importar)
			if w.Code != http.StatusOK {
				t.Fatalf("pasada %d: %d %s", pasada+1, w.Code, w.Body.String())
			}
			res.Fallos = nil
			_ = json.Unmarshal(w.Body.Bytes(), &res)
			if res.Creados != quiero.creados || res.Actualizados != quiero.actualizados || len(res.Fallos) != 0 {
				t.Fatalf("pasada %d = %+v", pasada+1, res)
			}
		}
		// El proveedor se dio de alta UNA vez, y se encuentra por su NIF escrito de otra forma.
		proveedor, err := st.BuscarPorNIF(ctx, "contacto", "b-98765432")
		if err != nil {
			t.Fatalf("el proveedor no se dio de alta: %v", err)
		}
		factura, err := st.BuscarPorExterno(ctx, "factura_compra", "B98765432/F-2026-0042")
		if err != nil {
			t.Fatalf("la factura recibida no está: %v", err)
		}
		if factura.Data["proveedor"] != proveedor || factura.Data["base"] != float64(123456) || factura.Data["total"] != float64(149382) {
			t.Errorf("factura recibida = %+v", factura.Data)
		}

		// A nombre de otra organización: se cuenta como fallo y no entra.
		ajena := strings.Replace(ublRecibida, "ESB12345678", "ESB00000000", 1)
		w := llamar(t, "acme", http.MethodPost, "/api/v1/factura_compra/importar", "application/xml", ajena, compra, hs.Importar)
		res.Fallos = nil
		_ = json.Unmarshal(w.Body.Bytes(), &res)
		if w.Code != http.StatusOK || len(res.Fallos) != 1 || !strings.Contains(res.Fallos[0].Error, "no de esta organización") {
			t.Errorf("factura ajena: %d %s", w.Code, w.Body.String())
		}
		// Un contacto no se importa desde una factura.
		w = llamar(t, "acme", http.MethodPost, "/api/v1/contacto/importar", "application/xml", ublRecibida,
			map[string]string{"modelo": "contacto"}, hs.Importar)
		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("factura en contactos: %d", w.Code)
		}

		// La FV-2026-00001 es para un cliente sin dirección: Facturae lo dice antes de descargar.
		fv1, err := st.BuscarPorRef(ctx, "factura_venta", "FV-2026-00001")
		if err != nil {
			t.Fatal(err)
		}
		w = llamar(t, "acme", http.MethodGet, "/api/v1/factura_venta/"+fv1.ID+"/facturae", "", "",
			map[string]string{"modelo": "factura_venta", "id": fv1.ID}, hs.Facturae)
		if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "dirección") {
			t.Errorf("Facturae sin dirección del cliente: %d %s", w.Code, w.Body.String())
		}
		// Al proveedor, que sí la tiene, se le puede facturar y sale completa.
		w = llamar(t, "acme", http.MethodPost, "/api/v1/factura_venta/importar", "text/csv",
			"Cliente;Fecha;Base;IVA;Total\nProveedora Levante SL;2026-05-01;100,00;21,00;121,00\n",
			map[string]string{"modelo": "factura_venta"}, hs.Importar)
		if w.Code != http.StatusOK {
			t.Fatalf("factura al proveedor: %d %s", w.Code, w.Body.String())
		}
		fv2, err := st.BuscarPorRef(ctx, "factura_venta", "FV-2026-00002")
		if err != nil {
			t.Fatal(err)
		}
		w = llamar(t, "acme", http.MethodGet, "/api/v1/factura_venta/"+fv2.ID+"/facturae", "", "",
			map[string]string{"modelo": "factura_venta", "id": fv2.ID}, hs.Facturae)
		if w.Code != http.StatusOK {
			t.Fatalf("facturae: %d %s", w.Code, w.Body.String())
		}
		x := w.Body.String()
		for _, quiero := range []string{
			`<fe:Facturae xmlns:fe="http://www.facturae.gob.es/formato/Versiones/Facturaev3_2_2.xml">`,
			"<InvoiceNumber>FV-2026-00002</InvoiceNumber>", "<TaxIdentificationNumber>B98765432</TaxIdentificationNumber>",
			"<Province>València/Valencia</Province>", "<InvoiceTotal>121.00</InvoiceTotal>",
		} {
			if !strings.Contains(x, quiero) {
				t.Errorf("falta %s en:\n%s", quiero, x)
			}
		}
	})
}
