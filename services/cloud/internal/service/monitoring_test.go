// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"testing"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// TestAlertEventForBusType fija el enrutado de tipos de evento del bus a los
// eventos cortos de alert_policies. Un tipo no cubierto NO alimenta alertas
// (ok=false) — así el consumer ignora el resto del canal "cloud".
func TestAlertEventForBusType(t *testing.T) {
	cases := []struct {
		busType string
		want    string
		wantOK  bool
	}{
		{store.EventDeployReady, domain.AlertEventDeployReady, true},
		{store.EventDeployError, domain.AlertEventDeployFailed, true},
		{store.EventUptimeDown, domain.AlertEventUptimeDown, true},
		{store.EventUptimeUp, domain.AlertEventUptimeUp, true},
		{"cloud.build.enqueued", "", false},
		{"cloud.deploy.requested", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := alertEventForBusType(c.busType)
		if got != c.want || ok != c.wantOK {
			t.Errorf("alertEventForBusType(%q) = (%q,%v), quiero (%q,%v)",
				c.busType, got, ok, c.want, c.wantOK)
		}
	}
}

// CLOUD-19 — una alerta con canal mailto: nace sin confirmar (Confirmed=false),
// guarda el hash SHA-256 de un token aleatorio y solo queda confirmada tras
// presentar el token en ConfirmAlertPolicy. Los webhooks HTTP(S) nacen ya
// confirmados.
func TestCreateAlertPolicyMailtoRequiereConfirmacionPorEnlace(t *testing.T) {
	fs := &fakeStore{}
	s := newTestService(fs, nil)
	ctx := tenantCtx(t)

	// Webhook HTTP(S): confirmado desde el alta.
	wh, err := s.CreateAlertPolicy(ctx, CreateAlertPolicyInput{
		Event:      domain.AlertEventDeployFailed,
		ChannelURL: "https://hooks.slack.com/services/T00/B00/X00",
	})
	if err != nil {
		t.Fatalf("CreateAlertPolicy webhook: %v", err)
	}
	if !wh.Confirmed || fs.newAlert == nil || !fs.newAlert.Confirmed || fs.newAlert.ConfirmTokenHash != nil {
		t.Fatalf("un webhook debe nacer confirmado y sin token: policy=%+v rec=%+v", wh, fs.newAlert)
	}

	// Canal mailto: nace sin confirmar y con hash de token.
	mailPol, err := s.CreateAlertPolicy(ctx, CreateAlertPolicyInput{
		Event:      domain.AlertEventUptimeDown,
		ChannelURL: "mailto:ops@sytue.eu",
	})
	if err != nil {
		t.Fatalf("CreateAlertPolicy mailto: %v", err)
	}
	if mailPol.Confirmed {
		t.Fatal("una alerta mailto: debe nacer sin confirmar (Confirmed=false)")
	}
	if fs.newAlert == nil || fs.newAlert.Confirmed || fs.newAlert.ConfirmTokenHash == nil || len(*fs.newAlert.ConfirmTokenHash) != 64 {
		t.Fatalf("debe persistirse el hash SHA-256 del token de confirmación: %+v", fs.newAlert)
	}
}
