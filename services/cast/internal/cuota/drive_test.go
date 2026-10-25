// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cuota

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQuotaPreservesExternalStorageAndUnknownOlderDrive(t *testing.T) {
	for _, tc := range []struct {
		name, field     string
		known, external bool
	}{
		{name: "internal", field: `,"external_storage":false`, known: true},
		{name: "external", field: `,"external_storage":true`, known: true, external: true},
		{name: "older Drive"},
		{name: "unknown", field: `,"external_storage":null`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/quota" || r.Header.Get("Authorization") != "Bearer token-for-test" {
					t.Errorf("unexpected quota request: %s", r.URL.Path)
				}
				_, _ = io.WriteString(w, `{"limit_bytes":40,"used_bytes":13,"available_bytes":27`+tc.field+`}`)
			}))
			defer server.Close()
			q, err := New(server.URL, "").Consultar(context.Background(), "token-for-test")
			if err != nil || (q.ExternalStorage != nil) != tc.known || tc.known && *q.ExternalStorage != tc.external || q.LimitBytes != 40 || q.UsedBytes != 13 {
				t.Fatalf("must preserve declared origin and absence: %+v %v", q, err)
			}
		})
	}
}

func TestLasNubesConectadasNoFinancianOriginalesDeCastEnSytue(t *testing.T) {
	for _, tc := range []struct {
		name, base string
		available  int64
	}{
		{name: "base libre", base: `,"base_limit_bytes":100,"base_used_bytes":20`, available: 80},
		{name: "base llena", base: `,"base_limit_bytes":100,"base_used_bytes":100`},
		{name: "base superada", base: `,"base_limit_bytes":100,"base_used_bytes":120`},
		{name: "base cero", base: `,"base_limit_bytes":0,"base_used_bytes":0`},
		{name: "base incompleta", base: `,"base_limit_bytes":100`},
		{name: "Drive anterior", available: 9000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, `{"limit_bytes":10000,"used_bytes":1000,"available_bytes":9000,"external_storage":true`+tc.base+`}`)
			}))
			defer server.Close()
			got, err := New(server.URL, "").Disponible(context.Background(), "token-for-test")
			if err != nil || got != tc.available {
				t.Fatalf("espacio externo acreditado a Cast: libre=%d, esperado=%d, error=%v", got, tc.available, err)
			}
		})
	}
}
