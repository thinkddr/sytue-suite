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
