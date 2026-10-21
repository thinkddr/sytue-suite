// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ws

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// fakeAuth implementa Authorizer con una decisión fija. host, si no está vacío, es
// la organización anfitriona que concede el acceso (conversación de otra
// organización); vacío = la de la conexión.
type fakeAuth struct {
	allow bool
	host  string
}

func (f fakeAuth) CanAccessChannel(_ context.Context, tenant string, _ uuid.UUID, _ string) (string, bool) {
	if f.host != "" {
		tenant = f.host
	}
	return tenant, f.allow
}

// clientWithHub construye un Client de test enlazado a un Hub y un Authorizer.
func clientWithHub(h *Hub, auth Authorizer) *Client {
	return &Client{
		hub:     h,
		auth:    auth,
		tenant:  "acme",
		subject: "sub-1",
		name:    "Alice",
		send:    make(chan ServerFrame, sendBuffer),
		subs:    map[uuid.UUID]string{},
	}
}

func TestClientSubject(t *testing.T) {
	c := clientWithHub(NewHub(nil), fakeAuth{allow: true})
	if c.Subject() != "sub-1" {
		t.Fatalf("Subject = %q", c.Subject())
	}
}

func TestOnSubscribeAuthorized(t *testing.T) {
	h := NewHub(nil)
	c := clientWithHub(h, fakeAuth{allow: true})
	ch := uuid.New()
	c.onSubscribe(context.Background(), ch)
	if !c.subscribed(ch) {
		t.Fatal("debería estar suscrito")
	}
	// Idempotente: segunda suscripción no duplica ni error.
	c.onSubscribe(context.Background(), ch)
	// Entregar al canal alcanza al cliente.
	h.Deliver("acme", ch, ServerFrame{Type: ServerMessage}, nil)
	if got := drain(c); len(got) != 1 {
		t.Fatalf("esperaba 1 frame, got %d", len(got))
	}
}

func TestOnSubscribeNilChannel(t *testing.T) {
	c := clientWithHub(NewHub(nil), fakeAuth{allow: true})
	c.onSubscribe(context.Background(), uuid.Nil)
	frames := drain(c)
	if len(frames) != 1 || frames[0].Type != ServerError || frames[0].Code != "bad_request" {
		t.Fatalf("esperaba error bad_request, got %+v", frames)
	}
}

func TestOnSubscribeForbidden(t *testing.T) {
	c := clientWithHub(NewHub(nil), fakeAuth{allow: false})
	c.onSubscribe(context.Background(), uuid.New())
	frames := drain(c)
	if len(frames) != 1 || frames[0].Code != "forbidden" {
		t.Fatalf("esperaba forbidden, got %+v", frames)
	}
}

func TestOnUnsubscribe(t *testing.T) {
	h := NewHub(nil)
	c := clientWithHub(h, fakeAuth{allow: true})
	ch := uuid.New()
	c.onSubscribe(context.Background(), ch)
	drain(c)
	c.onUnsubscribe(ch)
	if c.subscribed(ch) {
		t.Fatal("no debería seguir suscrito")
	}
	// Unsubscribe de un canal no suscrito es no-op.
	c.onUnsubscribe(uuid.New())
}

func TestOnTyping(t *testing.T) {
	h := NewHub(nil)
	sender := clientWithHub(h, fakeAuth{allow: true})
	other := clientWithHub(h, fakeAuth{allow: true})
	ch := uuid.New()
	sender.onSubscribe(context.Background(), ch)
	other.onSubscribe(context.Background(), ch)
	drain(sender)
	drain(other)

	sender.onTyping(context.Background(), ch)
	// El emisor se excluye.
	if got := drain(sender); len(got) != 0 {
		t.Fatalf("el emisor no debe recibir su typing, got %v", got)
	}
	got := drain(other)
	if len(got) != 1 || got[0].Type != ServerTyping || got[0].Subject != "sub-1" {
		t.Fatalf("typing mal difundido: %+v", got)
	}

	// Typing en canal no suscrito es no-op.
	sender.onTyping(context.Background(), uuid.New())
	if got := drain(other); len(got) != 0 {
		t.Fatal("typing en canal ajeno no debe difundirse")
	}
}

func TestOnPresence(t *testing.T) {
	h := NewHub(nil)
	c := clientWithHub(h, fakeAuth{allow: true})
	other := clientWithHub(h, fakeAuth{allow: true})
	ch := uuid.New()
	c.onSubscribe(context.Background(), ch)
	other.onSubscribe(context.Background(), ch)
	drain(c)
	drain(other)

	c.onPresence(context.Background(), ClientFrame{Type: ClientPresence, Status: "away"})
	got := drain(other)
	if len(got) != 1 || got[0].Type != ServerPresence || got[0].Status != "away" {
		t.Fatalf("presence away mal difundida: %+v", got)
	}

	// Status desconocido cae a "online".
	c.onPresence(context.Background(), ClientFrame{Type: ClientPresence, Status: "bogus"})
	got = drain(other)
	if len(got) != 1 || got[0].Status != "online" {
		t.Fatalf("presence debe normalizar a online, got %+v", got)
	}
}

func TestHandleDispatch(t *testing.T) {
	h := NewHub(nil)
	c := clientWithHub(h, fakeAuth{allow: true})
	ctx := context.Background()

	// ping -> pong
	c.handle(ctx, ClientFrame{Type: ClientPing})
	if got := drain(c); len(got) != 1 || got[0].Type != ServerPong {
		t.Fatalf("ping debe responder pong, got %+v", got)
	}

	// subscribe vía handle.
	ch := uuid.New()
	c.handle(ctx, ClientFrame{Type: ClientSubscribe, ChannelID: ch})
	if !c.subscribed(ch) {
		t.Fatal("handle subscribe no suscribió")
	}
	drain(c)

	// tipo desconocido es no-op.
	c.handle(ctx, ClientFrame{Type: "nope"})
	if got := drain(c); len(got) != 0 {
		t.Fatalf("tipo desconocido no debe emitir, got %v", got)
	}
}

func TestCleanupUnsubscribesAll(t *testing.T) {
	h := NewHub(nil)
	c := clientWithHub(h, fakeAuth{allow: true})
	ch1, ch2 := uuid.New(), uuid.New()
	c.onSubscribe(context.Background(), ch1)
	c.onSubscribe(context.Background(), ch2)
	if len(h.rooms) != 2 {
		t.Fatalf("esperaba 2 salas, got %d", len(h.rooms))
	}
	c.cleanup()
	if len(h.rooms) != 0 {
		t.Fatalf("cleanup debe vaciar las salas, quedan %d", len(h.rooms))
	}
	if c.subscribed(ch1) || c.subscribed(ch2) {
		t.Fatal("cleanup debe limpiar las suscripciones locales")
	}
}

func TestErrorFrame(t *testing.T) {
	f := errorFrame("code_x", "mensaje")
	if f.Type != ServerError || f.Code != "code_x" || f.Message != "mensaje" {
		t.Fatalf("errorFrame inesperado: %+v", f)
	}
}

// Una conversación de OTRA organización donde el usuario está como persona de
// fuera se difunde en la organización anfitriona (ADR 0040): la suscripción tiene
// que quedar en SU sala, o los mensajes no llegan nunca a quien viene de fuera.
func TestOnSubscribeConversacionDeOtraOrganizacion(t *testing.T) {
	h := NewHub(nil)
	c := clientWithHub(h, fakeAuth{allow: true, host: "otra"})
	ch := uuid.New()
	c.onSubscribe(context.Background(), ch)

	h.Deliver("acme", ch, ServerFrame{Type: ServerMessage}, nil)
	if got := drain(c); len(got) != 0 {
		t.Fatalf("la sala de su propia organización no es la de esa conversación: %d frames", len(got))
	}
	h.Deliver("otra", ch, ServerFrame{Type: ServerMessage}, nil)
	if got := drain(c); len(got) != 1 {
		t.Fatalf("esperaba el frame de la organización anfitriona, got %d", len(got))
	}

	// Lo que escribe también sale en la sala anfitriona.
	otro := clientWithHub(h, fakeAuth{allow: true, host: "otra"})
	otro.subject = "sub-2"
	otro.onSubscribe(context.Background(), ch)
	c.onTyping(context.Background(), ch)
	if got := drain(otro); len(got) != 1 || got[0].Type != ServerTyping {
		t.Fatalf("el «escribiendo» debía llegar por la sala anfitriona: %+v", got)
	}

	c.onUnsubscribe(ch)
	h.Deliver("otra", ch, ServerFrame{Type: ServerMessage}, nil)
	if got := drain(c); len(got) != 0 {
		t.Fatalf("tras darse de baja no debe llegar nada: %d", len(got))
	}
}
