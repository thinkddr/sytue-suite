// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/livekit"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/store"
)

var errBoom = errors.New("boom")

// fakeStore implementa dataStore en memoria.
type fakeStore struct {
	filaBorrada bool
	room        domain.Room
	rooms       []domain.Room // lo que devuelve ListRooms
	// Sala de espera: la cola, quién está admitido y la última respuesta dada.
	knocks       []domain.Admission
	admitidos    map[string]bool
	admitidoDe   string
	admitidoOK   bool
	roomErr      error
	roomByName   domain.Room
	roomByNameEr error
	findRoom     domain.Room
	findFound    bool
	findErr      error
	newRoom      *store.NewRoomRecord
	closeRoom    domain.Room
	closeErr     error
	startErr     error
	endErr       error
	joinInput    *store.RecordJoinInput
	joinErr      error
	leaveErr     error
	upsertKey    string
	upsertStat   domain.RecordingStatus
	upsertErr    error
	// Grabaciones que devuelve ListRecordings (para simular «ya se está grabando»).
	grabaciones []domain.Recording
	// Quién ha estado dentro (ListParticipants).
	participantes []domain.Participant
}

func (f *fakeStore) CreateRoom(_ context.Context, rec store.NewRoomRecord) (domain.Room, error) {
	f.newRoom = &rec
	return f.room, f.roomErr
}

func (f *fakeStore) FindRoomBySource(context.Context, domain.SourceKind, string) (domain.Room, bool, error) {
	return f.findRoom, f.findFound, f.findErr
}

func (f *fakeStore) GetRoom(context.Context, uuid.UUID) (domain.Room, error) {
	return f.room, f.roomErr
}

func (f *fakeStore) GetRoomByName(context.Context, string) (domain.Room, error) {
	return f.roomByName, f.roomByNameEr
}

func (f *fakeStore) ListRooms(context.Context, domain.RoomStatus, int, string) ([]domain.Room, error) {
	return f.rooms, f.roomErr
}

func (f *fakeStore) ListParticipants(context.Context, uuid.UUID, bool) ([]domain.Participant, error) {
	return f.participantes, f.roomErr
}

func (f *fakeStore) ListRecordings(context.Context, uuid.UUID) ([]domain.Recording, error) {
	return f.grabaciones, f.roomErr
}

func (f *fakeStore) DeleteRecording(context.Context, uuid.UUID, uuid.UUID) error {
	f.filaBorrada = true
	return nil
}

func (f *fakeStore) CloseRoom(context.Context, uuid.UUID) (domain.Room, error) {
	return f.closeRoom, f.closeErr
}

func (f *fakeStore) Knock(_ context.Context, roomID uuid.UUID, identity, name string) (domain.Admission, error) {
	f.knocks = append(f.knocks, domain.Admission{RoomID: roomID, Identity: identity, Name: name})
	return f.knocks[len(f.knocks)-1], nil
}

func (f *fakeStore) SetAdmission(_ context.Context, _ uuid.UUID, identity string, admitido bool) error {
	f.admitidoDe, f.admitidoOK = identity, admitido
	return nil
}

func (f *fakeStore) IsAdmitted(_ context.Context, _ uuid.UUID, identity string) (bool, error) {
	return f.admitidos[identity], nil
}

func (f *fakeStore) ListKnocks(_ context.Context, _ uuid.UUID) ([]domain.Admission, error) {
	return f.knocks, nil
}

func (f *fakeStore) SetRoomLocked(_ context.Context, _ uuid.UUID, locked bool) (domain.Room, error) {
	f.room.Locked = locked
	return f.room, nil
}

func (f *fakeStore) MarkRoomStarted(context.Context, string, string, time.Time) error {
	return f.startErr
}
func (f *fakeStore) MarkRoomEnded(context.Context, string, time.Time) error { return f.endErr }
func (f *fakeStore) RecordJoin(_ context.Context, in store.RecordJoinInput) error {
	f.joinInput = &in
	return f.joinErr
}

func (f *fakeStore) RecordLeave(context.Context, string, string, time.Time) error {
	return f.leaveErr
}

func (f *fakeStore) UpsertRecording(_ context.Context, _, _ string, status domain.RecordingStatus, storageKey string, _, _ int64, _ *time.Time) error {
	f.upsertKey = storageKey
	f.upsertStat = status
	return f.upsertErr
}

var _ dataStore = (*fakeStore)(nil)

// fakeLK implementa roomProvisioner capturando llamadas.
type fakeLK struct {
	createErr error
	deleted   []string
	deleteErr error
	removed   []string
	removeErr error
	// Participante que devuelve GetParticipant y las pistas que se silenciaron.
	participante livekit.ParticipantInfo
	// Grabación: la ruta con la que se pidió, las paradas, y los fallos a simular.
	rutaGrabacion string
	paradas       []string
	egressErr     error
	pararErr      error
	getErr        error
	muteErr       error
	silenciadas   []string
	// Palabra retirada/devuelta: a quién y con qué valor.
	palabraDe string
	palabraOK bool
	datosOK   bool
}

func (l *fakeLK) SetCanPublish(_ context.Context, _, identity string, canPublish bool) error {
	l.palabraDe, l.palabraOK = identity, canPublish
	return nil
}

func (l *fakeLK) CreateRoom(context.Context, livekit.CreateRoomRequest) (livekit.Room, error) {
	return livekit.Room{}, l.createErr
}

func (l *fakeLK) DeleteRoom(_ context.Context, name string) error {
	l.deleted = append(l.deleted, name)
	return l.deleteErr
}

func (l *fakeLK) RemoveParticipant(_ context.Context, _, identity string) error {
	l.removed = append(l.removed, identity)
	return l.removeErr
}

func (l *fakeLK) GetParticipant(_ context.Context, _, _ string) (livekit.ParticipantInfo, error) {
	return l.participante, l.getErr
}

func (l *fakeLK) MutePublishedTrack(_ context.Context, _, _, trackSID string, _ bool) error {
	if l.muteErr != nil {
		return l.muteErr
	}
	l.silenciadas = append(l.silenciadas, trackSID)
	return nil
}

func (l *fakeLK) StartRoomComposite(_ context.Context, _, _, ruta string, _ livekit.DestinoS3) (livekit.EgressInfo, error) {
	if l.egressErr != nil {
		return livekit.EgressInfo{}, l.egressErr
	}
	l.rutaGrabacion = ruta
	return livekit.EgressInfo{EgressID: "EG_TEST", Status: "EGRESS_STARTING"}, nil
}

func (l *fakeLK) StopEgress(_ context.Context, egressID string) (livekit.EgressInfo, error) {
	l.paradas = append(l.paradas, egressID)
	return livekit.EgressInfo{EgressID: egressID, Status: "EGRESS_ENDING"}, l.pararErr
}

var _ roomProvisioner = (*fakeLK)(nil)

type capturePub struct{ types []string }

func (c *capturePub) Publish(_ context.Context, ev events.Event) error {
	c.types = append(c.types, ev.Type)
	return nil
}

func newSvc(fs *fakeStore, lk roomProvisioner, pub EventPublisher) *Service {
	return &Service{
		cfg: config.Config{
			LiveKitURL:       "wss://sfu.sytue.eu",
			AccessTokenTTL:   time.Hour,
			RoomEmptyTimeout: 5 * time.Minute,
			MaxParticipants:  50,
		},
		store:  fs,
		lk:     lk,
		signer: livekit.NewSigner("apikey", "apisecretapisecretapisecret0123456789"),
		pub:    pub,
	}
}

func tenantCtx(t *testing.T) context.Context {
	t.Helper()
	tn, err := tenancy.New("acme")
	if err != nil {
		t.Fatal(err)
	}
	return tenancy.Into(context.Background(), tn)
}

func TestCreateRoomValidation(t *testing.T) {
	s := newSvc(&fakeStore{}, &fakeLK{}, nil)
	ctx := tenantCtx(t)
	if _, err := s.CreateRoom(ctx, CreateRoomInput{Title: ""}); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("título vacío: %v", err)
	}
	// Nombre explícito inválido.
	if _, err := s.CreateRoom(ctx, CreateRoomInput{Title: "Daily", Name: "Bad Name!"}); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("nombre inválido: %v", err)
	}
	// Origen cross-app sin RefID es inválido.
	if _, err := s.CreateRoom(ctx, CreateRoomInput{Title: "Daily", Source: domain.RoomSource{Kind: domain.SourceChat}}); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("cross-app sin refID: %v", err)
	}
}

func TestCreateRoomHappyPath(t *testing.T) {
	fs := &fakeStore{room: domain.Room{ID: uuid.New(), Name: "room-x", Title: "Daily"}}
	lk := &fakeLK{}
	pub := &capturePub{}
	s := newSvc(fs, lk, pub)
	room, err := s.CreateRoom(tenantCtx(t), CreateRoomInput{Title: "Daily", CreatedBy: "alice"})
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if room.ID == uuid.Nil {
		t.Fatal("room vacía")
	}
	// Genera nombre y fija el tenant en la metadata.
	if fs.newRoom.Name == "" || !domain.ValidRoomName(fs.newRoom.Name) {
		t.Fatalf("nombre generado inválido: %q", fs.newRoom.Name)
	}
	if fs.newRoom.Metadata["sytue_tenant"] != "acme" {
		t.Errorf("tenant no embebido en metadata: %v", fs.newRoom.Metadata)
	}
	// Acota MaxParticipants al límite de config.
	if fs.newRoom.MaxParticipants != 50 {
		t.Errorf("MaxParticipants acotado, got %d", fs.newRoom.MaxParticipants)
	}
	if len(pub.types) != 1 || pub.types[0] != "meet.room.created" {
		t.Fatalf("esperaba room.created, got %v", pub.types)
	}
}

func TestCreateRoomLiveKitFailureNonFatal(t *testing.T) {
	fs := &fakeStore{room: domain.Room{ID: uuid.New(), Name: "room-x"}}
	lk := &fakeLK{createErr: errBoom}
	s := newSvc(fs, lk, &capturePub{})
	// Aunque LiveKit falle, la sala se crea (best-effort).
	if _, err := s.CreateRoom(tenantCtx(t), CreateRoomInput{Title: "Daily"}); err != nil {
		t.Fatalf("fallo de LiveKit no debe romper CreateRoom: %v", err)
	}
}

func TestCreateRoomStoreError(t *testing.T) {
	fs := &fakeStore{roomErr: errBoom}
	s := newSvc(fs, &fakeLK{}, nil)
	if _, err := s.CreateRoom(tenantCtx(t), CreateRoomInput{Title: "Daily"}); !errors.Is(err, errBoom) {
		t.Fatalf("error del store debe propagarse, got %v", err)
	}
}

func TestCreateRoomFromSource(t *testing.T) {
	fs := &fakeStore{room: domain.Room{ID: uuid.New(), Name: "room-y"}}
	s := newSvc(fs, &fakeLK{}, &capturePub{})
	// Verja de origen: crear la sala de un canal exige poder leerlo (ver
	// requireChannelSource). Aquí se ejercita la forma, así que la sonda concede.
	s.SetMessagingNotifier(&fakeMessagingNotifier{allow: true})
	if _, err := s.CreateRoomFromSource(tenantCtx(t), domain.SourceChat, "channel-123", "Standup", "bob", 10); err != nil {
		t.Fatalf("CreateRoomFromSource: %v", err)
	}
	// El origen cross-app se refleja en la metadata.
	if fs.newRoom.Metadata["source"] == nil {
		t.Errorf("metadata sin origen: %v", fs.newRoom.Metadata)
	}
	// Kind no cross-app se rechaza.
	if _, err := s.CreateRoomFromSource(tenantCtx(t), domain.SourceDirect, "", "x", "bob", 1); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("kind directo no es cross-app: %v", err)
	}
}

// TestCreateRoomFromSourceIdempotent: si ya existe una sala viva enlazada al
// (kind, ref_id), from-source la devuelve y NO crea una duplicada.
func TestCreateRoomFromSourceIdempotent(t *testing.T) {
	existing := domain.Room{ID: uuid.New(), Name: "room-existing"}
	fs := &fakeStore{findRoom: existing, findFound: true}
	s := newSvc(fs, &fakeLK{}, &capturePub{})
	// Crear la sala de un CANAL exige poder leerlo (verja de origen: crear la sala del
	// canal de otros daba HOST al creador). Este test ejercita la idempotencia, no la
	// autorización, así que la sonda concede.
	s.SetMessagingNotifier(&fakeMessagingNotifier{allow: true})
	got, err := s.CreateRoomFromSource(tenantCtx(t), domain.SourceChat, "channel-123", "Standup", "bob", 10)
	if err != nil {
		t.Fatalf("CreateRoomFromSource idempotente: %v", err)
	}
	if got.ID != existing.ID {
		t.Fatalf("esperaba la sala existente %s, got %s", existing.ID, got.ID)
	}
	if fs.newRoom != nil {
		t.Fatal("no debe crear una sala nueva si ya existe una enlazada al origen")
	}
}

// RoomBySource devuelve la sala viva del origen SIN crearla; 404 si no hay; y
// rechaza un kind no cross-app.
func TestRoomBySource(t *testing.T) {
	existing := domain.Room{
		ID: uuid.New(), Name: "room-chat",
		Metadata: map[string]any{"source": map[string]any{"kind": "chat", "ref_id": "chan-1"}},
	}
	fs := &fakeStore{findRoom: existing, findFound: true}
	s := newSvc(fs, &fakeLK{}, &capturePub{})
	s.SetMessagingNotifier(&fakeMessagingNotifier{allow: true})

	got, err := s.RoomBySource(sesion(t, "bob"), domain.SourceChat, "chan-1")
	if err != nil {
		t.Fatalf("RoomBySource: %v", err)
	}
	if got.ID != existing.ID {
		t.Fatalf("esperaba %s, got %s", existing.ID, got.ID)
	}
	if fs.newRoom != nil {
		t.Fatal("RoomBySource no debe crear ninguna sala")
	}

	// Sin sala viva -> ErrNotFound.
	fsNone := &fakeStore{findFound: false}
	if _, err := newSvc(fsNone, &fakeLK{}, &capturePub{}).RoomBySource(tenantCtx(t), domain.SourceChat, "chan-2"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("sin sala debe ser ErrNotFound, got %v", err)
	}

	// kind no cross-app -> ErrInvalidName.
	if _, err := s.RoomBySource(tenantCtx(t), domain.SourceDirect, "x"); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("kind directo: %v", err)
	}
}

func TestCloseRoom(t *testing.T) {
	fs := &fakeStore{closeRoom: domain.Room{ID: uuid.New(), Name: "room-z"}}
	lk := &fakeLK{}
	pub := &capturePub{}
	s := newSvc(fs, lk, pub)
	if _, err := s.CloseRoom(tenantCtx(t), uuid.New()); err != nil {
		t.Fatalf("CloseRoom: %v", err)
	}
	if len(lk.deleted) != 1 || lk.deleted[0] != "room-z" {
		t.Fatalf("no se cerró la room en LiveKit: %v", lk.deleted)
	}
	if len(pub.types) != 1 || pub.types[0] != "meet.room.closed" {
		t.Fatalf("esperaba room.closed, got %v", pub.types)
	}
}

func TestMuteParticipantSoloTocaElAudio(t *testing.T) {
	// Silenciar la CÁMARA de alguien desde fuera no resuelve ningún problema de la
	// reunión y sorprende a quien la tiene puesta; para eso está expulsar. Así que
	// de las pistas publicadas solo se tocan las de audio.
	lk := &fakeLK{participante: livekit.ParticipantInfo{
		Identity: "ruidoso",
		Tracks: []livekit.TrackInfo{
			{SID: "audio-1", Type: "AUDIO"},
			{SID: "video-1", Type: "VIDEO"},
			{SID: "audio-2", Type: "AUDIO", Muted: true}, // ya callada: no se repite
			{SID: "audio-3", Type: "AUDIO"},
		},
	}}
	s := newSvc(&fakeStore{}, lk, nil)

	if err := s.MuteParticipant(tenantCtx(t), uuid.New(), "ruidoso"); err != nil {
		t.Fatalf("MuteParticipant: %v", err)
	}
	if len(lk.silenciadas) != 2 {
		t.Fatalf("silenciadas = %v; solo las dos de audio que sonaban", lk.silenciadas)
	}
	for _, sid := range lk.silenciadas {
		if sid == "video-1" {
			t.Error("se silenció la cámara: silenciar es del micrófono, para lo demás está expulsar")
		}
		if sid == "audio-2" {
			t.Error("se volvió a silenciar una pista que ya estaba callada")
		}
	}
}

func TestMuteParticipantSinNadaQueSilenciarNoEsError(t *testing.T) {
	// Quien ya estaba callado sigue callado. Devolver un fallo aquí haría que la
	// interfaz mintiera: diría que no se pudo silenciar a alguien que no suena.
	lk := &fakeLK{participante: livekit.ParticipantInfo{Identity: "mudo"}}
	s := newSvc(&fakeStore{}, lk, nil)
	if err := s.MuteParticipant(tenantCtx(t), uuid.New(), "mudo"); err != nil {
		t.Errorf("sin pistas de audio no debería fallar: %v", err)
	}
	if len(lk.silenciadas) != 0 {
		t.Errorf("no había nada que silenciar y silenció %v", lk.silenciadas)
	}
}

func TestRemoveParticipant(t *testing.T) {
	fs := &fakeStore{room: domain.Room{ID: uuid.New(), Name: "room-a"}}
	lk := &fakeLK{}
	pub := &capturePub{}
	s := newSvc(fs, lk, pub)
	if err := s.RemoveParticipant(tenantCtx(t), uuid.New(), "user-1"); err != nil {
		t.Fatalf("RemoveParticipant: %v", err)
	}
	if len(lk.removed) != 1 || lk.removed[0] != "user-1" {
		t.Fatalf("participante no expulsado: %v", lk.removed)
	}
	if len(pub.types) != 1 || pub.types[0] != "meet.participant.removed" {
		t.Fatalf("esperaba participant.removed, got %v", pub.types)
	}
	// Error de LiveKit se propaga.
	lk.removeErr = errBoom
	if err := s.RemoveParticipant(tenantCtx(t), uuid.New(), "user-2"); !errors.Is(err, errBoom) {
		t.Fatalf("error de LiveKit debe propagarse, got %v", err)
	}
}

func TestJoinRoomMintsToken(t *testing.T) {
	fs := &fakeStore{room: domain.Room{ID: uuid.New(), Name: "room-b", CreatedBy: "alice", Status: domain.RoomActive}}
	pub := &capturePub{}
	s := newSvc(fs, &fakeLK{}, pub)
	res, err := s.JoinRoom(tenantCtx(t), uuid.New(), "alice", "Alice", domain.RoleViewer, false)
	if err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	// El creador se convierte en host aunque pida viewer.
	if res.Role != domain.RoleHost {
		t.Errorf("creador debe ser host, got %s", res.Role)
	}
	if res.Token == "" || res.LiveKitURL != "wss://sfu.sytue.eu" {
		t.Fatalf("resultado incompleto: %+v", res)
	}
	if len(pub.types) != 1 || pub.types[0] != "meet.participant.joining" {
		t.Fatalf("esperaba participant.joining, got %v", pub.types)
	}
}

func TestJoinRoomClosed(t *testing.T) {
	fs := &fakeStore{room: domain.Room{ID: uuid.New(), Status: domain.RoomEnded, CreatedBy: "x"}}
	s := newSvc(fs, &fakeLK{}, nil)
	if _, err := s.JoinRoom(tenantCtx(t), uuid.New(), "x", "X", domain.RoleViewer, false); !errors.Is(err, domain.ErrRoomClosed) {
		t.Fatalf("sala cerrada debe rechazar join, got %v", err)
	}
}

func TestJoinRoomNotFound(t *testing.T) {
	fs := &fakeStore{roomErr: errBoom}
	s := newSvc(fs, &fakeLK{}, nil)
	if _, err := s.JoinRoom(tenantCtx(t), uuid.New(), "x", "X", domain.RoleViewer, false); !errors.Is(err, errBoom) {
		t.Fatalf("error de GetRoom debe propagarse, got %v", err)
	}
}

func TestHandleWebhookEventTenantFromMetadata(t *testing.T) {
	fs := &fakeStore{}
	pub := &capturePub{}
	s := newSvc(fs, &fakeLK{}, pub)
	ev := livekit.WebhookEvent{
		Event: livekit.EventRoomStarted,
		Room:  &livekit.WebhookRoom{Name: "room-c", SID: "SID1", Metadata: `{"sytue_tenant":"acme"}`},
	}
	if err := s.HandleWebhookEvent(context.Background(), ev); err != nil {
		t.Fatalf("HandleWebhookEvent: %v", err)
	}
	if len(pub.types) != 1 || pub.types[0] != "meet.room.started" {
		t.Fatalf("esperaba room.started, got %v", pub.types)
	}
}

func TestHandleWebhookEventNoTenantIgnored(t *testing.T) {
	fs := &fakeStore{} // ni metadata ni sala conocida con ese nombre
	s := newSvc(fs, &fakeLK{}, &capturePub{})
	// Sin tenant identificable, se ignora sin error.
	ev := livekit.WebhookEvent{Event: livekit.EventRoomStarted, Room: &livekit.WebhookRoom{Name: "x"}}
	if err := s.HandleWebhookEvent(context.Background(), ev); err != nil {
		t.Fatalf("evento sin tenant no debe fallar: %v", err)
	}
}

// Una room de LiveKit puede NO llevar nuestra metadata: si `CreateRoom` no llegó, o si
// la sala caducó por empty_timeout y LiveKit la recreó sola al volver a entrar alguien.
// El `room_finished` de esa sala se descartaba con un WARN y un 200 — la sala quedaba
// «activa» para siempre y el canal nunca recibía su tarjeta. El nombre siempre viaja.
func TestHandleWebhookEventResuelveTenantPorNombreSinMetadata(t *testing.T) {
	fs := &fakeStore{}
	pub := &capturePub{}
	s := newSvc(fs, &fakeLK{}, pub)

	ev := livekit.WebhookEvent{Event: livekit.EventRoomFinished, Room: &livekit.WebhookRoom{Name: "acme_sinmetadata"}}
	if err := s.HandleWebhookEvent(context.Background(), ev); err != nil {
		t.Fatalf("HandleWebhookEvent: %v", err)
	}
	if len(pub.types) != 1 || pub.types[0] != "meet.room.finished" {
		t.Fatalf("el evento debe procesarse, publicó %v", pub.types)
	}
}

func TestHandleWebhookEventEgressResolvesTenant(t *testing.T) {
	fs := &fakeStore{}
	pub := &capturePub{}
	s := newSvc(fs, &fakeLK{}, pub)
	ev := livekit.WebhookEvent{
		Event:      livekit.EventEgressEnded,
		EgressInfo: &livekit.WebhookEgress{EgressID: "eg1", RoomName: "acme_d", Status: "EGRESS_COMPLETE", File: &livekit.WebhookEgressFile{Filename: "rec.mp4", Size: 100}},
	}
	if err := s.HandleWebhookEvent(context.Background(), ev); err != nil {
		t.Fatalf("HandleWebhookEvent egress: %v", err)
	}
	if fs.upsertKey != "rec.mp4" || fs.upsertStat != domain.RecordingCompleted {
		t.Fatalf("recording no upserteada: key=%q status=%q", fs.upsertKey, fs.upsertStat)
	}
}

func TestHandleWebhookLifecycle(t *testing.T) {
	fs := &fakeStore{}
	pub := &capturePub{}
	s := newSvc(fs, &fakeLK{}, pub)
	ctx := tenantCtx(t)

	cases := []struct {
		name string
		ev   livekit.WebhookEvent
		want string
	}{
		{"finished", livekit.WebhookEvent{Event: livekit.EventRoomFinished, Room: &livekit.WebhookRoom{Name: "r"}}, "meet.room.finished"},
		{"joined", livekit.WebhookEvent{Event: livekit.EventParticipantJoined, Room: &livekit.WebhookRoom{Name: "r"}, Participant: &livekit.WebhookPart{Identity: "u", SID: "s"}}, "meet.participant.joined"},
		{"left", livekit.WebhookEvent{Event: livekit.EventParticipantLeft, Room: &livekit.WebhookRoom{Name: "r"}, Participant: &livekit.WebhookPart{Identity: "u", SID: "s"}}, "meet.participant.left"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pub.types = nil
			if err := s.HandleWebhook(ctx, c.ev); err != nil {
				t.Fatalf("HandleWebhook: %v", err)
			}
			if len(pub.types) != 1 || pub.types[0] != c.want {
				t.Fatalf("esperaba %s, got %v", c.want, pub.types)
			}
		})
	}

	// Eventos sin room/participant se ignoran sin error ni evento.
	pub.types = nil
	if err := s.HandleWebhook(ctx, livekit.WebhookEvent{Event: livekit.EventRoomStarted}); err != nil {
		t.Fatalf("evento incompleto no debe fallar: %v", err)
	}
	if len(pub.types) != 0 {
		t.Fatal("evento incompleto no debe publicar")
	}
}

func TestHandleWebhookStoreError(t *testing.T) {
	fs := &fakeStore{startErr: errBoom}
	s := newSvc(fs, &fakeLK{}, &capturePub{})
	ev := livekit.WebhookEvent{Event: livekit.EventRoomStarted, Room: &livekit.WebhookRoom{Name: "r"}}
	if err := s.HandleWebhook(tenantCtx(t), ev); !errors.Is(err, errBoom) {
		t.Fatalf("error del store debe propagarse, got %v", err)
	}
}

// fakeMessagingNotifier captura la última tarjeta «llamada finalizada» que se envía a Chat.
type fakeMessagingNotifier struct {
	calls []CallEndedInput
	err   error
	// allow es la respuesta de la sonda de acceso a canal; accessErr simula un fallo
	// de red (que debe DENEGAR: fail-closed).
	allow     bool
	accessErr error
	accessLog []string
}

func (f *fakeMessagingNotifier) CallEnded(_ context.Context, in CallEndedInput) error {
	f.calls = append(f.calls, in)
	return f.err
}

func (f *fakeMessagingNotifier) ChannelAccess(_ context.Context, tenant, channelID, subject string) (bool, error) {
	f.accessLog = append(f.accessLog, tenant+"/"+channelID+"/"+subject)
	if f.accessErr != nil {
		return false, f.accessErr
	}
	return f.allow, nil
}

// TestRoomFinishedNotifiesChat: al finalizar una sala ligada a un canal de chat (aún no
// 'ended'), Meet avisa a Chat con el canal, el autor y la duración (ended - started).
func TestRoomFinishedNotifiesChat(t *testing.T) {
	started := time.Unix(1_700_000_000, 0)
	fs := &fakeStore{roomByName: domain.Room{
		Name:      "r",
		Status:    domain.RoomActive,
		CreatedBy: "alice",
		StartedAt: &started,
		Metadata:  map[string]any{"source": map[string]any{"kind": "chat", "ref_id": "chan-42"}},
	}}
	notifier := &fakeMessagingNotifier{}
	s := newSvc(fs, &fakeLK{}, &capturePub{})
	s.SetMessagingNotifier(notifier)

	ended := started.Add(90 * time.Second)
	ev := livekit.WebhookEvent{Event: livekit.EventRoomFinished, Room: &livekit.WebhookRoom{Name: "r"}, CreatedAt: livekit.Int64(ended.Unix())}
	if err := s.HandleWebhook(tenantCtx(t), ev); err != nil {
		t.Fatalf("HandleWebhook: %v", err)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("esperaba 1 aviso a chat, got %d", len(notifier.calls))
	}
	got := notifier.calls[0]
	if got.ChannelID != "chan-42" || got.AuthorSub != "alice" || got.Tenant != "acme" {
		t.Fatalf("aviso incorrecto: %+v", got)
	}
	if got.DurationSec != 90 {
		t.Fatalf("duración esperada 90s, got %d", got.DurationSec)
	}
}

// TestRoomFinishedNoNotifyWhenNotChatOrEnded: no se avisa a Chat si la sala no tiene
// origen chat, o si ya estaba 'ended' (reentrega del webhook — dedupe).
func TestRoomFinishedNoNotifyWhenNotChatOrEnded(t *testing.T) {
	notifier := &fakeMessagingNotifier{}
	ev := livekit.WebhookEvent{Event: livekit.EventRoomFinished, Room: &livekit.WebhookRoom{Name: "r"}, CreatedAt: 1_700_000_100}

	// Origen no-chat (calendar): sin aviso.
	fs := &fakeStore{roomByName: domain.Room{
		Name: "r", Status: domain.RoomActive,
		Metadata: map[string]any{"source": map[string]any{"kind": "calendar", "ref_id": "ev-1"}},
	}}
	s := newSvc(fs, &fakeLK{}, &capturePub{})
	s.SetMessagingNotifier(notifier)
	if err := s.HandleWebhook(tenantCtx(t), ev); err != nil {
		t.Fatalf("HandleWebhook (calendar): %v", err)
	}

	// Sala ya finalizada (reentrega): sin aviso, aunque el origen sea chat.
	fs.roomByName = domain.Room{
		Name: "r", Status: domain.RoomEnded, CreatedBy: "alice",
		Metadata: map[string]any{"source": map[string]any{"kind": "chat", "ref_id": "chan-42"}},
	}
	if err := s.HandleWebhook(tenantCtx(t), ev); err != nil {
		t.Fatalf("HandleWebhook (ya finalizada): %v", err)
	}
	if len(notifier.calls) != 0 {
		t.Fatalf("no debía avisar a chat, got %d", len(notifier.calls))
	}
}

func TestFirstFileAndU32(t *testing.T) {
	// firstFile prefiere File sobre FileResults.
	eg := &livekit.WebhookEgress{File: &livekit.WebhookEgressFile{Filename: "a"}}
	if f := firstFile(eg); f == nil || f.Filename != "a" {
		t.Fatalf("firstFile File: %+v", f)
	}
	eg = &livekit.WebhookEgress{FileResults: []livekit.WebhookEgressFile{{Filename: "b"}}}
	if f := firstFile(eg); f == nil || f.Filename != "b" {
		t.Fatalf("firstFile FileResults: %+v", f)
	}
	if f := firstFile(&livekit.WebhookEgress{}); f != nil {
		t.Fatalf("firstFile vacío debe ser nil, got %+v", f)
	}
	if u32(-1) != 0 {
		t.Error("u32 negativo debe ser 0")
	}
	if u32(42) != 42 {
		t.Error("u32 identidad")
	}
}

// TestGrabarSinDestinoNoArranca: sin bucket configurado se corta ANTES de tocar
// el SFU.
//
// Si se dejara pasar, el egress se uniría a la sala —avisando a todos de que se
// está grabando—, grabaría, y fallaría al subir el fichero. Se habría grabado a
// la gente para nada, que es el peor de los desenlaces posibles.
func TestGrabarSinDestinoNoArranca(t *testing.T) {
	lk := &fakeLK{}
	s := newSvc(&fakeStore{}, lk, nil)
	_, err := s.StartRecording(tenantCtx(t), uuid.New())
	if !errors.Is(err, ErrGrabacionNoConfigurada) {
		t.Fatalf("err = %v, quería ErrGrabacionNoConfigurada", err)
	}
	if lk.rutaGrabacion != "" {
		t.Error("se llamó al SFU pese a no tener dónde guardar la grabación")
	}
}

// TestGrabarDosVecesLaMismaSala: una sala, una grabación.
//
// Dos egress sobre la misma sala son dos ficheros a medias, dos facturas y
// ningún acta completa.
func TestGrabarDosVecesLaMismaSala(t *testing.T) {
	fs := &fakeStore{grabaciones: []domain.Recording{{
		EgressID: "EG_YA", Status: domain.RecordingStarted,
	}}}
	lk := &fakeLK{}
	s := conDestino(newSvc(fs, lk, nil))
	if _, err := s.StartRecording(tenantCtx(t), uuid.New()); !errors.Is(err, ErrYaSeEstaGrabando) {
		t.Fatalf("err = %v, quería ErrYaSeEstaGrabando", err)
	}
	if lk.rutaGrabacion != "" {
		t.Error("se pidió un SEGUNDO egress sobre una sala que ya se estaba grabando")
	}
}

// TestGrabarAnotaAntesDeQueLleguenLosWebhooks: la fila se escribe al arrancar.
//
// Si se esperara al webhook y el proceso se cayera en medio, habría un egress
// corriendo en LiveKit que nadie sabría que existe —ni podría parar—.
func TestGrabarAnotaAntesDeQueLleguenLosWebhooks(t *testing.T) {
	fs := &fakeStore{}
	s := conDestino(newSvc(fs, &fakeLK{}, nil))
	egressID, err := s.StartRecording(tenantCtx(t), uuid.New())
	if err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	if egressID == "" {
		t.Error("no se devolvió el egress_id: sin él nadie puede parar la grabación")
	}
	if fs.upsertStat != domain.RecordingStarted {
		t.Errorf("estado anotado = %q, quería started", fs.upsertStat)
	}
	if fs.upsertKey == "" {
		t.Error("se anotó sin la ruta del fichero")
	}
}

// TestPararUnaGrabacionQueYaHabiaTerminado: el fallo del SFU no deja la fila
// colgada.
//
// El egress pudo acabar solo —la sala se vació, el proceso murió—. Si un error
// al pararlo dejara la grabación como «en curso» para siempre, esa sala no se
// podría volver a grabar nunca.
func TestPararUnaGrabacionQueYaHabiaTerminado(t *testing.T) {
	fs := &fakeStore{grabaciones: []domain.Recording{{
		EgressID: "EG_MUERTO", Status: domain.RecordingStarted,
	}}}
	lk := &fakeLK{pararErr: errors.New("egress no encontrado")}
	s := conDestino(newSvc(fs, lk, nil))
	if err := s.StopRecording(tenantCtx(t), uuid.New()); err != nil {
		t.Fatalf("parar algo que ya terminó no debe fallar: %v", err)
	}
	if fs.upsertStat != domain.RecordingCompleted {
		t.Errorf("la grabación quedó en %q: esa sala no se podría volver a grabar", fs.upsertStat)
	}
}

// TestPararSinGrabacionEnCurso devuelve «no hay nada», no un error interno.
func TestPararSinGrabacionEnCurso(t *testing.T) {
	s := conDestino(newSvc(&fakeStore{}, &fakeLK{}, nil))
	if err := s.StopRecording(tenantCtx(t), uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v, quería ErrNotFound", err)
	}
}

// conDestino deja el servicio con un bucket configurado.
// conDestino deja el servicio como cuando SÍ hay dónde grabar: config completa y
// un bucket que respondió al arrancar. Las dos cosas, porque grabar exige las dos
// —config sin bucket vivo significa grabar a la gente y perder el fichero—.
func conDestino(s *Service) *Service {
	s.cfg.RecordingBucket = "sytue-prod-recordings"
	s.cfg.RecordingAccessKey = "ak"
	s.cfg.RecordingSecretKey = "sk"
	s.cfg.RecordingRegion = "fr-par"
	s.objetos = almacenFalso{}
	return s
}

// almacenFalso firma sin salir del proceso.
type almacenFalso struct{}

func (almacenFalso) PresignGet(_ context.Context, _ tenancy.Tenant, key string, _ time.Duration) (*url.URL, error) {
	return url.Parse("https://s3.example/" + key + "?firma=x")
}

func (almacenFalso) Delete(context.Context, tenancy.Tenant, string) error { return nil }

// almacenQueNoBorra es el bucket que responde mal al borrar: red caída, permiso
// retirado, objeto bloqueado. Existe para fijar el orden de DeleteRecording.
type almacenQueNoBorra struct{ almacenFalso }

func (almacenQueNoBorra) Delete(context.Context, tenancy.Tenant, string) error {
	return errors.New("s3 dijo que no")
}

// TestBorrarGrabacionNoDejaHuerfanos: si el fichero no se puede borrar, la FILA
// SE QUEDA.
//
// Es la invariante entera de DeleteRecording. Al revés —borrar la fila y luego
// el objeto— un fallo a mitad deja en el bucket la conversación de un equipo que
// ya no aparece en ninguna consulta: nadie la puede borrar porque nadie sabe que
// está. Así el fallo es visible y se reintenta.
func TestBorrarGrabacionNoDejaHuerfanos(t *testing.T) {
	rec := uuid.New()
	sala := uuid.New()
	fs := &fakeStore{grabaciones: []domain.Recording{{
		ID: rec, Status: domain.RecordingCompleted, StorageKey: "grabaciones/2026/x.mp4",
	}}}
	s := conDestino(newSvc(fs, &fakeLK{}, nil))
	s.objetos = almacenQueNoBorra{}

	if err := s.DeleteRecording(tenantCtx(t), sala, rec); err == nil {
		t.Fatal("borrar tenía que fallar: el bucket rechazó el objeto")
	}
	if fs.filaBorrada {
		t.Error("la fila se borró con el fichero todavía en el bucket: eso deja una grabación que nadie puede alcanzar")
	}
}

// Y el camino bueno: se borran los dos, en ese orden.
func TestBorrarGrabacionBorraFicheroYFila(t *testing.T) {
	rec := uuid.New()
	fs := &fakeStore{grabaciones: []domain.Recording{{
		ID: rec, Status: domain.RecordingCompleted, StorageKey: "grabaciones/2026/x.mp4",
	}}}
	s := conDestino(newSvc(fs, &fakeLK{}, nil))
	if err := s.DeleteRecording(tenantCtx(t), uuid.New(), rec); err != nil {
		t.Fatalf("borrar falló: %v", err)
	}
	if !fs.filaBorrada {
		t.Error("el fichero se borró y la fila no: la grabación seguiría apareciendo en la lista")
	}
}

// Una grabación que no existe no es un error interno.
func TestBorrarGrabacionQueNoExiste(t *testing.T) {
	s := conDestino(newSvc(&fakeStore{}, &fakeLK{}, nil))
	if err := s.DeleteRecording(tenantCtx(t), uuid.New(), uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v, quería ErrNotFound", err)
	}
}

// TestNoSeGrabaSinBucketVivo: con la config puesta pero sin almacén que haya
// respondido, grabar se rechaza a la entrada.
//
// Es el caso de un bucket bien escrito en los values que nadie llegó a crear.
// Sin esta condición el egress entra en la sala, avisa a todos de que se está
// grabando y falla al subir: se grabó a la gente para nada, que es el peor de
// los desenlaces posibles.
func TestNoSeGrabaSinBucketVivo(t *testing.T) {
	s := conDestino(newSvc(&fakeStore{}, &fakeLK{}, nil))
	s.objetos = nil
	if _, err := s.StartRecording(tenantCtx(t), uuid.New()); !errors.Is(err, ErrGrabacionNoConfigurada) {
		t.Fatalf("err = %v, quería ErrGrabacionNoConfigurada", err)
	}
}

// TestClaveRelativa fija la traducción de lo que informa el egress a la clave
// que guarda la base.
//
// Importa porque el webhook PISA la clave que escribió `StartRecording`: si se
// cuela la ruta absoluta, `objectstore` le vuelve a anteponer el prefijo del
// tenant y la descarga busca en `tenants/acme/tenants/acme/...`. El fallo
// aparece al terminar de grabar, no al arrancar, así que en local no se ve.
func TestClaveRelativa(t *testing.T) {
	tn := tenancy.Tenant{Slug: "acme"}
	casos := []struct{ dentro, fuera string }{
		// Lo normal: el egress devuelve el `filepath` que le dimos.
		{"tenants/acme/sala-1/1700000000.mp4", "sala-1/1700000000.mp4"},
		// Ya relativa (no debería llegar así, pero no se estropea).
		{"sala-1/1700000000.mp4", "sala-1/1700000000.mp4"},
		// `Location` como URL de S3: fuera esquema, host y bucket.
		{"s3://grabaciones/tenants/acme/sala-1/1.mp4", "sala-1/1.mp4"},
		{"https://s3.fr-par.scw.cloud/grabaciones/tenants/acme/sala-1/1.mp4", "sala-1/1.mp4"},
		// Barra inicial suelta.
		{"/tenants/acme/sala-1/1.mp4", "sala-1/1.mp4"},
		{"", ""},
	}
	for _, c := range casos {
		if got := claveRelativa(tn, c.dentro); got != c.fuera {
			t.Errorf("claveRelativa(%q) = %q, se esperaba %q", c.dentro, got, c.fuera)
		}
	}
}

// TestRetirarLaPalabraTambienCallaLoQueYaSonaba: quitar el permiso no apaga el
// micrófono que ya estaba abierto.
//
// El SFU deja de aceptar pistas NUEVAS, no corta las vivas. Sin silenciar además,
// el botón parecería no funcionar justo en el caso que lo justifica: alguien con
// el micro abierto al que ya no se quiere oír.
func TestRetirarLaPalabraTambienCallaLoQueYaSonaba(t *testing.T) {
	lk := &fakeLK{participante: livekit.ParticipantInfo{
		Tracks: []livekit.TrackInfo{{SID: "TR_AUDIO", Type: "AUDIO"}},
	}}
	s := newSvc(&fakeStore{room: domain.Room{ID: uuid.New(), Name: "r"}}, lk, nil)

	if err := s.SetCanPublish(tenantCtx(t), uuid.New(), "ruidoso", false); err != nil {
		t.Fatalf("SetCanPublish: %v", err)
	}
	if lk.palabraDe != "ruidoso" || lk.palabraOK {
		t.Errorf("permiso: de=%q canPublish=%v", lk.palabraDe, lk.palabraOK)
	}
	if len(lk.silenciadas) != 1 {
		t.Error("retirar la palabra tiene que cortar además la pista que ya sonaba")
	}

	// Devolverla NO silencia: sería quitar con una mano lo que se da con la otra.
	lk.silenciadas = nil
	if err := s.SetCanPublish(tenantCtx(t), uuid.New(), "ruidoso", true); err != nil {
		t.Fatalf("devolver la palabra: %v", err)
	}
	if !lk.palabraOK || len(lk.silenciadas) != 0 {
		t.Errorf("devolver la palabra no debe silenciar: canPublish=%v silenciadas=%v",
			lk.palabraOK, lk.silenciadas)
	}
}

func (f *fakeLK) SetMediaPermissions(ctx context.Context, room, identity string, publish, data bool) error {
	f.datosOK = data
	return f.SetCanPublish(ctx, room, identity, publish)
}
