// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package buildcache

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestElClienteDeScalewayPideLoQueAcotaLaKey ejercita el mapeo REST contra un Scaleway
// falso: busca antes de crear (con nombre EXACTO), el espacio de nombres es privado, la
// política lleva UNA regla con `project_ids` del proyecto de caché (nunca de organización)
// y rotar borra la key anterior antes de acuñar. Todas las peticiones con el token.
func TestElClienteDeScalewayPideLoQueAcotaLaKey(t *testing.T) {
	var mu sync.Mutex
	var vistas []string
	cuerpos := map[string]map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("X-Auth-Token") != "tok" {
			t.Errorf("%s %s sin el token", r.Method, r.URL.Path)
		}
		vistas = append(vistas, r.Method+" "+r.URL.Path)
		if r.Body != nil {
			b, _ := io.ReadAll(r.Body)
			if len(b) > 0 {
				var m map[string]any
				_ = json.Unmarshal(b, &m)
				cuerpos[r.Method+" "+r.URL.Path] = m
			}
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /account/v3/projects":
			// Coincidencia parcial de Scaleway: un nombre parecido no vale.
			_, _ = io.WriteString(w, `{"projects":[{"id":"p-otro","name":"sytue-cloud-cache-acme2"}]}`)
		case "POST /account/v3/projects":
			_, _ = io.WriteString(w, `{"id":"p-1","name":"sytue-cloud-cache-acme"}`)
		case "GET /registry/v1/regions/fr-par/namespaces":
			_, _ = io.WriteString(w, `{"namespaces":[]}`)
		case "POST /registry/v1/regions/fr-par/namespaces":
			_, _ = io.WriteString(w, `{"id":"ns-1","name":"sytue-cache-acme","endpoint":"rg.fr-par.scw.cloud/sytue-cache-acme"}`)
		case "GET /iam/v1alpha1/applications":
			_, _ = io.WriteString(w, `{"applications":[{"id":"a-1","name":"sytue-cloud-cache-acme"}]}`)
		case "GET /iam/v1alpha1/policies":
			_, _ = io.WriteString(w, `{"policies":[]}`)
		case "POST /iam/v1alpha1/policies":
			_, _ = io.WriteString(w, `{"id":"pol-1"}`)
		case "GET /iam/v1alpha1/api-keys":
			_, _ = io.WriteString(w, `{"api_keys":[{"access_key":"SCWVIEJA"}]}`)
		case "DELETE /iam/v1alpha1/api-keys/SCWVIEJA":
			w.WriteHeader(http.StatusNoContent)
		case "POST /iam/v1alpha1/api-keys":
			_, _ = io.WriteString(w, `{"access_key":"SCWNUEVA","secret_key":"s3cr3t"}`)
		default:
			t.Errorf("petición inesperada %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := NewScaleway(srv.URL, "fr-par", "tok", "org-9")
	ctx := t.Context()

	pid, err := c.EnsureProject(ctx, "sytue-cloud-cache-acme")
	if err != nil || pid != "p-1" {
		t.Fatalf("EnsureProject = %q, %v (el parecido no vale)", pid, err)
	}
	ep, err := c.EnsureRegistryNamespace(ctx, pid, "sytue-cache-acme")
	if err != nil || ep != "rg.fr-par.scw.cloud/sytue-cache-acme" {
		t.Fatalf("EnsureRegistryNamespace = %q, %v", ep, err)
	}
	if ns := cuerpos["POST /registry/v1/regions/fr-par/namespaces"]; ns["is_public"] != false || ns["project_id"] != "p-1" {
		t.Errorf("el espacio de nombres no es privado o no es del proyecto: %v", ns)
	}
	app, err := c.EnsureApplication(ctx, "sytue-cloud-cache-acme")
	if err != nil || app != "a-1" {
		t.Fatalf("EnsureApplication = %q, %v", app, err)
	}
	if err := c.EnsureProjectPolicy(ctx, "sytue-cloud-cache-acme", app, pid, PermisoRegistry); err != nil {
		t.Fatal(err)
	}
	pol := cuerpos["POST /iam/v1alpha1/policies"]
	reglas, _ := json.Marshal(pol["rules"])
	if pol["application_id"] != "a-1" || string(reglas) != `[{"permission_set_names":["ContainerRegistryFullAccess"],"project_ids":["p-1"]}]` {
		t.Errorf("la política no va acotada al proyecto: %v", pol)
	}
	if strings.Contains(string(reglas), "organization_id") {
		t.Errorf("la regla alcanza la organización: %s", reglas)
	}
	ak, sk, err := c.RotateAPIKey(ctx, app)
	if err != nil || ak != "SCWNUEVA" || sk != "s3cr3t" {
		t.Fatalf("RotateAPIKey = %q, %q, %v", ak, sk, err)
	}
	borrar, crear := -1, -1
	for i, v := range vistas {
		switch v {
		case "DELETE /iam/v1alpha1/api-keys/SCWVIEJA":
			borrar = i
		case "POST /iam/v1alpha1/api-keys":
			crear = i
		}
	}
	if borrar < 0 || crear < borrar {
		t.Errorf("rotar no borra la key anterior antes de acuñar: %v", vistas)
	}
}

// TestUnFalloDeScalewaySeDice: un 4xx/5xx vuelve como error, con su cuerpo acotado.
func TestUnFalloDeScalewaySeDice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, strings.Repeat("x", 10000), http.StatusForbidden)
	}))
	defer srv.Close()
	_, err := NewScaleway(srv.URL, "fr-par", "tok", "org").EnsureProject(t.Context(), "p")
	if err == nil || !strings.Contains(err.Error(), "403") || len(err.Error()) > 2300 {
		t.Errorf("err = %v", err)
	}
}
