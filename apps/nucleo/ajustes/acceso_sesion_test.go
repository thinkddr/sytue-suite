// SPDX-License-Identifier: Apache-2.0
package ajustes

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const testSessionStore = "sesion-12345678123456781234567812345678"

func TestSessionWorkWitnessRefreshPauseABAAndDrivePolicy(t *testing.T) {
	aparte(t)
	identity := strings.Repeat("a", 64)
	if err := Cambiar(func(a *Ajustes) { a.DriveSistemaDesactivado = true }); err != nil {
		t.Fatal(err)
	}
	if err := PonerAccesoSesionCon(t.Context(), testSessionStore, identity, true); err != nil {
		t.Fatal(err)
	}
	initial, err := LeerAccesoSesion(testSessionStore)
	if err != nil || !initial.Activo {
		t.Fatal(err)
	}
	if err := SuspenderAccesoSesion(t.Context(), testSessionStore, initial, true); err != nil {
		t.Fatal(err)
	}
	if err := PonerAccesoSesionCon(t.Context(), testSessionStore, identity, true); err != nil {
		t.Fatal(err)
	}
	refreshed, err := LeerAccesoSesion(testSessionStore)
	if err != nil || !refreshed.Suspendido || refreshed.Revision != initial.Revision {
		t.Fatal("refresh resumed drained work", refreshed, err)
	}
	if _, err := CerrojoTrabajoSesion(t.Context(), testSessionStore, initial, false); !errors.Is(err, ErrAccesoSesion) {
		t.Fatal("paused session admitted reader", err)
	}
	if err := SuspenderAccesoSesion(t.Context(), testSessionStore, initial, false); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{strings.Repeat("b", 64), identity} {
		if err := PonerAccesoSesionCon(t.Context(), testSessionStore, id, true); err != nil {
			t.Fatal(err)
		}
	}
	returned, err := LeerAccesoSesion(testSessionStore)
	if err != nil || returned.Revision == initial.Revision {
		t.Fatal("ABA reused incarnation", returned, err)
	}
	if err := SuspenderAccesoSesion(t.Context(), testSessionStore, initial, true); !errors.Is(err, ErrAccesoSesion) {
		t.Fatal("old logout paused a new login", err)
	}
	if _, err := CerrojoTrabajoSesion(t.Context(), testSessionStore, initial, false); !errors.Is(err, ErrAccesoSesion) {
		t.Fatal("old work survived ABA", err)
	}
}

func TestSessionScopeLeasesShareReadersWaitForCleanupAndSeparateLogins(t *testing.T) {
	aparte(t)
	identity := strings.Repeat("a", 64)
	if err := PonerAccesoSesionCon(t.Context(), testSessionStore, identity, true); err != nil {
		t.Fatal(err)
	}
	witness, err := LeerAccesoSesion(testSessionStore)
	if err != nil {
		t.Fatal(err)
	}
	first, err := CerrojoTrabajoSesion(t.Context(), testSessionStore, witness, false)
	if err != nil {
		t.Fatal(err)
	}
	secondCtx, secondCancel := context.WithTimeout(t.Context(), time.Second)
	second, err := CerrojoTrabajoSesion(secondCtx, testSessionStore, witness, false)
	secondCancel()
	if err != nil {
		first()
		t.Fatal("second reader blocked", err)
	}
	if err := SuspenderAccesoSesion(t.Context(), testSessionStore, witness, true); err != nil {
		first()
		second()
		t.Fatal(err)
	}
	deadline, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	_, err = CerrojoTrabajoSesion(deadline, testSessionStore, witness, true)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		first()
		second()
		t.Fatal("exclusive lease ignored active readers", err)
	}
	first()
	second()
	exclusive, err := CerrojoTrabajoSesion(t.Context(), testSessionStore, witness, true)
	if err != nil {
		t.Fatal(err)
	}
	defer exclusive()
	// A scope lease must not hold the global lock or stop a different login.
	global, err := CerrojoSesion(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	global()
	if err := PonerAccesoSesionCon(t.Context(), testSessionStore, strings.Repeat("b", 64), true); err != nil {
		t.Fatal(err)
	}
	next, err := LeerAccesoSesion(testSessionStore)
	if err != nil {
		t.Fatal(err)
	}
	another, err := CerrojoTrabajoSesion(t.Context(), testSessionStore, next, false)
	if err != nil {
		t.Fatal("old logout blocked new account", err)
	}
	another()
}
