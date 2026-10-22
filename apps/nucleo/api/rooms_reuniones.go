// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Las reuniones que no cuelgan de un canal (el panel «Reuniones» de la web,
// MeetingsPanel.tsx). Las sirve el MISMO servicio que la mensajería desde la fusión
// con Meet (services/rooms/internal/media/http/server.go), bajo /api/v1/rooms.

// RoomsMeeting es una sala.
type RoomsMeeting struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Title     string     `json:"title"`
	Status    string     `json:"status"`
	CreatedBy string     `json:"created_by,omitempty"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	// Locked es la puerta cerrada: nadie NUEVO entra; el anfitrión y los admin sí.
	Locked bool `json:"locked,omitempty"`
	// LinkKey es la clave del enlace de una reunión suelta: va en el enlace que se copia.
	LinkKey string `json:"link_key,omitempty"`
}

// Titulo es `r.title || r.name`.
func (m RoomsMeeting) Titulo() string {
	if m.Title != "" {
		return m.Title
	}
	return m.Name
}

// RoomsMeetingParticipant es quien está (o estuvo) en una sala.
type RoomsMeetingParticipant struct {
	ID       string `json:"id"`
	Identity string `json:"identity"`
	Name     string `json:"name,omitempty"`
}

// RoomsMeetingRecording es una grabación: solo las `completed` se descargan.
type RoomsMeetingRecording struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
	DurationSec int64  `json:"duration_sec,omitempty"`
}

// Meetings son las reuniones activas de la organización (`listMeetings('active')`).
func (r *Rooms) Meetings(ctx context.Context) ([]RoomsMeeting, error) {
	out, err := JSON[struct {
		Rooms []RoomsMeeting `json:"rooms"`
	}](ctx, r.c, Request{Path: "/rooms", Query: url.Values{"status": {"active"}, "limit": {"50"}}})
	return out.Rooms, err
}

// CreateMeeting convoca una reunión suelta con ese título.
func (r *Rooms) CreateMeeting(ctx context.Context, titulo string) (RoomsMeeting, error) {
	return PostJSON[RoomsMeeting](ctx, r.c, "/rooms", struct {
		Title string `json:"title"`
	}{titulo})
}

// CloseMeeting la cierra (anfitrión o admin).
func (r *Rooms) CloseMeeting(ctx context.Context, id string) error {
	return r.sinCuerpo(ctx, http.MethodDelete, "/rooms/"+url.PathEscape(id))
}

// MeetingParticipants son los que siguen dentro.
func (r *Rooms) MeetingParticipants(ctx context.Context, id string) ([]RoomsMeetingParticipant, error) {
	out, err := JSON[struct {
		Participants []RoomsMeetingParticipant `json:"participants"`
	}](ctx, r.c, Request{Path: "/rooms/" + url.PathEscape(id) + "/participants", Query: url.Values{"present": {"1"}}})
	return out.Participants, err
}

// MeetingRecordings son sus grabaciones.
func (r *Rooms) MeetingRecordings(ctx context.Context, id string) ([]RoomsMeetingRecording, error) {
	out, err := JSON[struct {
		Recordings []RoomsMeetingRecording `json:"recordings"`
	}](ctx, r.c, Request{Path: "/rooms/" + url.PathEscape(id) + "/recordings"})
	return out.Recordings, err
}

// RecordingURL es la URL firmada de una grabación terminada.
func (r *Rooms) RecordingURL(ctx context.Context, id, grabacion string) (string, error) {
	out, err := JSON[struct {
		URL string `json:"url"`
	}](ctx, r.c, Request{Path: "/rooms/" + url.PathEscape(id) + "/recordings/" + url.PathEscape(grabacion)})
	return out.URL, err
}

// KnockMeeting llama a la puerta de una sala cerrada: el anfitrión recibe el aviso. Con
// la puerta ya abierta el servicio contesta 409 y se puede entrar sin más.
//
// `clave` es la del enlace de una reunión suelta ("" en la de un canal): sin ella solo
// llaman quien la convocó y quien ya estuvo dentro.
func (r *Rooms) KnockMeeting(ctx context.Context, id, clave string) error {
	resp, err := r.c.Do(ctx, Request{
		Method: http.MethodPost, Path: "/rooms/" + url.PathEscape(id) + "/knock",
		Header: conClave(nil, clave),
	})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// CabeceraClave es la cabecera con la clave del enlace de una reunión suelta. Va en una
// cabecera y no en la ruta, que acaba en los registros de acceso.
const CabeceraClave = "X-Sytue-Room-Key"

func conClave(h http.Header, clave string) http.Header {
	if clave == "" {
		return h
	}
	if h == nil {
		h = http.Header{}
	}
	h.Set(CabeceraClave, clave)
	return h
}

// RemoveMeetingParticipant echa a alguien de la sala (anfitrión o admin).
func (r *Rooms) RemoveMeetingParticipant(ctx context.Context, id, identidad string) error {
	return r.sinCuerpo(ctx, http.MethodDelete, "/rooms/"+url.PathEscape(id)+"/participants/"+url.PathEscape(identidad))
}

// ---- Entrar en la llamada ----
//
// Es lo que el BFF de la web hace por dentro (`lib/meet.ts`: `joinMeeting`,
// `callForChannel`, `knockChannel`) contra el MISMO servicio y las mismas rutas: el
// token de LiveKit lo mintea `POST /rooms/{id}/join` tras validar la sesión de Sytue ID,
// y el cliente nativo no tiene un endpoint propio.

// RoomsCall es la conexión con el SFU: lo que `Room.connect(serverUrl, token)` de la web
// recibe, más la sala.
type RoomsCall struct {
	Room        RoomsMeeting         `json:"room"`
	Token       string               `json:"token"`
	LiveKitURL  string               `json:"livekit_url"`
	Identity    string               `json:"identity"`
	Role        string               `json:"role,omitempty"`
	JoinID      string               `json:"join_id"`
	Encryption  *RoomsCallEncryption `json:"encryption,omitempty"`
	ExpiresAtMS int64                `json:"expires_at_ms"`
}

// RoomsCallEncryption binds an SFU connection to a short-lived authorized
// Matrix device roster. It contains no media key; clients exchange keys through
// verified Olm devices and stop at ExpiresAtMS even if the network is unavailable.
type RoomsCallEncryption struct {
	CallID         string            `json:"call_id"`
	LiveKitRoom    string            `json:"livekit_room"`
	MatrixRoomID   string            `json:"matrix_room_id"`
	Generation     int64             `json:"generation"`
	ExpiresAtMS    int64             `json:"expires_at_ms"`
	LeaderSubject  string            `json:"leader_subject"`
	LeaderDeviceID string            `json:"leader_device_id"`
	Members        []RoomsCallMember `json:"members"`
}
type RoomsCallMember struct {
	Subject  string `json:"subject"`
	DeviceID string `json:"device_id"`
}

// ErrSalaCerrada dice si un error de entrar es la puerta echada (`room_locked`), que
// tiene respuesta —llamar— y no es una avería.
func ErrSalaCerrada(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == http.StatusConflict && e.Code() == "room_locked"
}

// JoinMeeting entra en una reunión por su id y la clave de su enlace (`?rooms=…&clave=…`;
// "" en la de un canal).
func (r *Rooms) JoinMeeting(ctx context.Context, id, nombre, clave string) (RoomsCall, error) {
	return r.JoinMeetingDevice(ctx, id, nombre, clave, "")
}

// JoinMeetingDevice requests a receive-only SFU token tied to the current Matrix
// device. Publishing is enabled only after CallEncryptionReady succeeds.
func (r *Rooms) JoinMeetingDevice(ctx context.Context, id, nombre, clave, device string) (RoomsCall, error) {
	datos, err := json.Marshal(struct {
		DisplayName    string `json:"display_name,omitempty"`
		MatrixDeviceID string `json:"matrix_device_id,omitempty"`
	}{nombre, device})
	if err != nil {
		return RoomsCall{}, err
	}
	return JSON[RoomsCall](ctx, r.c, Request{
		Method: http.MethodPost, Path: "/rooms/" + url.PathEscape(id) + "/join", Body: datos,
		Header: conClave(http.Header{"Content-Type": {"application/json"}}, clave),
	})
}

// CallChannel es «Llamar» en un canal: crea o reutiliza la sala del canal (lo decide el
// servicio, no el cliente) y entra.
func (r *Rooms) CallChannel(ctx context.Context, canal, titulo, nombre string) (RoomsCall, error) {
	return r.CallChannelDevice(ctx, canal, titulo, nombre, "")
}

func (r *Rooms) CallChannelDevice(ctx context.Context, canal, titulo, nombre, device string) (RoomsCall, error) {
	if strings.TrimSpace(titulo) == "" {
		titulo = "Sala del canal" // el saneo del BFF: el servicio exige título
	}
	sala, err := PostJSON[RoomsMeeting](ctx, r.c, "/rooms/from-source",
		map[string]string{"kind": "chat", "ref_id": canal, "title": titulo})
	if err != nil {
		return RoomsCall{}, err
	}
	return r.JoinMeetingDevice(ctx, sala.ID, nombre, "", device)
}

// CallEncryption refreshes authorization metadata only. The join nonce prevents
// delayed responses from a previous device connection from authorizing this one.
func (r *Rooms) CallEncryption(ctx context.Context, id, device, joinID string) (RoomsCallEncryption, error) {
	return JSON[RoomsCallEncryption](ctx, r.c, Request{Path: r.sala(id) + "/encryption", Query: url.Values{"matrix_device_id": {device}, "join_id": {joinID}}})
}

type callEncryptionRequest struct {
	MatrixDeviceID string `json:"matrix_device_id"`
	JoinID         string `json:"join_id"`
	Generation     int64  `json:"generation"`
}

func (r *Rooms) CallEncryptionReady(ctx context.Context, id, device, joinID string, generation int64) error {
	return r.conCuerpo(ctx, http.MethodPost, r.sala(id)+"/encryption/ready", callEncryptionRequest{device, joinID, generation})
}

func (r *Rooms) CallEncryptionReconnect(ctx context.Context, id, device, joinID string, generation int64) (RoomsCall, error) {
	return PostJSON[RoomsCall](ctx, r.c, r.sala(id)+"/encryption/reconnect", callEncryptionRequest{device, joinID, generation})
}

// KnockChannel llama a la puerta de la sala cerrada de un canal. Hace falta aparte de
// KnockMeeting porque quien se la encuentra cerrada no llegó a recibir el id: el join
// falló antes. La sala se busca por su origen, sin crearla.
func (r *Rooms) KnockChannel(ctx context.Context, canal string) error {
	sala, err := JSON[RoomsMeeting](ctx, r.c, Request{
		Path:  "/rooms/by-source",
		Query: url.Values{"kind": {"chat"}, "ref_id": {canal}},
	})
	if err != nil {
		return err
	}
	return r.KnockMeeting(ctx, sala.ID, "")
}

// PostCall anota la llamada en el historial del canal: el mensaje de sistema
// `type=call` con el cuerpo vacío que la web publica al empezar o unirse
// (`postCallMessage`). La tarjeta llega a todos por el canal en vivo.
func (r *Rooms) PostCall(ctx context.Context, canal string) error {
	return r.conCuerpo(ctx, http.MethodPost, "/channels/"+url.PathEscape(canal)+"/messages",
		map[string]string{"body": "", "type": "call"})
}

// ---- Moderar la llamada (anfitrión o admin; lo autoriza el servicio) ----
//
// Las rutas de `api/meeting.ts` de la web, contra el servicio directamente.

// RoomsKnock es quien espera en la puerta de una sala cerrada.
type RoomsKnock struct {
	Identity string `json:"identity"`
	Name     string `json:"name,omitempty"`
}

func (r *Rooms) sala(id string) string { return "/rooms/" + url.PathEscape(id) }

// MuteMeetingParticipant le corta el micrófono a alguien, sin echarlo.
func (r *Rooms) MuteMeetingParticipant(ctx context.Context, id, identidad string) error {
	return r.sinCuerpo(ctx, http.MethodPost, r.sala(id)+"/participants/"+url.PathEscape(identidad)+"/mute")
}

// MuteAllMeeting calla a todos menos a quien lo pide y dice a cuántos.
func (r *Rooms) MuteAllMeeting(ctx context.Context, id string) (int, error) {
	out, err := PostJSON[struct {
		Muted int `json:"muted"`
	}](ctx, r.c, r.sala(id)+"/participants/mute-all", struct{}{})
	return out.Muted, err
}

// SetMeetingSpeech devuelve (puede) o quita la palabra a alguien.
func (r *Rooms) SetMeetingSpeech(ctx context.Context, id, identidad string, puede bool) error {
	metodo := http.MethodDelete
	if puede {
		metodo = http.MethodPost
	}
	return r.sinCuerpo(ctx, metodo, r.sala(id)+"/participants/"+url.PathEscape(identidad)+"/speech")
}

// LockMeeting cierra (o reabre) la puerta de la sala: nadie NUEVO entra.
func (r *Rooms) LockMeeting(ctx context.Context, id string, cerrar bool) error {
	metodo := http.MethodDelete
	if cerrar {
		metodo = http.MethodPost
	}
	return r.sinCuerpo(ctx, metodo, r.sala(id)+"/lock")
}

// MeetingKnocks es quién espera en la puerta.
func (r *Rooms) MeetingKnocks(ctx context.Context, id string) ([]RoomsKnock, error) {
	out, err := JSON[struct {
		Knocks []RoomsKnock `json:"knocks"`
	}](ctx, r.c, Request{Path: r.sala(id) + "/knocks"})
	return out.Knocks, err
}

// AnswerMeetingKnock abre la puerta a alguien (admitir) o lo retira de la cola.
func (r *Rooms) AnswerMeetingKnock(ctx context.Context, id, identidad string, admitir bool) error {
	metodo := http.MethodDelete
	if admitir {
		metodo = http.MethodPost
	}
	return r.sinCuerpo(ctx, metodo, r.sala(id)+"/knocks/"+url.PathEscape(identidad))
}

// RecordMeeting empieza (o para) la grabación. El fichero lo sube el egress al
// terminar; que está grabando lo dice el SFU a todos, no esta respuesta.
func (r *Rooms) RecordMeeting(ctx context.Context, id string, grabar bool) error {
	if grabar {
		return r.conCuerpo(ctx, http.MethodPost, r.sala(id)+"/recording", struct{}{})
	}
	return r.sinCuerpo(ctx, http.MethodDelete, r.sala(id)+"/recording")
}
