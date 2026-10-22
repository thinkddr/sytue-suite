// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package meethttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/service"
)

// fakeSvc implementa roomService para ejercitar los handlers sin BD ni LiveKit.
type fakeSvc struct {
	nombreAlEntrar     string
	grabacionBorrada   bool
	borrarGrabacionErr error
	room               domain.Room
	rooms              []domain.Room
	parts              []domain.Participant
	recs               []domain.Recording
	join               service.JoinResult
	err                error
	removeErr          error
	removedID          string
	mutedID            string
	muteAllExcept      string
	lockPedido         *bool
	palabraDe          string
	palabraOK          bool
	llamoALaPuerta     string
	admitidoDe         string
	admitidoOK         bool
	cola               []domain.Admission
	grabando           bool
	grabarErr          error
	pararErr           error
	muteErr            error
	createIn           service.CreateRoomInput
	fromSrcKind        domain.SourceKind
}

func (f *fakeSvc) CreateRoom(_ context.Context, in service.CreateRoomInput) (domain.Room, error) {
	f.createIn = in
	return f.room, f.err
}

func (f *fakeSvc) CreateRoomFromSource(_ context.Context, kind domain.SourceKind, _, _, _ string, _ int) (domain.Room, error) {
	f.fromSrcKind = kind
	return f.room, f.err
}

func (f *fakeSvc) RoomBySource(_ context.Context, kind domain.SourceKind, _ string) (domain.Room, error) {
	f.fromSrcKind = kind
	return f.room, f.err
}

func (f *fakeSvc) GetRoom(_ context.Context, _ uuid.UUID) (domain.Room, error) {
	return f.room, f.err
}

func (f *fakeSvc) ListRooms(_ context.Context, _ domain.RoomStatus, _ int) ([]domain.Room, error) {
	return f.rooms, f.err
}

func (f *fakeSvc) ListParticipants(_ context.Context, _ uuid.UUID, _ bool) ([]domain.Participant, error) {
	return f.parts, f.err
}

func (f *fakeSvc) ListRecordings(_ context.Context, _ uuid.UUID) ([]domain.Recording, error) {
	return f.recs, f.err
}

func (f *fakeSvc) CloseRoom(_ context.Context, _ uuid.UUID) (domain.Room, error) {
	return f.room, f.err
}

func (f *fakeSvc) JoinRoom(_ context.Context, _ uuid.UUID, _, nombre string, _ domain.Role, _ bool) (service.JoinResult, error) {
	f.nombreAlEntrar = nombre
	return f.join, f.err
}

func (f *fakeSvc) RemoveParticipant(_ context.Context, _ uuid.UUID, identity string) error {
	f.removedID = identity
	return f.removeErr
}

func (f *fakeSvc) MuteParticipant(_ context.Context, _ uuid.UUID, identity string) error {
	f.mutedID = identity
	return f.muteErr
}

func (f *fakeSvc) StartRecording(_ context.Context, _ uuid.UUID) (string, error) {
	f.grabando = true
	return "EG_TEST", f.grabarErr
}

func (f *fakeSvc) StopRecording(_ context.Context, _ uuid.UUID) error {
	f.grabando = false
	return f.pararErr
}

func (f *fakeSvc) SetCanPublish(_ context.Context, _ uuid.UUID, identity string, canPublish bool) error {
	f.palabraDe, f.palabraOK = identity, canPublish
	return f.muteErr
}

func (f *fakeSvc) Knock(_ context.Context, roomID uuid.UUID, identity, name string) (domain.Admission, error) {
	f.llamoALaPuerta = identity
	return domain.Admission{RoomID: roomID, Identity: identity, Name: name}, f.err
}

func (f *fakeSvc) Admit(_ context.Context, _ uuid.UUID, identity string, admitido bool) error {
	f.admitidoDe, f.admitidoOK = identity, admitido
	return f.muteErr
}

func (f *fakeSvc) ListKnocks(_ context.Context, _ uuid.UUID) ([]domain.Admission, error) {
	return f.cola, f.err
}

func (f *fakeSvc) LockRoom(_ context.Context, _ uuid.UUID, locked bool) (domain.Room, error) {
	f.lockPedido = &locked
	r := f.room
	r.Locked = locked
	return r, f.err
}

func (f *fakeSvc) MuteAll(_ context.Context, _ uuid.UUID, excepto string) (int, error) {
	f.muteAllExcept = excepto
	return 3, f.muteErr
}

func (f *fakeSvc) RecordingURL(_ context.Context, _, _ uuid.UUID) (string, error) {
	return "https://s3.example/firmada", f.err
}

func (f *fakeSvc) DeleteRecording(_ context.Context, _, _ uuid.UUID) error {
	f.grabacionBorrada = true
	return f.borrarGrabacionErr
}

var _ roomService = (*fakeSvc)(nil)

func newHandlers(f *fakeSvc) *Handlers {
	return &Handlers{svc: f, cfg: config.Config{MaxParticipants: 100}}
}

func mkReq(t *testing.T, method, target string, claims *authn.Claims, params map[string]string, body string) *http.Request {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, target, http.NoBody)
	}
	if claims != nil {
		r = r.WithContext(authn.Into(r.Context(), *claims))
	}
	if params != nil {
		rc := chi.NewRouteContext()
		for k, v := range params {
			rc.URLParams.Add(k, v)
		}
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
	}
	return r
}

func TestCreateRoom(t *testing.T) {
	f := &fakeSvc{room: domain.Room{ID: uuid.New(), Name: "room-x", Title: "Reunión"}}
	h := newHandlers(f)

	// JSON inválido -> 400.
	rec := httptest.NewRecorder()
	h.CreateRoom(rec, mkReq(t, "POST", "/api/v1/rooms", &authn.Claims{Subject: "u"}, nil, `{bad`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("json inválido: code=%d", rec.Code)
	}

	// Feliz -> 201 y propaga created_by.
	rec = httptest.NewRecorder()
	h.CreateRoom(rec, mkReq(t, "POST", "/api/v1/rooms", &authn.Claims{Subject: "u"}, nil, `{"title":"Reunión"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("crear: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if f.createIn.CreatedBy != "u" {
		t.Errorf("CreatedBy = %q", f.createIn.CreatedBy)
	}

	// Error de dominio (nombre inválido) -> 400.
	f.err = domain.ErrInvalidName
	rec = httptest.NewRecorder()
	h.CreateRoom(rec, mkReq(t, "POST", "/api/v1/rooms", &authn.Claims{Subject: "u"}, nil, `{"title":"Reunión"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("dominio inválido: code=%d", rec.Code)
	}

	// Conflicto -> 409.
	f.err = domain.ErrConflict
	rec = httptest.NewRecorder()
	h.CreateRoom(rec, mkReq(t, "POST", "/api/v1/rooms", &authn.Claims{Subject: "u"}, nil, `{"title":"Reunión"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("conflicto: code=%d", rec.Code)
	}
}

func TestCreateRoomFromSource(t *testing.T) {
	f := &fakeSvc{room: domain.Room{ID: uuid.New()}}
	h := newHandlers(f)

	// kind directo (no cross-app) -> 400.
	rec := httptest.NewRecorder()
	h.CreateRoomFromSource(rec, mkReq(t, "POST", "/api/v1/rooms/from-source", &authn.Claims{Subject: "u"}, nil, `{"kind":"direct","ref_id":"x"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("kind directo: code=%d", rec.Code)
	}

	// kind calendar -> 201.
	rec = httptest.NewRecorder()
	h.CreateRoomFromSource(rec, mkReq(t, "POST", "/api/v1/rooms/from-source", &authn.Claims{Subject: "u"}, nil, `{"kind":"calendar","ref_id":"ev1","title":"Sync"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("calendar: code=%d", rec.Code)
	}
	if f.fromSrcKind != domain.SourceCalendar {
		t.Errorf("kind = %q", f.fromSrcKind)
	}
}

func TestRoomBySource(t *testing.T) {
	f := &fakeSvc{room: domain.Room{ID: uuid.New(), Name: "room-chat"}}
	h := newHandlers(f)

	// kind no cross-app -> 400.
	rec := httptest.NewRecorder()
	h.RoomBySource(rec, mkReq(t, "GET", "/api/v1/rooms/by-source?kind=direct&ref_id=c1", &authn.Claims{Subject: "u"}, nil, ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("kind directo: code=%d", rec.Code)
	}

	// falta ref_id -> 400.
	rec = httptest.NewRecorder()
	h.RoomBySource(rec, mkReq(t, "GET", "/api/v1/rooms/by-source?kind=chat", &authn.Claims{Subject: "u"}, nil, ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sin ref_id: code=%d", rec.Code)
	}

	// feliz -> 200 y propaga el kind al servicio.
	rec = httptest.NewRecorder()
	h.RoomBySource(rec, mkReq(t, "GET", "/api/v1/rooms/by-source?kind=chat&ref_id=chan-1", &authn.Claims{Subject: "u"}, nil, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("by-source: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if f.fromSrcKind != domain.SourceChat {
		t.Errorf("kind = %q", f.fromSrcKind)
	}

	// sin sala viva -> 404 (no crea nada).
	f.err = domain.ErrNotFound
	rec = httptest.NewRecorder()
	h.RoomBySource(rec, mkReq(t, "GET", "/api/v1/rooms/by-source?kind=chat&ref_id=chan-1", &authn.Claims{Subject: "u"}, nil, ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("sin sala: code=%d", rec.Code)
	}
}

func TestListRooms(t *testing.T) {
	f := &fakeSvc{rooms: []domain.Room{{ID: uuid.New(), Name: "r1"}}}
	h := newHandlers(f)

	// status inválido -> 400.
	rec := httptest.NewRecorder()
	h.ListRooms(rec, mkReq(t, "GET", "/api/v1/rooms?status=bogus", &authn.Claims{Subject: "u"}, nil, ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status inválido: code=%d", rec.Code)
	}

	// Feliz -> 200 con clave rooms.
	rec = httptest.NewRecorder()
	h.ListRooms(rec, mkReq(t, "GET", "/api/v1/rooms?status=active&limit=10", &authn.Claims{Subject: "u"}, nil, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("list: code=%d", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if _, ok := body["rooms"]; !ok {
		t.Error("falta clave rooms")
	}
}

func TestGetRoom(t *testing.T) {
	f := &fakeSvc{room: domain.Room{ID: uuid.New(), Name: "r"}}
	h := newHandlers(f)

	// ID inválido -> 400.
	rec := httptest.NewRecorder()
	h.GetRoom(rec, mkReq(t, "GET", "/api/v1/rooms/xxx", &authn.Claims{Subject: "u"}, map[string]string{"id": "not-a-uuid"}, ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("id inválido: code=%d", rec.Code)
	}

	// Feliz -> 200.
	id := uuid.New()
	rec = httptest.NewRecorder()
	h.GetRoom(rec, mkReq(t, "GET", "/api/v1/rooms/"+id.String(), &authn.Claims{Subject: "u"}, map[string]string{"id": id.String()}, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("get: code=%d", rec.Code)
	}

	// No encontrado -> 404.
	f.err = domain.ErrNotFound
	rec = httptest.NewRecorder()
	h.GetRoom(rec, mkReq(t, "GET", "/api/v1/rooms/"+id.String(), &authn.Claims{Subject: "u"}, map[string]string{"id": id.String()}, ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("not found: code=%d", rec.Code)
	}
}

func TestCloseRoom(t *testing.T) {
	id := uuid.New()

	// El anfitrión (creador) puede cerrar -> 200.
	f := &fakeSvc{room: domain.Room{ID: id, Name: "r", CreatedBy: "host"}}
	h := newHandlers(f)
	rec := httptest.NewRecorder()
	h.CloseRoom(rec, mkReq(t, "DELETE", "/api/v1/rooms/"+id.String(), &authn.Claims{Subject: "host"}, map[string]string{"id": id.String()}, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("host cierra: code=%d", rec.Code)
	}

	// Un no-anfitrión sin rol admin -> 403.
	rec = httptest.NewRecorder()
	h.CloseRoom(rec, mkReq(t, "DELETE", "/api/v1/rooms/"+id.String(), &authn.Claims{Subject: "otro"}, map[string]string{"id": id.String()}, ""))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("no-host: code=%d", rec.Code)
	}

	// Un admin (no creador) sí puede -> 200.
	rec = httptest.NewRecorder()
	h.CloseRoom(rec, mkReq(t, "DELETE", "/api/v1/rooms/"+id.String(), &authn.Claims{Subject: "admin", Roles: []string{"admin"}}, map[string]string{"id": id.String()}, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("admin cierra: code=%d", rec.Code)
	}
}

func TestJoinRoom(t *testing.T) {
	id := uuid.New()
	f := &fakeSvc{
		room: domain.Room{ID: id, Name: "r"},
		join: service.JoinResult{Token: "jwt", Role: domain.RoleSpeaker},
	}
	h := newHandlers(f)

	// Sin subject en claims -> 401.
	rec := httptest.NewRecorder()
	h.JoinRoom(rec, mkReq(t, "POST", "/api/v1/rooms/"+id.String()+"/join", &authn.Claims{}, map[string]string{"id": id.String()}, ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin subject: code=%d", rec.Code)
	}

	// role inválido -> 400.
	rec = httptest.NewRecorder()
	h.JoinRoom(rec, mkReq(t, "POST", "/api/v1/rooms/"+id.String()+"/join", &authn.Claims{Subject: "u"}, map[string]string{"id": id.String()}, `{"role":"boss"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("role inválido: code=%d", rec.Code)
	}

	// Feliz (cuerpo vacío usa defaults) -> 200 con token.
	rec = httptest.NewRecorder()
	h.JoinRoom(rec, mkReq(t, "POST", "/api/v1/rooms/"+id.String()+"/join", &authn.Claims{Subject: "u", Name: "Ana"}, map[string]string{"id": id.String()}, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("join: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var jr service.JoinResult
	_ = json.Unmarshal(rec.Body.Bytes(), &jr)
	if jr.Token != "jwt" {
		t.Errorf("token = %q", jr.Token)
	}

	// Sala cerrada -> 409.
	f.err = domain.ErrRoomClosed
	rec = httptest.NewRecorder()
	h.JoinRoom(rec, mkReq(t, "POST", "/api/v1/rooms/"+id.String()+"/join", &authn.Claims{Subject: "u"}, map[string]string{"id": id.String()}, ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("sala cerrada: code=%d", rec.Code)
	}
}

func TestListParticipantsAndRecordings(t *testing.T) {
	id := uuid.New()
	f := &fakeSvc{
		parts: []domain.Participant{{ID: uuid.New(), Identity: "u"}},
		recs:  []domain.Recording{{ID: uuid.New(), EgressID: "e1"}},
	}
	h := newHandlers(f)

	rec := httptest.NewRecorder()
	h.ListParticipants(rec, mkReq(t, "GET", "/api/v1/rooms/"+id.String()+"/participants?present=1", &authn.Claims{Subject: "u"}, map[string]string{"id": id.String()}, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("participants: code=%d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ListRecordings(rec, mkReq(t, "GET", "/api/v1/rooms/"+id.String()+"/recordings", &authn.Claims{Subject: "u"}, map[string]string{"id": id.String()}, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("recordings: code=%d", rec.Code)
	}
}

func TestRemoveParticipant(t *testing.T) {
	id := uuid.New()
	// La sala la creó "ana": ella es la ANFITRIONA.
	f := &fakeSvc{room: domain.Room{ID: id, Name: "r", CreatedBy: "ana"}}
	h := newHandlers(f)

	quitar := func(c *authn.Claims, identity string) int {
		rec := httptest.NewRecorder()
		h.RemoveParticipant(rec, mkReq(t, "DELETE",
			"/api/v1/rooms/"+id.String()+"/participants/"+identity, c,
			map[string]string{"id": id.String(), "identity": identity}, ""))
		return rec.Code
	}

	// Falta identity -> 400.
	if got := quitar(&authn.Claims{Subject: "ana"}, ""); got != http.StatusBadRequest {
		t.Fatalf("sin identity: code=%d", got)
	}

	// El ANFITRIÓN puede, sin ser admin de la organización. Este es el caso que
	// antes daba 403: la ruta exigía RequireRole("admin","owner"), así que quien
	// convocaba la reunión no podía echar a un intruso de su propia sala.
	if got := quitar(&authn.Claims{Subject: "ana"}, "u2"); got != http.StatusNoContent {
		t.Fatalf("el anfitrión debería poder expulsar: code=%d", got)
	}
	if f.removedID != "u2" {
		t.Errorf("removedID = %q", f.removedID)
	}

	// Un admin de la organización que NO es el anfitrión, también.
	if got := quitar(&authn.Claims{Subject: "otro", Roles: []string{"admin"}}, "u4"); got != http.StatusNoContent {
		t.Fatalf("un admin debería poder expulsar: code=%d", got)
	}

	// Un participante cualquiera, NO. Esto es lo que la puerta tiene que impedir:
	// sin ello, quien entra por un enlace podría echar al resto.
	if got := quitar(&authn.Claims{Subject: "intruso"}, "ana"); got != http.StatusForbidden {
		t.Fatalf("un tercero NO debería poder expulsar: code=%d", got)
	}

	// Error del servicio -> 500.
	f.removeErr = errors.New("boom")
	if got := quitar(&authn.Claims{Subject: "ana"}, "u3"); got != http.StatusInternalServerError {
		t.Fatalf("error servicio: code=%d", got)
	}
}

func TestMuteParticipant(t *testing.T) {
	// Silenciar es la otra mitad de la moderación, y la que se necesita más a
	// menudo: un micrófono abierto se corta sin echar a nadie de la reunión.
	id := uuid.New()
	f := &fakeSvc{room: domain.Room{ID: id, Name: "r", CreatedBy: "ana"}}
	h := newHandlers(f)

	silenciar := func(c *authn.Claims, identity string) int {
		rec := httptest.NewRecorder()
		h.MuteParticipant(rec, mkReq(t, "POST",
			"/api/v1/rooms/"+id.String()+"/participants/"+identity+"/mute", c,
			map[string]string{"id": id.String(), "identity": identity}, ""))
		return rec.Code
	}

	if got := silenciar(&authn.Claims{Subject: "ana"}, "ruidoso"); got != http.StatusNoContent {
		t.Fatalf("el anfitrión debería poder silenciar: code=%d", got)
	}
	if f.mutedID != "ruidoso" {
		t.Errorf("mutedID = %q", f.mutedID)
	}
	if got := silenciar(&authn.Claims{Subject: "otro", Roles: []string{"owner"}}, "x"); got != http.StatusNoContent {
		t.Fatalf("un owner debería poder silenciar: code=%d", got)
	}
	if got := silenciar(&authn.Claims{Subject: "cualquiera"}, "ana"); got != http.StatusForbidden {
		t.Fatalf("un tercero NO debería poder silenciar el micro de otro: code=%d", got)
	}
	if got := silenciar(&authn.Claims{Subject: "ana"}, ""); got != http.StatusBadRequest {
		t.Fatalf("sin identity: code=%d", got)
	}
}

// TestGrabarExigeSerAnfitrionOAdmin comprueba la puerta de la grabación.
//
// Grabar a los demás no es una acción cualquiera de un participante: es la que
// convierte una conversación en un fichero que sobrevive a la reunión. La puerta
// tiene que ser la misma que expulsar o silenciar, y sobre todo NO puede estar
// abierta para cualquiera que haya entrado por un enlace.
func TestGrabarExigeSerAnfitrionOAdmin(t *testing.T) {
	id := uuid.New()
	f := &fakeSvc{room: domain.Room{ID: id, Name: "r", CreatedBy: "ana"}}
	h := newHandlers(f)

	grabar := func(c *authn.Claims) int {
		rec := httptest.NewRecorder()
		h.StartRecording(rec, mkReq(t, "POST", "/api/v1/rooms/"+id.String()+"/recording", c,
			map[string]string{"id": id.String()}, ""))
		return rec.Code
	}
	parar := func(c *authn.Claims) int {
		rec := httptest.NewRecorder()
		h.StopRecording(rec, mkReq(t, "DELETE", "/api/v1/rooms/"+id.String()+"/recording", c,
			map[string]string{"id": id.String()}, ""))
		return rec.Code
	}

	// El anfitrión, sin ser admin de la organización.
	if got := grabar(&authn.Claims{Subject: "ana"}); got != http.StatusAccepted {
		t.Fatalf("el anfitrión debería poder grabar: code=%d", got)
	}
	if !f.grabando {
		t.Error("no se llamó al servicio para arrancar la grabación")
	}
	if got := parar(&authn.Claims{Subject: "ana"}); got != http.StatusNoContent {
		t.Fatalf("el anfitrión debería poder parar: code=%d", got)
	}

	// Un admin de la organización que no es el anfitrión.
	if got := grabar(&authn.Claims{Subject: "otro", Roles: []string{"admin"}}); got != http.StatusAccepted {
		t.Fatalf("un admin debería poder grabar: code=%d", got)
	}

	// Cualquier otro, NO. Este es el caso que de verdad importa.
	if got := grabar(&authn.Claims{Subject: "invitado"}); got != http.StatusForbidden {
		t.Errorf("un participante cualquiera NO debería poder grabar a los demás: code=%d", got)
	}
	if got := parar(&authn.Claims{Subject: "invitado"}); got != http.StatusForbidden {
		t.Errorf("un participante cualquiera NO debería poder parar la grabación: code=%d", got)
	}
}

// TestDescargarGrabacionExigeHaberEstado comprueba la verja de la descarga.
//
// Es la puerta más fácil de dejar abierta de todo el servicio: `ListRecordings`
// solo pide sesión del tenant, y copiar ese criterio aquí significaría que
// cualquier empleado con una URL puede ver el vídeo de una reunión de recursos
// humanos. La verja tiene que ser más ancha que moderar —quien estuvo dentro
// tiene derecho a su acta— y más estrecha que la organización entera.
func TestDescargarGrabacionExigeHaberEstado(t *testing.T) {
	id, recID := uuid.New(), uuid.New()
	f := &fakeSvc{
		room:  domain.Room{ID: id, Name: "r", CreatedBy: "ana"},
		parts: []domain.Participant{{Identity: "luis"}},
	}
	h := newHandlers(f)

	descargar := func(c *authn.Claims) int {
		rec := httptest.NewRecorder()
		h.GetRecording(rec, mkReq(t, "GET",
			"/api/v1/rooms/"+id.String()+"/recordings/"+recID.String(), c,
			map[string]string{"id": id.String(), "recID": recID.String()}, ""))
		return rec.Code
	}

	if got := descargar(&authn.Claims{Subject: "ana"}); got != http.StatusOK {
		t.Errorf("el anfitrión debería poder descargar: code=%d", got)
	}
	if got := descargar(&authn.Claims{Subject: "otro", Roles: []string{"admin"}}); got != http.StatusOK {
		t.Errorf("un admin debería poder descargar: code=%d", got)
	}
	// Ya se fue de la reunión (`left_at` puesto) y sigue teniendo derecho al acta:
	// por eso la comprobación NO filtra por presentes.
	if got := descargar(&authn.Claims{Subject: "luis"}); got != http.StatusOK {
		t.Errorf("quien participó debería poder descargar: code=%d", got)
	}
	if got := descargar(&authn.Claims{Subject: "curioso"}); got != http.StatusForbidden {
		t.Fatalf("un tercero del tenant NO debería poder descargar: code=%d", got)
	}

	// Un identificador de grabación que no es un UUID se rechaza antes de tocar
	// el almacén: si no, la clave la elegiría quien llama.
	rec := httptest.NewRecorder()
	h.GetRecording(rec, mkReq(t, "GET", "/api/v1/rooms/"+id.String()+"/recordings/x",
		&authn.Claims{Subject: "ana"}, map[string]string{"id": id.String(), "recID": "x"}, ""))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("recID no-UUID: code=%d", rec.Code)
	}
}

// TestSilenciarATodosNoSeSilenciaAQuienLoPide: el botón no puede ser una trampa.
//
// Quien pide silencio es quien dirige la reunión y acaba de hacerlo para poder
// hablar: callarle a él también convierte «silenciar a todos» en un tiro en el
// pie. La exclusión viaja hasta el servicio; aquí se comprueba que llega.
func TestSilenciarATodosNoSeSilenciaAQuienLoPide(t *testing.T) {
	id := uuid.New()
	f := &fakeSvc{room: domain.Room{ID: id, Name: "r", CreatedBy: "ana"}}
	h := newHandlers(f)

	todos := func(c *authn.Claims) int {
		rec := httptest.NewRecorder()
		h.MuteAll(rec, mkReq(t, "POST", "/api/v1/rooms/"+id.String()+"/participants/mute-all", c,
			map[string]string{"id": id.String()}, ""))
		return rec.Code
	}

	if got := todos(&authn.Claims{Subject: "ana"}); got != http.StatusOK {
		t.Fatalf("el anfitrión debería poder silenciar a todos: code=%d", got)
	}
	if f.muteAllExcept != "ana" {
		t.Errorf("excepto = %q, se esperaba a quien lo pidió", f.muteAllExcept)
	}
	if got := todos(&authn.Claims{Subject: "otro", Roles: []string{"admin"}}); got != http.StatusOK {
		t.Errorf("un admin debería poder: code=%d", got)
	}
	if got := todos(&authn.Claims{Subject: "cualquiera"}); got != http.StatusForbidden {
		t.Fatalf("un participante cualquiera NO debería poder callar la sala: code=%d", got)
	}
}

// COM-10. El nombre visible y los metadatos de la sala no los pone el navegador.
func TestElNombreYLosMetadatosNoLosPoneElCliente(t *testing.T) {
	id := uuid.New()
	f := &fakeSvc{room: domain.Room{ID: id, Name: "r"}, join: service.JoinResult{Token: "jwt", Role: domain.RoleSpeaker}}
	h := newHandlers(f)
	rec := httptest.NewRecorder()
	h.JoinRoom(rec, mkReq(t, "POST", "/api/v1/rooms/"+id.String()+"/join", &authn.Claims{Subject: "u", Name: "Ana"},
		map[string]string{"id": id.String()}, `{"display_name":"La Directora"}`))
	if f.nombreAlEntrar != "Ana" {
		t.Fatalf("nombre al entrar = %q; quiero el del token", f.nombreAlEntrar)
	}
	rec = httptest.NewRecorder()
	h.CreateRoom(rec, mkReq(t, "POST", "/api/v1/rooms", &authn.Claims{Subject: "u", Name: "Ana"}, nil,
		`{"title":"x","metadata":{"aviso":"falso"}}`))
	if len(f.createIn.Metadata) != 0 {
		t.Fatalf("metadatos del cliente en la sala: %v", f.createIn.Metadata)
	}
}
