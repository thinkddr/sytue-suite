// SPDX-License-Identifier: Apache-2.0
package notify

import (
	"encoding/json"
	"testing"

	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

func TestRelayPreservesPrivateRoutingAndCrossOrganizationOrigin(t *testing.T) {
	original := wireEnvelope{
		Tenant: "origin-org", Subject: "email:recipient@example.test",
		Notification: domain.Notification{Tenant: "origin-org", Subject: "email:recipient@example.test", Title: "synthetic"},
	}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded wireEnvelope
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	event := decoded.evento()
	n, ok := event.Datos.(domain.Notification)
	if !ok || event.Nombre != "notification" || n.Tenant != original.Tenant || n.Subject != original.Subject {
		t.Fatal("relay lost its internal routing context")
	}
	if got := n.VistoDesde("other-org"); got.Organization != "origin-org" {
		t.Fatal("cross-organization notification lost its origin", got.Organization)
	}
	if got := n.VistoDesde("origin-org"); got.Organization != "" {
		t.Fatal("same-organization notification was marked as external", got.Organization)
	}
	public, err := json.Marshal(n.VistoDesde("other-org"))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(public, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["tenant"]; ok {
		t.Fatal("internal tenant was exposed")
	}
	if _, ok := fields["subject"]; ok {
		t.Fatal("internal recipient was exposed")
	}
}

func TestRelayLeavesStateEventUnchanged(t *testing.T) {
	original := wireEnvelope{Tenant: "origin-org", Subject: "person", Event: "avatar", Data: json.RawMessage(`{"version":1}`)}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded wireEnvelope
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	event := decoded.evento()
	data, ok := event.Datos.(json.RawMessage)
	if event.Nombre != "avatar" || !ok || string(data) != `{"version":1}` {
		t.Fatal("state event changed while restoring notification context")
	}
}
