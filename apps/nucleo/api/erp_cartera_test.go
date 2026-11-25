// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// La cartera, los recuentos filtrados, exportar en orden y el recordatorio hablan el
// mismo cable que `apps/frontend/apps/erp`.
func TestERPCarteraHablaElCableDelServicio(t *testing.T) {
	var vistas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		vistas = append(vistas, strings.TrimSpace(r.Method+" "+r.URL.RequestURI()+" "+string(cuerpo)))
		switch r.URL.Path {
		case "/api/v1/cobros/antiguedad":
			_, _ = io.WriteString(w, `{"modelo":"factura_venta","tramos":[{"tramo":"mas_90","centimos":400,"documentos":1}],"total":400,"vencido":400}`)
		case "/api/v1/contacto/c1/estado-cuenta":
			_, _ = io.WriteString(w, `{"ventas":{"facturado":400,"pendiente":100,"vencido":100},"documentos":[{"id":"f1","modelo":"factura_venta","ref":"FV-1","estado":"confirmado","total":100,"dias_vencida":24}]}`)
		case "/api/v1/inventario/bajo-minimo":
			_, _ = io.WriteString(w, `{"productos":[{"id":"p1","nombre":"Tornillo","minimo":"100","stock":"50.5","faltan":"49.5","almacenes":2}]}`)
		case "/api/v1/mayor":
			_, _ = io.WriteString(w, `{"cuenta":"430000","saldo_inicial":5,"apuntes":[{"asiento_id":"a1","ref":"V-1","fecha":"2026-11-02","debe":10,"haber":0,"saldo":15}],"saldo_final":15}`)
		case "/api/v1/factura_venta/estadisticas":
			_, _ = io.WriteString(w, `{"total":9,"filtrados":2,"por_estado":{}}`)
		case "/api/v1/factura_venta/export":
			w.Header().Set("Content-Disposition", `attachment; filename="facturas.csv"`)
			_, _ = io.WriteString(w, "ref\n")
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	defer srv.Close()
	e, ctx := NewERP(srv.URL, nil, nil, "es"), context.Background()

	if a, err := e.Antiguedad(ctx, "pagos", "2026-11-25"); err != nil || a.Vencido != 400 || a.Tramos[0].Tramo != "mas_90" {
		t.Fatalf("Antiguedad = %+v, %v", a, err)
	}
	if c, err := e.EstadoDeCuenta(ctx, "c1", "2026-11-25"); err != nil || c.Ventas.Vencido != 100 || c.Documentos[0].DiasVencida != 24 {
		t.Fatalf("EstadoDeCuenta = %+v, %v", c, err)
	}
	if ps, err := e.BajoMinimo(ctx); err != nil || len(ps) != 1 || ps[0].Stock != "50.5" {
		t.Fatalf("BajoMinimo = %+v, %v", ps, err)
	}
	if m, err := e.Mayor(ctx, "430000", "2026-01-01", ""); err != nil || m.SaldoFinal != 15 || len(m.Apuntes) != 1 {
		t.Fatalf("Mayor = %+v, %v", m, err)
	}
	if s, err := e.EstadisticasDe(ctx, "factura_venta", "acme", "pagado"); err != nil || s.Total != 9 {
		t.Fatalf("EstadisticasDe = %+v, %v", s, err)
	}
	cuerpo, nombre, err := e.ExportarEnOrden(ctx, "factura_venta", "", "pagado", nil, "total", false)
	if err != nil || nombre != "facturas.csv" {
		t.Fatalf("ExportarEnOrden = %q, %v", nombre, err)
	}
	_ = cuerpo.Close()
	if err := e.EnviarFactura(ctx, "f1", " ana@acme.eu ", "Recordatorio", "Hola", true); err != nil {
		t.Fatal(err)
	}
	quiero := []string{
		"GET /api/v1/cobros/antiguedad?hoy=2026-11-25&tipo=pagos",
		"GET /api/v1/contacto/c1/estado-cuenta?hoy=2026-11-25",
		"GET /api/v1/inventario/bajo-minimo",
		"GET /api/v1/mayor?cuenta=430000&desde=2026-01-01",
		"GET /api/v1/factura_venta/estadisticas?estado=pagado&q=acme",
		"GET /api/v1/factura_venta/export?estado=pagado&orden=total",
		`POST /api/v1/factura_venta/f1/enviar {"asunto":"Recordatorio","mensaje":"Hola","para":"ana@acme.eu","recordatorio":true}`,
	}
	if !slices.Equal(vistas, quiero) {
		t.Fatalf("el cable:\n%s\nquería:\n%s", strings.Join(vistas, "\n"), strings.Join(quiero, "\n"))
	}
}
