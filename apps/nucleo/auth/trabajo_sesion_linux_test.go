// SPDX-License-Identifier: Apache-2.0
//go:build linux && !android

package auth

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zalando/go-keyring"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

func TestSessionWorkRefreshSurvivesDriveDisabledAndRejectsStaleLogout(t *testing.T) {
	aparte(t)
	keyring.MockInit()
	store := (&OIDC{Issuer: "https://id.example"}).Almacen()
	tokens := Tokens{Refresh: "synthetic-a", Issuer: store.emisor, Cliente: store.cliente, Subject: "ana", Organizacion: "acme"}
	if err := store.Guardar(tokens); err != nil {
		t.Fatal(err)
	}
	session := NewSession(tokens, nil, store.Guardar)
	first, err := store.TestigoTrabajo(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := ajustes.Cambiar(func(a *ajustes.Ajustes) { a.DriveSistemaDesactivado = true }); err != nil {
		t.Fatal(err)
	}
	tokens.Refresh = "synthetic-refresh"
	if err := store.Guardar(tokens); err != nil {
		t.Fatal(err)
	}
	refreshed, err := store.TestigoTrabajo(t.Context())
	if err != nil || refreshed.Acceso != first.Acceso {
		t.Fatal("refresh or Drive setting invalidated all apps", err)
	}
	for _, subject := range []string{"bea", "ana"} {
		tokens.Subject = subject
		if err := store.Guardar(tokens); err != nil {
			t.Fatal(err)
		}
	}
	if err := session.LogoutConTestigo(t.Context(), store, first); !errors.Is(err, ErrTrabajoSesion) {
		t.Fatal("old logout erased new incarnation", err)
	}
	remaining, err := store.Cargar()
	if err != nil || remaining.Refresh == "" || remaining.Subject != "ana" || session.Subject() != "ana" {
		t.Fatal("stale logout changed memory or persistent login", err)
	}
	current, err := store.TestigoTrabajo(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := session.LogoutConTestigo(t.Context(), store, current); err != nil {
		t.Fatal(err)
	}
	if session.Subject() != "" {
		t.Fatal("confirmed logout retained memory")
	}
	remaining, err = store.Cargar()
	if err != nil || remaining.Refresh != "" {
		t.Fatal("confirmed logout retained credentials", err)
	}
}

func TestInteractiveSameAccountLoginInvalidatesEarlierLogoutWitness(t *testing.T) {
	aparte(t)
	keyring.MockInit()
	store := (&OIDC{Issuer: "https://id.example"}).Almacen()
	tokens := Tokens{Refresh: "synthetic-old", Issuer: store.emisor, Cliente: store.cliente, Subject: "ana", Organizacion: "acme"}
	session := NewSession(tokens, nil, store.Guardar)
	if err := store.Guardar(tokens); err != nil {
		t.Fatal(err)
	}
	before, err := store.TestigoTrabajo(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	tokens.Refresh = "synthetic-new-interactive"
	if err := session.AdoptarCon(t.Context(), tokens); err != nil {
		t.Fatal(err)
	}
	if err := session.LogoutConTestigo(t.Context(), store, before); !errors.Is(err, ErrTrabajoSesion) {
		t.Fatal("old logout erased same-account login", err)
	}
	stored, err := store.Cargar()
	if err != nil || stored.Refresh != tokens.Refresh {
		t.Fatal("new interactive login was removed", err)
	}
}

func TestIndividualWitnessKeepsRefreshAndSeparatesOrganisationContext(t *testing.T) {
	aparte(t)
	keyring.MockInit()
	store := (&OIDC{Issuer: "https://id.example"}).Almacen()
	tokens := Tokens{Refresh: "synthetic-personal", Subject: "ana"}
	if err := store.Guardar(tokens); err != nil {
		t.Fatal(err)
	}
	first, err := store.TestigoTrabajo(t.Context())
	if err != nil || !first.Acceso.Activo {
		t.Fatal("individual session has no native witness", err)
	}
	drive, err := ajustes.LeerAccesoDrive()
	if err != nil || drive.Activo {
		t.Fatal("individual marker enabled Drive without tenant", err)
	}
	tokens.Refresh = "synthetic-rotated"
	if err := store.Guardar(tokens); err != nil {
		t.Fatal(err)
	}
	refreshed, err := store.TestigoTrabajo(t.Context())
	if err != nil || refreshed.Acceso != first.Acceso {
		t.Fatal("individual refresh rotated witness", err)
	}
	tokens.Organizacion = "acme"
	if err := store.Guardar(tokens); err != nil {
		t.Fatal(err)
	}
	organisation, err := store.TestigoTrabajo(t.Context())
	if err != nil || organisation.Acceso.Identidad == first.Acceso.Identidad || organisation.Acceso.Revision == first.Acceso.Revision {
		t.Fatal("individual and organisation contexts collided", err)
	}
	tokens.Organizacion = ""
	if err := store.Guardar(tokens); err != nil {
		t.Fatal(err)
	}
	returned, err := store.TestigoTrabajo(t.Context())
	if err != nil || returned.Acceso.Revision == first.Acceso.Revision {
		t.Fatal("individual→organisation→individual reused nonce", err)
	}
}

func TestGuardedLegacyCloseRechecksPersistedIdentityAfterWaitingForLock(t *testing.T) {
	aparte(t)
	keyring.MockInit()
	store := (&OIDC{Issuer: "https://id.example"}).Almacen()
	a := Tokens{Refresh: "synthetic-a", Subject: "ana"}
	if err := store.Guardar(a); err != nil {
		t.Fatal(err)
	}
	session := NewSession(a, nil, store.Guardar)
	witness, err := store.TestigoTrabajo(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	release, err := ajustes.CerrojoSesion(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	first := make(chan struct{})
	var once sync.Once
	cleaned := false
	done := make(chan error, 1)
	go func() {
		done <- session.LogoutConGuardia(t.Context(), func() error {
			err := witness.Validar()
			once.Do(func() { close(first) })
			return err
		}, func() error { cleaned = true; return nil })
	}()
	<-first
	b := a
	b.Subject = "bea"
	b.Refresh = "synthetic-b"
	if err := store.Guardar(b); err != nil {
		release()
		t.Fatal(err)
	}
	release()
	if err := <-done; !errors.Is(err, ErrTrabajoSesion) {
		t.Fatal("late close accepted another process's new login", err)
	}
	saved, err := store.Cargar()
	if err != nil || saved.Subject != "bea" || saved.Refresh != b.Refresh || session.Subject() != "ana" || cleaned {
		t.Fatal("stale close erased retained credentials or local data", err, cleaned)
	}
}

func TestSessionMigrationBootstrapsIndividualWorkWitnessWithoutRenewing(t *testing.T) {
	aparte(t)
	keyring.MockInit()
	store := (&OIDC{Issuer: "https://id.example"}).Almacen()
	tokens := Tokens{Subject: "ana", Refresh: "synthetic-existing-individual"}
	if err := store.Guardar(tokens); err != nil {
		t.Fatal(err)
	}
	dir, err := ajustes.Carpeta()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, store.Identificador()+".acceso.json")); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrar(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	witness, err := ajustes.LeerAccesoSesion(store.Identificador())
	if err != nil || !witness.Activo {
		t.Fatal("existing login has no session witness after update", err)
	}
	saved, err := store.Cargar()
	if err != nil || saved.Refresh != tokens.Refresh || saved.Organizacion != "" {
		t.Fatal("bootstrap rewrote or invented the login", err)
	}
}

func TestNativeCleanupFailureKeepsCapturedCredentialAndMemoryForRetry(t *testing.T) {
	aparte(t)
	keyring.MockInit()
	store := (&OIDC{Issuer: "https://id.example"}).Almacen()
	tokens := Tokens{Issuer: store.emisor, Cliente: store.cliente, Subject: "ana", Refresh: "synthetic-retained"}
	if err := store.Guardar(tokens); err != nil {
		t.Fatal(err)
	}
	session := NewSession(tokens, nil, store.Guardar)
	witness, err := store.TestigoTrabajo(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("local cleanup unavailable")
	if err := session.LogoutConTestigoConLimpieza(t.Context(), store, witness, func() error { return failure }); !errors.Is(err, failure) {
		t.Fatal("cleanup failure hidden", err)
	}
	saved, err := store.Cargar()
	if err != nil || saved.Refresh != tokens.Refresh || session.Subject() != "ana" {
		t.Fatal("failed local cleanup erased the captured session", err)
	}
	if err := witness.Validar(); err != nil {
		t.Fatal("failed cleanup invalidated retry witness", err)
	}
	if err := session.LogoutConTestigoConLimpieza(t.Context(), store, witness, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if session.Subject() != "" {
		t.Fatal("confirmed retry did not close session")
	}
}

func TestSessionWorkCorruptMarkerRejectsAdoptionBeforeCredentialWrite(t *testing.T) {
	aparte(t)
	keyring.MockInit()
	store := (&OIDC{Issuer: "https://id.example"}).Almacen()
	old := Tokens{Refresh: "synthetic-kept", Subject: "ana", Organizacion: "acme"}
	if err := store.Guardar(old); err != nil {
		t.Fatal(err)
	}
	dir, err := ajustes.Carpeta()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, store.Identificador()+".acceso.json"), []byte(`{"version":1,"active":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	session := NewSession(old, nil, store.Guardar)
	next := old
	next.Refresh = "synthetic-must-not-replace"
	if err := session.AdoptarCon(t.Context(), next); !errors.Is(err, ErrSesionNoDisponible) {
		t.Fatal("corrupt work marker authorized credential replacement", err)
	}
	stored, err := store.Cargar()
	if err != nil || stored.Refresh != old.Refresh {
		t.Fatal("failed broker witness changed persistent credentials", err)
	}
}
