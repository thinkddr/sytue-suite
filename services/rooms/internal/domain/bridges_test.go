// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

// Un puente reenvía TODO lo que se escribe en el canal: solo puede apuntar al
// webhook entrante auténtico de cada proveedor.
func TestValidBridgeURL(t *testing.T) {
	casos := []struct {
		platform, url string
		ok            bool
	}{
		{BridgeSlack, "https://hooks.slack.com/services/T000/B000/XXXX", true},
		{BridgeSlack, "https://hooks.slack.com/triggers/T000/123/abc", true},
		{BridgeSlack, "http://hooks.slack.com/services/T000/B000/XXXX", false},      // sin TLS
		{BridgeSlack, "https://hooks.slack.com.evil.example/services/T/B/X", false}, // sufijo falso
		{BridgeSlack, "https://user:pass@hooks.slack.com/services/T/B/X", false},    // credenciales
		{BridgeSlack, "https://hooks.slack.com:8443/services/T/B/X", false},         // otro puerto
		{BridgeTeams, "https://acme.webhook.office.com/webhookb2/abc@def/IncomingWebhook/x/y", true},
		{BridgeTeams, "https://prod-12.westeurope.logic.azure.com:443/workflows/abc/triggers/manual/paths/invoke?sig=x", true},
		{BridgeTeams, "https://default123.environment.api.powerplatform.com/powerautomate/automations/direct/workflows/abc", true},
		{BridgeTeams, "https://hooks.slack.com/services/T/B/X", false}, // host de otra plataforma
		{BridgeTeams, "https://logic.azure.com.example.org/workflows/abc", false},
		{BridgeGoogleChat, "https://chat.googleapis.com/v1/spaces/AAAA/messages?key=k&token=t", true},
		{BridgeGoogleChat, "https://chat.googleapis.com/v1/users/me", false},
		{"discord", "https://discord.com/api/webhooks/1/abc", false},
	}
	for _, c := range casos {
		if got := ValidBridgeURL(c.platform, c.url); got != c.ok {
			t.Errorf("ValidBridgeURL(%s, %s) = %v, quiero %v", c.platform, c.url, got, c.ok)
		}
	}
	if got := MaskBridgeURL("https://hooks.slack.com/services/T000/B000/XXXXabcd"); got != "hooks.slack.com/…abcd" {
		t.Errorf("MaskBridgeURL = %q", got)
	}
}
