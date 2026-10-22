// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"
)

func TestValidPushSubscription(t *testing.T) {
	ok := PushSubscription{Endpoint: "https://fcm.googleapis.com/fcm/send/abc", P256dh: "BPk...", Auth: "abc123"}
	if !ValidPushSubscription(ok) {
		t.Fatal("suscripción válida rechazada")
	}
	cases := []struct {
		name string
		sub  PushSubscription
	}{
		{"endpoint vacío", PushSubscription{P256dh: "x", Auth: "y"}},
		{"endpoint http (no https)", PushSubscription{Endpoint: "http://x/y", P256dh: "x", Auth: "y"}},
		{"endpoint demasiado largo", PushSubscription{Endpoint: "https://x/" + strings.Repeat("a", 3000), P256dh: "x", Auth: "y"}},
		{"p256dh vacía", PushSubscription{Endpoint: "https://x/y", Auth: "y"}},
		{"auth vacía", PushSubscription{Endpoint: "https://x/y", P256dh: "x"}},
		{"clave demasiado larga", PushSubscription{Endpoint: "https://x/y", P256dh: strings.Repeat("a", 300), Auth: "y"}},
	}
	for _, c := range cases {
		if ValidPushSubscription(c.sub) {
			t.Errorf("%s: debería rechazarse", c.name)
		}
	}
}
