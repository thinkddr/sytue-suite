// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// cajaFalsa es el servicio de TPV de mentira: emite una factura por operación, como
// `recuperarCobroTPV`, y puede CORTAR la conexión después de emitirla, que es la
// respuesta perdida que obliga a reintentar.
type cajaFalsa struct {
	mu       sync.Mutex
	facturas map[string]string // operación → id de factura
	cuerpos  []string
	cortar   int // cuántas respuestas más se pierden
}

func (c *cajaFalsa) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cuerpo, _ := io.ReadAll(r.Body)
	var cobro ERPCobroTPV
	if r.Method != http.MethodPost || r.URL.Path != "/api/v1/tpv/cobrar" || json.Unmarshal(cuerpo, &cobro) != nil {
		http.Error(w, `{"code":"bad_request"}`, http.StatusBadRequest)
		return
	}
	c.mu.Lock()
	c.cuerpos = append(c.cuerpos, string(cuerpo))
	id, ok := c.facturas[cobro.Operacion]
	if !ok {
		id = "fv" + string(rune('0'+len(c.facturas)+1))
		c.facturas[cobro.Operacion] = id
	}
	cortar := c.cortar > 0
	if cortar {
		c.cortar--
	}
	c.mu.Unlock()
	if cortar {
		// La venta YA está registrada; lo que se pierde es la respuesta.
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
		return
	}
	_, _ = io.WriteString(w, `{"factura":{"id":"`+id+`","modelo":"factura_venta","ref":"FS-2026-00007","data":{"total":1815}},"cambio":185}`)
}

// TestERPCobrarTPVReintentaSinDuplicar: la primera respuesta se corta, el reintento va
// con el MISMO cuerpo —la misma operación— y la caja acaba con UNA factura.
//
// Guarda que el cliente no invente la operación por su cuenta: con un UUID nuevo en cada
// llamada, el reintento emitiría fv2.
func TestERPCobrarTPVReintentaSinDuplicar(t *testing.T) {
	caja := &cajaFalsa{facturas: map[string]string{}, cortar: 1}
	srv := httptest.NewServer(caja)
	defer srv.Close()
	c, ctx := NewERP(srv.URL, nil, nil, "es"), context.Background()
	cobro := ERPCobroTPV{
		Operacion: "7b1c1d52-4f3e-4a59-9a43-0d5c6f0e2a11", Sesion: "s1", Medio: ERPMedioEfectivo, Recibido: 2000,
		Lineas: []ERPLineaTPV{{Producto: "p1", Cantidad: 3, Precio: 605, TipoIVA: "21"}},
	}
	if _, err := c.CobrarTPV(ctx, cobro); err == nil {
		t.Fatal("con la respuesta cortada, el cobro debe dar error: no se sabe si se registró")
	}
	v, err := c.CobrarTPV(ctx, cobro)
	if err != nil {
		t.Fatalf("reintento: %v", err)
	}
	if v.Factura.ID != "fv1" || v.Factura.Ref != "FS-2026-00007" || v.Cambio != 185 {
		t.Errorf("venta: %+v", v)
	}
	if len(caja.facturas) != 1 {
		t.Errorf("%d facturas para un solo cobro", len(caja.facturas))
	}
	if len(caja.cuerpos) != 2 || caja.cuerpos[0] != caja.cuerpos[1] {
		t.Errorf("el reintento debe mandar el mismo cuerpo:\n%q", caja.cuerpos)
	}
	quiere := `{"operacion":"7b1c1d52-4f3e-4a59-9a43-0d5c6f0e2a11","sesion":"s1","cliente":"","medio":"efectivo","recibido":2000,` +
		`"lineas":[{"producto":"p1","cantidad":3,"precio":605,"tipo_iva":"21"}]}`
	if caja.cuerpos[0] != quiere {
		t.Errorf("cuerpo:\n%s\nquiere\n%s", caja.cuerpos[0], quiere)
	}
}

// TestERPTPVHablaElIdiomaDelServicio lee el servicio: los nombres del cuerpo y la ruta.
func TestERPTPVHablaElIdiomaDelServicio(t *testing.T) {
	for fichero, literales := range map[string][]string{
		"services/syerp/internal/store/tpv.go": {
			`json:"operacion"`, `json:"sesion"`, `json:"cliente"`, `json:"medio"`, `json:"recibido"`, `json:"lineas"`,
			`json:"producto"`, `json:"cantidad"`, `json:"precio"`, `json:"tipo_iva"`, `json:"factura"`, `json:"cambio"`,
			`c.Medio != "efectivo" && c.Medio != "tarjeta"`,
		},
		"services/syerp/internal/http/server.go": {`Post("/tpv/cobrar"`},
	} {
		codigo := webref.Read(t, fichero)
		for _, l := range literales {
			if !strings.Contains(codigo, l) {
				t.Errorf("%s ya no dice %s", fichero, l)
			}
		}
	}
}
