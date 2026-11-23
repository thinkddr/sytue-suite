// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"fyne.io/fyne/v2/test"

	photoView "git.sytue.eu/sytue/one/apps/fyne/apps/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// driveConFuente es el Drive del servidor que solo contesta el estado de su fuente.
func driveConFuente(t *testing.T, modo string) string {
	t.Helper()
	estado := `{"scope":{"tenant":"acme","subject":"ada","product":"drive"},"mode":"legacy","rows":0,"blobs":0}`
	if modo != "legacy" {
		estado = fmt.Sprintf(`{"scope":{"tenant":"acme","subject":"ada","product":"drive"},"mode":%q,"epoch":"12345678-1234-4234-8234-123456789abc","started_at":"2026-11-01T10:00:00Z","rows":2,"blobs":1}`, modo)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/private-storage/migration" {
			t.Errorf("petición inesperada al Drive del servidor: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, estado)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// Por la API, la galería sube al Drive del servidor mientras su fuente siga en
// `legacy`; con el Drive en almacenamiento privado, al Drive privado de la cuenta
// (cifrado, sin índice del servidor), y si aún no está abierto lo dice para que la
// galería pida desbloquearlo. Nunca empieza una migración.
func TestPhotosPorLaAPISubeAlDriveQueAdmiteEscrituras(t *testing.T) {
	test.NewTempApp(t)
	text := func(k string) string { return k }
	cuenta := cuentaDePrueba{scope: storage.Scope{Tenant: "acme", Subject: "ada", Product: "photos"}, session: sesionDePrueba{}}
	identity := auth.IdentitySnapshot{Subject: "ada", Organization: "acme"}

	host := &photosStorage{}
	servidor, err := host.driveDeSubida(cuenta, identity, driveConFuente(t, "legacy"), text)(t.Context())
	if err != nil || servidor == nil || servidor.Privado() {
		t.Fatalf("con la fuente en legacy: %v, privado=%v", err, servidor.Privado())
	}

	frozen := driveConFuente(t, "frozen")
	if _, err := host.driveDeSubida(cuenta, identity, frozen, text)(t.Context()); !errors.Is(err, photoView.ErrDriveCifradoCerrado) {
		t.Fatalf("sin el Drive privado abierto: %v", err)
	}
	if !host.needed || host.neededBy != identity {
		t.Fatal("no se pidió abrir el Drive privado")
	}

	host.driveRepository.Store(&photosDrive{repository: &privateDrive.Repository{}, resources: cuenta})
	privado, err := host.driveDeSubida(cuenta, identity, frozen, text)(t.Context())
	if err != nil || !privado.Privado() {
		t.Fatalf("con el Drive privado abierto: %v", err)
	}

	host.driveFailure = errors.New("no hay Drive privado en este dispositivo")
	host.driveRepository.Store(nil)
	if _, err := host.driveDeSubida(cuenta, identity, frozen, text)(t.Context()); !errors.Is(err, host.driveFailure) || !errors.Is(err, photoView.ErrDriveCifradoNoDisponible) {
		t.Fatalf("con el Drive privado roto: %v", err)
	}
}
