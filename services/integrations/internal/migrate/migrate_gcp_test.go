// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate_test

// Tests OFFLINE de la migración de Google Cloud y Firebase a Sytue Cloud. Lo que
// se prueba es lo que distingue una migración honesta de un informe bonito: que
// la aplicación llega con su configuración utilizable, que los objetos llegan
// enteros, que la tarea programada vuelve a estar programada, y que lo que NO se
// puede copiar (una base de datos) aparece dicho por su nombre en vez de
// desaparecer del informe.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/migrate"
)

// gcpServer simula las cinco APIs de Google que toca la migración.
func gcpServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}

	// Cloud Run: la aplicación vive en europe-west1; el resto de regiones
	// responden 403 como haría un proyecto real sin la API habilitada allí.
	mux.HandleFunc("/v2/projects/acme-prod/locations/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/services") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if !strings.Contains(r.URL.Path, "/locations/europe-west1/") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		writeJSON(w, map[string]any{"services": []map[string]any{{
			"name":        "projects/acme-prod/locations/europe-west1/services/tienda",
			"description": "API de la tienda",
			"uri":         "https://tienda-abc.a.run.app",
			"template": map[string]any{
				"scaling": map[string]any{"minInstanceCount": 2, "maxInstanceCount": 10},
				"containers": []map[string]any{{
					"image": "europe-docker.pkg.dev/acme-prod/apps/tienda:1.4.2",
					"env": []map[string]any{
						{"name": "LOG_LEVEL", "value": "info"},
						{"name": "DB_PASSWORD", "valueSource": map[string]any{
							"secretKeyRef": map[string]any{"secret": "projects/acme-prod/secrets/db-password", "version": "latest"},
						}},
					},
					"resources": map[string]any{"limits": map[string]string{"cpu": "2", "memory": "2Gi"}},
				}},
			},
		}}})
	})

	// Firebase Hosting.
	mux.HandleFunc("/v1beta1/projects/acme-prod/sites", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"sites": []map[string]any{
			{"name": "projects/acme-prod/sites/acme-web", "siteId": "acme-web", "defaultUrl": "https://acme-web.web.app"},
		}})
	})

	// Cloud Storage: un bucket normal y otro en ARCHIVE (que NO se copia).
	mux.HandleFunc("/storage/v1/b", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"items": []map[string]any{
			{"name": "acme-uploads", "location": "EU", "storageClass": "STANDARD"},
			{"name": "acme-frios", "location": "EU", "storageClass": "ARCHIVE"},
		}})
	})
	mux.HandleFunc("/storage/v1/b/acme-uploads/o", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"items": []map[string]any{
			{"name": "facturas/", "size": "0", "contentType": "application/x-directory"},
			{"name": "facturas/2026-01.pdf", "size": "12", "contentType": "application/pdf"},
		}})
	})
	mux.HandleFunc("/storage/v1/b/acme-uploads/o/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "PDF-FALSO-12")
	})

	// Cloud Scheduler: una tarea HTTP, una en pausa y una de Pub/Sub.
	mux.HandleFunc("/v1/projects/acme-prod/locations/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/jobs") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if !strings.Contains(r.URL.Path, "/locations/europe-west1/") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		writeJSON(w, map[string]any{"jobs": []map[string]any{
			{
				"name":     "projects/acme-prod/locations/europe-west1/jobs/cierre-diario",
				"schedule": "0 2 * * *", "timeZone": "Europe/Madrid", "state": "ENABLED",
				"httpTarget": map[string]any{
					"uri": "https://tienda.acme.example/cierre", "httpMethod": "POST",
					"body": "eyJtb2RvIjoiZGlhcmlvIn0=", // {"modo":"diario"}
				},
			},
			{
				"name":     "projects/acme-prod/locations/europe-west1/jobs/reindexar",
				"schedule": "@daily", "state": "PAUSED",
				"httpTarget": map[string]any{"uri": "https://tienda.acme.example/reindex", "httpMethod": "POST"},
			},
			{
				"name":     "projects/acme-prod/locations/europe-west1/jobs/eventos",
				"schedule": "*/5 * * * *", "state": "ENABLED",
				"pubsubTarget": map[string]any{"topicName": "projects/acme-prod/topics/eventos"},
			},
		}})
	})

	// Cloud SQL y Firestore: se enumeran, no se copian.
	mux.HandleFunc("/v1/projects/acme-prod/instances", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"items": []map[string]any{
			{"name": "tienda-db", "databaseVersion": "POSTGRES_15"},
		}})
	})
	mux.HandleFunc("/v1/projects/acme-prod/databases", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"databases": []map[string]any{
			{"name": "projects/acme-prod/databases/(default)", "type": "FIRESTORE_NATIVE"},
		}})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// fakeCloud es Sytue Cloud: guarda lo que se le pide crear.
type fakeCloud struct {
	srv      *httptest.Server
	projects map[string]map[string]any // slug → cuerpo de creación
	env      map[string][]map[string]any
	services []map[string]any
	domains  []string
	objects  map[string][]byte
}

func newFakeCloud(t *testing.T) *fakeCloud {
	t.Helper()
	f := &fakeCloud{
		projects: map[string]map[string]any{},
		env:      map[string][]map[string]any{},
		objects:  map[string][]byte{},
	}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}

	mux.HandleFunc("/api/v1/projects", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, map[string]any{"projects": []any{}})
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		slug, _ := body["slug"].(string)
		f.projects[slug] = body
		writeJSON(w, http.StatusCreated, map[string]any{"id": "proj-" + slug, "slug": slug})
	})

	// Subida de objetos por URL prefirmada: apunta a este mismo servidor.
	mux.HandleFunc("/upload/", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		f.objects[strings.TrimPrefix(r.URL.Path, "/upload/")] = raw
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/api/v1/projects/", func(w http.ResponseWriter, r *http.Request) {
		proj := strings.TrimPrefix(r.URL.Path, "/api/v1/projects/")
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case strings.HasSuffix(proj, "/env"):
			id := strings.TrimSuffix(proj, "/env")
			f.env[id] = append(f.env[id], body)
			writeJSON(w, http.StatusOK, map[string]any{"id": "env-1"})
		case strings.HasSuffix(proj, "/services"):
			if r.Method == http.MethodGet {
				out := make([]map[string]any, 0, len(f.services))
				for i, svc := range f.services {
					out = append(out, map[string]any{"id": fmt.Sprintf("svc-%d", i), "kind": svc["kind"]})
				}
				writeJSON(w, http.StatusOK, map[string]any{"services": out})
				return
			}
			f.services = append(f.services, body)
			writeJSON(w, http.StatusAccepted, map[string]any{"id": fmt.Sprintf("svc-%d", len(f.services)-1)})
		case strings.HasSuffix(proj, "/domains"):
			host, _ := body["hostname"].(string)
			f.domains = append(f.domains, host)
			writeJSON(w, http.StatusCreated, map[string]any{"id": "dom-1"})
		case strings.HasSuffix(proj, "/storage/presign"):
			key, _ := body["key"].(string)
			writeJSON(w, http.StatusOK, map[string]any{"url": f.srv.URL + "/upload/" + key})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// gcpInstance apunta las seis APIs de Google al servidor simulado.
func gcpInstance(base string) connector.Instance {
	inst := googleConsentInstance(base)
	for _, k := range []string{
		migrate.ConfigGCPRunBase, migrate.ConfigGCPStorageBase, migrate.ConfigGCPSchedulerBase,
		migrate.ConfigGCPSQLBase, migrate.ConfigGCPFirestoreBase, migrate.ConfigGCPHostingBase,
	} {
		inst.Config[k] = base
	}
	inst.Config[migrate.ConfigGCPProject] = "acme-prod"
	// Sin esto, el barrido probaría las 22 regiones conocidas contra el simulador.
	inst.Config[migrate.ConfigGCPRegions] = "europe-west1,us-central1"
	return inst
}

// TestCloudLlegaUtilizable: la aplicación llega como proyecto con su tamaño y sus
// variables, el objeto llega entero, la tarea vuelve a estar programada, y lo que
// no se puede copiar aparece EN EL INFORME con su motivo.
func TestCloudLlegaUtilizable(t *testing.T) {
	allowLoopback(t)
	gsrv := gcpServer(t)
	cloud := newFakeCloud(t)

	j := job("job-cloud", domain.SourceGoogle, domain.DomainCloud)
	j.SourceUser = ""
	st := newMemStore("org-1")
	st.put(j)
	src, err := migrate.NewSource(context.Background(), gmailRuntime(), gcpInstance(gsrv.URL), j)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	deps := migrate.Deps{
		Src:   src,
		Cloud: migrate.NewSytueCloud(cloud.srv.URL, migrate.StaticToken("tok"), nil),
		Store: st,
	}
	if err := migrate.Run(context.Background(), deps, j); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := st.get("job-cloud")
	if got.Status != domain.MigrationDone {
		t.Fatalf("estado final = %q (%s)", got.Status, got.LastError)
	}

	// La aplicación de Cloud Run llega con el tamaño que cubre sus 2 GiB y 2 CPU,
	// y conserva el mínimo de instancias: migrar no puede degradar el servicio.
	tienda, ok := cloud.projects["tienda"]
	if !ok {
		t.Fatalf("no se creó el proyecto de la aplicación: %v", cloud.projects)
	}
	if size, _ := tienda["instance_size"].(string); size != "pro" {
		t.Errorf("instance_size = %q; 2 CPU y 2 GiB caben en 'pro'", size)
	}
	if n, _ := tienda["instance_count"].(float64); n != 2 {
		t.Errorf("instance_count = %v; el origen tenía 2 instancias mínimas", n)
	}
	if _, ok := cloud.projects["acme-web"]; !ok {
		t.Errorf("el sitio de Firebase Hosting no llegó como proyecto: %v", cloud.projects)
	}

	// La variable normal llega con su valor; la que vivía en Secret Manager llega
	// COMO SECRETO y vacía, no se cuela en claro ni desaparece.
	vars := cloud.env["proj-tienda"]
	var logLevel, dbPass map[string]any
	for _, v := range vars {
		switch v["key"] {
		case "LOG_LEVEL":
			logLevel = v
		case "DB_PASSWORD":
			dbPass = v
		}
	}
	if logLevel == nil || logLevel["value"] != "info" {
		t.Errorf("LOG_LEVEL no llegó con su valor: %v", vars)
	}
	if dbPass == nil {
		t.Fatalf("DB_PASSWORD no llegó: %v", vars)
	}
	if secret, _ := dbPass["is_secret"].(bool); !secret {
		t.Error("DB_PASSWORD debía llegar marcada como secreto")
	}
	if val, _ := dbPass["value"].(string); val != "" {
		t.Errorf("DB_PASSWORD llegó con valor %q; el secreto vive en Google y NO se copia", val)
	}

	// El objeto llega entero; la «carpeta» de cero bytes no se copia.
	if got := string(cloud.objects["facturas/2026-01.pdf"]); got != "PDF-FALSO-12" {
		t.Errorf("el objeto no llegó entero: %q", got)
	}
	if _, ok := cloud.objects["facturas/"]; ok {
		t.Error("se copió el objeto-carpeta de cero bytes")
	}

	// La tarea programada vuelve a estar programada, con su horario y su cuerpo.
	var cron map[string]any
	for _, s := range cloud.services {
		if s["kind"] == "cron" {
			cron = s
		}
	}
	if cron == nil {
		t.Fatalf("la tarea programada no se recreó: %v", cloud.services)
	}
	config, _ := cron["config"].(map[string]any)
	if config["schedule"] != "0 2 * * *" {
		t.Errorf("horario = %v; debía conservarse", config["schedule"])
	}
	if config["url"] != "https://tienda.acme.example/cierre" {
		t.Errorf("destino = %v", config["url"])
	}
	if body, _ := config["body"].(string); !strings.Contains(body, "diario") {
		t.Errorf("el cuerpo de la tarea no llegó: %q", body)
	}

	// Y lo que NO se copia se DICE: bucket en archivo, tarea en pausa, tarea de
	// Pub/Sub, Cloud SQL y Firestore, cada uno con su motivo.
	motivos := map[string]string{}
	for _, it := range st.items["org-1/job-cloud"] {
		if it.SkipReason != "" {
			motivos[it.SourceID] = it.SkipReason
		}
	}
	for _, id := range []string{
		"bucket:acme-frios", "job:reindexar", "job:eventos", "db:tienda-db", "db:(default)",
	} {
		if motivos[id] == "" {
			t.Errorf("%s salió del informe sin motivo; omitir en silencio es mentir: %v", id, motivos)
		}
	}
	if !strings.Contains(motivos["db:tienda-db"], "pg_dump") {
		t.Errorf("el motivo de Cloud SQL no dice qué hacer: %q", motivos["db:tienda-db"])
	}
}

// TestCloudSegundaPasadaNoRepite: reanudar un trabajo ya hecho no vuelve a crear
// nada. Es la garantía de idempotencia del motor, y en Cloud importa el doble:
// crear un proyecto dos veces es una factura dos veces.
func TestCloudSegundaPasadaNoRepite(t *testing.T) {
	allowLoopback(t)
	gsrv := gcpServer(t)
	cloud := newFakeCloud(t)

	j := job("job-cloud-2", domain.SourceGoogle, domain.DomainCloud)
	j.SourceUser = ""
	st := newMemStore("org-1")
	st.put(j)
	newDeps := func() migrate.Deps {
		src, err := migrate.NewSource(context.Background(), gmailRuntime(), gcpInstance(gsrv.URL), j)
		if err != nil {
			t.Fatalf("NewSource: %v", err)
		}
		return migrate.Deps{
			Src:   src,
			Cloud: migrate.NewSytueCloud(cloud.srv.URL, migrate.StaticToken("tok"), nil),
			Store: st,
		}
	}
	if err := migrate.Run(context.Background(), newDeps(), j); err != nil {
		t.Fatalf("primera pasada: %v", err)
	}
	objetos := len(cloud.objects)
	servicios := len(cloud.services)

	segunda, _ := st.get("job-cloud-2")
	if err := migrate.Run(context.Background(), newDeps(), *segunda); err != nil {
		t.Fatalf("segunda pasada: %v", err)
	}
	if len(cloud.objects) != objetos {
		t.Errorf("la segunda pasada volvió a subir objetos: %d → %d", objetos, len(cloud.objects))
	}
	if len(cloud.services) != servicios {
		t.Errorf("la segunda pasada volvió a crear servicios: %d → %d", servicios, len(cloud.services))
	}
}
