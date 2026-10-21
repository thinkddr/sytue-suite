// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// chatService es la superficie del servicio que consume la API REST. Se declara
// como interfaz (satisfecha por *service.Service) para poder inyectar dobles en
// los tests sin depender de una base de datos.
type chatService interface {
	StartMatrixNative(context.Context, string, string, string) (service.MatrixNativeStart, error)
	PreviewMatrixNative(context.Context, string, string) (service.MatrixNativePreview, error)
	AcceptMatrixNative(context.Context, string, string, string) (service.MatrixNativeAccepted, error)
	FinishMatrixNative(context.Context, string, string, string, string) (string, error)

	BootstrapMatrix(context.Context, string, string) (service.MatrixBootstrap, error)
	MatrixAccountAccess(context.Context, string, string, int64) error
	MatrixTenantAccess(context.Context, string, string, string, int64) error
	EditMatrixMessage(context.Context, uuid.UUID, string, string, domain.MatrixEventRef) (domain.Message, error)
	ListTeams(ctx context.Context, sub string) ([]domain.Team, error)
	EnsureDefaultTeams(ctx context.Context, sub, subName string) ([]domain.Team, error)
	CreateTeam(ctx context.Context, name, description, sub, subName string) (domain.Team, error)
	GetTeam(ctx context.Context, id uuid.UUID, sub string) (domain.Team, error)
	ListTeamMembers(ctx context.Context, id uuid.UUID, sub string) ([]domain.Member, error)
	AddTeamMember(ctx context.Context, teamID uuid.UUID, actor, sub, name string, role domain.MemberRole) error
	RemoveTeamMember(ctx context.Context, teamID uuid.UUID, actor, sub string) error
	Unread(ctx context.Context, teamID uuid.UUID, sub string) ([]domain.ReadState, error)
	SearchMessages(ctx context.Context, teamID uuid.UUID, sub, query string, limit int) ([]domain.Message, error)
	SearchAll(ctx context.Context, sub, query string, limit int) ([]domain.Message, error)

	CreateChannel(ctx context.Context, in service.CreateChannelInput) (domain.Channel, error)
	ListChannels(ctx context.Context, teamID uuid.UUID, sub string, includeArchived bool) ([]domain.Channel, error)
	ListOwnMessages(ctx context.Context, sub string, afterAt time.Time, afterID uuid.UUID, limit int) ([]domain.Message, error)
	GetChannel(ctx context.Context, id uuid.UUID, sub string) (domain.Channel, error)
	UpdateChannel(ctx context.Context, id uuid.UUID, actor string, upd store.UpdateChannelRecord) (domain.Channel, error)
	JoinChannel(ctx context.Context, id uuid.UUID, sub, name string) (domain.Channel, error)
	OpenDirect(ctx context.Context, teamID uuid.UUID, actorSub, otherSub string) (domain.Channel, error)
	OpenGroupDirect(ctx context.Context, teamID uuid.UUID, actorSub string, otherSubs []string) (domain.Channel, error)
	SetChannelPref(ctx context.Context, id uuid.UUID, sub, name string, favorite, muted *bool, notifyLevel *string) (domain.Channel, error)
	GetNotifyPrefs(ctx context.Context, sub string) (domain.NotifyPrefs, error)
	PutNotifyPrefs(ctx context.Context, sub string, p domain.NotifyPrefs) (domain.NotifyPrefs, error)
	VAPIDPublicKey() string
	SubscribePush(ctx context.Context, subject string, sub domain.PushSubscription) error
	UnsubscribePush(ctx context.Context, subject, endpoint string) error
	RegisterAPNSDevice(ctx context.Context, subject, token string) error
	ForgetAPNSDevice(ctx context.Context, subject, token string) error
	ListJoinableChannels(ctx context.Context, teamID uuid.UUID, sub, query string, limit int) ([]domain.Channel, error)
	LeaveChannel(ctx context.Context, id uuid.UUID, sub string) error
	DeleteChannel(ctx context.Context, id uuid.UUID, actor string) error
	HideDirect(ctx context.Context, id uuid.UUID, sub string) error
	MarkRead(ctx context.Context, channelID uuid.UUID, sub string, messageID *uuid.UUID) error

	ListChannelMembers(ctx context.Context, channelID uuid.UUID, actor string) ([]domain.Member, error)
	AddChannelMember(ctx context.Context, channelID uuid.UUID, actor, subject, name string) (domain.Member, error)
	RemoveChannelMember(ctx context.Context, channelID uuid.UUID, actor, subject string) error
	SetChannelMemberRole(ctx context.Context, channelID uuid.UUID, actor, subject string, role domain.MemberRole) (domain.Member, error)

	ListCategories(ctx context.Context, sub string) ([]domain.Category, error)
	CreateCategory(ctx context.Context, sub, name string) (domain.Category, error)
	UpdateCategory(ctx context.Context, id uuid.UUID, sub string, upd store.UpdateCategoryRecord) (domain.Category, error)
	DeleteCategory(ctx context.Context, id uuid.UUID, sub string) error
	SetChannelCategory(ctx context.Context, id uuid.UUID, sub, name string, categoryID *uuid.UUID, sortOrder *int) (domain.Channel, error)
	MarkUnread(ctx context.Context, messageID uuid.UUID, sub string) error

	ListMessages(ctx context.Context, channelID uuid.UUID, sub string, before *time.Time, limit int) ([]domain.Message, error)
	MessagesAround(ctx context.Context, channelID uuid.UUID, sub string, messageID uuid.UUID, limit int) ([]domain.Message, uuid.UUID, error)
	Unfurl(ctx context.Context, url string) (domain.LinkUnfurl, error)
	// ImagenDeUnfurl devuelve (bytes, content-type, ETag) de la miniatura de la
	// imagen de una vista previa, para no mandar al navegador al host remoto.
	ImagenDeUnfurl(ctx context.Context, hash string) ([]byte, string, string, error)
	DMReceipts(ctx context.Context, channelID uuid.UUID, sub string) ([]domain.Receipt, error)
	PostMessage(ctx context.Context, in service.PostMessageInput) (domain.Message, error)
	AckMessage(ctx context.Context, messageID uuid.UUID, sub string) (int, error)
	MessageReaders(ctx context.Context, messageID uuid.UUID, sub string) ([]store.MessageReader, int, error)
	PostScheduled(ctx context.Context, in service.ScheduleMessageInput) (domain.ScheduledMessage, error)
	ListScheduled(ctx context.Context, sub string, channelID *uuid.UUID) ([]domain.ScheduledMessage, error)
	ListDrafts(ctx context.Context, sub string) ([]domain.Draft, error)
	PutDraft(ctx context.Context, sub string, channelID uuid.UUID, parentID *uuid.UUID, body string) (domain.Draft, error)
	UpdateScheduled(ctx context.Context, id uuid.UUID, actor string, body *string, sendAt *time.Time, fileIDs *[]uuid.UUID) (domain.ScheduledMessage, error)
	CancelScheduled(ctx context.Context, id uuid.UUID, actor string) error
	Thread(ctx context.Context, rootID uuid.UUID, sub string) (domain.Message, []domain.Message, error)
	FollowThread(ctx context.Context, rootID uuid.UUID, sub string) error
	UnfollowThread(ctx context.Context, rootID uuid.UUID, sub string) error
	IsFollowingThread(ctx context.Context, rootID uuid.UUID, sub string) (bool, error)
	ListFollowedThreads(ctx context.Context, sub string, before *time.Time, limit int) ([]domain.ThreadSummary, *time.Time, error)
	EditMessage(ctx context.Context, id uuid.UUID, actor, body string) (domain.Message, error)
	DeleteMessage(ctx context.Context, id uuid.UUID, actor string) error

	AddReaction(ctx context.Context, messageID uuid.UUID, sub, emoji string) error
	RemoveReaction(ctx context.Context, messageID uuid.UUID, sub, emoji string) error

	SetCardVote(ctx context.Context, messageID uuid.UUID, sub, value string) ([]domain.CardVote, error)
	ClearCardVote(ctx context.Context, messageID uuid.UUID, sub, value string) ([]domain.CardVote, error)

	PinMessage(ctx context.Context, messageID uuid.UUID, sub string) (domain.Message, error)
	UnpinMessage(ctx context.Context, messageID uuid.UUID, sub string) error
	ListPins(ctx context.Context, channelID uuid.UUID, sub string) ([]domain.Message, error)
	SaveMessage(ctx context.Context, messageID uuid.UUID, sub string) error
	UnsaveMessage(ctx context.Context, messageID uuid.UUID, sub string) error
	ListSaved(ctx context.Context, sub string, before *time.Time, limit int) ([]domain.Message, *time.Time, error)

	ListCustomEmoji(ctx context.Context) ([]domain.CustomEmoji, error)
	CreateCustomEmoji(ctx context.Context, name string, fileID uuid.UUID, actor string) (domain.CustomEmoji, error)
	DeleteCustomEmoji(ctx context.Context, name, actor string) error

	// Personas de otra organización (ADR 0040, externos.go).
	ResolvePerson(ctx context.Context, consulta string) (service.Persona, error)
	ListExternalChannels(ctx context.Context, sub string) ([]domain.Channel, []domain.ReadState, error)
	Invitaciones(ctx context.Context, sub string) ([]domain.Invitacion, error)
	AceptarInvitacion(ctx context.Context, sub, name string, channelID uuid.UUID) error
	RechazarInvitacion(ctx context.Context, sub string, channelID uuid.UUID) error
	Anfitrion(ctx context.Context, subject string, ref service.RefAnfitrion) (string, error)
}

// Handlers agrupa los manejadores de la API REST de Chat.
type Handlers struct {
	svc chatService
	cfg config.Config
}

// toAPIError traduce errores de dominio a *httpx.Error canónicos.
func toAPIError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrEncryptedRequired):
		return httpx.ErrConflict.WithCode("encrypted_client_required").WithMessage("Open an encrypted Rooms client to perform this action.")
	case errors.Is(err, domain.ErrMatrixUnavailable):
		return httpx.ErrUnavailable.WithCode("encrypted_messaging_unavailable").WithMessage("Encrypted messaging is unavailable. Try again.")
	case errors.Is(err, service.ErrRoomsIntegrationUnavailable):
		return httpx.ErrUnavailable.WithMessage("The connected service is unavailable. Try again.")
	case errors.Is(err, domain.ErrNotFound):
		return httpx.ErrNotFound
	case errors.Is(err, domain.ErrLastAdmin):
		return httpx.ErrConflict.WithCode("last_admin").WithMessage("You are the last admin of this private channel. Make someone else an admin first.")
	case errors.Is(err, domain.ErrConflict):
		return httpx.ErrConflict.WithMessage("Something with that name already exists, or the channel is archived.")
	case errors.Is(err, domain.ErrInvalidName):
		return httpx.ErrBadRequest.WithMessage("Invalid name.")
	case errors.Is(err, domain.ErrInvalidInput):
		return httpx.ErrBadRequest.WithMessage("Invalid parameters.")
	case errors.Is(err, domain.ErrEmptyMessage):
		return httpx.ErrBadRequest.WithMessage("The message cannot be empty.")
	case errors.Is(err, domain.ErrNotMember):
		// Distinto de ErrForbidden: lo que falla no es el permiso del ACTOR sino que
		// el sujeto de la operación no pertenece al equipo (añadir a un canal).
		return httpx.ErrForbidden.WithMessage("That person is not a member of the channel's team.")
	case errors.Is(err, domain.ErrBlocked):
		// El mismo texto para las dos personas: no dice quién bloqueó a quién.
		return httpx.ErrForbidden.WithMessage("Messages cannot be sent in this conversation.").WithCode("blocked")
	case errors.Is(err, domain.ErrForbidden):
		return httpx.ErrForbidden
	case errors.Is(err, domain.ErrTooManyLookups):
		return httpx.ErrTooManyRequests
	default:
		return httpx.ErrInternal.Wrap(err)
	}
}

// parseID extrae y valida un UUID de la ruta.
func parseID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		return uuid.Nil, httpx.ErrBadRequest.WithMessage("Invalid identifier.")
	}
	return id, nil
}

// caller devuelve el subject y nombre legible del usuario autenticado.
func caller(r *http.Request) (sub, name string) {
	if c, ok := authn.FromContext(r.Context()); ok {
		name = c.Name
		if name == "" {
			name = c.Email
		}
		return c.Subject, name
	}
	return "", ""
}

// ---------------------------------------------------------------------------
// Teams
// ---------------------------------------------------------------------------

// ListTeams lista los teams del usuario. En el primer acceso lo incorpora al
// team por defecto del tenant ("general" + canal #General), como Mail/Calendar/
// Contacts auto-aprovisionan sus colecciones.
func (h *Handlers) ListTeams(w http.ResponseWriter, r *http.Request) {
	sub, name := caller(r)
	teams, err := h.svc.EnsureDefaultTeams(r.Context(), sub, name)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if teams == nil {
		teams = []domain.Team{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"teams": teams})
}

type createTeamRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// CreateTeam crea un team.
func (h *Handlers) CreateTeam(w http.ResponseWriter, r *http.Request) {
	var req createTeamRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, name := caller(r)
	tm, err := h.svc.CreateTeam(r.Context(), req.Name, req.Description, sub, name)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, tm)
}

// GetTeam devuelve un team del usuario.
func (h *Handlers) GetTeam(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "teamID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	tm, err := h.svc.GetTeam(r.Context(), id, sub)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, tm)
}

// ListTeamMembers lista los miembros de un team.
func (h *Handlers) ListTeamMembers(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "teamID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	members, err := h.svc.ListTeamMembers(r.Context(), id, sub)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if members == nil {
		members = []domain.Member{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"members": members})
}

type addMemberRequest struct {
	Subject string `json:"subject"`
	Name    string `json:"name,omitempty"`
	Role    string `json:"role,omitempty"`
}

// AddTeamMember añade un miembro al team (admin).
func (h *Handlers) AddTeamMember(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "teamID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req addMemberRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	if err := h.svc.AddTeamMember(r.Context(), id, actor, strings.TrimSpace(req.Subject), req.Name, domain.MemberRole(req.Role)); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// RemoveTeamMember elimina un miembro del team.
func (h *Handlers) RemoveTeamMember(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "teamID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	target := strings.TrimSpace(chi.URLParam(r, "sub"))
	if target == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid subject."))
		return
	}
	actor, _ := caller(r)
	if err := h.svc.RemoveTeamMember(r.Context(), id, actor, target); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// Unread devuelve los no leídos por canal en un team.
func (h *Handlers) Unread(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "teamID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	states, err := h.svc.Unread(r.Context(), id, sub)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if states == nil {
		states = []domain.ReadState{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"channels": states})
}

// SearchAll busca mensajes en todos los canales accesibles del usuario, cross-team
// (?q=...&limit=...). Respalda la búsqueda global de Rooms y el conector federado del Hub.
func (h *Handlers) SearchAll(w http.ResponseWriter, r *http.Request) {
	// `httpx.Consulta` Y NO `r.URL.Query()`: esta ruta está montada con
	// `GetYQuery`, y por QUERY los parámetros viajan en el CUERPO —la query
	// string llega vacía—. Leyéndolos de la URL, la búsqueda de Rooms se quedaba sin
	// argumentos y respondía como si no le hubieran pedido nada. Pasó con el
	// buscador global del Hub, que devolvía 400 en cada pulsación.
	params, err := httpx.Consulta(w, r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	q := strings.TrimSpace(params.Get("q"))
	if q == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The search parameter 'q' is required."))
		return
	}
	limit, _ := strconv.Atoi(params.Get("limit"))
	sub, _ := caller(r)
	msgs, err := h.svc.SearchAll(r.Context(), sub, q, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if msgs == nil {
		msgs = []domain.Message{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"messages": msgs})
}

// SearchMessages busca mensajes en un team (?q=...&limit=...).
func (h *Handlers) SearchMessages(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "teamID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The search parameter 'q' is required."))
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	sub, _ := caller(r)
	msgs, err := h.svc.SearchMessages(r.Context(), id, sub, q, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if msgs == nil {
		msgs = []domain.Message{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"messages": msgs})
}

// ---------------------------------------------------------------------------
// Canales
// ---------------------------------------------------------------------------

type createChannelRequest struct {
	Name    string   `json:"name"`
	Topic   string   `json:"topic,omitempty"`
	Kind    string   `json:"kind,omitempty"`
	Members []string `json:"members,omitempty"`
}

// CreateChannel crea un canal en un team.
func (h *Handlers) CreateChannel(w http.ResponseWriter, r *http.Request) {
	teamID, err := parseID(r, "teamID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req createChannelRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	ch, err := h.svc.CreateChannel(r.Context(), service.CreateChannelInput{
		TeamID:  teamID,
		Name:    req.Name,
		Topic:   req.Topic,
		Kind:    domain.ChannelKind(req.Kind),
		Members: req.Members,
		Creator: sub,
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, ch)
}

// ListChannels lista los canales accesibles del usuario en un team (?archived=1).
func (h *Handlers) ListChannels(w http.ResponseWriter, r *http.Request) {
	teamID, err := parseID(r, "teamID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	includeArchived := r.URL.Query().Get("archived") == "1"
	channels, err := h.svc.ListChannels(r.Context(), teamID, sub, includeArchived)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if channels == nil {
		channels = []domain.Channel{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"channels": channels})
}

// GetChannel devuelve un canal accesible.
func (h *Handlers) GetChannel(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	ch, err := h.svc.GetChannel(r.Context(), id, sub)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, ch)
}

type updateChannelRequest struct {
	Name     *string `json:"name,omitempty"`
	Topic    *string `json:"topic,omitempty"`
	Purpose  *string `json:"purpose,omitempty"`
	Archived *bool   `json:"archived,omitempty"`
}

// UpdateChannel renombra/retematiza/archiva un canal (admin).
func (h *Handlers) UpdateChannel(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req updateChannelRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.Name == nil && req.Topic == nil && req.Purpose == nil && req.Archived == nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Nothing to update."))
		return
	}
	actor, _ := caller(r)
	ch, err := h.svc.UpdateChannel(r.Context(), id, actor, store.UpdateChannelRecord{
		Name: req.Name, Topic: req.Topic, Purpose: req.Purpose, Archived: req.Archived,
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, ch)
}

// JoinChannel une al usuario a un canal público.
func (h *Handlers) JoinChannel(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, name := caller(r)
	ch, err := h.svc.JoinChannel(r.Context(), id, sub, name)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, ch)
}

// LeaveChannel saca al usuario de un canal.
func (h *Handlers) LeaveChannel(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	if err := h.svc.LeaveChannel(r.Context(), id, sub); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// DeleteChannel borra un canal público o privado (admin del canal o del equipo).
func (h *Handlers) DeleteChannel(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	if err := h.svc.DeleteChannel(r.Context(), id, sub); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// HideDirect borra un mensaje directo solo para quien lo pide.
func (h *Handlers) HideDirect(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	if err := h.svc.HideDirect(r.Context(), id, sub); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ---------------------------------------------------------------------------
// Miembros de canal (roster)
// ---------------------------------------------------------------------------

type channelMemberRequest struct {
	Subject string `json:"subject"`
	Name    string `json:"name,omitempty"`
}

type channelMemberRoleRequest struct {
	Role string `json:"role"`
}

// ListChannelMembers lista los miembros de un canal accesible por el usuario.
func (h *Handlers) ListChannelMembers(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	members, err := h.svc.ListChannelMembers(r.Context(), id, sub)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if members == nil {
		members = []domain.Member{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"members": members})
}

// AddChannelMember añade a un miembro del equipo al canal (admin del canal o del team).
func (h *Handlers) AddChannelMember(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req channelMemberRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	target := strings.TrimSpace(req.Subject)
	if target == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid subject."))
		return
	}
	actor, _ := caller(r)
	m, err := h.svc.AddChannelMember(r.Context(), id, actor, target, req.Name)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, m)
}

// RemoveChannelMember saca a alguien del canal (admin del canal o del team; o uno mismo).
func (h *Handlers) RemoveChannelMember(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	target := strings.TrimSpace(chi.URLParam(r, "sub"))
	if target == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid subject."))
		return
	}
	actor, _ := caller(r)
	if err := h.svc.RemoveChannelMember(r.Context(), id, actor, target); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// SetChannelMemberRole fija el rol (admin|member) de un miembro del canal.
func (h *Handlers) SetChannelMemberRole(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	target := strings.TrimSpace(chi.URLParam(r, "sub"))
	if target == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid subject."))
		return
	}
	var req channelMemberRoleRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	m, err := h.svc.SetChannelMemberRole(r.Context(), id, actor, target, domain.MemberRole(strings.TrimSpace(req.Role)))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, m)
}

type openDirectRequest struct {
	// Subject es el sub del otro participante de un DM 1:1 (forma retrocompatible).
	Subject string `json:"subject,omitempty"`
	// Subjects es la lista de subs de los otros participantes de un DM de grupo (o de
	// uno solo, un 1:1). Forma canónica; tiene prioridad sobre Subject si viene.
	Subjects []string `json:"subjects,omitempty"`
}

// OpenDirect abre (idempotente) un DM del usuario actual con uno (1:1) o varios
// (grupo, hasta 8 en total) miembros del team. Acepta {subject} (1:1) o {subjects:[…]}
// (canónica; sirve para 1:1 y grupo). Devuelve el canal directo (nuevo o reutilizado).
func (h *Handlers) OpenDirect(w http.ResponseWriter, r *http.Request) {
	teamID, err := parseID(r, "teamID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req openDirectRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	var ch domain.Channel
	switch {
	case len(req.Subjects) > 0:
		// Group DM (o 1:1 vía la forma canónica). El store valida el conjunto.
		ch, err = h.svc.OpenGroupDirect(r.Context(), teamID, actor, req.Subjects)
	case strings.TrimSpace(req.Subject) != "":
		ch, err = h.svc.OpenDirect(r.Context(), teamID, actor, req.Subject)
	default:
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Missing the recipient's subject (or subjects)."))
		return
	}
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, ch)
}

type channelPrefsRequest struct {
	Favorite *bool `json:"favorite,omitempty"`
	Muted    *bool `json:"muted,omitempty"`
	// NotifyLevel ('all'|'mentions'|'none') es la política de aviso del canal (Ola 13);
	// el servicio la valida. Fuente de verdad frente a `muted` (que deriva de 'none').
	NotifyLevel *string `json:"notify_level,omitempty"`
}

// SetChannelPrefs fija favorito y/o nivel de notificación del canal para el usuario
// actual (acepta el `muted` legado, que el store traduce a notify_level).
func (h *Handlers) SetChannelPrefs(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req channelPrefsRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.Favorite == nil && req.Muted == nil && req.NotifyLevel == nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Nothing to update."))
		return
	}
	sub, name := caller(r)
	ch, err := h.svc.SetChannelPref(r.Context(), id, sub, name, req.Favorite, req.Muted, req.NotifyLevel)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, ch)
}

type notifyPrefsRequest struct {
	Keywords []string `json:"keywords"`
	DNDStart int      `json:"dnd_start"`
	DNDEnd   int      `json:"dnd_end"`
}

// GetNotifyPrefs devuelve las preferencias de notificación del usuario actual
// (palabras clave + ventana «No molestar»).
func (h *Handlers) GetNotifyPrefs(w http.ResponseWriter, r *http.Request) {
	sub, _ := caller(r)
	prefs, err := h.svc.GetNotifyPrefs(r.Context(), sub)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if prefs.Keywords == nil {
		prefs.Keywords = []string{}
	}
	httpx.JSON(w, r, http.StatusOK, prefs)
}

// PutNotifyPrefs fija las preferencias de notificación del usuario actual (el servicio
// las sanea: dedupe/recorte de palabras clave, DND acotado).
func (h *Handlers) PutNotifyPrefs(w http.ResponseWriter, r *http.Request) {
	var req notifyPrefsRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	prefs, err := h.svc.PutNotifyPrefs(r.Context(), sub, domain.NotifyPrefs{
		Keywords: req.Keywords, DNDStart: req.DNDStart, DNDEnd: req.DNDEnd,
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if prefs.Keywords == nil {
		prefs.Keywords = []string{}
	}
	httpx.JSON(w, r, http.StatusOK, prefs)
}

// ---------------------------------------------------------------------------
// Web Push (Ola 18)
// ---------------------------------------------------------------------------

// PushKey devuelve la clave pública VAPID (applicationServerKey del navegador). Vacía si el
// push no está configurado en el servidor; el cliente entonces oculta el toggle de push.
func (h *Handlers) PushKey(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, r, http.StatusOK, map[string]any{"key": h.svc.VAPIDPublicKey()})
}

type pushSubscribeRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// SubscribePush registra la suscripción Web Push del navegador del usuario (upsert). El
// servicio valida la forma y el endpoint (https + no privado) antes de persistir.
func (h *Handlers) SubscribePush(w http.ResponseWriter, r *http.Request) {
	var req pushSubscribeRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	if err := h.svc.SubscribePush(r.Context(), sub, domain.PushSubscription{
		Endpoint: strings.TrimSpace(req.Endpoint),
		P256dh:   strings.TrimSpace(req.Keys.P256dh),
		Auth:     strings.TrimSpace(req.Keys.Auth),
	}); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

type apnsDeviceRequest struct {
	// Token es el token de dispositivo que da APNs a la app nativa (hex).
	Token string `json:"token"`
}

// RegisterAPNSDevice registra el aparato de iOS al que mandar el aviso «solo despertar»
// (ver service/apns.go: a Apple no le llega ni el texto ni el autor).
func (h *Handlers) RegisterAPNSDevice(w http.ResponseWriter, r *http.Request) {
	var req apnsDeviceRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	if err := h.svc.RegisterAPNSDevice(r.Context(), sub, strings.TrimSpace(req.Token)); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ForgetAPNSDevice borra el aparato: al cerrar sesión o al apagar los avisos.
func (h *Handlers) ForgetAPNSDevice(w http.ResponseWriter, r *http.Request) {
	var req apnsDeviceRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	if err := h.svc.ForgetAPNSDevice(r.Context(), sub, strings.TrimSpace(req.Token)); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

type pushUnsubscribeRequest struct {
	Endpoint string `json:"endpoint"`
}

// UnsubscribePush borra la suscripción Web Push (por endpoint) del usuario.
func (h *Handlers) UnsubscribePush(w http.ResponseWriter, r *http.Request) {
	var req pushUnsubscribeRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	if err := h.svc.UnsubscribePush(r.Context(), sub, strings.TrimSpace(req.Endpoint)); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ---------------------------------------------------------------------------
// Categorías del sidebar (Ola 11)
// ---------------------------------------------------------------------------

// ListCategories lista las categorías del sidebar del usuario actual (ordenadas).
func (h *Handlers) ListCategories(w http.ResponseWriter, r *http.Request) {
	sub, _ := caller(r)
	cats, err := h.svc.ListCategories(r.Context(), sub)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if cats == nil {
		cats = []domain.Category{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"categories": cats})
}

type createCategoryRequest struct {
	Name string `json:"name"`
}

// CreateCategory crea una categoría del sidebar del usuario actual.
func (h *Handlers) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var req createCategoryRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	cat, err := h.svc.CreateCategory(r.Context(), sub, req.Name)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, cat)
}

type updateCategoryRequest struct {
	Name      *string `json:"name,omitempty"`
	SortOrder *int    `json:"sort_order,omitempty"`
	Collapsed *bool   `json:"collapsed,omitempty"`
}

// UpdateCategory renombra, colapsa/expande y/o reordena una categoría del usuario.
func (h *Handlers) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "categoryID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req updateCategoryRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.Name == nil && req.SortOrder == nil && req.Collapsed == nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Nothing to update."))
		return
	}
	sub, _ := caller(r)
	cat, err := h.svc.UpdateCategory(r.Context(), id, sub, store.UpdateCategoryRecord{
		Name: req.Name, SortOrder: req.SortOrder, Collapsed: req.Collapsed,
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, cat)
}

// DeleteCategory borra una categoría del usuario; sus canales caen a la por defecto.
func (h *Handlers) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "categoryID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	if err := h.svc.DeleteCategory(r.Context(), id, sub); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

type setChannelCategoryRequest struct {
	// CategoryID vacío/ausente = categoría por defecto según kind (mueve el canal fuera
	// de cualquier categoría personalizada). Un uuid lo asigna a esa categoría.
	CategoryID *string `json:"category_id,omitempty"`
	SortOrder  *int    `json:"sort_order,omitempty"`
}

// SetChannelCategory asigna el canal a una categoría del sidebar del usuario y,
// opcionalmente, fija su orden dentro de ella.
func (h *Handlers) SetChannelCategory(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req setChannelCategoryRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var catID *uuid.UUID
	if req.CategoryID != nil && strings.TrimSpace(*req.CategoryID) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(*req.CategoryID))
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid category_id."))
			return
		}
		catID = &parsed
	}
	sub, name := caller(r)
	ch, err := h.svc.SetChannelCategory(r.Context(), id, sub, name, catID, req.SortOrder)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, ch)
}

// ListJoinableChannels lista canales públicos del team a los que el usuario no
// pertenece (explorar canales): ?q=texto&limit=N.
func (h *Handlers) ListJoinableChannels(w http.ResponseWriter, r *http.Request) {
	teamID, err := parseID(r, "teamID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	channels, err := h.svc.ListJoinableChannels(r.Context(), teamID, sub, q, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if channels == nil {
		channels = []domain.Channel{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"channels": channels})
}

type markReadRequest struct {
	MessageID string `json:"message_id,omitempty"`
}

// MarkRead marca un canal como leído.
func (h *Handlers) MarkRead(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req markReadRequest
	// Cuerpo opcional (sin message_id => marca hasta el último).
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(w, r, &req); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	var mid *uuid.UUID
	if req.MessageID != "" {
		parsed, err := uuid.Parse(req.MessageID)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid message_id."))
			return
		}
		mid = &parsed
	}
	sub, _ := caller(r)
	if err := h.svc.MarkRead(r.Context(), id, sub, mid); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ---------------------------------------------------------------------------
// Mensajes / hilos
// ---------------------------------------------------------------------------

type postMessageRequest struct {
	Body        string                 `json:"body"`
	MatrixEvent *domain.MatrixEventRef `json:"matrix_event,omitempty"`
	Mentions    []string               `json:"mentions,omitempty"`
	FileIDs     []string               `json:"file_ids,omitempty"`
	// Type es opcional: "user" (por defecto) o "call" (tarjeta de llamada de
	// sistema, cuerpo vacío admitido). El servicio valida el valor.
	Type string `json:"type,omitempty"`
	// Priority ("normal"|"important"|"urgent") y RequiresAck son de la Ola 12; el
	// servicio valida la prioridad. SendAt (RFC3339 futuro) programa el envío en vez
	// de publicarlo ya: el mensaje entra en la cola de programados.
	Priority    string `json:"priority,omitempty"`
	RequiresAck bool   `json:"requires_ack,omitempty"`
	SendAt      string `json:"send_at,omitempty"`
	// Card publica el mensaje como objeto rico: encuesta ({"kind":"poll","options":
	// [...]}) o tarea ({"kind":"task","assignee_sub":…}). El cuerpo sigue siendo la
	// pregunta o el título. El servicio la sanea (domain.NormalizeCard).
	Card *domain.Card `json:"card,omitempty"`
}

// ListMessages devuelve una página de mensajes (?before=RFC3339&limit=N).
func (h *Handlers) ListMessages(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var before *time.Time
	if b := r.URL.Query().Get("before"); b != "" {
		ts, err := time.Parse(time.RFC3339Nano, b)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("before must be an RFC3339 date."))
			return
		}
		before = &ts
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	sub, _ := caller(r)
	msgs, err := h.svc.ListMessages(r.Context(), id, sub, before, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if msgs == nil {
		msgs = []domain.Message{}
	}
	resp := map[string]any{"messages": msgs}
	if n := len(msgs); n > 0 {
		resp["next_before"] = msgs[n-1].CreatedAt
	}
	httpx.JSON(w, r, http.StatusOK, resp)
}

// MessagesAround devuelve una ventana de mensajes centrada en un mensaje objetivo
// (?message=<id>&limit=N): N/2 anteriores + el objetivo + N/2 posteriores, en orden
// cronológico ASC. Respalda el permalink / scroll-to-message (Ola 16).
func (h *Handlers) MessagesAround(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	target, err := uuid.Parse(strings.TrimSpace(r.URL.Query().Get("message")))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("message must be a UUID."))
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	sub, _ := caller(r)
	msgs, root, err := h.svc.MessagesAround(r.Context(), id, sub, target, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if msgs == nil {
		msgs = []domain.Message{}
	}
	out := map[string]any{"messages": msgs}
	if root != uuid.Nil {
		// El objetivo era una respuesta: el feed viene anclado en la raíz y el
		// cliente tiene que abrir ese hilo para enseñar el mensaje que se pidió.
		out["thread_root_id"] = root
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

// Unfurl devuelve la vista previa (OpenGraph) de una URL http/https (?url=). Cacheada
// por tenant y obtenida con el cliente SSRF-safe; degrada a un unfurl vacío si la URL
// no da metadatos. Autenticado (bajo /api/v1). Ola 16.
func (h *Handlers) Unfurl(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("url"))
	if raw == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The 'url' parameter is required."))
		return
	}
	u, err := h.svc.Unfurl(r.Context(), raw)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	// La imagen NO se devuelve con su URL de origen. Si lo hiciera, el navegador
	// de cada persona que lee el canal la pediría al host remoto, y ese host
	// —elegido por quien pegó el enlace, no por quien lee— vería su dirección IP
	// y la hora. Se sustituye por la ruta del proxy, que sirve una miniatura
	// desde aquí. Ver services/rooms/internal/service/unfurl_imagen.go.
	if strings.TrimSpace(u.ImageURL) != "" {
		u.ImageURL = "/api/v1/unfurl/imagen/" + service.HashDeEnlace(u.URL)
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"unfurl": u})
}

// UnfurlImagen sirve la miniatura de la imagen de una vista previa, generada y
// servida desde aquí para que el host remoto no vea quién la mira.
//
// Responde 404 —no un error— cuando el hash no corresponde a ningún enlace
// cacheado de este tenant, cuando el enlace no traía imagen o cuando lo que hay
// en esa URL no se puede decodificar. Para el cliente eso es «no hay imagen»,
// que es lo que ya sabe manejar: pinta la vista previa sin ella.
func (h *Handlers) UnfurlImagen(w http.ResponseWriter, r *http.Request) {
	hash := strings.TrimSpace(chi.URLParam(r, "hash"))
	datos, tipo, etag, err := h.svc.ImagenDeUnfurl(r.Context(), hash)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	// El ETag es el hash del enlace y no cambia, así que una revisita no vuelve a
	// descargar los bytes. `private` y no `public`: la miniatura solo debe
	// cachearla el navegador de quien tiene acceso al canal, nunca un proxy
	// compartido por el que pasen varios usuarios.
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", tipo)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=21600")
	// Lo que viene de un tercero se sirve como imagen y solo como imagen: sin
	// estas dos cabeceras, un origen que devolviera algo interpretable como HTML
	// podría acabar ejecutándose en el contexto de nuestro dominio.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	_, _ = w.Write(datos)
}

// Receipts devuelve el acuse de lectura de los otros miembros de un DM («Visto», Ola
// 16): su last_read_at, para marcar «Visto» en el último mensaje del usuario. Vacío
// en canales no directos (los receipts solo se calculan para DMs).
func (h *Handlers) Receipts(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	receipts, err := h.svc.DMReceipts(r.Context(), id, sub)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if receipts == nil {
		receipts = []domain.Receipt{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"receipts": receipts})
}

// PostMessage publica un mensaje en un canal.
func (h *Handlers) PostMessage(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.postMessage(w, r, id, nil)
}

// PostReply publica una respuesta dentro del hilo de un mensaje raíz.
func (h *Handlers) PostReply(w http.ResponseWriter, r *http.Request) {
	root, err := parseID(r, "messageID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// El canal se deriva del padre; resolvemos el canal del mensaje raíz.
	sub, _ := caller(r)
	rootMsg, _, err := h.svc.Thread(r.Context(), root, sub)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	h.postMessage(w, r, rootMsg.ChannelID, &root)
}

// postMessage es la lógica común de publicar mensaje/respuesta.
func (h *Handlers) postMessage(w http.ResponseWriter, r *http.Request, channelID uuid.UUID, parentID *uuid.UUID) {
	var req postMessageRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	fileIDs, err := parseUUIDs(req.FileIDs)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("file_ids contains an invalid identifier."))
		return
	}
	sub, name := caller(r)

	// send_at presente => programar el envío (cola de programados) en vez de publicar ya.
	if strings.TrimSpace(req.SendAt) != "" {
		sendAt, perr := time.Parse(time.RFC3339Nano, req.SendAt)
		if perr != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("send_at must be an RFC3339 date."))
			return
		}
		sm, serr := h.svc.PostScheduled(r.Context(), service.ScheduleMessageInput{
			ChannelID:  channelID,
			ParentID:   parentID,
			Body:       req.Body,
			FileIDs:    fileIDs,
			SendAt:     sendAt,
			AuthorSub:  sub,
			AuthorName: name,
		})
		if serr != nil {
			httpx.Fail(w, r, toAPIError(serr))
			return
		}
		httpx.JSON(w, r, http.StatusCreated, sm)
		return
	}

	m, err := h.svc.PostMessage(r.Context(), service.PostMessageInput{
		ChannelID:   channelID,
		ParentID:    parentID,
		Body:        req.Body,
		MatrixEvent: req.MatrixEvent,
		Type:        req.Type,
		Mentions:    req.Mentions,
		FileIDs:     fileIDs,
		AuthorSub:   sub,
		AuthorName:  name,
		Priority:    req.Priority,
		RequiresAck: req.RequiresAck,
		Card:        req.Card,
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, m)
}

// GetThread devuelve un mensaje raíz con sus respuestas.
func (h *Handlers) GetThread(w http.ResponseWriter, r *http.Request) {
	root, err := parseID(r, "messageID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	rootMsg, replies, err := h.svc.Thread(r.Context(), root, sub)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if replies == nil {
		replies = []domain.Message{}
	}
	// ¿Sigo este hilo? (botón Seguir/Siguiendo). Best-effort: si la consulta falla, el
	// hilo carga igual con following=false. Marcar el hilo como visto lo hace Thread.
	following, _ := h.svc.IsFollowingThread(r.Context(), root, sub)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"root": rootMsg, "replies": replies, "following": following})
}

// FollowThread hace que el usuario siga el hilo del mensaje raíz (Ola 9).
func (h *Handlers) FollowThread(w http.ResponseWriter, r *http.Request) {
	root, err := parseID(r, "messageID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	if err := h.svc.FollowThread(r.Context(), root, sub); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// UnfollowThread deja de seguir el hilo del mensaje raíz.
func (h *Handlers) UnfollowThread(w http.ResponseWriter, r *http.Request) {
	root, err := parseID(r, "messageID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	if err := h.svc.UnfollowThread(r.Context(), root, sub); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ListMyThreads lista los hilos que el usuario sigue (vista global «Hilos»), con
// no-leídos por hilo y paginación keyset por última actividad (?before=RFC3339&limit=N).
func (h *Handlers) ListMyThreads(w http.ResponseWriter, r *http.Request) {
	var before *time.Time
	if b := r.URL.Query().Get("before"); b != "" {
		ts, err := time.Parse(time.RFC3339Nano, b)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("before must be an RFC3339 date."))
			return
		}
		before = &ts
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	sub, _ := caller(r)
	threads, next, err := h.svc.ListFollowedThreads(r.Context(), sub, before, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if threads == nil {
		threads = []domain.ThreadSummary{}
	}
	resp := map[string]any{"threads": threads}
	if next != nil {
		resp["next_before"] = *next
	}
	httpx.JSON(w, r, http.StatusOK, resp)
}

type editMessageRequest struct {
	Body        string                 `json:"body"`
	MatrixEvent *domain.MatrixEventRef `json:"matrix_event,omitempty"`
}

// EditMessage edita el cuerpo de un mensaje (autor).
func (h *Handlers) EditMessage(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "messageID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req editMessageRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	var m domain.Message
	if req.MatrixEvent != nil {
		m, err = h.svc.EditMatrixMessage(r.Context(), id, actor, req.Body, *req.MatrixEvent)
	} else {
		m, err = h.svc.EditMessage(r.Context(), id, actor, req.Body)
	}
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, m)
}

// DeleteMessage borra un mensaje (autor o admin).
func (h *Handlers) DeleteMessage(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "messageID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	if err := h.svc.DeleteMessage(r.Context(), id, actor); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ---------------------------------------------------------------------------
// Reacciones
// ---------------------------------------------------------------------------

// AddReaction añade una reacción al mensaje.
func (h *Handlers) AddReaction(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "messageID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	emoji := strings.TrimSpace(chi.URLParam(r, "emoji"))
	sub, _ := caller(r)
	if err := h.svc.AddReaction(r.Context(), id, sub, emoji); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// RemoveReaction retira una reacción del mensaje.
func (h *Handlers) RemoveReaction(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "messageID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	emoji := strings.TrimSpace(chi.URLParam(r, "emoji"))
	sub, _ := caller(r)
	if err := h.svc.RemoveReaction(r.Context(), id, sub, emoji); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ---------------------------------------------------------------------------
// Tarjetas: encuestas y tareas
// ---------------------------------------------------------------------------

// SetCardVote marca un valor de la tarjeta de un mensaje (votar una opción de la
// encuesta, dar por hecha la tarea). Devuelve el recuento agregado, para que quien
// vota pinte el resultado con la respuesta y no espere a su propio evento WS.
func (h *Handlers) SetCardVote(w http.ResponseWriter, r *http.Request) {
	h.cardVote(w, r, true)
}

// ClearCardVote retira el valor marcado por el usuario (desvotar, reabrir la tarea).
func (h *Handlers) ClearCardVote(w http.ResponseWriter, r *http.Request) {
	h.cardVote(w, r, false)
}

func (h *Handlers) cardVote(w http.ResponseWriter, r *http.Request, set bool) {
	id, err := parseID(r, "messageID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	value := strings.TrimSpace(chi.URLParam(r, "value"))
	sub, _ := caller(r)
	var votes []domain.CardVote
	if set {
		votes, err = h.svc.SetCardVote(r.Context(), id, sub, value)
	} else {
		votes, err = h.svc.ClearCardVote(r.Context(), id, sub, value)
	}
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if votes == nil {
		votes = []domain.CardVote{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"votes": votes})
}

// ---------------------------------------------------------------------------
// Fijados (pinned) / Guardados (saved) / Marcar como no leído
// ---------------------------------------------------------------------------

// PinMessage fija un mensaje en su canal.
func (h *Handlers) PinMessage(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "messageID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	m, err := h.svc.PinMessage(r.Context(), id, sub)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, m)
}

// UnpinMessage desfija un mensaje.
func (h *Handlers) UnpinMessage(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "messageID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	if err := h.svc.UnpinMessage(r.Context(), id, sub); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ListPins lista los mensajes fijados de un canal.
func (h *Handlers) ListPins(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	pins, err := h.svc.ListPins(r.Context(), id, sub)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if pins == nil {
		pins = []domain.Message{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"pins": pins})
}

// SaveMessage guarda un mensaje para el usuario.
func (h *Handlers) SaveMessage(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "messageID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	if err := h.svc.SaveMessage(r.Context(), id, sub); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// UnsaveMessage retira un mensaje de los guardados del usuario.
func (h *Handlers) UnsaveMessage(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "messageID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	if err := h.svc.UnsaveMessage(r.Context(), id, sub); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ListSaved lista los mensajes guardados por el usuario, cross-canal
// (?before=RFC3339&limit=N), con paginación keyset por fecha de guardado.
func (h *Handlers) ListSaved(w http.ResponseWriter, r *http.Request) {
	var before *time.Time
	if b := r.URL.Query().Get("before"); b != "" {
		ts, err := time.Parse(time.RFC3339Nano, b)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("before must be an RFC3339 date."))
			return
		}
		before = &ts
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	sub, _ := caller(r)
	msgs, next, err := h.svc.ListSaved(r.Context(), sub, before, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if msgs == nil {
		msgs = []domain.Message{}
	}
	resp := map[string]any{"messages": msgs}
	if next != nil {
		resp["next_before"] = *next
	}
	httpx.JSON(w, r, http.StatusOK, resp)
}

// MarkUnread deja el canal del mensaje como no leído a partir de ese mensaje.
func (h *Handlers) MarkUnread(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "messageID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	if err := h.svc.MarkUnread(r.Context(), id, sub); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ---------------------------------------------------------------------------
// Acuse de recibo (Ola 12)
// ---------------------------------------------------------------------------

// AckMessage registra el acuse de recibo del usuario sobre un mensaje que lo pide;
// devuelve el nuevo total de confirmaciones.
func (h *Handlers) AckMessage(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "messageID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	count, err := h.svc.AckMessage(r.Context(), id, sub)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"count": count})
}

// MessageReaders responde GET /messages/{messageID}/readers: quién ha visto el
// mensaje y cuándo lo vio cada uno.
//
// Sin esto, quien escribe en un canal no sabe si alguien lo ha mirado — y la
// alternativa que la gente usa es preguntar «¿has visto lo que puse?», que es
// exactamente el trabajo que un chat debería ahorrar. El dato ya se guardaba para
// pintar el contador de no leídos; solo faltaba mirarlo del derecho.
func (h *Handlers) MessageReaders(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "messageID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, _ := caller(r)
	lectores, total, err := h.svc.MessageReaders(r.Context(), id, sub)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if lectores == nil {
		lectores = []store.MessageReader{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"readers": lectores, "total": total})
}

// ---------------------------------------------------------------------------
// Borradores (paridad §4)
// ---------------------------------------------------------------------------

// ListDrafts lista los borradores del usuario (cross-canal, del más reciente al más
// antiguo). Es lo que puebla la vista «Borradores» del sidebar y lo que hace que lo
// escrito en un dispositivo aparezca en el otro.
func (h *Handlers) ListDrafts(w http.ResponseWriter, r *http.Request) {
	sub, _ := caller(r)
	list, err := h.svc.ListDrafts(r.Context(), sub)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if list == nil {
		list = []domain.Draft{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"drafts": list})
}

type putDraftRequest struct {
	ChannelID string  `json:"channel_id"`
	ParentID  *string `json:"parent_id,omitempty"`
	Body      string  `json:"body"`
}

// PutDraft guarda (o borra, si el cuerpo queda en blanco) el borrador del usuario en
// un canal o en un hilo.
func (h *Handlers) PutDraft(w http.ResponseWriter, r *http.Request) {
	var req putDraftRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	channelID, err := uuid.Parse(strings.TrimSpace(req.ChannelID))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid channel_id."))
		return
	}
	var parentID *uuid.UUID
	if req.ParentID != nil && strings.TrimSpace(*req.ParentID) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(*req.ParentID))
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid parent_id."))
			return
		}
		parentID = &parsed
	}
	sub, _ := caller(r)
	d, err := h.svc.PutDraft(r.Context(), sub, channelID, parentID, req.Body)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, d)
}

// ---------------------------------------------------------------------------
// Mensajes programados (Ola 12)
// ---------------------------------------------------------------------------

// ListScheduled lista los mensajes programados del usuario (?channel_id= filtra).
func (h *Handlers) ListScheduled(w http.ResponseWriter, r *http.Request) {
	sub, _ := caller(r)
	var channelID *uuid.UUID
	if c := strings.TrimSpace(r.URL.Query().Get("channel_id")); c != "" {
		parsed, err := uuid.Parse(c)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid channel_id."))
			return
		}
		channelID = &parsed
	}
	list, err := h.svc.ListScheduled(r.Context(), sub, channelID)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if list == nil {
		list = []domain.ScheduledMessage{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"scheduled": list})
}

type updateScheduledRequest struct {
	Body    *string   `json:"body,omitempty"`
	SendAt  *string   `json:"send_at,omitempty"`
	FileIDs *[]string `json:"file_ids,omitempty"`
}

// UpdateScheduled edita el cuerpo/instante/adjuntos de un programado del usuario.
func (h *Handlers) UpdateScheduled(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "scheduledID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req updateScheduledRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.Body == nil && req.SendAt == nil && req.FileIDs == nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Nothing to update."))
		return
	}
	var sendAt *time.Time
	if req.SendAt != nil {
		ts, perr := time.Parse(time.RFC3339Nano, *req.SendAt)
		if perr != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("send_at must be an RFC3339 date."))
			return
		}
		sendAt = &ts
	}
	var fileIDs *[]uuid.UUID
	if req.FileIDs != nil {
		parsed, perr := parseUUIDs(*req.FileIDs)
		if perr != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("file_ids contains an invalid identifier."))
			return
		}
		if parsed == nil {
			parsed = []uuid.UUID{}
		}
		fileIDs = &parsed
	}
	actor, _ := caller(r)
	sm, err := h.svc.UpdateScheduled(r.Context(), id, actor, req.Body, sendAt, fileIDs)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, sm)
}

// CancelScheduled cancela (borra) un programado del usuario.
func (h *Handlers) CancelScheduled(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "scheduledID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	if err := h.svc.CancelScheduled(r.Context(), id, actor); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ---------------------------------------------------------------------------
// Emoji personalizados (Ola 14)
// ---------------------------------------------------------------------------

// ListCustomEmoji lista los emoji personalizados del tenant.
func (h *Handlers) ListCustomEmoji(w http.ResponseWriter, r *http.Request) {
	emoji, err := h.svc.ListCustomEmoji(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if emoji == nil {
		emoji = []domain.CustomEmoji{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"emoji": emoji})
}

type createEmojiRequest struct {
	Name   string `json:"name"`
	FileID string `json:"file_id"`
}

// CreateCustomEmoji crea un emoji personalizado {name, file_id}. La imagen la
// resuelve el servicio en Drive (debe ser una imagen accesible por el autor).
func (h *Handlers) CreateCustomEmoji(w http.ResponseWriter, r *http.Request) {
	var req createEmojiRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	fileID, err := uuid.Parse(strings.TrimSpace(req.FileID))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid file_id."))
		return
	}
	actor, _ := caller(r)
	e, err := h.svc.CreateCustomEmoji(r.Context(), strings.TrimSpace(req.Name), fileID, actor)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, e)
}

// DeleteCustomEmoji borra un emoji personalizado por nombre (autor o admin).
func (h *Handlers) DeleteCustomEmoji(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(chi.URLParam(r, "name"))
	if name == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid name."))
		return
	}
	actor, _ := caller(r)
	if err := h.svc.DeleteCustomEmoji(r.Context(), name, actor); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// parseUUIDs convierte una lista de strings a UUIDs, rechazando inválidos.
func parseUUIDs(in []string) ([]uuid.UUID, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]uuid.UUID, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}
