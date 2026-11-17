// SPDX-License-Identifier: Apache-2.0
package nativestoragetest

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestSessionPortsKeepRealOriginalWitnessAndExplicitBroker(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("isolated remote Linux preference and OS lease probe; native Apple hardware is separate")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	session := auth.NewSession(auth.Tokens{Issuer: "https://id.example.test", Cliente: "suite", Subject: "alice", Organizacion: "acme", Access: "synthetic-session-port-access", Expiry: time.Now().Add(time.Hour)}, nil, nil)
	ctx := session.WithAuthorizationIntent(t.Context())
	registrations := 0
	ports, err := NewSessionPorts(ctx, session, "sesion-eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", func(offline.PrivateWork, auth.TestigoTrabajo) (func(), error) {
		registrations++
		return func() { registrations-- }, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}
	witness, err := ports.CaptureWork(ctx, scope)
	if err != nil || witness.Acceso.Identidad != session.SnapshotIdentity().IdentidadSesion() {
		t.Fatal("captured a different owner", err)
	}
	release, err := witness.Compartido(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	blocked, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
	defer cancel()
	if exclusive, err := witness.Exclusivo(blocked); err == nil {
		exclusive()
		t.Fatal("synthetic session ports bypassed the real OS lease")
	}
	if identity, inside := ports.CurrentIdentity(); !inside || identity != session.SnapshotIdentity() {
		t.Fatal("identity port changed the session")
	}
	if bound, err := ports.BindHTTP(ctx, witness); err != nil || bound != session {
		t.Fatal("HTTP port did not retain its supplied session", err)
	}
	unregister, err := ports.RegisterWork(nil, witness)
	if err != nil || registrations != 1 {
		t.Fatal("explicit caller broker was bypassed", registrations, err)
	}
	unregister()
	if registrations != 0 {
		t.Fatal("caller broker registration was not released")
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, err := ports.CaptureWork(canceled, scope); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled capture was accepted", err)
	}
	if _, err := ports.BindHTTP(canceled, witness); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled HTTP binding was accepted", err)
	}
}

func TestSessionPortsRejectMissingSessionOrBrokerBeforePreferencesIO(t *testing.T) {
	if _, err := NewSessionPorts(t.Context(), nil, "", nil); err == nil {
		t.Fatal("missing isolated session was accepted")
	}
	if _, err := NewSessionPorts(t.Context(), auth.NewSession(auth.Tokens{}, nil, nil), "", nil); err == nil {
		t.Fatal("implicit broker was accepted")
	}
}
