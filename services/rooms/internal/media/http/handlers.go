// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package meethttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/service"
)

// roomService es la superficie mínima del servicio de aplicación que consumen
// los handlers REST. La satisface *service.Service en producción; declararla como
// interfaz permite inyectar un fake en los tests HTTP sin PostgreSQL ni LiveKit.
type roomService interface {
	CreateRoom(ctx context.Context, in service.CreateRoomInput) (domain.Room, error)
	CreateRoomFromSource(ctx context.Context, kind domain.SourceKind, refID, title, createdBy string, maxParticipants int) (domain.Room, error)
	RoomBySource(ctx context.Context, kind domain.SourceKind, refID string) (domain.Room, error)
	GetRoom(ctx context.Context, id uuid.UUID) (domain.Room, error)
	ListRooms(ctx context.Context, status domain.RoomStatus, limit int) ([]domain.Room, error)
	ListParticipants(ctx context.Context, roomID uuid.UUID, onlyPresent bool) ([]domain.Participant, error)
	ListRecordings(ctx context.Context, roomID uuid.UUID) ([]domain.Recording, error)
	CloseRoom(ctx context.Context, id uuid.UUID) (domain.Room, error)
	JoinRoom(ctx context.Context, roomID uuid.UUID, identity, displayName string, requested domain.Role, isAdmin bool) (service.JoinResult, error)
	RemoveParticipant(ctx context.Context, roomID uuid.UUID, identity string) error
	MuteParticipant(ctx context.Context, roomID uuid.UUID, identity string) error
	MuteAll(ctx context.Context, roomID uuid.UUID, excepto string) (int, error)
	SetCanPublish(ctx context.Context, roomID uuid.UUID, identity string, canPublish bool) error
	LockRoom(ctx context.Context, roomID uuid.UUID, locked bool) (domain.Room, error)
	Knock(ctx context.Context, roomID uuid.UUID, identity, name string) (domain.Admission, error)
	Admit(ctx context.Context, roomID uuid.UUID, identity string, admitido bool) error
	ListKnocks(ctx context.Context, roomID uuid.UUID) ([]domain.Admission, error)
	StartRecording(ctx context.Context, roomID uuid.UUID) (string, error)
	StopRecording(ctx context.Context, roomID uuid.UUID) error
	RecordingURL(ctx context.Context, roomID, recordingID uuid.UUID) (string, error)
	DeleteRecording(ctx context.Context, roomID, recordingID uuid.UUID) error
}

// Handlers agrupa los manejadores de la API REST de Meet.
type Handlers struct {
	svc roomService
	cfg config.Config
}

// toAPIError traduce errores de dominio a *httpx.Error canónicos.
func toAPIError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrEncryptionRequired):
		return httpx.NewError(http.StatusConflict, "matrix_device_required", "Connect a verified Matrix device before joining this call.")
	case errors.Is(err, domain.ErrEncryptionUnavailable):
		return httpx.NewError(http.StatusServiceUnavailable, "call_encryption_unavailable", "Encrypted calling is temporarily unavailable.")
	case errors.Is(err, domain.ErrEncryptionGeneration):
		return httpx.NewError(http.StatusConflict, "call_encryption_changed", "Refresh call encryption before publishing.")
	case errors.Is(err, domain.ErrEncryptedOperation):
		return httpx.NewError(http.StatusConflict, "encrypted_call_operation_unavailable", "This operation is unavailable for encrypted calls.")
	case errors.Is(err, domain.ErrNotFound):
		return httpx.ErrNotFound
	case errors.Is(err, domain.ErrConflict):
		return httpx.ErrConflict.WithMessage("A room with that name already exists.")
	case errors.Is(err, domain.ErrInvalidName):
		return httpx.ErrBadRequest.WithMessage("Invalid room name or parameter.")
	case errors.Is(err, domain.ErrRoomClosed):
		return httpx.ErrConflict.WithMessage("The room is closed.")
	// Código PROPIO, no el genérico de conflicto: la interfaz tiene que poder
	// decir «el anfitrión ha bloqueado la sala» y no «la reunión ha terminado»,
	// que sería mentira y mandaría a la gente a preguntar por qué se cortó.
	case errors.Is(err, domain.ErrRoomOpen):
		return httpx.ErrConflict.WithMessage("The room is not locked; you can join directly.")
	case errors.Is(err, domain.ErrRoomLocked):
		return httpx.NewError(http.StatusConflict, "room_locked",
			"The host locked this room; no one else can join.")
	case errors.Is(err, domain.ErrRoomFull):
		return httpx.NewError(http.StatusConflict, "room_full", "The room has reached its maximum capacity.")
	case errors.Is(err, domain.ErrPermission):
		return httpx.ErrForbidden
	default:
		return httpx.ErrInternal.Wrap(err)
	}
}

// parseID extrae y valida el UUID del parámetro de ruta "id".
func parseID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, httpx.ErrBadRequest.WithMessage("Invalid identifier.")
	}
	return id, nil
}

// claimsFromCtx devuelve los claims del usuario autenticado.
func claimsFromCtx(r *http.Request) authn.Claims {
	c, _ := authn.FromContext(r.Context())
	return c
}

// ---------------------------------------------------------------------------
// Salas
// ---------------------------------------------------------------------------

type roomSourceRequest struct {
	Kind  string `json:"kind"`
	RefID string `json:"ref_id"`
	Title string `json:"title,omitempty"`
}

type createRoomRequest struct {
	Title           string             `json:"title"`
	Name            string             `json:"name,omitempty"`
	MaxParticipants int                `json:"max_participants,omitempty"`
	Intent          string             `json:"intent,omitempty"`
	Source          *roomSourceRequest `json:"source,omitempty"`
	// Metadata se admite por compatibilidad y se IGNORA: los metadatos de la sala los
	// ve todo participante en LiveKit y los escribe el servicio (COM-10).
	Metadata map[string]any `json:"metadata,omitempty"`
}

// CreateRoom crea una sala y la aprovisiona en LiveKit. Acepta opcionalmente un
// origen cross-app (evento de Calendar o canal de Chat) para el enlace inverso.
func (h *Handlers) CreateRoom(w http.ResponseWriter, r *http.Request) {
	var req createRoomRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var src domain.RoomSource
	if req.Source != nil {
		src = domain.RoomSource{
			Kind:  domain.SourceKind(strings.TrimSpace(req.Source.Kind)),
			RefID: strings.TrimSpace(req.Source.RefID),
			Title: strings.TrimSpace(req.Source.Title),
		}
	}
	room, err := h.svc.CreateRoom(r.Context(), service.CreateRoomInput{
		Title:           strings.TrimSpace(req.Title),
		Name:            strings.TrimSpace(req.Name),
		CreatedBy:       claimsFromCtx(r).Subject,
		MaxParticipants: req.MaxParticipants,
		Intent:          strings.TrimSpace(req.Intent),
		Source:          src,
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, room)
}

type createFromSourceRequest struct {
	Kind            string `json:"kind"` // calendar | chat
	RefID           string `json:"ref_id"`
	Title           string `json:"title"`
	MaxParticipants int    `json:"max_participants,omitempty"`
}

// CreateRoomFromSource crea una sala enlazada a un evento de Sytue Calendar o a
// un canal de Sytue Rooms (intent cross-app "crear sala desde evento"). Emite el
// evento meet.room.created con la referencia de origen para el enlace inverso.
func (h *Handlers) CreateRoomFromSource(w http.ResponseWriter, r *http.Request) {
	var req createFromSourceRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	kind := domain.SourceKind(strings.TrimSpace(req.Kind))
	if !kind.IsCrossAppKind() {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("kind must be calendar or chat."))
		return
	}
	room, err := h.svc.CreateRoomFromSource(r.Context(), kind,
		strings.TrimSpace(req.RefID), strings.TrimSpace(req.Title),
		claimsFromCtx(r).Subject, req.MaxParticipants)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, room)
}

// RoomBySource devuelve la sala VIVA enlazada a un origen cross-app (kind+ref_id)
// SIN crearla — 404 si no hay ninguna. Solo lectura: la usa Sytue Rooms para el
// indicador de «llamada en curso» de un canal sin materializar una sala al sondear.
func (h *Handlers) RoomBySource(w http.ResponseWriter, r *http.Request) {
	kind := domain.SourceKind(strings.TrimSpace(r.URL.Query().Get("kind")))
	if !kind.IsCrossAppKind() {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("kind must be calendar or chat."))
		return
	}
	refID := strings.TrimSpace(r.URL.Query().Get("ref_id"))
	if refID == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("ref_id is required."))
		return
	}
	room, err := h.svc.RoomBySource(r.Context(), kind, refID)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, room)
}

// ListRooms lista las salas del tenant (?status=active&limit=50).
func (h *Handlers) ListRooms(w http.ResponseWriter, r *http.Request) {
	status := domain.RoomStatus(strings.TrimSpace(r.URL.Query().Get("status")))
	if status != "" && !status.Valid() {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("status must be scheduled, active or ended."))
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rooms, err := h.svc.ListRooms(r.Context(), status, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if rooms == nil {
		rooms = []domain.Room{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"rooms": rooms})
}

// GetRoom devuelve una sala por ID.
func (h *Handlers) GetRoom(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	room, err := h.svc.GetRoom(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, room)
}

// CloseRoom finaliza una sala (host/admin) y la cierra en LiveKit.
func (h *Handlers) CloseRoom(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	room, err := h.svc.GetRoom(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if err := puedeModerar(r, room, "end the meeting"); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	closed, err := h.svc.CloseRoom(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, closed)
}

type joinRoomRequest struct {
	MatrixDeviceID string `json:"matrix_device_id,omitempty"`
	// Role solicitado (host|speaker|viewer). El servicio lo acota según permisos.
	Role string `json:"role,omitempty"`
	// DisplayName se admite por compatibilidad y se IGNORA: el nombre que ven los
	// demás lo da Sytue ID (ADR 0040; COM-10). Antes un participante se presentaba
	// como otra persona.
	DisplayName string `json:"display_name,omitempty"`
}

// JoinRoom mintea el JWT de LiveKit para el participante TRAS validar OIDC.
func (h *Handlers) JoinRoom(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req joinRoomRequest
	// El cuerpo es opcional; un cuerpo vacío usa los valores por defecto.
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(w, r, &req); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	c := claimsFromCtx(r)
	if c.Subject == "" {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	name := c.Name
	role := domain.Role(strings.TrimSpace(req.Role))
	if role == "" {
		role = domain.RoleSpeaker
	}
	if !domain.ValidRole(role) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("role must be host, speaker or viewer."))
		return
	}

	res, err := h.svc.JoinRoom(service.WithMatrixDevice(r.Context(), req.MatrixDeviceID), id, c.Subject, name, role, c.HasAnyRole("admin", "owner"))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, res)
}

// ListParticipants lista los participantes de una sala (?present=1 para presentes).
func (h *Handlers) ListParticipants(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	onlyPresent := r.URL.Query().Get("present") == "1"
	parts, err := h.svc.ListParticipants(r.Context(), id, onlyPresent)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if parts == nil {
		parts = []domain.Participant{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"participants": parts})
}

// ListRecordings lista los metadatos de grabación de una sala.
func (h *Handlers) ListRecordings(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	recs, err := h.svc.ListRecordings(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if recs == nil {
		recs = []domain.Recording{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"recordings": recs})
}

// GetRecording entrega una URL temporal para descargar una grabación terminada.
//
// El fichero NO pasa por aquí: se firma una URL contra el bucket y el navegador
// va directo. Lo que sí pasa por aquí es la autorización, y es más ancha que la
// de moderar a propósito —quien estuvo en la reunión tiene derecho a su acta—,
// pero no es «cualquiera del tenant»: una grabación de una reunión de recursos
// humanos no la abre quien pase por la URL con una sesión válida.
func (h *Handlers) GetRecording(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	recID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "recID")))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid recording identifier."))
		return
	}
	room, err := h.svc.GetRoom(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if err := h.puedeVerGrabacion(r, room); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	u, err := h.svc.RecordingURL(r.Context(), id, recID)
	if err != nil {
		if errors.Is(err, service.ErrGrabacionNoConfigurada) {
			httpx.Fail(w, r, httpx.ErrNotFound.WithMessage("This installation does not keep recordings."))
			return
		}
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"url": u})
}

// DeleteRecording suprime una grabación: el fichero del bucket y su fila.
//
// La puerta es la de MODERAR, no la de ver. Descargar lo puede hacer quien
// estuvo en la reunión; borrar la conversación entera de un equipo, solo el
// anfitrión o un administrador. Con la puerta de ver, cualquier participante
// podría destruir el acta de todos los demás.
//
// Es lo que hacía falta para que el derecho de supresión del RGPD se pueda
// ejercer aquí: hasta el 2026-08-18 una grabación subida no se podía borrar por
// ningún camino —ni endpoint, ni expiración en el bucket—.
func (h *Handlers) DeleteRecording(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	recID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "recID")))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid recording identifier."))
		return
	}
	room, err := h.svc.GetRoom(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if err := puedeModerar(r, room, "delete a recording"); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.DeleteRecording(r.Context(), id, recID); err != nil {
		if errors.Is(err, service.ErrGrabacionNoConfigurada) {
			httpx.Fail(w, r, httpx.ErrNotFound.WithMessage("This installation does not keep recordings."))
			return
		}
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// puedeVerGrabacion autoriza la descarga: el anfitrión, un admin de la
// organización, o quien estuvo dentro de esa sala (aunque ya se haya ido).
func (h *Handlers) puedeVerGrabacion(r *http.Request, room domain.Room) error {
	c := claimsFromCtx(r)
	if room.CreatedBy == c.Subject || c.HasAnyRole("admin", "owner") {
		return nil
	}
	// `onlyPresent=false`: la reunión terminó hace días, no queda nadie dentro.
	partes, err := h.svc.ListParticipants(r.Context(), room.ID, false)
	if err != nil {
		return toAPIError(err)
	}
	for _, p := range partes {
		if p.Identity == c.Subject {
			return nil
		}
	}
	return httpx.ErrForbidden.WithMessage("Only people who took part in the meeting can view its recording.")
}

// puedeModerar autoriza una acción de moderación sobre una sala: el ANFITRIÓN
// que la creó, o un admin/owner de la organización.
//
// Vive en un solo sitio a propósito. Esta regla estaba escrita a mano en
// `CloseRoom` y la ruta de expulsar exigía en cambio `RequireRole("admin",
// "owner")`, con lo que el anfitrión de su propia reunión no podía echar a nadie
// —aunque el JWT que se le mintea SÍ le da RoomAdmin en LiveKit—. Dos reglas
// para la misma pregunta acaban divergiendo; esta es la respuesta.
func puedeModerar(r *http.Request, room domain.Room, accion string) error {
	c := claimsFromCtx(r)
	if room.CreatedBy == c.Subject || c.HasAnyRole("admin", "owner") {
		return nil
	}
	return httpx.ErrForbidden.WithMessage(
		fmt.Sprintf("Only the host or an administrator can %s.", accion),
	)
}

// moderar resuelve lo común a expulsar y silenciar: leer la sala, autorizar por
// anfitrión y validar la identidad del participante.
func (h *Handlers) moderar(w http.ResponseWriter, r *http.Request, accion string) (uuid.UUID, string, bool) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return uuid.Nil, "", false
	}
	identity := strings.TrimSpace(chi.URLParam(r, "identity"))
	if identity == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Missing the participant's identity."))
		return uuid.Nil, "", false
	}
	room, err := h.svc.GetRoom(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return uuid.Nil, "", false
	}
	if err := puedeModerar(r, room, accion); err != nil {
		httpx.Fail(w, r, err)
		return uuid.Nil, "", false
	}
	return id, identity, true
}

// RemoveParticipant expulsa a un participante de la sala (anfitrión o admin).
func (h *Handlers) RemoveParticipant(w http.ResponseWriter, r *http.Request) {
	id, identity, ok := h.moderar(w, r, "remove a participant")
	if !ok {
		return
	}
	if err := h.svc.RemoveParticipant(r.Context(), id, identity); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// Knock llama a la puerta de una sala bloqueada.
//
// SIN verja de moderación, a propósito: llamar es justo lo que hace quien NO
// puede entrar. La comprobación de que la sala es suya —miembro del canal, o
// invitado— la hace el servicio, la misma que al entrar.
func (h *Handlers) Knock(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c := claimsFromCtx(r)
	if c.Subject == "" {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	a, err := h.svc.Knock(r.Context(), id, c.Subject, c.Name)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusAccepted, a)
}

// ListKnocks devuelve la cola de la puerta (anfitrión o admin).
func (h *Handlers) ListKnocks(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	room, err := h.svc.GetRoom(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if err := puedeModerar(r, room, "ver quién espera en la puerta"); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cola, err := h.svc.ListKnocks(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if cola == nil {
		cola = []domain.Admission{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"knocks": cola})
}

// Admit abre la puerta a alguien (POST) y Deny lo retira de la cola (DELETE).
func (h *Handlers) Admit(w http.ResponseWriter, r *http.Request) { h.responderPuerta(w, r, true) }

// Deny retira a alguien de la cola de la puerta.
func (h *Handlers) Deny(w http.ResponseWriter, r *http.Request) { h.responderPuerta(w, r, false) }

func (h *Handlers) responderPuerta(w http.ResponseWriter, r *http.Request, admitido bool) {
	accion := "rechazar a quien llama a la puerta"
	if admitido {
		accion = "admitir a quien llama a la puerta"
	}
	id, identity, ok := h.moderar(w, r, accion)
	if !ok {
		return
	}
	if err := h.svc.Admit(r.Context(), id, identity, admitido); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// RevokeSpeech retira la palabra a un participante (POST) y GrantSpeech se la
// devuelve (DELETE).
//
// Es el escalón que faltaba entre silenciar y expulsar: cortar la pista deja al
// otro lado con su botón de micrófono y quien está molestando lo vuelve a pulsar.
func (h *Handlers) RevokeSpeech(w http.ResponseWriter, r *http.Request) {
	h.palabra(w, r, false)
}

// GrantSpeech devuelve la palabra.
func (h *Handlers) GrantSpeech(w http.ResponseWriter, r *http.Request) {
	h.palabra(w, r, true)
}

func (h *Handlers) palabra(w http.ResponseWriter, r *http.Request, canPublish bool) {
	accion := "retirar la palabra a un participante"
	if canPublish {
		accion = "devolver la palabra a un participante"
	}
	id, identity, ok := h.moderar(w, r, accion)
	if !ok {
		return
	}
	if err := h.svc.SetCanPublish(r.Context(), id, identity, canPublish); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// LockRoom cierra (POST) o reabre (DELETE) la puerta de la sala.
//
// Dos verbos sobre la misma ruta y no un cuerpo con `{locked:true}`: así el
// permiso, el registro de auditoría y el propio log dicen QUÉ pasó sin tener que
// abrir el cuerpo de la petición.
func (h *Handlers) LockRoom(w http.ResponseWriter, r *http.Request) {
	h.bloquear(w, r, true)
}

// UnlockRoom reabre la sala.
func (h *Handlers) UnlockRoom(w http.ResponseWriter, r *http.Request) {
	h.bloquear(w, r, false)
}

func (h *Handlers) bloquear(w http.ResponseWriter, r *http.Request, locked bool) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	room, err := h.svc.GetRoom(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	accion := "unlock the room"
	if locked {
		accion = "lock the room"
	}
	if err := puedeModerar(r, room, accion); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actualizada, err := h.svc.LockRoom(r.Context(), id, locked)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, actualizada)
}

// MuteAll silencia a todos menos a quien lo pide (anfitrión o admin).
//
// Devuelve a cuántos ha callado, y 200 aunque sean cero: que nadie tuviera el
// micro abierto no es un fallo, y un 404 aquí haría que la interfaz dijera que
// algo salió mal cuando salió exactamente como se pedía.
func (h *Handlers) MuteAll(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	room, err := h.svc.GetRoom(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if err := puedeModerar(r, room, "mute everybody"); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	n, err := h.svc.MuteAll(r.Context(), id, claimsFromCtx(r).Subject)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"muted": n})
}

// StartRecording arranca la grabación de la sala (anfitrión o admin).
//
// Grabar a los demás es una decisión de quien dirige la reunión, no de cualquiera
// que esté dentro: la puerta es la misma que expulsar y silenciar.
func (h *Handlers) StartRecording(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	room, err := h.svc.GetRoom(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if err := puedeModerar(r, room, "record the meeting"); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	egressID, err := h.svc.StartRecording(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusAccepted, map[string]any{"egress_id": egressID})
}

// StopRecording detiene la grabación en curso (anfitrión o admin).
func (h *Handlers) StopRecording(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	room, err := h.svc.GetRoom(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if err := puedeModerar(r, room, "stop the recording"); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.StopRecording(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// MuteParticipant corta el audio de un participante sin echarlo de la sala.
func (h *Handlers) MuteParticipant(w http.ResponseWriter, r *http.Request) {
	id, identity, ok := h.moderar(w, r, "mute a participant")
	if !ok {
		return
	}
	if err := h.svc.MuteParticipant(r.Context(), id, identity); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}
