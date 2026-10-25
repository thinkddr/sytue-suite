// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

func TestStorageWidgetReusesCastQuotaAndKeepsLateAccountPrivate(t *testing.T) {
	t.Cleanup(widgets.SimularLector(true))
	internal, external := false, true
	for _, tc := range []struct {
		name           string
		external       *bool
		capacity       int64
		changed, empty bool
		status         int
		want           int64
	}{
		{name: "internal storage", external: &internal, capacity: 40, want: 32},
		{name: "external storage", external: &external, capacity: 40, empty: true},
		{name: "unknown backend", capacity: 40, empty: true},
		{name: "unknown capacity", external: &internal, empty: true},
		{name: "network failure", status: http.StatusServiceUnavailable, want: 80},
		{name: "late account response", external: &internal, capacity: 40, changed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("HOME", t.TempDir())
			ajustes.MarcarSesion()
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/quota" {
					t.Errorf("unexpected widget request: %s", r.URL.Path)
				}
				requests.Add(1)
				if tc.status != 0 {
					http.Error(w, "unavailable", tc.status)
					return
				}
				if tc.changed {
					ajustes.MarcarSesion()
				}
				_ = json.NewEncoder(w).Encode(api.CastQuota{UsedBytes: 13, LimitBytes: tc.capacity, CastBytes: 7, ExternalStorage: tc.external})
			}))
			defer server.Close()
			a := &App{cli: api.NewCast(server.URL, nil, nil), ConSesion: func() bool { return true }}
			a.lanzar = func(f func()) { f() }
			// This test exercises the existing request, without building the upload UI.
			a.enLaInterfaz = func(func()) {}
			a.Widget = func() widgets.View {
				return widgets.NewView("eu.sytue.cast", "cast", widgets.Capture(ajustes.Leer().Sesion))
			}
			if tc.empty || tc.status != 0 {
				if err := a.Widget.View().Publicar(context.Background(), widgets.Counter{Kind: "storage_percent", Value: 80}); err != nil {
					t.Fatal(err)
				}
			}
			a.pedirCuota()
			if requests.Load() != 1 {
				t.Fatalf("widget added quota requests: %d", requests.Load())
			}
			snapshot, err := widgets.Read("eu.sytue.cast")
			if tc.changed {
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("late quota published: %+v %v", snapshot, err)
				}
				return
			}
			if tc.empty {
				if err != nil || len(snapshot.Counters) != 0 {
					t.Fatalf("unknown or external capacity retained an old percentage: %+v %v", snapshot, err)
				}
				return
			}
			if err != nil || len(snapshot.Counters) != 1 || snapshot.Counters[0] != (widgets.Counter{Kind: "storage_percent", Value: tc.want}) {
				t.Fatalf("must use complete storage quota, not CastBytes: %+v %v", snapshot, err)
			}
		})
	}
}
