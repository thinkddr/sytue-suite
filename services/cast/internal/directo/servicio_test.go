// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Lo del directo que se prueba sin Postgres ni LiveKit: los nombres (room,
// identidades), los tokens de espectador y de emisor, y que los avisos de rooms
// ajenas se ignoran sin tocar la base. El ciclo completo va en
// servicio_integration_test.go.
package directo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/livekit"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
)

func TestNombreDeRoomIdaYVuelta(t *testing.T) {
	id := uuid.New()
	for _, tenant := range []string{"acme", "mi-empresa-larga", "p-ana-acme-es"} {
		room := nombreRoom(tenant, id)
		if !strings.HasPrefix(room, "cast-"+tenant+"-") {
			t.Errorf("room = %q", room)
		}
		got, ok := roomDe(room)
		if !ok || got != tenant {
			t.Errorf("roomDe(%q) = %q, %v; quería %q", room, got, ok, tenant)
		}
	}
	// Lo que no es de Cast: las rooms de Rooms comparten el SFU y sus avisos llegan aquí.
	for _, room := range []string{"", "acme", "sala-de-rooms", "cast-", "cast-acme", "cast-acme-no-es-uuid", "cast--" + id.String(), "cast-Acme_Mal-" + id.String()} {
		if tenant, ok := roomDe(room); ok {
			t.Errorf("roomDe(%q) aceptó %q", room, tenant)
		}
	}
}

func TestIdentidades(t *testing.T) {
	if identidadEmisor("ana") != "emisor-ana" {
		t.Error(identidadEmisor("ana"))
	}
	a, b := identidadAnonima(), identidadAnonima()
	if !strings.HasPrefix(a, "anon-") || len(a) != len("anon-")+16 || a == b {
		t.Errorf("identidades anónimas: %q %q", a, b)
	}
}

func TestEnlaceDelDirecto(t *testing.T) {
	id := uuid.MustParse("6f1d2b0e-4d1c-4b2a-9c3d-0e1f2a3b4c5d")
	if got := enlaceDirecto("https://cast.sytue.eu/", "acme", id); got != "https://cast.sytue.eu/cast/acme/watch/"+id.String() {
		t.Errorf("enlace = %q", got)
	}
	if got := enlaceDirecto("no-es-url", "acme", id); got != "" {
		t.Errorf("un PUBLIC_URL inservible debería dar enlace vacío, no %q", got)
	}
}

// grants decodifica el claim video de un JWT de LiveKit sin verificarlo.
func grants(t *testing.T, tok string) (sub string, exp int64, g livekit.VideoGrant) {
	t.Helper()
	partes := strings.Split(tok, ".")
	if len(partes) != 3 {
		t.Fatalf("JWT malformado: %q", tok)
	}
	pb, err := base64.RawURLEncoding.DecodeString(partes[1])
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Sub   string             `json:"sub"`
		Exp   int64              `json:"exp"`
		Video livekit.VideoGrant `json:"video"`
	}
	if err := json.Unmarshal(pb, &c); err != nil {
		t.Fatal(err)
	}
	return c.Sub, c.Exp, c.Video
}

func servicioSinBase() *Servicio {
	return New(Deps{Firmante: livekit.NewSigner("clave", "secreto"), Cfg: Config{LiveKitURL: "wss://sfu.sytue.test"}})
}

func TestPlayDaTokenDeSoloSuscripcion(t *testing.T) {
	s := servicioSinBase()
	v := domain.Video{ID: uuid.New(), Tenant: "acme", Status: domain.StatusLive, LiveRoom: "cast-acme-x"}

	c, err := s.Play(context.Background(), v, service.Quien{Sub: "zoe", Tenant: "globex", Nombre: "Zoe"})
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	if c.ServerURL != "wss://sfu.sytue.test" || c.Room != "cast-acme-x" || c.Identity != "zoe" {
		t.Errorf("conexión: %+v", c)
	}
	sub, exp, g := grants(t, c.Token)
	if sub != "zoe" || g.Room != "cast-acme-x" || !g.RoomJoin {
		t.Errorf("claims: %q %+v", sub, g)
	}
	// LA REGLA QUE NO SE SIMPLIFICA: quien mira no publica ni manda datos.
	if g.CanSubscribe == nil || !*g.CanSubscribe || g.CanPublish == nil || *g.CanPublish || g.CanPublishData == nil || *g.CanPublishData {
		t.Errorf("un espectador con permiso de publicar: %+v", g)
	}
	if g.RoomAdmin || g.RoomRecord || g.IngressAdmin || g.RoomCreate {
		t.Errorf("un espectador con permisos de servidor: %+v", g)
	}
	if ttl := time.Until(time.Unix(exp, 0)); ttl > 2*time.Hour+time.Minute || ttl < 2*time.Hour-time.Minute {
		t.Errorf("TTL del espectador = %v, quería 2 h", ttl)
	}

	// Sin sesión: identidad anónima.
	c, err = s.Play(context.Background(), v, service.Nadie)
	if err != nil || !strings.HasPrefix(c.Identity, "anon-") {
		t.Errorf("anónimo: %+v %v", c, err)
	}
	// Un directo que no está en emisión no da token.
	v.Status = domain.StatusScheduled
	if _, err := s.Play(context.Background(), v, service.Nadie); err == nil {
		t.Error("un directo programado dio token de espectador")
	}
}

func TestTokenDeEmisorSoloPublica(t *testing.T) {
	s := servicioSinBase()
	v := domain.Video{ID: uuid.New(), Tenant: "acme", OwnerSub: "ana", OwnerName: "Ana", Status: domain.StatusScheduled, LiveRoom: "cast-acme-y"}
	c, err := s.emision(v)
	if err != nil {
		t.Fatal(err)
	}
	sub, _, g := grants(t, c.Token)
	if sub != "emisor-ana" || c.Identity != "emisor-ana" {
		t.Errorf("identidad del emisor: %q %q", sub, c.Identity)
	}
	if g.CanPublish == nil || !*g.CanPublish || g.RoomAdmin || g.RoomRecord || g.IngressAdmin {
		t.Errorf("grants del emisor: %+v", g)
	}
	v.LiveRoom = ""
	if _, err := s.emision(v); err == nil {
		t.Error("sin room no debería haber token")
	}
}

// Un aviso de una room que no es de Cast se ignora ANTES de tocar la base: el
// servicio de aquí no tiene pool, así que si lo tocara, fallaría.
func TestAvisosDeRoomsAjenasSeIgnoran(t *testing.T) {
	s := servicioSinBase()
	casos := []livekit.WebhookEvent{
		{Event: livekit.EventParticipantJoined, Room: &livekit.WebhookRoom{Name: "sala-de-rooms"}, Participant: &livekit.WebhookPart{Identity: "u1"}},
		{Event: livekit.EventRoomFinished, Room: &livekit.WebhookRoom{Name: "reunion-x"}},
		{Event: livekit.EventTrackPublished, Room: &livekit.WebhookRoom{Name: "cast-acme-" + uuid.NewString()}, Participant: &livekit.WebhookPart{Identity: "zoe"}},
		{Event: livekit.EventEgressEnded, EgressInfo: &livekit.WebhookEgress{EgressID: "EG_1", RoomName: "reunion-x"}},
		{Event: livekit.EventParticipantJoined, Room: &livekit.WebhookRoom{Name: "cast-acme-" + uuid.NewString()}, Participant: &livekit.WebhookPart{Identity: "EG_1", Kind: livekit.ParticipanteEgress}},
		{Event: "room_started", Room: &livekit.WebhookRoom{Name: "cast-acme-" + uuid.NewString()}},
		{Event: livekit.EventIngressStarted},
	}
	for _, ev := range casos {
		if err := s.aplicar(context.Background(), ev); err != nil {
			t.Errorf("%s en %v: %v", ev.Event, ev.Room, err)
		}
	}
}

func TestWebhookMalFirmado(t *testing.T) {
	s := servicioSinBase()
	err := s.Webhook(context.Background(), "", []byte(`{"event":"room_finished"}`))
	if err == nil || !strings.Contains(err.Error(), ErrWebhookInvalido.Error()) {
		t.Fatalf("sin firma debería ser ErrWebhookInvalido: %v", err)
	}
	otro := livekit.NewSigner("clave", "otro-secreto")
	tok, _ := otro.SignWebhook([]byte(`{"event":"room_finished"}`), time.Minute)
	if err := s.Webhook(context.Background(), tok, []byte(`{"event":"room_finished"}`)); err == nil {
		t.Fatal("una firma con otro secreto se aceptó")
	}
}
