// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ws

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// testClient construye un Client mínimo apto para probar el Hub sin conexión WS
// real: Deliver solo toca el canal de salida `send` (el `conn` únicamente se usa
// si la cola se desborda, caso que estos tests evitan).
func testClient() *Client {
	return &Client{send: make(chan ServerFrame, sendBuffer), subs: map[uuid.UUID]string{}}
}

// drain lee sin bloquear cuántos frames hay en la cola de un cliente.
func drain(c *Client) []ServerFrame {
	var out []ServerFrame
	for {
		select {
		case f := <-c.send:
			out = append(out, f)
		default:
			return out
		}
	}
}

func TestHubDeliverToSubscribers(t *testing.T) {
	t.Parallel()
	h := NewHub(nil)
	ch := uuid.New()
	a, b := testClient(), testClient()
	h.subscribe("acme", ch, a)
	h.subscribe("acme", ch, b)

	frame := ServerFrame{Type: ServerMessage}
	h.Deliver("acme", ch, frame, nil)

	if got := drain(a); len(got) != 1 || got[0].Type != ServerMessage {
		t.Fatalf("a debe recibir 1 mensaje, got %v", got)
	}
	if got := drain(b); len(got) != 1 {
		t.Fatalf("b debe recibir 1 mensaje, got %v", got)
	}
}

func TestHubDeliverExcludesSender(t *testing.T) {
	t.Parallel()
	h := NewHub(nil)
	ch := uuid.New()
	sender, other := testClient(), testClient()
	h.subscribe("acme", ch, sender)
	h.subscribe("acme", ch, other)

	h.Deliver("acme", ch, ServerFrame{Type: ServerTyping}, sender)

	if got := drain(sender); len(got) != 0 {
		t.Fatalf("el emisor no debe recibir su propio frame, got %v", got)
	}
	if got := drain(other); len(got) != 1 {
		t.Fatalf("el otro debe recibir el frame, got %v", got)
	}
}

func TestHubTenantIsolation(t *testing.T) {
	t.Parallel()
	h := NewHub(nil)
	ch := uuid.New() // mismo id de canal en dos tenants distintos
	acme, globex := testClient(), testClient()
	h.subscribe("acme", ch, acme)
	h.subscribe("globex", ch, globex)

	h.Deliver("acme", ch, ServerFrame{Type: ServerMessage}, nil)

	if got := drain(acme); len(got) != 1 {
		t.Fatalf("acme debe recibir, got %v", got)
	}
	if got := drain(globex); len(got) != 0 {
		t.Fatalf("globex NO debe recibir (aislamiento por tenant), got %v", got)
	}
}

func TestHubUnsubscribeAndRoomCleanup(t *testing.T) {
	t.Parallel()
	h := NewHub(nil)
	ch := uuid.New()
	c := testClient()
	h.subscribe("acme", ch, c)

	if n := len(h.rooms); n != 1 {
		t.Fatalf("debe haber 1 sala, hay %d", n)
	}
	h.unsubscribe("acme", ch, c)
	if n := len(h.rooms); n != 0 {
		t.Fatalf("la sala vacía debe eliminarse, quedan %d", n)
	}
	// Entregar a una sala inexistente no debe entrar en pánico.
	h.Deliver("acme", ch, ServerFrame{Type: ServerMessage}, nil)
	if got := drain(c); len(got) != 0 {
		t.Fatalf("cliente desuscrito no debe recibir, got %v", got)
	}
}

// TestDeliverRemote cubre el relay cross-réplica: un frame efímero publicado
// por OTRA réplica se entrega a los clientes locales de cada canal listado,
// mientras que el eco de la propia réplica y los payloads malformados se
// descartan.
func TestDeliverRemote(t *testing.T) {
	t.Parallel()
	h := NewHub(nil)
	ch1, ch2 := uuid.New(), uuid.New()
	a, b := testClient(), testClient()
	h.subscribe("acme", ch1, a)
	h.subscribe("acme", ch2, b)

	// Presencia remota: un publish con varios canales se abre en un Deliver por
	// canal, fijando ChannelID en cada frame.
	payload, err := json.Marshal(ephemeralMsg{
		Origin:   "otra-replica",
		Tenant:   "acme",
		Channels: []uuid.UUID{ch1, ch2},
		Frame:    ServerFrame{Type: ServerPresence, Subject: "sub-9", Status: "away"},
	})
	if err != nil {
		t.Fatal(err)
	}
	h.deliverRemote(payload)

	got := drain(a)
	if len(got) != 1 || got[0].Type != ServerPresence || got[0].ChannelID == nil || *got[0].ChannelID != ch1 {
		t.Fatalf("a debe recibir la presencia remota con su channel_id, got %+v", got)
	}
	got = drain(b)
	if len(got) != 1 || got[0].ChannelID == nil || *got[0].ChannelID != ch2 {
		t.Fatalf("b debe recibir la presencia remota con su channel_id, got %+v", got)
	}

	// Eco propio (mismo origin): ya se entregó localmente, debe descartarse.
	payload, err = json.Marshal(ephemeralMsg{
		Origin:   h.origin,
		Tenant:   "acme",
		Channels: []uuid.UUID{ch1},
		Frame:    ServerFrame{Type: ServerTyping},
	})
	if err != nil {
		t.Fatal(err)
	}
	h.deliverRemote(payload)
	if got := drain(a); len(got) != 0 {
		t.Fatalf("el eco de la propia réplica no debe reentregarse, got %v", got)
	}

	// Payload malformado: descartado sin pánico.
	h.deliverRemote([]byte("{no es json"))
	if got := drain(a); len(got) != 0 {
		t.Fatalf("payload malformado no debe entregar nada, got %v", got)
	}
}

// TestPublishEphemeralSinRelay verifica la degradación: sin relay (nil) el
// publish es no-op y no entra en pánico (los efímeros quedan intra-réplica).
func TestPublishEphemeralSinRelay(t *testing.T) {
	t.Parallel()
	h := NewHub(nil)
	h.publishEphemeral(t.Context(), "acme", []uuid.UUID{uuid.New()}, ServerFrame{Type: ServerTyping})
}

func TestClientEnqueueDoesNotBlock(t *testing.T) {
	t.Parallel()
	// Llenamos exactamente el buffer: no debe bloquear ni requerir conn.
	c := testClient()
	for i := 0; i < sendBuffer; i++ {
		c.enqueue(ServerFrame{Type: ServerMessage})
	}
	if got := len(drain(c)); got != sendBuffer {
		t.Fatalf("se esperaban %d frames encolados, got %d", sendBuffer, got)
	}
}

// COM-08. Quien deja un canal deja de recibirlo en el acto, aunque no cierre el socket;
// las demás personas de la sala siguen recibiendo.
func TestExpulsarDejaDeEntregarAQuienSale(t *testing.T) {
	t.Parallel()
	h := NewHub(nil)
	ch := uuid.New()
	ana, ana2, bob := testClient(), testClient(), testClient()
	ana.subject, ana2.subject, bob.subject = "ana", "ana", "bob"
	for _, c := range []*Client{ana, ana2, bob} {
		c.subs[ch] = "acme"
		h.subscribe("acme", ch, c)
	}

	h.Expulsar("acme", ch, "ana")
	h.Deliver("acme", ch, ServerFrame{Type: ServerMessage}, nil)

	if got := len(drain(ana)) + len(drain(ana2)); got != 0 {
		t.Fatalf("ana sigue recibiendo el canal del que salió: %d frames", got)
	}
	if _, sigue := ana.subs[ch]; sigue {
		t.Fatal("la suscripción de ana sigue apuntada en su conexión")
	}
	if got := drain(bob); len(got) != 1 {
		t.Fatalf("bob debe seguir recibiendo, got %v", got)
	}
}
