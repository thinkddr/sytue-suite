// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

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

func TestStorageWidgetReusesQuotaWithoutPublishingUnknownCapacity(t *testing.T) {
	t.Cleanup(widgets.SimularLector(true))
	for _, tc := range []struct {
		name    string
		quota   api.DriveQuota
		changed bool
		want    int64
		visible bool
		empty   bool
		status  int
	}{
		{name: "complete", quota: api.DriveQuota{UsedBytes: 13, LimitBytes: 40}, want: 32, visible: true},
		{name: "external", quota: api.DriveQuota{UsedBytes: 13, LimitBytes: 40, ExternalStorage: true}, visible: true, empty: true},
		{name: "unknown capacity", quota: api.DriveQuota{UsedBytes: 13}, visible: true, empty: true},
		{name: "invalid used", quota: api.DriveQuota{UsedBytes: -1, LimitBytes: 40}, visible: true, empty: true},
		{name: "network failure", status: http.StatusServiceUnavailable, visible: true, want: 80},
		{name: "late account response", quota: api.DriveQuota{UsedBytes: 13, LimitBytes: 40}, changed: true},
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
				_ = json.NewEncoder(w).Encode(tc.quota)
			}))
			defer server.Close()
			p := NuevaPantalla(traductor(t), api.NewDrive(server.URL, sesionDoble{}, nil), nil, nil)
			p.Widget = func() widgets.View {
				return widgets.NewView("eu.sytue.drive", "drive", widgets.Capture(ajustes.Leer().Sesion))
			}
			if tc.empty || tc.status != 0 {
				if err := p.Widget.View().Publicar(context.Background(), widgets.Counter{Kind: "storage_percent", Value: 80}); err != nil {
					t.Fatal(err)
				}
			}
			p.cargarCuota()
			if requests.Load() != 1 {
				t.Fatalf("widget added quota requests: %d", requests.Load())
			}
			snapshot, err := widgets.Read("eu.sytue.drive")
			if !tc.visible {
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("published unknown or foreign quota: %+v %v", snapshot, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.empty {
				if len(snapshot.Counters) != 0 {
					t.Fatalf("unrepresentable capacity retained an old percentage: %+v", snapshot)
				}
			} else if len(snapshot.Counters) != 1 || snapshot.Counters[0] != (widgets.Counter{Kind: "storage_percent", Value: tc.want}) {
				t.Fatalf("storage percent: %+v", snapshot)
			}
		})
	}
}
