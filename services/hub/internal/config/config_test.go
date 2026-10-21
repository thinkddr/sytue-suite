// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package config

import (
	"strings"
	"testing"
	"time"
)

func clearHubEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"ENV", "HTTP_ADDR", "LOG_LEVEL", "LOG_FORMAT", "SHUTDOWN_TIMEOUT", "VERSION",
		"PORT", "PUBLIC_URL", "DATABASE_URL", "DB_MAX_CONNS",
		"OIDC_ISSUER", "OIDC_JWKS_URL", "OIDC_AUDIENCE",
		"DRIVE_BASE_URL", "MAIL_BASE_URL", "CALENDAR_BASE_URL", "CONTACTS_BASE_URL", "ROOMS_BASE_URL", "CHAT_BASE_URL", "CAST_BASE_URL", "PAGES_BASE_URL", "BASES_BASE_URL",
		"DRIVE_PUBLIC_URL", "MAIL_PUBLIC_URL", "CALENDAR_PUBLIC_URL", "CONTACTS_PUBLIC_URL", "ROOMS_PUBLIC_URL", "CHAT_PUBLIC_URL", "NOTES_PUBLIC_URL", "ERP_PUBLIC_URL", "PAGES_PUBLIC_URL", "BASES_PUBLIC_URL",
		"SEARCH_TIMEOUT", "SEARCH_LIMIT", "UPSTREAM_TIMEOUT", "PRESENCE_TTL", "NOTIFY_RETENTION",
	} {
		t.Setenv(k, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearHubEnv(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/hub")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if cfg.ServiceName != ServiceName {
		t.Errorf("ServiceName = %q", cfg.ServiceName)
	}
	if cfg.HTTPAddr != ":8092" {
		t.Errorf("HTTPAddr default = %q, want :8092", cfg.HTTPAddr)
	}
	if cfg.SearchLimit != 20 {
		t.Errorf("SearchLimit default = %d", cfg.SearchLimit)
	}
	if cfg.PresenceTTL != 45*time.Second {
		t.Errorf("PresenceTTL default = %v", cfg.PresenceTTL)
	}
	if cfg.OIDCJWKSURL != cfg.OIDCIssuer+"/.well-known/jwks.json" {
		t.Errorf("OIDCJWKSURL derivado = %q", cfg.OIDCJWKSURL)
	}
	if cfg.DriveBaseURL == "" || cfg.RoomsPublicURL == "" {
		t.Error("faltan URLs de upstream por defecto")
	}
	// Sytue Cast escucha en el 8099 (CONVENTIONS §6) y su Service se llama `cast`.
	if cfg.CastBaseURL != "http://cast:8099" {
		t.Errorf("CastBaseURL por defecto = %q, quiero http://cast:8099", cfg.CastBaseURL)
	}
	// Y Sytue Web en el 8100, con su Service `pages`.
	if cfg.PagesBaseURL != "http://pages:8100" {
		t.Errorf("PagesBaseURL por defecto = %q, quiero http://pages:8100", cfg.PagesBaseURL)
	}
}

// Los enlaces que devuelve la búsqueda se componen como <PUBLIC_URL>/<ruta de la app>
// (p. ej. `/channels/<id>`), así que la base tiene que traer el prefijo de la app
// (`appPaths` de @sytue/config), como hace el chart. Sin él, un resultado de Rooms abría
// `rooms.<dominio>/channels/…`, que no existe.
func TestLasURLPublicasPorDefectoLlevanElPrefijoDeLaApp(t *testing.T) {
	clearHubEnv(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/hub")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	for url, prefijo := range map[string]string{
		cfg.DrivePublicURL: "/drive", cfg.MailPublicURL: "/mail", cfg.CalendarPublicURL: "/calendar",
		cfg.ContactsPublicURL: "/contacts", cfg.NotesPublicURL: "/notes", cfg.RoomsPublicURL: "/rooms",
		cfg.ERPPublicURL: "/erp", cfg.PagesPublicURL: "/web", cfg.BasesPublicURL: "/bases",
	} {
		if !strings.HasSuffix(url, prefijo) {
			t.Errorf("%q no acaba en %q", url, prefijo)
		}
	}
}

func TestLoadPortOverride(t *testing.T) {
	clearHubEnv(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/hub")
	t.Setenv("PORT", "7100")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if cfg.HTTPAddr != ":7100" {
		t.Errorf("PORT debe fijar HTTPAddr, got %q", cfg.HTTPAddr)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	clearHubEnv(t)
	if _, err := Load(); err == nil {
		t.Fatal("Load() debe fallar sin DATABASE_URL")
	}
}

func TestValidateSearchLimit(t *testing.T) {
	clearHubEnv(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/hub")
	t.Setenv("SEARCH_LIMIT", "999")
	if _, err := Load(); err == nil {
		t.Fatal("SEARCH_LIMIT fuera de rango debe fallar")
	}
}

// El renombrado de Chat a Rooms tocó las variables de entorno del despliegue. Un
// entorno a medio migrar (las viejas puestas, las nuevas no) debe seguir apuntando al
// servicio, no caer al default: el Hub se quedaría sin buscar en los mensajes sin que
// nada fallara a la vista.
func TestLoadAceptaLosNombresViejosDeRooms(t *testing.T) {
	clearHubEnv(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/hub")
	t.Setenv("CHAT_BASE_URL", "http://viejo:8085")
	t.Setenv("CHAT_PUBLIC_URL", "https://viejo.example")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RoomsBaseURL != "http://viejo:8085" || cfg.RoomsPublicURL != "https://viejo.example" {
		t.Fatalf("respaldo ignorado: %q / %q", cfg.RoomsBaseURL, cfg.RoomsPublicURL)
	}

	// Y si están las dos, manda la nueva.
	t.Setenv("ROOMS_BASE_URL", "http://rooms:8085")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RoomsBaseURL != "http://rooms:8085" {
		t.Fatalf("la variable nueva debe ganar, salió %q", cfg.RoomsBaseURL)
	}
}
