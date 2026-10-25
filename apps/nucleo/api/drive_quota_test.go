// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDriveQuotaConservaLaBaseYLaCapacidadDesconocida(t *testing.T) {
	for _, caso := range []struct {
		nombre, cuerpo string
		completa       bool
		base           bool
	}{
		{"combinada", `{"limit_bytes":1100,"used_bytes":220,"available_bytes":880,"base_limit_bytes":100,"base_used_bytes":20,"external_storage":true,"external_capacity":{"total_bytes":1000,"used_bytes":200,"available_bytes":800}}`, true, true},
		{"parcial", `{"limit_bytes":1100,"used_bytes":220,"available_bytes":880,"base_limit_bytes":100,"base_used_bytes":20,"external_storage":true,"external_capacity":{"total_bytes":1000,"used_bytes":200,"available_bytes":800,"partial":true}}`, true, true},
		{"desconocida", `{"limit_bytes":100,"used_bytes":20,"available_bytes":80,"base_limit_bytes":100,"base_used_bytes":20,"external_storage":true,"external_capacity":{"total_bytes":null,"used_bytes":null,"available_bytes":null}}`, false, true},
		{"cero_real", `{"limit_bytes":100,"used_bytes":20,"available_bytes":80,"base_limit_bytes":100,"base_used_bytes":20,"external_storage":true,"external_capacity":{"total_bytes":0,"used_bytes":0,"available_bytes":0}}`, true, true},
		{"anterior", `{"limit_bytes":100,"used_bytes":20,"external_storage":true}`, false, false},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/quota" {
					t.Errorf("ruta = %s", r.URL.Path)
				}
				_, _ = io.WriteString(w, caso.cuerpo)
			}))
			t.Cleanup(srv.Close)
			q, err := NewDrive(srv.URL, &sesionFalsa{tokens: []string{"tok"}}, nil).Quota(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !q.ExternalStorage || q.ExternalCapacity.Complete() != caso.completa {
				t.Fatalf("cuota = %+v", q)
			}
			if (q.BaseLimitBytes != nil) != caso.base || (q.BaseUsedBytes != nil) != caso.base {
				t.Fatalf("base = %+v", q)
			}
			if caso.base && (*q.BaseLimitBytes != 100 || *q.BaseUsedBytes != 20) {
				t.Fatalf("se alteró la cuota base: %+v", q)
			}
			if caso.nombre == "combinada" && (q.LimitBytes != 1100 || q.UsedBytes != 220 || q.AvailableBytes != 880) {
				t.Fatalf("se alteró el total combinado: %+v", q)
			}
			if caso.nombre == "parcial" && !q.ExternalCapacity.Partial {
				t.Fatalf("se perdió el aviso de fuentes desconocidas: %+v", q.ExternalCapacity)
			}
			if caso.nombre == "desconocida" && (q.ExternalCapacity.TotalBytes != nil || q.ExternalCapacity.UsedBytes != nil || q.ExternalCapacity.AvailableBytes != nil) {
				t.Fatalf("desconocido convertido a cero: %+v", q.ExternalCapacity)
			}
		})
	}
}
