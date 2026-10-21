// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubhttp

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/portabilidad"
	"git.sytue.eu/sytue/one/services/hub/internal/config"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

func TestExportJuntaLasEntregasEnUnZIP(t *testing.T) {
	var (
		mu     sync.Mutex
		tokens []string
	)
	producto := func(fin string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			tokens = append(tokens, r.Header.Get("Authorization"))
			mu.Unlock()
			if fin == "404" {
				http.NotFound(w, r)
				return
			}
			p, err := portabilidad.Nuevo(w, r)
			if err != nil {
				t.Errorf("formato: %v", err)
				return
			}
			_ = p.Fichero("Carpeta/nota.txt", time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC), strings.NewReader("uno"))
			_ = p.Fichero("Carpeta/NOTA.txt", time.Time{}, strings.NewReader("dos"))
			_ = p.Fichero("../../fuera:de?raiz.txt", time.Time{}, strings.NewReader("tres"))
			if fin == "fallo" {
				_ = p.Fallo(errors.New("el almacén no responde"))
				return
			}
			_ = p.Cerrar()
		}))
	}
	correo, agenda, fotos := producto(""), producto("fallo"), producto("404")
	defer correo.Close()
	defer agenda.Close()
	defer fotos.Close()

	h := &Handlers{cfg: config.Config{
		MailBaseURL: correo.URL, CalendarBaseURL: agenda.URL, PhotosBaseURL: fotos.URL,
		AppsDesplegadas: []string{"mail", "calendar", "photos"},
	}, store: &fakeStore{items: []domain.Notification{{Title: "de fuera", Tenant: "globex"}}}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/export?format=json", http.NoBody)
	req.Header.Set("Authorization", "Bearer tok")
	req = req.WithContext(authn.Into(req.Context(), authn.Claims{Subject: "u", Tenant: "acme"}))
	// La cuenta no está en este despliegue de prueba: su URL vacía no responde.
	rec := httptest.NewRecorder()
	h.Export(rec, req)

	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("respuesta: %d %s", rec.Code, rec.Body.String())
	}
	// Una copia por organización: el nombre dice de cuál es.
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "sytue-one-acme-") {
		t.Fatalf("nombre del ZIP: %q", rec.Header().Get("Content-Disposition"))
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("ZIP ilegible: %v", err)
	}
	ficheros := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		ficheros[strings.SplitN(f.Name, "/", 2)[1]] = string(b)
	}
	// Sus avisos, también los de otras organizaciones (ADR 0053).
	if a := ficheros["hub/notifications.json"]; !strings.Contains(a, "globex") || !strings.Contains(a, "de fuera") {
		t.Errorf("«Tus datos» sin los avisos de la persona: %q", a)
	}
	// «nota.txt» y «NOTA.txt» son el mismo fichero en Windows y macOS: el segundo
	// cambia de nombre. Y nada se sale de la carpeta del producto.
	for nombre, contenido := range map[string]string{
		"mail/Carpeta/nota.txt":     "uno",
		"mail/Carpeta/NOTA (2).txt": "dos",
		"mail/fuera_de_raiz.txt":    "tres",
	} {
		if ficheros[nombre] != contenido {
			t.Errorf("%s = %q; ficheros: %v", nombre, ficheros[nombre], keys(ficheros))
		}
	}

	var resumen []filaResumen
	if err := json.Unmarshal([]byte(ficheros["summary.json"]), &resumen); err != nil {
		t.Fatalf("resumen: %v", err)
	}
	estado := map[string]filaResumen{}
	for _, f := range resumen {
		estado[f.Product] = f
	}
	if estado["mail"].Status != "complete" || estado["mail"].Files != 3 {
		t.Errorf("correo: %+v", estado["mail"])
	}
	if estado["calendar"].Status != "incomplete" || estado["calendar"].Error != "el almacén no responde" {
		t.Errorf("un producto que falla a mitad no puede salir como completo: %+v", estado["calendar"])
	}
	if estado["photos"].Status != "unavailable" || estado["account"].Status != "unavailable" {
		t.Errorf("los que no entregan nada: %+v %+v", estado["photos"], estado["account"])
	}
	mu.Lock()
	defer mu.Unlock()
	if len(tokens) != 3 {
		t.Errorf("se abren las tres entregas: %d", len(tokens))
	}
	for _, tok := range tokens {
		if tok != "Bearer tok" {
			t.Errorf("cada producto recibe el token de la persona: %q", tok)
		}
	}
}

func TestExportRechazaFormatosDesconocidos(t *testing.T) {
	h := &Handlers{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/export?format=pdf", http.NoBody)
	rec := httptest.NewRecorder()
	h.Export(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("formato desconocido: %d", rec.Code)
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
