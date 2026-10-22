// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate_test

// Tests OFFLINE del camino de SALIDA: de Sytue a un tercero. Lo que se prueba es
// que irse funciona igual de bien que llegar —el árbol se recrea, los ficheros
// van enteros, los contactos conservan sus correos— y que una segunda pasada no
// duplica nada, que en un Drive ajeno es la diferencia entre una copia y un
// estropicio.

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/migrate"
)

// sytueServer simula Sytue Drive y Sytue Contacts (el ORIGEN de una salida).
func sytueServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}

	mux.HandleFunc("/api/v1/folders/root", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"id": "root-1"})
	})
	mux.HandleFunc("/api/v1/folders/root-1/children", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"folders": []map[string]any{{"id": "f-proyectos", "name": "Proyectos"}},
			"files": []map[string]any{
				{"id": "file-1", "name": "memoria.pdf", "size_bytes": 7, "content_type": "application/pdf"},
			},
		})
	})
	mux.HandleFunc("/api/v1/folders/f-proyectos/children", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"folders": []any{},
			"files": []map[string]any{
				{"id": "file-2", "name": "plan.txt", "size_bytes": 4, "content_type": "text/plain"},
			},
		})
	})
	mux.HandleFunc("/api/v1/files/file-1/content", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "PDF-UNO")
	})
	mux.HandleFunc("/api/v1/files/file-2/content", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "PLAN")
	})
	mux.HandleFunc("/api/v1/contacts/all", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") != "" {
			writeJSON(w, map[string]any{"contacts": []any{}})
			return
		}
		writeJSON(w, map[string]any{"contacts": []map[string]any{{
			"id": "c-1", "formatted_name": "Ana Ruiz", "given_name": "Ana", "family_name": "Ruiz",
			"organization": "Acme", "title": "Dirección",
			"emails": []map[string]any{{"email": "ana@acme.example"}},
			"phones": []map[string]any{{"phone": "+34600000000"}},
		}}})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// fakeGoogleWrite es Google como DESTINO: apunta lo que se le crea.
type fakeGoogleWrite struct {
	srv      *httptest.Server
	folders  map[string]string // id → nombre
	files    map[string]string // nombre → contenido
	parents  map[string]string // nombre → id del padre
	contacts []map[string]any
	next     int
}

func newFakeGoogleWrite(t *testing.T) *fakeGoogleWrite {
	t.Helper()
	f := &fakeGoogleWrite{
		folders: map[string]string{}, files: map[string]string{}, parents: map[string]string{},
	}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}

	// Búsqueda de la carpeta «Sytue» (idempotencia de la raíz).
	mux.HandleFunc("/drive/v3/files", func(w http.ResponseWriter, _ *http.Request) {
		out := []map[string]any{}
		for id, name := range f.folders {
			if name == "Sytue" {
				out = append(out, map[string]any{"id": id})
			}
		}
		writeJSON(w, map[string]any{"files": out})
	})

	mux.HandleFunc("/upload/drive/v3/files", func(w http.ResponseWriter, r *http.Request) {
		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		//nolint:tagliatelle // el simulador habla el contrato de Google (camelCase).
		var meta struct {
			Name     string   `json:"name"`
			MimeType string   `json:"mimeType"`
			Parents  []string `json:"parents"`
		}
		content := ""
		for i := 0; ; i++ {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			raw, _ := io.ReadAll(part)
			if i == 0 {
				_ = json.Unmarshal(raw, &meta)
				continue
			}
			content = string(raw)
		}
		f.next++
		id := "g-" + meta.Name
		if meta.MimeType == "application/vnd.google-apps.folder" {
			f.folders[id] = meta.Name
		} else {
			f.files[meta.Name] = content
		}
		if len(meta.Parents) > 0 {
			f.parents[meta.Name] = meta.Parents[0]
		}
		writeJSON(w, map[string]any{"id": id})
	})

	mux.HandleFunc("/v1/people:createContact", func(w http.ResponseWriter, r *http.Request) {
		var person map[string]any
		_ = json.NewDecoder(r.Body).Decode(&person)
		f.contacts = append(f.contacts, person)
		writeJSON(w, map[string]any{"resourceName": "people/c1"})
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// TestSalidaLlevaElArbolYLosContactos: el árbol se recrea bajo una carpeta
// propia, los ficheros llegan enteros y el contacto conserva lo suyo.
func TestSalidaLlevaElArbolYLosContactos(t *testing.T) {
	allowLoopback(t)
	suite := sytueServer(t)
	google := newFakeGoogleWrite(t)

	inst := googleConsentInstance(google.srv.URL)
	inst.Config[migrate.ConfigGooglePeopleBase] = google.srv.URL

	// 1) Los ficheros.
	jDrive := job("job-out-drive", domain.SourceGoogle, domain.DomainDrive)
	jDrive.Direction = domain.DirectionOut
	jDrive.SourceUser = ""
	st := newMemStore("org-1")
	st.put(jDrive)
	dst, err := migrate.NewOutboundDestination(context.Background(), gmailRuntime(), inst, jDrive)
	if err != nil {
		t.Fatalf("NewOutboundDestination: %v", err)
	}
	deps := migrate.Deps{
		Src:   migrate.NewSytueSource(suite.URL, suite.URL, migrate.StaticToken("tok"), nil),
		Dst:   dst,
		Store: st,
	}
	if err := migrate.Run(context.Background(), deps, jDrive); err != nil {
		t.Fatalf("Run (drive): %v", err)
	}
	got, _ := st.get("job-out-drive")
	if got.Status != domain.MigrationDone {
		t.Fatalf("estado final = %q (%s)", got.Status, got.LastError)
	}
	if google.files["memoria.pdf"] != "PDF-UNO" || google.files["plan.txt"] != "PLAN" {
		t.Errorf("los ficheros no llegaron enteros: %v", google.files)
	}
	// El árbol se recrea: plan.txt cuelga de «Proyectos», no de la raíz.
	if google.parents["plan.txt"] != "g-Proyectos" {
		t.Errorf("plan.txt colgó de %q; debía conservar su carpeta", google.parents["plan.txt"])
	}
	// Y todo cuelga de una carpeta propia, no de la raíz del Drive ajeno.
	if google.folders["g-Sytue"] != "Sytue" {
		t.Errorf("no se creó la carpeta contenedora: %v", google.folders)
	}

	// 2) Los contactos.
	jContacts := job("job-out-contacts", domain.SourceGoogle, domain.DomainContacts)
	jContacts.Direction = domain.DirectionOut
	jContacts.SourceUser = ""
	st.put(jContacts)
	dstC, err := migrate.NewOutboundDestination(context.Background(), gmailRuntime(), inst, jContacts)
	if err != nil {
		t.Fatalf("NewOutboundDestination (contacts): %v", err)
	}
	depsC := migrate.Deps{
		Src:   migrate.NewSytueSource(suite.URL, suite.URL, migrate.StaticToken("tok"), nil),
		Dst:   dstC,
		Store: st,
	}
	if err := migrate.Run(context.Background(), depsC, jContacts); err != nil {
		t.Fatalf("Run (contacts): %v", err)
	}
	if len(google.contacts) != 1 {
		t.Fatalf("contactos creados = %d; esperaba 1", len(google.contacts))
	}
	raw, _ := json.Marshal(google.contacts[0])
	for _, esperado := range []string{"Ana", "Ruiz", "ana@acme.example", "+34600000000", "Acme"} {
		if !strings.Contains(string(raw), esperado) {
			t.Errorf("el contacto perdió %q por el camino: %s", esperado, raw)
		}
	}
}

// TestSalidaSegundaPasadaNoDuplica: reanudar no vuelve a subir nada. En un Drive
// ajeno duplicar es peor que fallar: nadie sabe cuál de las dos copias es buena.
func TestSalidaSegundaPasadaNoDuplica(t *testing.T) {
	allowLoopback(t)
	suite := sytueServer(t)
	google := newFakeGoogleWrite(t)
	inst := googleConsentInstance(google.srv.URL)

	j := job("job-out-2", domain.SourceGoogle, domain.DomainDrive)
	j.Direction = domain.DirectionOut
	j.SourceUser = ""
	st := newMemStore("org-1")
	st.put(j)
	run := func(job domain.MigrationJob) {
		dst, err := migrate.NewOutboundDestination(context.Background(), gmailRuntime(), inst, job)
		if err != nil {
			t.Fatalf("NewOutboundDestination: %v", err)
		}
		deps := migrate.Deps{
			Src:   migrate.NewSytueSource(suite.URL, suite.URL, migrate.StaticToken("tok"), nil),
			Dst:   dst,
			Store: st,
		}
		if err := migrate.Run(context.Background(), deps, job); err != nil {
			t.Fatalf("Run: %v", err)
		}
	}
	run(j)
	subidas := google.next

	segunda, _ := st.get("job-out-2")
	run(*segunda)
	if google.next != subidas {
		t.Errorf("la segunda pasada volvió a escribir en Google: %d → %d", subidas, google.next)
	}
}

// TestSalidaDelegadaSoloPermiteHostsOficialesDelManifiesto fija la decisión sobre la
// salida delegada de las migraciones: ni las raíces de Google/Microsoft ni el token_uri
// de una cuenta de servicio pueden apuntar a un servidor ajeno al manifiesto oficial.
func TestSalidaDelegadaSoloPermiteHostsOficialesDelManifiesto(t *testing.T) {
	ctx := context.Background()
	rt := gmailRuntime()

	// (1) Google: raíz de Drive o People fuera de *.googleapis.com.
	for _, clave := range []string{migrate.ConfigGoogleDriveBase, migrate.ConfigGooglePeopleBase} {
		inst := connector.Instance{
			ID: "i-g", OrgID: "org-1", HasBearer: true,
			Config: map[string]string{clave: "https://evil.example/drive"},
		}
		j := job("job-g", domain.SourceGoogle, domain.DomainDrive)
		j.Direction = domain.DirectionOut
		j.SourceUser = ""
		if _, err := migrate.NewOutboundDestination(ctx, rt, inst, j); err == nil {
			t.Errorf("Google (%s) aceptó un host fuera del manifiesto oficial", clave)
		}
	}

	// (2) Google con delegación de dominio: token_uri fuera de Google en service_account_key.
	instDelegada := connector.Instance{
		ID: "i-gd", OrgID: "org-1",
		Secrets: map[string]string{
			"service_account_key": `{"client_email":"sa@acme.iam.gserviceaccount.com","private_key":"-----BEGIN PRIVATE KEY-----\n-----END PRIVATE KEY-----","token_uri":"https://evil.example/token"}`,
		},
	}
	jDel := job("job-gd", domain.SourceGoogle, domain.DomainDrive)
	jDel.Direction = domain.DirectionOut
	jDel.SourceUser = "ana@acme.com"
	if _, err := migrate.NewOutboundDestination(ctx, rt, instDelegada, jDel); err == nil {
		t.Error("Google delegado aceptó un token_uri fuera de los hosts oficiales")
	}

	// (3) Microsoft 365: graph_base o authority_base fuera de los hosts oficiales.
	for _, clave := range []string{"graph_base", "authority_base"} {
		inst := connector.Instance{
			ID: "i-m", OrgID: "org-1", HasBearer: true,
			Config: map[string]string{clave: "https://evil.example/v1.0"},
		}
		j := job("job-m", domain.SourceM365, domain.DomainDrive)
		j.Direction = domain.DirectionOut
		j.SourceUser = "ana@acme.com"
		if _, err := migrate.NewOutboundDestination(ctx, rt, inst, j); err == nil {
			t.Errorf("Microsoft 365 (%s) aceptó un host fuera del manifiesto oficial", clave)
		}
	}
}
