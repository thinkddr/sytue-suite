// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/livekit"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
)

func TestResolveRole(t *testing.T) {
	room := domain.Room{CreatedBy: "owner-sub"}

	// El creador siempre es host, aunque pida viewer.
	if r := resolveRole(room, "owner-sub", domain.RoleViewer, false); r != domain.RoleHost {
		t.Errorf("creador: got %s want host", r)
	}
	// Un no-admin que pide host queda degradado a speaker.
	if r := resolveRole(room, "other", domain.RoleHost, false); r != domain.RoleSpeaker {
		t.Errorf("no-admin pide host: got %s want speaker", r)
	}
	// Un admin que pide host lo obtiene.
	if r := resolveRole(room, "other", domain.RoleHost, true); r != domain.RoleHost {
		t.Errorf("admin pide host: got %s want host", r)
	}
	// Rol solicitado válido se respeta.
	if r := resolveRole(room, "other", domain.RoleViewer, false); r != domain.RoleViewer {
		t.Errorf("viewer solicitado: got %s want viewer", r)
	}
	// Rol inválido cae a speaker.
	if r := resolveRole(room, "other", domain.Role("bogus"), false); r != domain.RoleSpeaker {
		t.Errorf("rol inválido: got %s want speaker", r)
	}
}

func TestRoleFromMetadata(t *testing.T) {
	if r := roleFromMetadata(`{"role":"host"}`); r != domain.RoleHost {
		t.Errorf("host: got %s", r)
	}
	if r := roleFromMetadata(`{"role":"viewer"}`); r != domain.RoleViewer {
		t.Errorf("viewer: got %s", r)
	}
	// Metadata vacía, inválida o rol desconocido -> speaker por defecto.
	for _, meta := range []string{"", "no-json", `{"role":"bogus"}`, `{}`} {
		if r := roleFromMetadata(meta); r != domain.RoleSpeaker {
			t.Errorf("metadata %q: got %s want speaker", meta, r)
		}
	}
}

func TestTenantFromMetadata(t *testing.T) {
	if got := tenantFromMetadata(`{"sytue_tenant":"acme"}`); got != "acme" {
		t.Errorf("tenant: got %q", got)
	}
	for _, meta := range []string{"", "no-json", `{"other":"x"}`} {
		if got := tenantFromMetadata(meta); got != "" {
			t.Errorf("metadata %q: got %q want vacío", meta, got)
		}
	}
}

func TestTenantFromEvent(t *testing.T) {
	ev := livekit.WebhookEvent{Room: &livekit.WebhookRoom{Metadata: `{"sytue_tenant":"acme"}`}}
	if got := tenantFromEvent(ev); got != "acme" {
		t.Errorf("tenant de room: got %q", got)
	}
	// Evento sin room (p. ej. egress) no resuelve tenant por metadata.
	if got := tenantFromEvent(livekit.WebhookEvent{}); got != "" {
		t.Errorf("evento sin room: got %q want vacío", got)
	}
}

func TestEventTime(t *testing.T) {
	if got := eventTime(1_700_000_000); got.Unix() != 1_700_000_000 {
		t.Errorf("epoch válido: got %d", got.Unix())
	}
	if got := eventTime(0); time.Since(got) > time.Minute {
		t.Errorf("epoch 0 debe caer a ahora, got %v", got)
	}
	if got := eventTime(-5); time.Since(got) > time.Minute {
		t.Errorf("epoch negativo debe caer a ahora, got %v", got)
	}
}

func TestRoomName(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		n := roomName("acme", "")
		if !domain.ValidRoomName(n) {
			t.Fatalf("nombre generado inválido: %q", n)
		}
		if seen[n] {
			t.Fatalf("nombre repetido (no impredecible): %q", n)
		}
		seen[n] = true
		if tenantFromRoomName(n) != "acme" {
			t.Fatalf("el nombre debe llevar su tenant: %q", n)
		}
	}
}

// El prefijo es la ÚNICA forma de atribuir un webhook del SFU a su tenant, así que un
// nombre propuesto por el llamante jamás puede ocupar esa posición: si pudiera, un
// tenant crearía salas que el SFU acabaría atribuyendo a otro.
func TestRoomNameNoDejaSuplantarElTenant(t *testing.T) {
	n := roomName("acme", "otrotenant_loquesea")
	if tenantFromRoomName(n) != "acme" {
		t.Fatalf("el tenant real debe mandar, salió %q de %q", tenantFromRoomName(n), n)
	}
}

// Nombres que no llevan tenant (salas anteriores a este formato, o creadas en LiveKit
// por fuera de Sytue) no deben atribuirse a nadie.
func TestTenantFromRoomNameSinPrefijo(t *testing.T) {
	for _, n := range []string{"", "room-abc123", "_abc", "ACME_x"} {
		if got := tenantFromRoomName(n); got != "" {
			t.Fatalf("%q no lleva tenant válido, devolvió %q", n, got)
		}
	}
}
