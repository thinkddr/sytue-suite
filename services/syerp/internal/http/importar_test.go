// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/erp/catalogo"
)

func TestImporteUBL(t *testing.T) {
	for c, quiero := range map[int64]string{123456: "1234.56", 5: "0.05", -100: "-1.00", 0: "0.00"} {
		if got := importe(c); got != quiero {
			t.Errorf("importe(%d) = %q, quiero %q", c, got, quiero)
		}
	}
}

// TestSimularSoloLoEntiendeElCSV: la vista previa manda `simular=1`, pero el servicio
// decide por el CONTENIDO —un XML va a importarFacturas y un Norma 43 a importarN43—, y
// ninguno de los dos lo leía: la «simulación» los escribía de verdad y confirmar, otra
// vez. Se rechazan con 422 antes de tocar la base (por eso basta un Handlers sin store:
// si llegaran a escribir, el store a nil revienta).
//
// VISTO EN ROJO: sin la comprobación de `simular` en Importar, la petición sigue hasta
// importarFacturas/importarN43 y entra en pánico con el store a nil.
func TestSimularSoloLoEntiendeElCSV(t *testing.T) {
	cat, err := catalogo.Cargar()
	if err != nil {
		t.Fatal(err)
	}
	h := &Handlers{cat: cat}
	n43 := "11" + strings.Repeat("0", 49) + strings.Repeat(" ", 29) + "\n"
	for _, c := range []struct{ modelo, cuerpo string }{
		{"linea_extracto", n43},
		{"extracto_bancario", n43},
		{"factura_compra", "<?xml version=\"1.0\"?><Invoice/>"},
		{"contacto", "  <Invoice/>"},
	} {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/"+c.modelo+"/importar?fichero=x.csv&simular=1",
			strings.NewReader(c.cuerpo))
		rc := chi.NewRouteContext()
		rc.URLParams.Add("modelo", c.modelo)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
		w := httptest.NewRecorder()
		h.Importar(w, r)
		if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "importacion_no_simulable") {
			t.Errorf("%s: %d %s", c.modelo, w.Code, w.Body.String())
		}
	}
}
