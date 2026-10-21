// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Las tres llamadas de «Clasificar» van a las rutas del servicio y quitar la etiqueta es
// un DELETE sin cuerpo, no un PUT con el id vacío (que el servicio rechazaría con 400).
func TestDriveEtiquetas(t *testing.T) {
	var pedidas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		pedidas = append(pedidas, r.Method+" "+r.URL.Path+" "+string(cuerpo))
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, `{"labels":[{"id":"l1","name":"Confidencial","rank":2,"block_external_share":true},{"id":"l2","name":"Interna","rank":1}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"f1"}`)
	}))
	defer srv.Close()
	ctx := context.Background()
	d := NewDrive(srv.URL, &sesionFalsa{tokens: []string{"tok"}}, nil)

	ls, err := d.SensitivityLabels(ctx)
	if err != nil || len(ls) != 2 || !ls[0].BlockExternalShare || ls[1].BlockExternalShare || ls[0].Name != "Confidencial" {
		t.Fatalf("SensitivityLabels = %+v, %v", ls, err)
	}
	if err := d.SetLabel(ctx, "f1", "l1"); err != nil {
		t.Fatal(err)
	}
	if err := d.SetLabel(ctx, "f1", ""); err != nil {
		t.Fatal(err)
	}
	quiero := []string{
		"GET /api/v1/sensitivity-labels ",
		`PUT /api/v1/files/f1/label {"label_id":"l1"}`,
		"DELETE /api/v1/files/f1/label ",
	}
	if len(pedidas) != len(quiero) {
		t.Fatalf("peticiones = %q", pedidas)
	}
	for i := range quiero {
		if pedidas[i] != quiero[i] {
			t.Errorf("petición %d = %q; quiero %q", i, pedidas[i], quiero[i])
		}
	}
}
