// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package livekit

import (
	"testing"
	"time"
)

// SignWebhook es la contraparte de ParseWebhook: lo que firma uno lo acepta el
// otro, y solo con el mismo secreto y el mismo cuerpo.
func TestSignWebhookCierraElCirculo(t *testing.T) {
	s := NewSigner(testKey, testSecret)
	body := []byte(`{"event":"ingress_started","ingressInfo":{"ingressId":"IN_1"}}`)
	tok, err := s.SignWebhook(body, 0)
	if err != nil {
		t.Fatalf("SignWebhook: %v", err)
	}
	ev, err := s.ParseWebhook(tok, body)
	if err != nil || ev.Event != EventIngressStarted {
		t.Fatalf("lo firmado no se acepta: %v %+v", err, ev)
	}
	if _, err := s.ParseWebhook(tok, []byte(`{"event":"ingress_ended"}`)); err == nil {
		t.Error("otro cuerpo con la misma firma se aceptó")
	}
	if _, err := NewSigner(testKey, "otro").ParseWebhook(tok, body); err == nil {
		t.Error("otro secreto aceptó la firma")
	}
}

// Los eventos que Sytue Cast consume del webhook y que Rooms no miraba: la
// ingesta que empieza y acaba, la pista publicada, la grabación troceada y la
// clase del participante (quien mira, quien emite, quien graba). Los cuerpos
// llevan los nombres camelCase y los enteros entre comillas de protojson, que
// es como los manda LiveKit de verdad.
func TestParseWebhookIngresoYSegmentos(t *testing.T) {
	s := NewSigner(testKey, testSecret)
	firmar := func(body string) WebhookEvent {
		t.Helper()
		ev, err := s.ParseWebhook(signWebhook(t, testSecret, []byte(body), time.Now().Add(time.Minute).Unix()), []byte(body))
		if err != nil {
			t.Fatalf("ParseWebhook: %v", err)
		}
		return ev
	}

	ev := firmar(`{"event":"ingress_started","id":"EV_1","createdAt":"1785238899",` +
		`"ingressInfo":{"ingressId":"IN_1","name":"acme/ana","roomName":"cast-acme-9f","participantIdentity":"emisor-ana",` +
		`"participantName":"Ana","state":{"status":"ENDPOINT_PUBLISHING","roomId":"RM_1","startedAt":"1785238890"}}}`)
	if ev.Event != EventIngressStarted || ev.IngressInfo == nil {
		t.Fatalf("evento de ingreso no decodificado: %+v", ev)
	}
	if ev.IngressInfo.IngressID != "IN_1" || ev.IngressInfo.RoomName != "cast-acme-9f" || ev.IngressInfo.ParticipantIdentity != "emisor-ana" {
		t.Errorf("ingressInfo: %+v", ev.IngressInfo)
	}
	if ev.IngressInfo.State == nil || ev.IngressInfo.State.Status != IngressPublicando || ev.IngressInfo.State.StartedAt != 1785238890 {
		t.Errorf("estado del ingreso: %+v", ev.IngressInfo.State)
	}

	ev = firmar(`{"event":"track_published","room":{"name":"cast-acme-9f"},` +
		`"participant":{"identity":"emisor-ana","kind":"INGRESS"},"track":{"sid":"TR_1","type":"VIDEO","source":"CAMERA"}}`)
	if ev.Event != EventTrackPublished || ev.Track == nil || ev.Track.Type != "VIDEO" {
		t.Errorf("pista no decodificada: %+v", ev)
	}
	if ev.Participant == nil || ev.Participant.Espectador() {
		t.Error("el ingress cuenta como espectador y no debería")
	}

	ev = firmar(`{"event":"participant_joined","room":{"name":"cast-acme-9f"},"participant":{"identity":"zoe","kind":"STANDARD"}}`)
	if !ev.Participant.Espectador() {
		t.Error("una persona STANDARD es espectadora")
	}
	// Un servidor viejo no manda `kind`: quien mira sigue contando.
	ev = firmar(`{"event":"participant_joined","room":{"name":"cast-acme-9f"},"participant":{"identity":"zoe"}}`)
	if !ev.Participant.Espectador() {
		t.Error("sin kind se lee como STANDARD")
	}
	ev = firmar(`{"event":"participant_left","room":{"name":"cast-acme-9f"},"participant":{"identity":"EG_1","kind":"EGRESS"}}`)
	if ev.Participant.Espectador() {
		t.Error("el egress cuenta como espectador y no debería")
	}

	ev = firmar(`{"event":"egress_ended","egressInfo":{"egressId":"EG_1","roomName":"cast-acme-9f","status":"EGRESS_COMPLETE",` +
		`"segmentResults":[{"playlistName":"tenants/acme/v1/live/index.m3u8","livePlaylistName":"tenants/acme/v1/live/live.m3u8",` +
		`"playlistLocation":"https://s3/cast/tenants/acme/v1/live/index.m3u8","duration":"125000000000","size":"48000000","segmentCount":"32"}]}}`)
	if ev.Event != EventEgressEnded || ev.EgressInfo == nil || len(ev.EgressInfo.SegmentResults) != 1 {
		t.Fatalf("segmentos no decodificados: %+v", ev)
	}
	seg := ev.EgressInfo.SegmentResults[0]
	if seg.PlaylistName != "tenants/acme/v1/live/index.m3u8" || seg.Duration != 125_000_000_000 || seg.Size != 48_000_000 || seg.SegmentCount != 32 {
		t.Errorf("segmentos: %+v", seg)
	}
}
