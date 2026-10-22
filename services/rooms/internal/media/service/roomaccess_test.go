// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests de la VERJA DE ACCESO A SALA (requireRoomAccess). Sin ella, GetRoom solo
// filtraba por (tenant, id), así que cualquier usuario autenticado de la organización
// podía unirse a la videollamada de un canal privado o de un DM y recibir audio y
// vídeo en vivo: el permiso de la llamada se evaluaba en el ámbito TENANT de Meet
// mientras su padre —el canal— se gobierna por el roster en Chat.
//
// Es la misma clase de fallo que los adjuntos de canal (commit 16c3b70) y se prueba
// igual: quien no puede leer el canal no entra, y quien sí, entra.
package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
)

// chatRoom es una sala viva ligada a un canal de Chat.
func chatRoom() domain.Room {
	return domain.Room{
		ID:        uuid.New(),
		Name:      "r-chat",
		Status:    domain.RoomActive,
		CreatedBy: "alice",
		Metadata:  map[string]any{"source": map[string]any{"kind": "chat", "ref_id": "chan-42"}},
	}
}

func accessCtx(t *testing.T) context.Context {
	t.Helper()
	tn, err := tenancy.New("acme")
	if err != nil {
		t.Fatalf("tenancy.New: %v", err)
	}
	return tenancy.Into(context.Background(), tn)
}

// TestRoomAccessDeniesNonChannelMember: un tercero del tenant que NO puede leer el
// canal no accede a la sala de ese canal, aunque conozca su id.
func TestRoomAccessDeniesNonChannelMember(t *testing.T) {
	room := chatRoom()
	s := &Service{store: &fakeStore{room: room}}
	notifier := &fakeMessagingNotifier{allow: false}
	s.SetMessagingNotifier(notifier)

	err := s.requireRoomAccess(accessCtx(t), room, "eve", false)
	if !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("un no-miembro del canal NO debe acceder a su llamada; err=%v", err)
	}
	if len(notifier.accessLog) != 1 || notifier.accessLog[0] != "acme/chan-42/eve" {
		t.Fatalf("debe preguntarse a Chat por (tenant, canal, sujeto): %v", notifier.accessLog)
	}
}

// TestRoomAccessAllowsChannelMember: quien SÍ puede leer el canal entra.
func TestRoomAccessAllowsChannelMember(t *testing.T) {
	room := chatRoom()
	s := &Service{store: &fakeStore{room: room}}
	s.SetMessagingNotifier(&fakeMessagingNotifier{allow: true})

	if err := s.requireRoomAccess(accessCtx(t), room, "bob", false); err != nil {
		t.Fatalf("un miembro del canal debe poder entrar: %v", err)
	}
}

// TestRoomAccessFailsClosed: si la sonda a Chat falla (red, servicio caído) o no está
// configurada, se DENIEGA. Una llamada privada no se sirve «por si acaso».
func TestRoomAccessFailsClosed(t *testing.T) {
	room := chatRoom()

	s := &Service{store: &fakeStore{room: room}}
	s.SetMessagingNotifier(&fakeMessagingNotifier{accessErr: errors.New("red caída")})
	if err := s.requireRoomAccess(accessCtx(t), room, "eve", false); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("sonda con error debe DENEGAR (fail-closed); err=%v", err)
	}

	sinSonda := &Service{store: &fakeStore{room: room}} // chatNotifier nil
	if err := sinSonda.requireRoomAccess(accessCtx(t), room, "eve", false); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("sin sonda configurada debe DENEGAR; err=%v", err)
	}
}

// Active calls always use the current channel ACL, including creators, tenant
// administrators and historical participants whose access has since been removed.
func TestActiveRoomAccessCannotBypassCurrentChannelACL(t *testing.T) {
	room := chatRoom()
	s := &Service{store: &fakeStore{room: room, participantes: []domain.Participant{{Identity: "bob"}}}}
	s.SetMessagingNotifier(&fakeMessagingNotifier{allow: false})
	for _, tc := range []struct {
		actor string
		admin bool
	}{{"alice", false}, {"root", true}, {"bob", false}} {
		if err := s.requireRoomAccess(accessCtx(t), room, tc.actor, tc.admin); !errors.Is(err, domain.ErrPermission) {
			t.Errorf("actor %s bypassed revoked ACL: %v", tc.actor, err)
		}
	}
}

type calendarioFalso map[string]bool // sub → invitado al evento uid-1

func (c calendarioFalso) EsInvitado(ctx context.Context, uid string) bool {
	cl, _ := authn.FromContext(ctx)
	return uid == "uid-1" && c[cl.Subject]
}

// La reunión de un evento la abren sus invitados aunque su enlace sea de antes de la
// clave: lo dice Calendar. Quien no está en el evento y trae el enlace viejo, no entra.
//
// Visto en rojo antes de la sonda: Bob, invitado al evento, se quedaba fuera.
func TestLaReunionDeUnEventoLaAbrenSusInvitados(t *testing.T) {
	room := domain.Room{
		ID: uuid.New(), Name: "r-evento", Status: domain.RoomActive, CreatedBy: "alice", LinkKey: "k",
		Metadata: map[string]any{"source": map[string]any{"kind": "calendar", "ref_id": "uid-1"}},
	}
	s := &Service{store: &fakeStore{room: room}}
	s.SetCalendarGate(calendarioFalso{"bob": true})
	if err := s.requireRoomAccessDe(sesion(t, "bob"), room); err != nil {
		t.Errorf("Bob está invitado al evento: %v", err)
	}
	if err := s.requireRoomAccessDe(sesion(t, "eve"), room); err == nil {
		t.Error("Eve no está en el evento y entra sin clave")
	}
	directa := room
	directa.Metadata = nil
	if err := s.requireRoomAccessDe(sesion(t, "bob"), directa); err == nil {
		t.Error("la sonda de Calendar abre una reunión que no es de un evento")
	}
}

// Reunión suelta (decidido por el dueño el 2026-10-16: «Invitados y quien tenga el
// enlace»): la abría cualquiera de la organización que conociera su id. Ahora entran
// quien la convocó, quien ya estuvo dentro y quien trae la clave del enlace; el rol de
// administrador no abre la puerta. Vale igual para la de un evento de Calendar.
//
// Visto en rojo contra main: Eve, sin clave, entraba.
func TestReunionSueltaSoloConInvitacionOEnlace(t *testing.T) {
	for _, origen := range []map[string]any{nil, {"source": map[string]any{"kind": "calendar", "ref_id": "uid-1"}}} {
		room := domain.Room{
			ID: uuid.New(), Name: "r-suelta", Status: domain.RoomActive, CreatedBy: "alice",
			LinkKey: "clave-del-enlace-de-la-reunion", Metadata: origen,
		}
		fs := &fakeStore{room: room, participantes: []domain.Participant{{Identity: "bob"}}}
		s := &Service{store: fs}
		s.SetMessagingNotifier(&fakeMessagingNotifier{allow: true})
		ctx := accessCtx(t)
		casos := []struct {
			quien  string
			admin  bool
			clave  string
			entra  bool
			porque string
		}{
			{"alice", false, "", true, "la convocó"},
			{"bob", false, "", true, "ya estuvo dentro"},
			{"eve", false, "", false, "sin clave"},
			{"eve", true, "", false, "administra, sin clave"},
			{"eve", false, "otra-clave", false, "con una clave ajena"},
			{"eve", false, room.LinkKey, true, "con la clave del enlace"},
		}
		for _, c := range casos {
			err := s.requireRoomAccess(ConClave(ctx, c.clave), room, c.quien, c.admin)
			if (err == nil) != c.entra {
				t.Errorf("origen %v, %s (%s): err=%v", origen, c.quien, c.porque, err)
			}
		}
	}
}

// TestSalaBloqueadaNoDejaEntrarANadieNuevo: la puerta echada solo la cruzan
// quienes pueden reabrirla.
//
// Bloquear existe para el caso de que se cuele alguien por el enlace: expulsarlo
// no vale de nada si vuelve a entrar tres segundos después. Y el anfitrión TIENE
// que pasar, porque si no se quedaría fuera al recargar la página —con la sala
// cerrada por él mismo y sin forma de reabrirla ni de echar a quien está dentro—.
func TestSalaBloqueadaNoDejaEntrarANadieNuevo(t *testing.T) {
	room := chatRoom()
	room.Locked = true
	s := newSvc(&fakeStore{room: room}, &fakeLK{}, nil)
	s.SetMessagingNotifier(&fakeMessagingNotifier{allow: true})
	ctx := accessCtx(t)

	if _, err := s.JoinRoom(ctx, room.ID, "bob", "Bob", domain.RoleSpeaker, false); !errors.Is(err, domain.ErrRoomLocked) {
		t.Fatalf("un tercero NO debe entrar en una sala bloqueada; err=%v", err)
	}
	if _, err := s.JoinRoom(ctx, room.ID, "alice", "Alice", domain.RoleSpeaker, false); err != nil {
		t.Errorf("el anfitrión debe poder volver a entrar en su propia sala: %v", err)
	}
	if _, err := s.JoinRoom(ctx, room.ID, "root", "Root", domain.RoleSpeaker, true); err != nil {
		t.Errorf("un admin debe poder entrar para desbloquear: %v", err)
	}

	// Y sin bloquear, el tercero entra como siempre: la puerta solo estorba
	// cuando está echada.
	abierta := chatRoom()
	s2 := newSvc(&fakeStore{room: abierta}, &fakeLK{}, nil)
	s2.SetMessagingNotifier(&fakeMessagingNotifier{allow: true})
	if _, err := s2.JoinRoom(ctx, abierta.ID, "bob", "Bob", domain.RoleSpeaker, false); err != nil {
		t.Errorf("sin bloqueo, un miembro del canal entra: %v", err)
	}
}

// TestSalaDeEsperaAbreAUnoSinReabrirlaParaTodos: la admisión es individual.
//
// Es la razón de ser de la sala de espera: bloquear deja fuera a todo el mundo y
// eso resuelve lo urgente, pero cuando llega quien tenía que llegar la única
// alternativa era reabrir la sala —y con ella, volver a dejar entrar a quien
// acababas de echar—.
func TestSalaDeEsperaAbreAUnoSinReabrirlaParaTodos(t *testing.T) {
	room := chatRoom()
	room.Locked = true
	fs := &fakeStore{room: room, admitidos: map[string]bool{"invitada": true}}
	s := newSvc(fs, &fakeLK{}, nil)
	s.SetMessagingNotifier(&fakeMessagingNotifier{allow: true})
	ctx := accessCtx(t)

	if _, err := s.JoinRoom(ctx, room.ID, "invitada", "Invitada", domain.RoleSpeaker, false); err != nil {
		t.Errorf("quien tiene la puerta abierta debe entrar: %v", err)
	}
	// Y la sala SIGUE cerrada para el resto: abrirle a uno no es reabrirla.
	if _, err := s.JoinRoom(ctx, room.ID, "otro", "Otro", domain.RoleSpeaker, false); !errors.Is(err, domain.ErrRoomLocked) {
		t.Fatalf("abrirle a uno no puede abrir la sala; err=%v", err)
	}
}

// TestLlamarALaPuertaDeUnaSalaAbiertaEsUnError: no se apunta a nadie en una cola
// que no existe.
//
// Devolver «apuntado, espera» dejaría a alguien aguardando a que le abran una
// puerta que estaba abierta, y esa espera no la interrumpe nadie porque el
// anfitrión no ve ninguna cola.
func TestLlamarALaPuertaDeUnaSalaAbiertaEsUnError(t *testing.T) {
	room := chatRoom() // sin Locked
	fs := &fakeStore{room: room}
	s := newSvc(fs, &fakeLK{}, nil)
	s.SetMessagingNotifier(&fakeMessagingNotifier{allow: true})

	if _, err := s.Knock(accessCtx(t), room.ID, "bob", "Bob"); !errors.Is(err, domain.ErrRoomOpen) {
		t.Fatalf("err = %v, quería ErrRoomOpen", err)
	}
	if len(fs.knocks) != 0 {
		t.Error("no se puede apuntar a nadie en la cola de una sala abierta")
	}
}

// TestLlamarALaPuertaExigeAccesoALaSala: llamar no es una rendija.
//
// Sin esto, cualquiera del tenant con el id de una sala podría apuntarse en la
// cola de una reunión de un canal del que no es miembro. No entraría —hace falta
// que le abran— pero el anfitrión vería su nombre, y eso ya es enterarse de que
// esa reunión existe y de quién la tiene.
func TestLlamarALaPuertaExigeAccesoALaSala(t *testing.T) {
	room := chatRoom()
	room.Locked = true
	fs := &fakeStore{room: room}
	s := newSvc(fs, &fakeLK{}, nil)
	s.SetMessagingNotifier(&fakeMessagingNotifier{allow: false})

	if _, err := s.Knock(accessCtx(t), room.ID, "eve", "Eve"); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("un ajeno al canal no puede ni llamar; err=%v", err)
	}
	if len(fs.knocks) != 0 {
		t.Error("no se apunta en la cola a quien no tiene acceso a la sala")
	}
}

// sesion es el contexto de una petición de `sub` en acme, con o sin rol de la organización.
func sesion(t *testing.T, sub string, roles ...string) context.Context {
	t.Helper()
	return authn.Into(accessCtx(t), authn.Claims{Subject: sub, Tenant: "acme", Roles: roles})
}

// COM-02. Por POST /rooms con `source` cualquiera de la organización creaba la sala
// de un canal ajeno: la verja estaba solo en from-source. Como creador pasaba después
// requireRoomAccess sin preguntar al canal, y cuando alguien del canal llamaba,
// from-source le devolvía ESA sala con el intruso de anfitrión.
func TestCrearLaSalaDeUnCanalPasaPorLaVerjaDelCanal(t *testing.T) {
	origen := domain.RoomSource{Kind: domain.SourceChat, RefID: "chan-42"}

	fs := &fakeStore{room: domain.Room{ID: uuid.New(), Name: "r"}}
	s := newSvc(fs, &fakeLK{}, &capturePub{})
	sonda := &fakeMessagingNotifier{allow: false}
	s.SetMessagingNotifier(sonda)
	if _, err := s.CreateRoom(accessCtx(t), CreateRoomInput{Title: "Dirección", CreatedBy: "eve", Source: origen}); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("quien no está en el canal NO crea su sala; err=%v", err)
	}
	if fs.newRoom != nil {
		t.Fatal("la sala se ha creado igualmente")
	}
	if len(sonda.accessLog) != 1 || sonda.accessLog[0] != "acme/chan-42/eve" {
		t.Fatalf("debe preguntarse a la mensajería por (organización, canal, quien crea): %v", sonda.accessLog)
	}

	// Quien sí está, la crea.
	s.SetMessagingNotifier(&fakeMessagingNotifier{allow: true})
	if _, err := s.CreateRoom(accessCtx(t), CreateRoomInput{Title: "Dirección", CreatedBy: "bob", Source: origen}); err != nil {
		t.Fatalf("un miembro del canal debe poder crear su sala: %v", err)
	}
	if fs.newRoom == nil || fs.newRoom.Metadata["source"] == nil {
		t.Fatalf("la sala del miembro no lleva su origen: %+v", fs.newRoom)
	}
}

// El origen de una sala lo escribe el servicio, tras la verja. Si el cliente pudiera
// meterlo en `metadata`, la sala de un canal se crearía sin pasar por ella.
func TestElOrigenNoSeCuelaPorLosMetadatos(t *testing.T) {
	fs := &fakeStore{room: domain.Room{ID: uuid.New(), Name: "r"}}
	s := newSvc(fs, &fakeLK{}, &capturePub{})
	s.SetMessagingNotifier(&fakeMessagingNotifier{allow: false})

	_, err := s.CreateRoom(accessCtx(t), CreateRoomInput{Title: "Suelta", CreatedBy: "eve", Metadata: map[string]any{
		"source":       map[string]any{"kind": "chat", "ref_id": "chan-42"},
		"sytue_tenant": "globex",
		"color":        "verde",
	}})
	if err != nil {
		t.Fatalf("una sala suelta se crea sin más: %v", err)
	}
	meta := fs.newRoom.Metadata
	if meta["source"] != nil {
		t.Errorf("el cliente ha fijado el origen de la sala: %v", meta["source"])
	}
	if meta["sytue_tenant"] != "acme" {
		t.Errorf("sytue_tenant = %v, quiero la organización de la sesión", meta["sytue_tenant"])
	}
	if meta["color"] != "verde" {
		t.Errorf("los demás metadatos del cliente se conservan: %v", meta)
	}
}

// GET /rooms devolvía todas las salas de la organización, y con sus metadatos el canal
// de cada llamada: también el de los privados y los directos de otros.
func TestListRoomsSoloEnsenaLasSalasALasQueSeTieneAcceso(t *testing.T) {
	delCanal := chatRoom() // creada por alice, canal chan-42
	suelta := domain.Room{ID: uuid.New(), Name: "r-suelta", Status: domain.RoomActive, CreatedBy: "alice"}
	fs := &fakeStore{rooms: []domain.Room{delCanal, suelta}}

	ver := func(ctx context.Context, enElCanal bool) []uuid.UUID {
		t.Helper()
		s := &Service{store: &fakeStore{rooms: append([]domain.Room(nil), fs.rooms...)}}
		s.SetMessagingNotifier(&fakeMessagingNotifier{allow: enElCanal})
		got, err := s.ListRooms(ctx, "", 0)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]uuid.UUID, 0, len(got))
		for _, r := range got {
			ids = append(ids, r.ID)
		}
		return ids
	}

	if ids := ver(sesion(t, "eve"), false); len(ids) != 1 || ids[0] != suelta.ID {
		t.Fatalf("quien no está en el canal ve %v, quiero solo la sala suelta", ids)
	}
	for nombre, ids := range map[string][]uuid.UUID{
		"quien está en el canal":                ver(sesion(t, "bob"), true),
		"quien creó la sala":                    ver(sesion(t, "alice"), true),
		"quien administra y pertenece al canal": ver(sesion(t, "root", "admin"), true),
	} {
		if len(ids) != 2 {
			t.Errorf("%s ve %v, quiero las dos", nombre, ids)
		}
	}

	// Sin poder preguntar a la mensajería, la sala del canal no se enseña.
	sinSonda := &Service{store: &fakeStore{rooms: []domain.Room{delCanal, suelta}}}
	if got, _ := sinSonda.ListRooms(sesion(t, "eve"), "", 0); len(got) != 1 || got[0].ID != suelta.ID {
		t.Fatalf("sin sonda: %v, quiero solo la sala suelta", got)
	}
}

// La ficha de la sala y «¿hay llamada en este canal?» tampoco se contestan a quien no
// está en el canal: por ahí pasan además todas las acciones de moderación.
func TestLaSalaDeUnCanalNoSeLeeDesdeFuera(t *testing.T) {
	room := chatRoom()
	fs := &fakeStore{room: room, findRoom: room, findFound: true}
	s := &Service{store: fs}
	s.SetMessagingNotifier(&fakeMessagingNotifier{allow: false})

	if _, err := s.GetRoom(sesion(t, "eve"), room.ID); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("GetRoom desde fuera del canal; err=%v", err)
	}
	if _, err := s.RoomBySource(sesion(t, "eve"), domain.SourceChat, "chan-42"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("by-source desde fuera del canal no se distingue de que no haya llamada; err=%v", err)
	}

	s.SetMessagingNotifier(&fakeMessagingNotifier{allow: true})
	if got, err := s.GetRoom(sesion(t, "bob"), room.ID); err != nil || got.ID != room.ID {
		t.Fatalf("un miembro del canal lee su sala: %v %v", got.ID, err)
	}
	if got, err := s.RoomBySource(sesion(t, "bob"), domain.SourceChat, "chan-42"); err != nil || got.ID != room.ID {
		t.Fatalf("un miembro del canal encuentra la llamada: %v %v", got.ID, err)
	}
}
