// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/ws"
)

func TestDedupeNonEmpty(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil", nil, nil},
		{"vacios", []string{"", "  ", "\t"}, []string{}},
		{"trim y dedup", []string{" a ", "b", "a", "b ", "c"}, []string{"a", "b", "c"}},
		{"todos iguales", []string{"x", "x", "x"}, []string{"x"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := dedupeNonEmpty(c.in)
			if len(got) == 0 && len(c.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("dedupeNonEmpty(%v)=%v, quiero %v", c.in, got, c.want)
			}
		})
	}
}

func TestWithBearerRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if bearerFrom(ctx) != "" {
		t.Fatal("contexto sin token debe devolver vacío")
	}
	// Token vacío no envenena el contexto.
	if got := bearerFrom(WithBearer(ctx, "")); got != "" {
		t.Fatalf("token vacío no debe almacenarse, got %q", got)
	}
	ctx = WithBearer(ctx, "abc.def.ghi")
	if got := bearerFrom(ctx); got != "abc.def.ghi" {
		t.Fatalf("bearerFrom=%q", got)
	}
}

// fakeBroadcaster captura las entregas del relay para inspección.
type fakeBroadcaster struct {
	tenant     string
	channel    uuid.UUID
	frame      ws.ServerFrame
	calls      int
	expulsados []string
}

func (f *fakeBroadcaster) Expulsar(tenant string, channel uuid.UUID, subject string) {
	f.expulsados = append(f.expulsados, tenant+"/"+channel.String()+"/"+subject)
}

func (f *fakeBroadcaster) Deliver(tenant string, channel uuid.UUID, frame ws.ServerFrame, _ *ws.Client) {
	f.tenant = tenant
	f.channel = channel
	f.frame = frame
	f.calls++
}

// TestFanoutHandlerTranslatesEvents comprueba que el relay del outbox traduce cada
// tipo de evento de dominio al frame WebSocket correcto y lo entrega a la sala del
// canal indicado en el sobre.
func TestFanoutHandlerTranslatesEvents(t *testing.T) {
	t.Parallel()
	channelID := uuid.New()

	newEnvEvent := func(typ string) events.Event {
		payload, _ := json.Marshal(eventEnvelope{ChannelID: channelID, Data: map[string]any{"k": "v"}})
		return events.Event{Channel: EventChannel, Type: typ, Tenant: "acme", Payload: payload}
	}

	cases := []struct {
		evType    string
		wantFrame string
	}{
		{EventMessageCreated, ws.ServerMessage},
		{EventMessageEdited, ws.ServerMessageEdited},
		{EventMessageDeleted, ws.ServerMessageDeleted},
		{EventReactionChanged, ws.ServerReaction},
		{EventPinChanged, ws.ServerPinChanged},
		{EventChannelMemberAdded, ws.ServerChannelMemberAdded},
		{EventChannelMemberRemoved, ws.ServerChannelMemberRemoved},
		{EventChannelDeleted, ws.ServerChannelDeleted},
	}
	for _, c := range cases {
		t.Run(c.evType, func(t *testing.T) {
			fb := &fakeBroadcaster{}
			h := NewFanoutHandler(fb)
			if err := h(context.Background(), newEnvEvent(c.evType)); err != nil {
				t.Fatalf("handler: %v", err)
			}
			if fb.calls != 1 {
				t.Fatalf("Deliver llamado %d veces", fb.calls)
			}
			if fb.tenant != "acme" || fb.channel != channelID {
				t.Fatalf("entrega a tenant/canal incorrectos: %s/%s", fb.tenant, fb.channel)
			}
			if fb.frame.Type != c.wantFrame {
				t.Fatalf("frame.Type=%q, quiero %q", fb.frame.Type, c.wantFrame)
			}
			if fb.frame.ChannelID == nil || *fb.frame.ChannelID != channelID {
				t.Fatal("el frame debe portar el channel_id")
			}
		})
	}
}

// TestFanoutHandlerIgnoresIrrelevant verifica que eventos de otro canal lógico,
// sin tenant, con payload corrupto o de tipo desconocido no producen entrega ni
// error (no se reintentan eternamente).
func TestFanoutHandlerIgnoresIrrelevant(t *testing.T) {
	t.Parallel()
	channelID := uuid.New()
	good, _ := json.Marshal(eventEnvelope{ChannelID: channelID})

	cases := []struct {
		name string
		ev   events.Event
	}{
		{"otro canal", events.Event{Channel: "drive", Type: EventMessageCreated, Tenant: "acme", Payload: good}},
		{"sin tenant", events.Event{Channel: EventChannel, Type: EventMessageCreated, Tenant: "", Payload: good}},
		{"payload corrupto", events.Event{Channel: EventChannel, Type: EventMessageCreated, Tenant: "acme", Payload: json.RawMessage(`{`)}},
		{"tipo desconocido", events.Event{Channel: EventChannel, Type: "chat.unknown", Tenant: "acme", Payload: good}},
		{"canal nil en sobre", events.Event{Channel: EventChannel, Type: EventMessageCreated, Tenant: "acme", Payload: json.RawMessage(`{"channel_id":"00000000-0000-0000-0000-000000000000"}`)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fb := &fakeBroadcaster{}
			h := NewFanoutHandler(fb)
			if err := h(context.Background(), c.ev); err != nil {
				t.Fatalf("no debe devolver error: %v", err)
			}
			if fb.calls != 0 {
				t.Fatalf("no debe entregar (calls=%d)", fb.calls)
			}
		})
	}
}

// TestCallFanoutAnunciaLaLlamadaEnSuCanal comprueba el puente entre la
// videollamada y el WebSocket del chat.
//
// Importa porque hasta ahora no existía: el otro extremo se enteraba de que le
// estaban llamando SONDEANDO cada ~25 segundos. Los dos subsistemas viven en el
// mismo servicio desde la fusión, pero publican en canales de outbox distintos, y
// nadie cruzaba el puente.
func TestCallFanoutAnunciaLaLlamadaEnSuCanal(t *testing.T) {
	t.Parallel()
	canal := uuid.New()

	evento := func(tipo string, carga map[string]any) events.Event {
		p, _ := json.Marshal(carga)
		return events.Event{Channel: MediaEventChannel, Type: tipo, Tenant: "acme", Payload: p}
	}
	deChat := map[string]any{"source_kind": "chat", "source_ref_id": canal.String()}

	casos := []struct {
		tipo  string
		frame string
	}{
		{"meet.room.created", ws.ServerCallStarted},
		{"meet.room.closed", ws.ServerCallEnded},
		{"meet.room.finished", ws.ServerCallEnded},
	}
	for _, c := range casos {
		t.Run(c.tipo, func(t *testing.T) {
			fb := &fakeBroadcaster{}
			if err := NewCallFanoutHandler(fb)(context.Background(), evento(c.tipo, deChat)); err != nil {
				t.Fatalf("handler: %v", err)
			}
			if fb.calls != 1 {
				t.Fatalf("Deliver llamado %d veces", fb.calls)
			}
			if fb.frame.Type != c.frame {
				t.Errorf("frame = %q, quería %q", fb.frame.Type, c.frame)
			}
			if fb.tenant != "acme" || fb.channel != canal {
				t.Errorf("entregado a %s/%s", fb.tenant, fb.channel)
			}
		})
	}
}

func TestCallFanoutNoAvisaAQuienNoLeToca(t *testing.T) {
	t.Parallel()
	canal := uuid.New()
	evento := func(tipo string, carga map[string]any) events.Event {
		p, _ := json.Marshal(carga)
		return events.Event{Channel: MediaEventChannel, Type: tipo, Tenant: "acme", Payload: p}
	}

	casos := []struct {
		nombre string
		ev     events.Event
	}{
		{
			// Una reunión suelta no tiene canal al que avisar. Colar su evento
			// dejaría un aviso de llamada en un canal que no la tiene.
			"reunión sin canal de origen",
			evento("meet.room.created", map[string]any{"room_id": "x"}),
		},
		{
			// La sala de un evento de Calendar se anuncia en Calendar, no aquí.
			// El ref_id es un UUID VÁLIDO a propósito: con uno inválido este caso
			// pasaría por no poder parsearlo, y dejaría de comprobar lo que dice
			// comprobar —que lo descarta el ORIGEN—. Verificado quitando el filtro
			// de `kind`: sin este detalle, el sabotaje no rompía ningún test.
			"origen de otra app",
			evento("meet.room.created", map[string]any{
				"source_kind": "calendar", "source_ref_id": uuid.New().String(),
			}),
		},
		{
			"referencia que no es un UUID",
			evento("meet.room.created", map[string]any{"source_kind": "chat", "source_ref_id": "no-es-uuid"}),
		},
		{
			"evento de la llamada que no es de inicio ni de fin",
			evento("meet.participant.joined", map[string]any{"source_kind": "chat", "source_ref_id": canal.String()}),
		},
		{
			// El canal de chat lo maneja el OTRO handler; si este también lo
			// tradujera, cada mensaje llegaría dos veces.
			"evento del canal de chat",
			events.Event{Channel: EventChannel, Type: "meet.room.created", Tenant: "acme", Payload: []byte(`{}`)},
		},
		{
			"sin tenant",
			events.Event{Channel: MediaEventChannel, Type: "meet.room.created", Payload: []byte(`{}`)},
		},
		{
			"payload corrupto",
			events.Event{Channel: MediaEventChannel, Type: "meet.room.created", Tenant: "acme", Payload: []byte(`{roto`)},
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			fb := &fakeBroadcaster{}
			if err := NewCallFanoutHandler(fb)(context.Background(), c.ev); err != nil {
				t.Fatalf("un evento que no toca no debe devolver error: %v", err)
			}
			if fb.calls != 0 {
				t.Errorf("se avisó a %s cuando no tocaba", fb.channel)
			}
		})
	}
}

// COM-08. Al salir (o ser expulsado) de un canal, el relay da de baja a esa persona en la
// sala después de avisarla; los demás eventos no expulsan a nadie.
func TestFanoutExpulsaAQuienDejaElCanal(t *testing.T) {
	t.Parallel()
	canal := uuid.New()
	evento := func(typ string) events.Event {
		payload, _ := json.Marshal(eventEnvelope{ChannelID: canal, Data: memberEvent("removed", canal, "admin", &domain.Member{Subject: "ana"})})
		return events.Event{Channel: EventChannel, Type: typ, Tenant: "acme", Payload: payload}
	}
	fb := &fakeBroadcaster{}
	h := NewFanoutHandler(fb)
	if err := h(context.Background(), evento(EventChannelMemberRemoved)); err != nil {
		t.Fatal(err)
	}
	if want := "acme/" + canal.String() + "/ana"; fb.calls != 1 || len(fb.expulsados) != 1 || fb.expulsados[0] != want {
		t.Fatalf("avisos=%d expulsados=%v; quiero 1 aviso y %q", fb.calls, fb.expulsados, want)
	}
	if err := h(context.Background(), evento(EventChannelMemberAdded)); err != nil {
		t.Fatal(err)
	}
	if len(fb.expulsados) != 1 {
		t.Fatalf("un alta no expulsa a nadie: %v", fb.expulsados)
	}
}
