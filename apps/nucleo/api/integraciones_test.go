// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestIntegracionesHablanComoLaPasarela: las dos rutas, con token, los nombres de los
// campos de `handlers_myapps.go` y `handlers_substitution.go`, y el descarte de un
// sustituto sin dirección, que es lo que hace el BFF del portal.
func TestIntegracionesHablanComoLaPasarela(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Errorf("%s sin token", r.URL.Path)
		}
		switch r.URL.Path {
		case "/api/v1/my-apps":
			_, _ = w.Write([]byte(`{"apps":[{"key":"i1","name":"Holded","description":"ERP","href":"https://app.holded.com","iconUrl":"data:image/svg+xml;base64,PHN2Zy8+","color":"#e00","status":"active"}]}`))
		case "/api/v1/substitutions/resolved":
			_, _ = w.Write([]byte(`{"products":{"sheets":{"instance_id":"i2","name":"Google","href":"https://sheets.google.com"},"slides":{"name":"x","href":""}}}`))
		default:
			t.Errorf("ruta inesperada %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	i := NewIntegraciones(srv.URL, &sesionFalsa{tokens: []string{"t"}}, nil)
	apps, err := i.MisApps(context.Background())
	if err != nil || len(apps) != 1 || apps[0].IconURL == "" || apps[0].Href == "" {
		t.Fatalf("MisApps = %+v, %v", apps, err)
	}
	subs, err := i.Sustituciones(context.Background())
	if err != nil || len(subs) != 1 || subs["sheets"].Name != "Google" {
		t.Fatalf("Sustituciones = %+v, %v", subs, err)
	}

	for fichero, literales := range map[string][]string{
		"services/integrations/internal/http/handlers_myapps.go":       {"`json:\"iconUrl\"`", "`json:\"href\"`", `"apps": apps`},
		"services/integrations/internal/http/handlers_substitution.go": {`"name": inst.Name, "href": href`, `"products": resolved`},
		"services/integrations/internal/http/router.go":                {`"/my-apps"`, `"/substitutions/resolved"`},
	} {
		fuente := webref.Read(t, fichero)
		for _, l := range literales {
			if !strings.Contains(fuente, l) {
				t.Errorf("%s ya no dice %s", fichero, l)
			}
		}
	}
}
