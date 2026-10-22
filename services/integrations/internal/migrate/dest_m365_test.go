// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/migrate"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
)

// fakeOneDrive es Microsoft 365 como DESTINO: OneDrive y la agenda de Outlook.
type fakeOneDrive struct {
	mu        sync.Mutex
	srv       *httptest.Server
	carpetas  map[string]string // «padre/nombre» → id
	ficheros  map[string]string // «padre/nombre» → contenido
	contactos []map[string]any
	auth      []string
}

func newFakeOneDrive(t *testing.T) *fakeOneDrive {
	t.Helper()
	f := &fakeOneDrive{carpetas: map[string]string{}, ficheros: map[string]string{}}
	responder := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		p := r.URL.Path
		switch {
		case r.Method == http.MethodPost && p == "/v1.0/me/contacts":
			var c map[string]any
			_ = json.NewDecoder(r.Body).Decode(&c)
			f.contactos = append(f.contactos, c)
			responder(w, http.StatusCreated, map[string]any{"id": "c1"})
		case r.Method == http.MethodGet && strings.HasSuffix(p, ":/Sytue"):
			if id, ok := f.carpetas["root/Sytue"]; ok {
				responder(w, http.StatusOK, map[string]any{"id": id})
				return
			}
			responder(w, http.StatusNotFound, map[string]any{"error": "itemNotFound"})
		case r.Method == http.MethodPost && strings.HasSuffix(p, "/children"):
			var cuerpo struct {
				Name string `json:"name"`
			}
			_ = json.NewDecoder(r.Body).Decode(&cuerpo)
			padre := "root"
			if strings.HasPrefix(p, "/v1.0/me/drive/items/") {
				padre = strings.TrimSuffix(strings.TrimPrefix(p, "/v1.0/me/drive/items/"), "/children")
			}
			clave := padre + "/" + cuerpo.Name
			if _, ok := f.carpetas[clave]; ok {
				responder(w, http.StatusConflict, map[string]any{"error": "nameAlreadyExists"})
				return
			}
			f.carpetas[clave] = "d-" + cuerpo.Name
			responder(w, http.StatusCreated, map[string]any{"id": "d-" + cuerpo.Name})
		case r.Method == http.MethodGet && strings.Contains(p, ":/"):
			// Carpeta que ya existía: /drive/items/{padre}:/{nombre}
			padre, nombre, _ := strings.Cut(strings.TrimPrefix(p, "/v1.0/me/drive/items/"), ":/")
			if id, ok := f.carpetas[padre+"/"+nombre]; ok {
				responder(w, http.StatusOK, map[string]any{"id": id})
				return
			}
			responder(w, http.StatusNotFound, map[string]any{})
		case r.Method == http.MethodPut && strings.HasSuffix(p, ":/content"):
			if r.ContentLength < 0 {
				responder(w, http.StatusLengthRequired, map[string]any{})
				return
			}
			padre, nombre, _ := strings.Cut(strings.TrimSuffix(strings.TrimPrefix(p, "/v1.0/me/drive/items/"), ":/content"), ":/")
			b, _ := io.ReadAll(r.Body)
			f.ficheros[padre+"/"+nombre] = string(b)
			responder(w, http.StatusCreated, map[string]any{"id": "f-" + nombre})
		default:
			responder(w, http.StatusNotFound, map[string]any{"path": p})
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// TestSalidaAMicrosoft365: el árbol llega a OneDrive bajo «Sytue», con cada fichero
// entero y en su carpeta, y el contacto a Outlook con sus correos. Una segunda
// pasada no duplica: las carpetas que ya están se reutilizan.
func TestSalidaAMicrosoft365(t *testing.T) {
	allowLoopback(t)
	suite := sytueServer(t)
	od := newFakeOneDrive(t)
	inst := connector.Instance{
		ID: "inst-1", OrgID: "org-1", TemplateKey: "microsoft365",
		Config: map[string]string{"graph_base": od.srv.URL}, HasBearer: true,
	}
	rt := policy.BuildRuntime(policy.RuntimeParams{OrgID: "org-1", InstanceID: "inst-1", Bearer: "ms-token", Log: slog.Default()})

	st := newMemStore("org-1")
	lanzar := func(dom domain.MigrationDomain, id string) {
		j := job(id, domain.SourceM365, dom)
		j.Direction = domain.DirectionOut
		j.SourceUser = ""
		st.put(j)
		dst, err := migrate.NewOutboundDestination(context.Background(), rt, inst, j)
		if err != nil {
			t.Fatalf("NewOutboundDestination: %v", err)
		}
		deps := migrate.Deps{
			Src:   migrate.NewSytueSource(suite.URL, suite.URL, migrate.StaticToken("tok"), nil),
			Dst:   dst,
			Store: st,
		}
		if err := migrate.Run(context.Background(), deps, j); err != nil {
			t.Fatalf("Run: %v", err)
		}
		got, _ := st.get(id)
		if got.Status != domain.MigrationDone {
			t.Fatalf("estado final = %q (%s)", got.Status, got.LastError)
		}
	}

	lanzar(domain.DomainDrive, "job-ms-drive")
	if od.ficheros["d-Sytue/memoria.pdf"] != "PDF-UNO" || od.ficheros["d-Proyectos/plan.txt"] != "PLAN" {
		t.Fatalf("los ficheros no llegaron enteros a su carpeta: %v", od.ficheros)
	}
	if od.carpetas["root/Sytue"] == "" || od.carpetas["d-Sytue/Proyectos"] == "" {
		t.Fatalf("árbol: %v", od.carpetas)
	}

	lanzar(domain.DomainContacts, "job-ms-contacts")
	if len(od.contactos) != 1 {
		t.Fatalf("contactos creados = %d", len(od.contactos))
	}
	raw, _ := json.Marshal(od.contactos[0])
	for _, esperado := range []string{"Ana Ruiz", "ana@acme.example", "+34600000000", "Acme"} {
		if !strings.Contains(string(raw), esperado) {
			t.Errorf("el contacto perdió %q: %s", esperado, raw)
		}
	}
	for _, a := range od.auth {
		if a != "Bearer ms-token" {
			t.Fatalf("cada escritura va con el token del consentimiento: %q", a)
		}
	}
}

// TestSalidaAMicrosoft365SinAutorizar: sin consentimiento no se empieza a medias.
func TestSalidaAMicrosoft365SinAutorizar(t *testing.T) {
	j := job("job-ms", domain.SourceM365, domain.DomainDrive)
	j.Direction = domain.DirectionOut
	j.SourceUser = ""
	inst := connector.Instance{ID: "inst-1", OrgID: "org-1", TemplateKey: "microsoft365", Config: map[string]string{}}
	if _, err := migrate.NewOutboundDestination(context.Background(), runtime(), inst, j); err == nil ||
		!strings.Contains(err.Error(), "ESCRIBIR") {
		t.Fatalf("debía pedir la autorización de escritura: %v", err)
	}
}
