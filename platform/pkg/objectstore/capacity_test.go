// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package objectstore

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCapacityReadersKeepTheProvidedEgressClient(t *testing.T) {
	for _, tc := range []struct{ provider, host, path string }{
		{"google-drive", "www.googleapis.com", "/drive/v3/about"},
		{"onedrive", "graph.microsoft.com", "/v1.0/me/drive"},
		{"dropbox", "api.dropboxapi.com", "/2/users/get_space_usage"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			called := false
			client := &http.Client{Transport: dropboxTransport(func(r *http.Request) (*http.Response, error) {
				called = true
				if r.Header.Get("Authorization") != "Bearer fixture-personal" || r.URL.Host != tc.host || r.URL.Path != tc.path {
					t.Fatal("la lectura perdió la credencial de su actor")
				}
				response := httptest.NewRecorder()
				_, _ = response.WriteString(`{}`)
				return response.Result(), nil
			})}
			reader, err := ProviderCapacityReader(tc.provider, client, func(context.Context) (string, error) { return "fixture-personal", nil })
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reader.Capacity(context.Background()); err != nil || !called {
				t.Fatalf("no se usó el transporte de egress: llamada=%v err=%v", called, err)
			}
		})
	}
}

func TestCapacitySumPreservesKnownSourcesAndUnknownOnes(t *testing.T) {
	known := Capacity{TotalBytes: capacityPtr(1000), UsedBytes: capacityPtr(300), AvailableBytes: capacityPtr(700)}
	other := Capacity{TotalBytes: capacityPtr(200), UsedBytes: capacityPtr(50), AvailableBytes: capacityPtr(25)}
	for _, tc := range []struct {
		name  string
		parts []Capacity
		want  Capacity
	}{
		{"varias", []Capacity{known, other}, Capacity{TotalBytes: capacityPtr(1200), UsedBytes: capacityPtr(350), AvailableBytes: capacityPtr(725)}},
		{"s3 desconocido y google conocido", []Capacity{{}, known}, Capacity{TotalBytes: capacityPtr(1000), UsedBytes: capacityPtr(300), AvailableBytes: capacityPtr(700), Partial: true}},
		{"todas desconocidas", []Capacity{{}, {UsedBytes: capacityPtr(10)}}, Capacity{Partial: true}},
		{"cero no es desconocido", []Capacity{{TotalBytes: capacityPtr(0), UsedBytes: capacityPtr(0), AvailableBytes: capacityPtr(0)}}, Capacity{TotalBytes: capacityPtr(0), UsedBytes: capacityPtr(0), AvailableBytes: capacityPtr(0)}},
		{"parcial propagado", []Capacity{{TotalBytes: capacityPtr(1000), UsedBytes: capacityPtr(300), AvailableBytes: capacityPtr(700), Partial: true}}, Capacity{TotalBytes: capacityPtr(1000), UsedBytes: capacityPtr(300), AvailableBytes: capacityPtr(700), Partial: true}},
		{"overflow", []Capacity{known, {TotalBytes: capacityPtr(math.MaxInt64), UsedBytes: capacityPtr(0), AvailableBytes: capacityPtr(0)}}, Capacity{Partial: true}},
	} {
		t.Run(tc.name, func(t *testing.T) { requireCapacity(t, SumCapacities(tc.parts...), tc.want) })
	}
}
