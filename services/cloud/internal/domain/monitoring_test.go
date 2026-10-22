// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

// TestValidateUptimeTarget es la barrera anti-SSRF de la creación/edición de un
// uptime check (CRÍTICA): solo https públicas. Debe rechazar esquemas no-https y
// TODO destino interno por host (loopback/RFC1918/link-local/metadata/nombres del
// clúster) — un tenant no debe sondear recursos internos vía el prober.
func TestValidateUptimeTarget(t *testing.T) {
	blocked := []string{
		"",                          // vacío
		"http://example.com",        // http en claro (no https)
		"ftp://example.com",         // esquema peligroso
		"https://localhost",         // loopback por nombre
		"https://127.0.0.1",         // loopback
		"https://10.0.0.5",          // RFC1918
		"https://192.168.1.1",       // RFC1918
		"https://172.16.0.1",        // RFC1918
		"https://169.254.169.254/",  // metadata cloud (link-local)
		"https://[::1]",             // loopback IPv6
		"https://100.64.5.7/",       // CGNAT RFC6598: la CIDR de pods del clúster
		"https://100.127.255.254/",  // CGNAT, límite superior del /10
		"https://redis.internal",    // nombre interno
		"https://svc.cluster.local", // nombre de clúster k8s
		"https://kubernetes",        // nombre "desnudo" (servicio k8s)
		"https://app.local",         // .local
		"not a url with spaces",     // mal formada / sin esquema
	}
	for _, raw := range blocked {
		if err := ValidateUptimeTarget(raw); err == nil {
			t.Errorf("ValidateUptimeTarget(%q) = nil, quiero error (SSRF/entrada inválida)", raw)
		}
	}
	allowed := []string{
		"https://example.com",
		"https://app.apps.sytue.eu",
		"https://api.cliente.com/health",
	}
	for _, raw := range allowed {
		if err := ValidateUptimeTarget(raw); err != nil {
			t.Errorf("ValidateUptimeTarget(%q) = %v, quiero nil", raw, err)
		}
	}
}

func TestNormalizeInterval(t *testing.T) {
	cases := map[int]int{
		0:       60,                // default
		-5:      60,                // negativo → default
		10:      UptimeMinInterval, // por debajo del mínimo
		30:      30,
		120:     120,
		1000000: UptimeMaxInterval, // por encima del máximo
	}
	for in, want := range cases {
		if got := NormalizeInterval(in); got != want {
			t.Errorf("NormalizeInterval(%d) = %d, quiero %d", in, got, want)
		}
	}
}

func TestValidAlertEvent(t *testing.T) {
	validos := []string{
		AlertEventDeployFailed, AlertEventDeployReady, AlertEventUptimeDown,
		AlertEventUptimeUp, AlertEventSpendThreshold, AlertEventSizingAdvice,
	}
	for _, e := range validos {
		if !ValidAlertEvent(e) {
			t.Errorf("ValidAlertEvent(%q) = false, quiero true", e)
		}
	}
	for _, e := range []string{"", "deploy.error", "uptime", "cpu.high", "deploy.ready ", "UPTIME.DOWN"} {
		if ValidAlertEvent(e) {
			t.Errorf("ValidAlertEvent(%q) = true, quiero false", e)
		}
	}
}
