// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// TestConfianzaDelDispositivo: el claim `device_trust` se evalúa contra el estado DE
// HOY del dispositivo de la sesión (mig 0128). Quitarle la confianza, revocarlo o
// cerrar la sesión lo bajan; un dispositivo de otra persona atado a la sesión no
// cuenta aunque sea de confianza.
func TestConfianzaDelDispositivo(t *testing.T) {
	st, ctx := newStore(t)
	org, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "confianza", Name: "Confianza"})
	if err != nil {
		t.Fatal(err)
	}
	ana := seedUser(ctx, t, st, "ana@confianza.test")
	bruno := seedUser(ctx, t, st, "bruno@confianza.test")
	dev, err := st.UpsertDevice(ctx, nil, domain.Device{OrgID: org.ID, UserID: ana.ID, DeviceID: "conf-ana", Name: "Portátil"})
	if err != nil {
		t.Fatal(err)
	}
	devBruno, err := st.UpsertDevice(ctx, nil, domain.Device{OrgID: org.ID, UserID: bruno.ID, DeviceID: "conf-bruno", Name: "Otro"})
	if err != nil {
		t.Fatal(err)
	}
	sesion := func(user, deviceID, token string, amr ...string) string {
		t.Helper()
		now := time.Now()
		id, err := st.InsertSession(ctx, nil, domain.Session{
			TokenHash: hashOf(token), UserID: user, OrgID: org.ID, AuthTime: now,
			AMR: amr, ExpiresAt: now.Add(time.Hour), DeviceID: deviceID,
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	confianza := func(sessionID string) string {
		t.Helper()
		dt, err := st.ConfianzaDelDispositivo(ctx, sessionID)
		if err != nil {
			t.Fatal(err)
		}
		return dt
	}

	conPasskey := sesion(ana.ID, dev.ID, "s-passkey", "webauthn")
	conClave := sesion(ana.ID, dev.ID, "s-clave", "pwd", "mfa")
	if dt := confianza(conPasskey); dt != domain.DeviceTrustNone {
		t.Errorf("dispositivo aún sin confianza: %q", dt)
	}
	if _, err := st.SetDeviceTrustedForUser(ctx, ana.ID, dev.ID, true); err != nil {
		t.Fatal(err)
	}
	if dt := confianza(conPasskey); dt != domain.DeviceTrustStandard {
		t.Errorf("de confianza y con passkey: %q, quiero standard", dt)
	}
	if dt := confianza(conClave); dt != domain.DeviceTrustNone {
		t.Errorf("de confianza pero sin passkey: %q, quiero none", dt)
	}
	revocada := sesion(ana.ID, dev.ID, "s-revocada", "webauthn")
	if err := st.RevokeSession(ctx, nil, hashOf("s-revocada")); err != nil {
		t.Fatal(err)
	}
	if dt := confianza(revocada); dt != domain.DeviceTrustNone {
		t.Errorf("sesión cerrada: %q, quiero none", dt)
	}

	// Un lazo a un dispositivo AJENO no da confianza aunque el ajeno la tenga.
	if _, err := st.SetDeviceTrustedForUser(ctx, bruno.ID, devBruno.ID, true); err != nil {
		t.Fatal(err)
	}
	if dt := confianza(sesion(ana.ID, devBruno.ID, "s-ajeno", "webauthn")); dt != domain.DeviceTrustNone {
		t.Errorf("sesión de Ana atada al dispositivo de Bruno: %q, quiero none", dt)
	}

	// Revocar el dispositivo (borrarlo) deja la sesión sin él: vuelve a none.
	if ok, err := st.DeleteDeviceForUser(ctx, ana.ID, dev.ID); err != nil || !ok {
		t.Fatalf("DeleteDeviceForUser: %v %v", ok, err)
	}
	if dt := confianza(conPasskey); dt != domain.DeviceTrustNone {
		t.Errorf("tras revocar el dispositivo: %q, quiero none", dt)
	}

	// Sin sesión, o con una que no existe: none, sin error.
	if dt := confianza(""); dt != domain.DeviceTrustNone {
		t.Errorf("sin sesión: %q", dt)
	}
	if dt := confianza("00000000-0000-0000-0000-000000000000"); dt != domain.DeviceTrustNone {
		t.Errorf("sesión inexistente: %q", dt)
	}
}
