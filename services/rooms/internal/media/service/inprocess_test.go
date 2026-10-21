// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	roomsdomain "git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// msgSpy es la mensajería del mismo proceso, anotando lo que le llega.
type msgSpy struct {
	gotTenant   string
	gotChannel  uuid.UUID
	gotAuthor   string
	gotDuration int64
	err         error
}

func (m *msgSpy) PostCallEnded(ctx context.Context, channelID uuid.UUID, authorSub string, durationSec int64) (roomsdomain.Message, error) {
	if t, err := tenancy.From(ctx); err == nil {
		m.gotTenant = t.Slug
	}
	m.gotChannel, m.gotAuthor, m.gotDuration = channelID, authorSub, durationSec
	return roomsdomain.Message{}, m.err
}

func (m *msgSpy) GetChannel(ctx context.Context, id uuid.UUID, subject string) (roomsdomain.Channel, error) {
	if t, err := tenancy.From(ctx); err == nil {
		m.gotTenant = t.Slug
	}
	m.gotChannel, m.gotAuthor = id, subject
	return roomsdomain.Channel{ID: id}, m.err
}

// El puente traduce tenant y UUID y entrega la duración intacta: es lo que convierte el
// aviso del SFU en la tarjeta «Llamada finalizada · duración» del canal.
func TestInProcessNotifierCallEnded(t *testing.T) {
	t.Parallel()
	spy := &msgSpy{}
	id := uuid.New()
	if err := NewInProcessNotifier(spy).CallEnded(context.Background(), CallEndedInput{
		Tenant: "acme", ChannelID: id.String(), AuthorSub: "user-1", DurationSec: 93,
	}); err != nil {
		t.Fatalf("CallEnded: %v", err)
	}
	if spy.gotTenant != "acme" || spy.gotChannel != id || spy.gotAuthor != "user-1" || spy.gotDuration != 93 {
		t.Fatalf("llegó %q/%v/%q/%d", spy.gotTenant, spy.gotChannel, spy.gotAuthor, spy.gotDuration)
	}
}

// Un identificador que no es un UUID (o un tenant inválido) no puede convertirse en una
// escritura: se rechaza aquí, no en la base de datos.
func TestInProcessNotifierCallEndedRechazaEntradaInvalida(t *testing.T) {
	t.Parallel()
	for name, in := range map[string]CallEndedInput{
		"tenant vacío":  {Tenant: "", ChannelID: uuid.NewString()},
		"canal no-uuid": {Tenant: "acme", ChannelID: "no-soy-un-uuid"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			spy := &msgSpy{}
			if err := NewInProcessNotifier(spy).CallEnded(context.Background(), in); err == nil {
				t.Fatal("se esperaba error")
			}
			if spy.gotAuthor != "" {
				t.Fatal("no debe tocar la mensajería con entrada inválida")
			}
		})
	}
}

// La verja de la sala: poder LEER el canal es poder entrar a su llamada. Que la
// mensajería diga que no (no existe, no es miembro) no es una avería del puente — se
// traduce a «no puede», sin error, para que el llamante responda 403 y no 500.
func TestInProcessNotifierChannelAccess(t *testing.T) {
	t.Parallel()
	id := uuid.NewString()

	ok, err := NewInProcessNotifier(&msgSpy{}).ChannelAccess(context.Background(), "acme", id, "user-1")
	if err != nil || !ok {
		t.Fatalf("miembro: ok=%v err=%v", ok, err)
	}

	spy := &msgSpy{err: errors.New("no es miembro")}
	ok, err = NewInProcessNotifier(spy).ChannelAccess(context.Background(), "acme", id, "user-1")
	if err != nil {
		t.Fatalf("una negativa no es una avería: %v", err)
	}
	if ok {
		t.Fatal("no debe conceder acceso si la mensajería lo niega")
	}
}

// Sin mensajería no hay puente: el constructor devuelve nil para que el arranque falle
// a la vista en vez de dejar una verja que concede acceso a ciegas.
func TestNewInProcessNotifierSinMensajeria(t *testing.T) {
	t.Parallel()
	if NewInProcessNotifier(nil) != nil {
		t.Fatal("se esperaba nil")
	}
}
