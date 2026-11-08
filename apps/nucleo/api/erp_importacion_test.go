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

// TestERPImportacionConVistaPreviaYDeshacer: la vista previa va con `simular=1` y el
// nombre del fichero (que es con lo que se reconoce el lote), la de verdad devuelve el
// id del lote y deshacerlo es un POST a su ruta. Lo mismo que `lib/importacion.ts`.
//
// VISTO EN ROJO: sin poner `simular` en la consulta, la primera petición sale igual
// que la segunda; sin el `id` en ERPResultadoImportacion, el lote se pierde.
func TestERPImportacionConVistaPreviaYDeshacer(t *testing.T) {
	var mu sync.Mutex
	var vistas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		mu.Lock()
		vistas = append(vistas, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery+" "+string(cuerpo))
		mu.Unlock()
		switch {
		case r.URL.Path == "/api/v1/cliente/importar" && r.URL.Query().Get("simular") == "1":
			_, _ = io.WriteString(w, `{"filas":2,"creados":2,"actualizados":0,"fallos":[],"simulacion":true,"ignoradas":[]}`)
		case r.URL.Path == "/api/v1/cliente/importar":
			_, _ = io.WriteString(w, `{"id":"lote-1","filas":2,"creados":2,"actualizados":0,"fallos":[],"simulacion":false}`)
		case r.URL.Path == "/api/v1/importaciones/lote-1/revertir":
			_, _ = io.WriteString(w, `{"borrados":1,"restaurados":0,"intactos":1}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c, ctx := NewERP(srv.URL, nil, nil, "es"), context.Background()

	previa, err := c.ImportarCon(ctx, "cliente", []byte("nombre\nA\nB"), false,
		ERPOpcionesImportacion{Fichero: "clientes.csv", Simular: true})
	if err != nil || !previa.Simulacion || previa.ID != "" || previa.Creados != 2 {
		t.Fatalf("previa: %+v, %v", previa, err)
	}
	res, err := c.ImportarCon(ctx, "cliente", []byte("nombre\nA\nB"), false, ERPOpcionesImportacion{Fichero: "clientes.csv"})
	if err != nil || res.ID != "lote-1" {
		t.Fatalf("importar: %+v, %v", res, err)
	}
	rev, err := c.RevertirImportacion(ctx, res.ID)
	if err != nil || rev != (ERPReversion{Borrados: 1, Intactos: 1}) {
		t.Fatalf("revertir: %+v, %v", rev, err)
	}
	quiero := []string{
		"POST /api/v1/cliente/importar?fichero=clientes.csv&simular=1 nombre\nA\nB",
		"POST /api/v1/cliente/importar?fichero=clientes.csv nombre\nA\nB",
		"POST /api/v1/importaciones/lote-1/revertir? {}",
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(vistas, "\n") != strings.Join(quiero, "\n") {
		t.Errorf("peticiones:\n%s\nquiero:\n%s", strings.Join(vistas, "\n"), strings.Join(quiero, "\n"))
	}
}

// TestERPImportacionHablaElIdiomaDelServicio: si el servicio cambia el nombre de un
// parámetro o de un campo, esto se pone en rojo aquí y no en producción.
func TestERPImportacionHablaElIdiomaDelServicio(t *testing.T) {
	for fichero, literales := range map[string][]string{
		"services/syerp/internal/http/importar.go": {
			`Get("simular") == "1"`, `Get("fichero")`, `json:"simulacion"`,
		},
		"services/syerp/internal/http/server.go":       {`Post("/importaciones/{id}/revertir"`},
		"services/syerp/internal/store/importacion.go": {`json:"id,omitempty"`, `json:"borrados"`, `json:"restaurados"`, `json:"intactos"`},
	} {
		codigo := webref.Read(t, fichero)
		for _, l := range literales {
			if !strings.Contains(codigo, l) {
				t.Errorf("%s ya no dice %s", fichero, l)
			}
		}
	}
}
