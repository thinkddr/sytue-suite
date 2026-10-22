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

// TestERPBancoPideLoQueLaWeb: las tareas, casar, explicar, reabrir, las pistas y el
// extracto van a las rutas de `Banco.tsx`, y la respuesta se lee con sus importes.
//
// VISTO EN ROJO (frontal, 2026-09-23): con `json:"centimos"` —el nombre del campo en
// Go— en vez de `json:"importe"`, todos los movimientos saldrían a cero.
func TestERPBancoPideLoQueLaWeb(t *testing.T) {
	var vistas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		vistas = append(vistas, r.Method+" "+r.URL.Path+" "+string(cuerpo))
		switch r.URL.Path {
		case "/api/v1/banco":
			_, _ = io.WriteString(w, `{"pendientes":[{"id":"m1","fecha":"2026-09-01","descripcion":"GOOGLE","importe":-1210,
				"empresa":"Google","url_factura":"https://x","sugerencia":"otro","candidatas":[{"id":"f1","ref":"FC-1","tercero":"Google","total":1210,"segura":true}]}],
				"total_pendientes":1,"hechas":[],"motivos":[{"clave":"comision","rotulo":"Comisión"}]}`)
		case "/api/v1/banco/pistas":
			_, _ = io.WriteString(w, `{"buscadas":2}`)
		case "/api/v1/linea_extracto/importar":
			_, _ = io.WriteString(w, `{"cuentas":["1"],"nuevos":9,"casados":2,"explicados":1,"pendientes":6}`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	defer srv.Close()
	c, ctx := NewERP(srv.URL, nil, nil, "es"), context.Background()
	b, err := c.Banco(ctx)
	if err != nil || len(b.Pendientes) != 1 || b.Pendientes[0].Importe != -1210 || !b.Pendientes[0].Candidatas[0].Segura ||
		b.Pendientes[0].Empresa != "Google" || b.Motivos[0].Rotulo != "Comisión" {
		t.Fatalf("banco: %+v, %v", b, err)
	}
	_ = c.ConciliarMovimiento(ctx, "m1", "f1")
	_ = c.ExplicarMovimiento(ctx, "m1", "comision")
	_ = c.ReabrirMovimiento(ctx, "m1")
	if n, err := c.PistasBanco(ctx); err != nil || n != 2 {
		t.Errorf("pistas: %d, %v", n, err)
	}
	if r, err := c.ImportarExtracto(ctx, []byte("11\xd1")); err != nil || r.Nuevos != 9 || r.Pendientes != 6 {
		t.Errorf("extracto: %+v, %v", r, err)
	}
	quiero := []string{
		"GET /api/v1/banco ",
		`POST /api/v1/banco/m1/conciliar {"factura":"f1"}`,
		`POST /api/v1/banco/m1/explicar {"motivo":"comision"}`,
		"POST /api/v1/banco/m1/reabrir {}",
		"POST /api/v1/banco/pistas {}",
		"POST /api/v1/linea_extracto/importar 11\xd1",
	}
	if strings.Join(vistas, "\n") != strings.Join(quiero, "\n") {
		t.Errorf("peticiones:\n%s\nquiero:\n%s", strings.Join(vistas, "\n"), strings.Join(quiero, "\n"))
	}
}

// TestERPBancoHablaElIdiomaDelServicio lee el servicio.
func TestERPBancoHablaElIdiomaDelServicio(t *testing.T) {
	for fichero, literales := range map[string][]string{
		"services/syerp/internal/store/banca.go": {
			`json:"importe"`, `json:"factura_ref,omitempty"`, `json:"url_factura,omitempty"`, `json:"email_facturas,omitempty"`,
			`json:"tercero"`, `json:"total"`,
		},
		"services/syerp/internal/http/banca.go": {
			`json:"segura"`, `json:"sugerencia,omitempty"`, `json:"candidatas"`, `"total_pendientes"`, `json:"explicados"`,
		},
		"services/syerp/internal/http/server.go": {
			`Get("/banco"`, `"/banco/{id}/conciliar"`, `"/banco/{id}/explicar"`, `"/banco/{id}/reabrir"`, `Post("/banco/pistas"`,
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
