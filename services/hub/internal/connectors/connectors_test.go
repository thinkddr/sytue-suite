// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package connectors

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

// TestDriveConnectorDecodesFilesWrapper fija que el conector de Drive lee la
// envoltura {"files":[...]} real del servicio (no un array desnudo) y mapea al
// modelo canónico. Regresión: antes decodificaba []driveFile y fallaba.
func TestDriveConnectorDecodesFilesWrapper(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/api/v1/search" {
			t.Errorf("ruta inesperada: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"files":[{"id":"f1","name":"Informe.txt","content_type":"text/plain","size_bytes":2048}]}`))
	}))
	defer srv.Close()

	c := NewDriveConnector(srv.URL, "https://drive.sytue.localhost", time.Second)
	hits, err := c.Search(context.Background(), "tok", "informe", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits=%d, quiero 1", len(hits))
	}
	if hits[0].Source != domain.SourceDrive || hits[0].Title != "Informe.txt" {
		t.Errorf("hit inesperado: %+v", hits[0])
	}
	if hits[0].URL != "https://drive.sytue.localhost/files/f1" {
		t.Errorf("deep-link inesperado: %s", hits[0].URL)
	}
	// El tamaño va también como número, para que cada cliente lo diga en su idioma.
	// VISTO EN ROJO sin `SizeBytes` en el conector.
	if hits[0].SizeBytes == nil || *hits[0].SizeBytes != 2048 {
		t.Errorf("el tamaño no llega como número: %v", hits[0].SizeBytes)
	}
}

// TestDriveConnectorRecent fija que el conector de Drive lee la envoltura
// {"items":[...]} de GET /api/v1/recent (updated_at DESC), omite carpetas y
// mapea los ficheros al modelo canónico con su deep-link y timestamp de recencia.
func TestDriveConnectorRecent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/api/v1/recent" {
			t.Errorf("ruta inesperada: %s (quiero /api/v1/recent)", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[
			{"id":"f1","name":"Informe.txt","content_type":"text/plain","size_bytes":2048,"is_directory":false,"updated_at":"2026-07-10T10:00:00Z"},
			{"id":"d1","name":"Proyectos","is_directory":true,"updated_at":"2026-07-11T10:00:00Z"}
		]}`))
	}))
	defer srv.Close()

	c := NewDriveConnector(srv.URL, "https://drive.sytue.localhost", time.Second)
	hits, err := c.Recent(context.Background(), "tok", 5)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	// Solo ficheros: la carpeta d1 se omite.
	if len(hits) != 1 {
		t.Fatalf("hits=%d, quiero 1 (solo ficheros)", len(hits))
	}
	h := hits[0]
	if h.Source != domain.SourceDrive || h.ID != "f1" || h.Title != "Informe.txt" {
		t.Errorf("hit inesperado: %+v", h)
	}
	if h.URL != "https://drive.sytue.localhost/files/f1" {
		t.Errorf("deep-link inesperado: %s", h.URL)
	}
	if h.Timestamp == nil || !h.Timestamp.Equal(*mustTime("2026-07-10T10:00:00Z")) {
		t.Errorf("timestamp de recencia inesperado: %v", h.Timestamp)
	}
	// «Reciente» también manda el tamaño como número, para que el cliente lo escriba en
	// su idioma («2 KB», no «2.0 KiB»). VISTO EN ROJO sin `SizeBytes` en `Recent`.
	if h.SizeBytes == nil || *h.SizeBytes != 2048 {
		t.Errorf("el tamaño de «Reciente» no llega como número: %v", h.SizeBytes)
	}
}

func mustTime(v string) *time.Time {
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		panic(err)
	}
	return &t
}

// TestContactsConnectorPathAndFields fija que el conector de Contacts consulta
// GET /api/v1/contacts (no /contacts/search) y mapea formatted_name + emails[].
func TestContactsConnectorPathAndFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/api/v1/contacts" {
			t.Errorf("ruta inesperada: %s (quiero /api/v1/contacts)", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"contacts":[{"id":"c1","formatted_name":"Ada Lovelace","organization":"Analytical","emails":[{"email":"ada@sytue.localhost"}]}]}`))
	}))
	defer srv.Close()

	c := NewContactsConnector(srv.URL, "https://contacts.sytue.localhost", time.Second)
	hits, err := c.Search(context.Background(), "tok", "ada", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits=%d, quiero 1", len(hits))
	}
	h := hits[0]
	if h.Title != "Ada Lovelace" || h.Snippet != "ada@sytue.localhost · Analytical" {
		t.Errorf("mapeo inesperado: title=%q snippet=%q", h.Title, h.Snippet)
	}
	if h.URL != "https://contacts.sytue.localhost/contacts/c1" {
		t.Errorf("deep-link inesperado: %s", h.URL)
	}
}

func TestCalendarConnectorFechaYEnlace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events":[{"id":"e1","summary":"Comité","dtstart":"2026-07-01T09:00:00Z"}]}`))
	}))
	defer srv.Close()

	c := NewCalendarConnector(srv.URL, "https://calendar.sytue.localhost", time.Second)
	hits, err := c.Search(context.Background(), "tok", "comité", 5)
	if err != nil || len(hits) != 1 {
		t.Fatalf("Search: hits=%d err=%v", len(hits), err)
	}
	h := hits[0]
	if h.Timestamp == nil || !h.Timestamp.Equal(*mustTime("2026-07-01T09:00:00Z")) {
		t.Errorf("fecha inesperada: %v (Calendar la manda como dtstart)", h.Timestamp)
	}
	if h.URL != "https://calendar.sytue.localhost/events/e1" {
		t.Errorf("deep-link inesperado: %s", h.URL)
	}
}
