// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package service orquesta la lógica de aplicación de Sytue Rooms: combina el
// store (PostgreSQL) con la publicación de eventos de dominio (outbox sobre
// PostgreSQL) y la resolución de adjuntos contra Sytue Drive. El fan-out en
// tiempo real lo hace el relay del outbox (events.Dispatcher) invocando al Hub
// WebSocket — sin Redis/Valkey.
package service

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/cache"
	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	matrixclient "git.sytue.eu/sytue/one/services/rooms/internal/matrix"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// EventChannel es el canal lógico de eventos de Chat (events.Channel).
const EventChannel events.Channel = "chat"

// Tipos de evento de dominio publicados al outbox (y difundidos por WS).
const (
	EventMessageCreated  = "chat.message.created"
	EventMessageEdited   = "chat.message.edited"
	EventMessageDeleted  = "chat.message.deleted"
	EventReactionChanged = "chat.reaction.changed"
	EventPinChanged      = "chat.message.pin"
	// EventCardChanged anuncia el estado NUEVO y COMPLETO de la tarjeta de un
	// mensaje (votos de la encuesta, «hecha» de la tarea): el frame lleva el
	// recuento agregado, así que el cliente lo aplica sin releer el mensaje.
	EventCardChanged = "chat.message.card"
	// EventChannelMemberAdded cubre el alta Y el cambio de rol de un miembro del
	// canal: el frame lleva el miembro completo, así que el cliente hace UPSERT por
	// subject y no hacen falta dos tipos para la misma reconciliación del roster.
	EventChannelMemberAdded   = "chat.channel.member.added"
	EventChannelMemberRemoved = "chat.channel.member.removed"
	// EventChannelDeleted anuncia que un canal se ha borrado: los clientes lo
	// quitan de la barra lateral y lo cierran si lo tenían abierto.
	EventChannelDeleted = "chat.channel.deleted"
)

// Store es la superficie de persistencia que consume el servicio.
type Store = store.Store

// NewStore construye el store de Chat sobre el pool de PostgreSQL, con el
// cifrador de secretos en reposo (nil = sin KEK; entonces crear una integración
// con secreto de firma falla en vez de escribirlo en claro).
func NewStore(pool *pg.Pool, cipher *keycrypto.Cipher) *Store { return store.New(pool, cipher) }

// EventPublisher publica eventos de dominio (outbox).
type EventPublisher interface {
	Publish(ctx context.Context, ev events.Event) error
}

// Attachments resuelve metadatos de ficheros de Sytue Drive (clean-room: cliente
// HTTP propio a la API REST de Drive, sin acoplar el store). El CANAL es parte de la
// consulta: un adjunto vive en el ámbito de su canal y Drive exige membresía para
// devolverlo (ver drive.go y services/drive/internal/service/channelfiles.go).
type Attachments interface {
	Resolve(ctx context.Context, channelID, fileID uuid.UUID) (domain.Attachment, error)
}

// dataStore es la superficie mínima de persistencia que consume el Service. La
// satisface *store.Store (producción) y un fake en tests, permitiendo ejercitar
// la lógica de aplicación sin PostgreSQL. No cambia la firma pública de New.
type dataStore interface {
	CreateTeam(ctx context.Context, rec store.NewTeamRecord) (domain.Team, error)
	ListTeams(ctx context.Context, subject string) ([]domain.Team, error)
	EnsureDefaultTeamMembership(ctx context.Context, sub, name string) (domain.Team, bool, error)
	GetTeam(ctx context.Context, id uuid.UUID, subject string) (domain.Team, error)
	ListTeamMembers(ctx context.Context, teamID uuid.UUID, actor string) ([]domain.Member, error)
	AddTeamMember(ctx context.Context, teamID uuid.UUID, actor, subject, name string, role domain.MemberRole) error
	RemoveTeamMember(ctx context.Context, teamID uuid.UUID, actor, subject string) error
	CreateChannel(ctx context.Context, rec store.NewChannelRecord) (domain.Channel, error)
	ListChannels(ctx context.Context, teamID uuid.UUID, subject string, includeArchived bool) ([]domain.Channel, error)
	GetChannel(ctx context.Context, id uuid.UUID, subject string) (domain.Channel, error)
	UpdateChannel(ctx context.Context, id uuid.UUID, actor string, upd store.UpdateChannelRecord) (domain.Channel, error)
	JoinChannel(ctx context.Context, id uuid.UUID, subject, name string) (domain.Channel, error)
	OpenDirect(ctx context.Context, teamID uuid.UUID, actorSub, otherSub string) (domain.Channel, error)
	OpenGroupDirect(ctx context.Context, teamID uuid.UUID, actorSub string, otherSubs []string, externos map[string]string) (domain.Channel, error)
	SetChannelPref(ctx context.Context, id uuid.UUID, subject, name string, favorite, muted *bool, notifyLevel *string) (domain.Channel, error)
	ListJoinableChannels(ctx context.Context, teamID uuid.UUID, subject, query string, limit int) ([]domain.Channel, error)
	LeaveChannel(ctx context.Context, id uuid.UUID, subject string) error
	ListCategories(ctx context.Context, subject string) ([]domain.Category, error)
	CreateCategory(ctx context.Context, subject, name string) (domain.Category, error)
	UpdateCategory(ctx context.Context, id uuid.UUID, subject string, upd store.UpdateCategoryRecord) (domain.Category, error)
	DeleteCategory(ctx context.Context, id uuid.UUID, subject string) error
	SetChannelCategory(ctx context.Context, id uuid.UUID, subject, name string, categoryID *uuid.UUID, sortOrder *int) (domain.Channel, error)
	CreateMessage(ctx context.Context, rec store.NewMessageRecord) (domain.Message, error)
	ListMessages(ctx context.Context, channelID uuid.UUID, subject string, page store.MessagePage) ([]domain.Message, error)
	ListOwnMessages(ctx context.Context, subject string, afterAt time.Time, afterID uuid.UUID, limit int) ([]domain.Message, error)
	ListThread(ctx context.Context, rootID uuid.UUID, subject string) (domain.Message, []domain.Message, error)
	EditMessage(ctx context.Context, id uuid.UUID, actor, body string) (domain.Message, error)
	DeleteMessage(ctx context.Context, id uuid.UUID, actor string) (uuid.UUID, error)
	SearchMessages(ctx context.Context, teamID uuid.UUID, subject, query string, limit int) ([]domain.Message, error)
	SearchAll(ctx context.Context, subject, query string, limit int) ([]domain.Message, error)
	AddReaction(ctx context.Context, messageID uuid.UUID, subject, emoji string) (uuid.UUID, error)
	RemoveReaction(ctx context.Context, messageID uuid.UUID, subject, emoji string) (uuid.UUID, error)
	SetCardVote(ctx context.Context, messageID uuid.UUID, subject, value string) (uuid.UUID, []domain.CardVote, error)
	ClearCardVote(ctx context.Context, messageID uuid.UUID, subject, value string) (uuid.UUID, []domain.CardVote, error)
	MarkRead(ctx context.Context, channelID uuid.UUID, subject string, messageID *uuid.UUID) error
	MarkUnread(ctx context.Context, messageID uuid.UUID, subject string) error
	UnreadByChannel(ctx context.Context, teamID uuid.UUID, subject string) ([]domain.ReadState, error)
	PinMessage(ctx context.Context, messageID uuid.UUID, subject string) (domain.Message, error)
	UnpinMessage(ctx context.Context, messageID uuid.UUID, subject string) (uuid.UUID, error)
	ListPins(ctx context.Context, channelID uuid.UUID, subject string) ([]domain.Message, error)
	SaveMessage(ctx context.Context, messageID uuid.UUID, subject string) error
	UnsaveMessage(ctx context.Context, messageID uuid.UUID, subject string) error
	ListSaved(ctx context.Context, subject string, before *time.Time, limit int) ([]domain.Message, *time.Time, error)
	ListDrafts(ctx context.Context, subject string) ([]domain.Draft, error)
	PutDraft(ctx context.Context, subject string, channelID uuid.UUID, parentID *uuid.UUID, body string) (domain.Draft, error)
	DeleteDraft(ctx context.Context, subject string, channelID uuid.UUID, parentID *uuid.UUID) error
}

// Service implementa los casos de uso de Chat.
type Service struct {
	cfg    config.Config
	store  dataStore
	pstore *Store
	// efimero es la caché de Dragonfly. OPCIONAL y nil-safe: si no está, todo
	// sigue funcionando y solo se regenera más a menudo lo que se cachearía.
	efimero *cache.Client
	pub     EventPublisher
	files   Attachments
	// outgoing es el cliente HTTP SSRF-safe de los webhooks salientes (Ola 15):
	// dialer con denylist de IPs, sin proxy, sin redirects, timeout y cap. Se
	// construye una vez (reusa conexiones); ver outgoing.go.
	outgoing     *http.Client
	roomsGateway *roomsGatewayClient
	matrix       *matrixclient.Client
	// unfurl es el cliente HTTP SSRF-safe del unfurl de enlaces (Ola 16): MISMO dialer
	// (barrera 2) que outgoing, pero sigue redirects acotados re-validando cada salto;
	// ver unfurl.go. Se construye una vez (reusa conexiones).
	unfurl *http.Client
	// push es el enviador de Web Push (Ola 18): par VAPID + cliente SSRF-safe. nil =
	// push deshabilitado (sin claves VAPID). Lo inyecta main.go vía SetPushSender.
	push *PushSender
	// apns despierta la app nativa de iOS con un aviso VACÍO (ver apns.go). nil =
	// deshabilitado. Lo inyecta main.go vía SetAPNSSender.
	apns *APNSSender
	// directorio encuentra en Sytue ID a personas de otras organizaciones (ver
	// externos.go). nil = no se puede añadir a nadie de fuera.
	directorio Directorio
	// hubNotifier emite notificaciones a Sytue Hub para menciones y mensajes directos.
	hubNotifier *hubnotify.Client
	nombresOrgs NombresOrgs
	pushSt      pushStore
}

// New construye el servicio.
func New(cfg config.Config, st *Store, pub EventPublisher, files Attachments) *Service {
	var notifier *hubnotify.Client
	if cfg.HubBaseURL != "" && cfg.HubInternalSecret != "" {
		notifier = hubnotify.New(cfg.HubBaseURL, cfg.HubInternalSecret)
	}
	mc := matrixclient.New(matrixclient.Config{InternalURL: cfg.MatrixInternalURL, HomeserverURL: cfg.MatrixHomeserverURL, ServerName: cfg.MatrixServerName, SSOProviderID: cfg.MatrixSSOProviderID, AdminToken: cfg.MatrixAdminToken})
	if st != nil {
		st.SetMatrixGate(mc)
		st.SetCallMatrixGate(mc)
		st.SetDocumentCallsEnabled(cfg.EncryptedDocumentCalls)
		mc.SetIdentityGate(st)
	}
	return &Service{
		cfg: cfg, store: st, pstore: st, pushSt: st, pub: pub, files: files,
		outgoing: ssrfSafeClient(), unfurl: unfurlClient(),
		roomsGateway: newRoomsGateway(cfg.IntegrationsURL, cfg.IntegrationsInternalSecret),
		matrix:       mc,
		hubNotifier:  notifier,
	}
}

// SetHubNotifier inyecta el cliente de notificaciones de Hub (útil en tests o arranque).
func (s *Service) SetHubNotifier(n *hubnotify.Client) { s.hubNotifier = n }

// NombresOrgs da el nombre visible de una organización por su slug (personas.NombresOrgs).
type NombresOrgs interface {
	Nombres(ctx context.Context, slugs []string) map[string]string
}

// SetNombresOrgs pone de dónde sale el nombre visible de la organización que invita.
// Sin él, las invitaciones solo llevan su identificador.
func (s *Service) SetNombresOrgs(n NombresOrgs) { s.nombresOrgs = n }

// SetPushStoreForTest inyecta un store de push fake para pruebas unitarias.
func (s *Service) SetPushStoreForTest(ps pushStore) { s.pushSt = ps }

// Store expone el store subyacente concreto (para sondas de readiness).
func (s *Service) Store() *Store { return s.pstore }

// publish emite un evento de dominio al outbox (best effort; el relay garantiza
// la entrega y el fan-out a los clientes WS).
func (s *Service) publish(ctx context.Context, eventType string, channelID uuid.UUID, payload any) {
	if s.pub == nil {
		return
	}
	t, _ := tenancy.From(ctx)
	env := eventEnvelope{ChannelID: channelID, Data: payload}
	ev, err := events.NewEvent(EventChannel, eventType, t.Slug, env)
	if err != nil {
		log.From(ctx).Warn("chat: construir evento", "type", eventType, "error", err)
		return
	}
	if err := s.pub.Publish(ctx, ev); err != nil {
		log.From(ctx).Warn("chat: publicar evento", "type", eventType, "error", err)
	}
}

// eventEnvelope envuelve el payload de un evento con el canal afectado, para que
// el relay sepa a qué sala difundir sin volver a consultar la BD.
type eventEnvelope struct {
	ChannelID uuid.UUID `json:"channel_id"`
	Data      any       `json:"data,omitempty"`
}

// ---------------------------------------------------------------------------
// Teams
// ---------------------------------------------------------------------------

// CreateTeam crea un team (el creador queda como admin).
func (s *Service) CreateTeam(ctx context.Context, name, description, sub, subName string) (domain.Team, error) {
	if !domain.ValidName(name) {
		return domain.Team{}, domain.ErrInvalidName
	}
	slug := domain.Slugify(name)
	if slug == "" {
		return domain.Team{}, domain.ErrInvalidName
	}
	return s.store.CreateTeam(ctx, store.NewTeamRecord{
		Slug: slug, Name: name, Description: description, CreatedBy: sub, CreatorName: subName,
	})
}

// ListTeams lista los teams del usuario.
func (s *Service) ListTeams(ctx context.Context, sub string) ([]domain.Team, error) {
	return s.store.ListTeams(ctx, sub)
}

// EnsureDefaultTeams devuelve los teams del usuario, incorporándolo en su
// primer acceso al team por defecto del tenant ("general") — creándolo, con su
// canal #General, si aún no existe. Mismo patrón de auto-aprovisionamiento que
// Mail (buzones), Calendar (Personal) y Contacts (Contactos).
func (s *Service) EnsureDefaultTeams(ctx context.Context, sub, subName string) ([]domain.Team, error) {
	teams, err := s.store.ListTeams(ctx, sub)
	if err != nil || len(teams) > 0 {
		return teams, err
	}
	tm, _, err := s.store.EnsureDefaultTeamMembership(ctx, sub, subName)
	if err != nil {
		return nil, err
	}
	// #General idempotente: si el team quedó sin canales (primer acceso o fallo a
	// medias previo), se crea; un duplicado por carrera devuelve ErrConflict y se ignora.
	chans, err := s.store.ListChannels(ctx, tm.ID, sub, false)
	if err != nil {
		return nil, err
	}
	// ListChannels solo trae aquello de lo que ya es miembro: un equipo con canales
	// públicos a los que aún no se ha unido no está «sin canales».
	if len(chans) == 0 {
		joinable, err := s.store.ListJoinableChannels(ctx, tm.ID, sub, "", 1)
		if err != nil {
			return nil, err
		}
		chans = joinable
	}
	if len(chans) == 0 {
		if _, err := s.CreateChannel(ctx, CreateChannelInput{
			TeamID: tm.ID, Name: "General", Topic: "Canal general del equipo",
			Kind: domain.ChannelPublic, Creator: sub,
		}); err != nil && !errors.Is(err, domain.ErrConflict) {
			return nil, err
		}
	}
	return []domain.Team{tm}, nil
}

// GetTeam devuelve un team del que el usuario es miembro.
func (s *Service) GetTeam(ctx context.Context, id uuid.UUID, sub string) (domain.Team, error) {
	return s.store.GetTeam(ctx, id, sub)
}

// ListTeamMembers lista los miembros de un team.
func (s *Service) ListTeamMembers(ctx context.Context, id uuid.UUID, sub string) ([]domain.Member, error) {
	return s.store.ListTeamMembers(ctx, id, sub)
}

// AddTeamMember añade un miembro al team (acción de admin).
func (s *Service) AddTeamMember(ctx context.Context, teamID uuid.UUID, actor, sub, name string, role domain.MemberRole) error {
	if sub == "" {
		return domain.ErrInvalidInput
	}
	if !domain.ValidMemberRole(role) {
		role = domain.RoleMember
	}
	return s.store.AddTeamMember(ctx, teamID, actor, sub, name, role)
}

// RemoveTeamMember elimina un miembro del team.
func (s *Service) RemoveTeamMember(ctx context.Context, teamID uuid.UUID, actor, sub string) error {
	return s.store.RemoveTeamMember(ctx, teamID, actor, sub)
}

// ---------------------------------------------------------------------------
// Channels
// ---------------------------------------------------------------------------

// CreateChannelInput son los parámetros para crear un canal.
type CreateChannelInput struct {
	TeamID  uuid.UUID
	Name    string
	Topic   string
	Kind    domain.ChannelKind
	Members []string
	Creator string
}

// CreateChannel crea un canal en un team.
func (s *Service) CreateChannel(ctx context.Context, in CreateChannelInput) (domain.Channel, error) {
	if !domain.ValidName(in.Name) {
		return domain.Channel{}, domain.ErrInvalidName
	}
	if in.Kind == "" {
		in.Kind = domain.ChannelPublic
	}
	if !domain.ValidChannelKind(in.Kind) {
		return domain.Channel{}, domain.ErrInvalidInput
	}
	slug := domain.Slugify(in.Name)
	if slug == "" {
		return domain.Channel{}, domain.ErrInvalidName
	}
	return s.store.CreateChannel(ctx, store.NewChannelRecord{
		TeamID: in.TeamID, Slug: slug, Name: in.Name, Topic: in.Topic,
		Kind: in.Kind, CreatedBy: in.Creator, Members: in.Members,
	})
}

// ListChannels lista los canales accesibles del usuario en un team.
func (s *Service) ListChannels(ctx context.Context, teamID uuid.UUID, sub string, includeArchived bool) ([]domain.Channel, error) {
	return s.store.ListChannels(ctx, teamID, sub, includeArchived)
}

// ListOwnMessages pagina los mensajes que escribió sub (ver store.ListOwnMessages).
func (s *Service) ListOwnMessages(ctx context.Context, sub string, afterAt time.Time, afterID uuid.UUID, limit int) ([]domain.Message, error) {
	return s.store.ListOwnMessages(ctx, sub, afterAt, afterID, limit)
}

// GetChannel devuelve un canal accesible por el usuario.
func (s *Service) GetChannel(ctx context.Context, id uuid.UUID, sub string) (domain.Channel, error) {
	return s.store.GetChannel(ctx, id, sub)
}

// UpdateChannel renombra/retematiza/archiva un canal (admin).
func (s *Service) UpdateChannel(ctx context.Context, id uuid.UUID, actor string, upd store.UpdateChannelRecord) (domain.Channel, error) {
	return s.store.UpdateChannel(ctx, id, actor, upd)
}

// JoinChannel une al usuario a un canal público y anuncia el alta al roster.
func (s *Service) JoinChannel(ctx context.Context, id uuid.UUID, sub, name string) (domain.Channel, error) {
	ch, err := s.store.JoinChannel(ctx, id, sub, name)
	if err != nil {
		return domain.Channel{}, err
	}
	s.publish(ctx, EventChannelMemberAdded, id,
		memberEvent("added", id, sub, &domain.Member{Subject: sub, Name: name, Role: domain.RoleMember}))
	return ch, nil
}

// ---------------------------------------------------------------------------
// Miembros de canal (roster)
// ---------------------------------------------------------------------------
//
// Delegaciones sobre el store CONCRETO (s.pstore, como los seguimientos de hilo): la
// autorización —admin del canal o del team— y el invariante «el añadido ha de estar en
// el team» viven en el store, pegados a las verjas que ya los aplican. Las mutaciones
// publican su evento al outbox y el relay lo difunde a la sala WS del canal.
//
// ponytail: la difusión llega a quien YA está suscrito a la sala del canal (el roster
// abierto se repinta en vivo). La sesión del recién añadido no está en esa sala todavía,
// así que ve el canal nuevo al recargar o al refrescar su lista; entregárselo al
// instante pediría enrutado por SUBJECT en el Hub (hoy solo hay salas por canal).

// ListChannelMembers lista el roster de un canal accesible por el usuario.
func (s *Service) ListChannelMembers(ctx context.Context, channelID uuid.UUID, actor string) ([]domain.Member, error) {
	return s.pstore.ListChannelMembers(ctx, channelID, actor)
}

// AddChannelMember añade al canal a un miembro del equipo o, si no lo es, a una
// persona de otra organización que exista en Sytue ID (admin del canal o del team).
func (s *Service) AddChannelMember(ctx context.Context, channelID uuid.UUID, actor, subject, name string) (domain.Member, error) {
	m, err := s.pstore.AddChannelMember(ctx, channelID, actor, subject, name)
	if errors.Is(err, domain.ErrNotMember) && s.directorio != nil {
		// No está en el equipo: puede ser alguien de fuera. El nombre lo pone Sytue
		// ID, no quien le añade (ADR 0040); si no existe, sigue siendo «no es del
		// equipo», que es lo que se le puede decir a quien no sabía que no existía.
		p, perr := s.directorio.PorID(ctx, subject)
		switch {
		case perr == nil:
			m, err = s.pstore.AddExternalChannelMember(ctx, channelID, actor, subject, nombreDePersona(p))
		case !errors.Is(perr, domain.ErrNotFound):
			err = perr
		}
	}
	if err != nil {
		return domain.Member{}, err
	}
	// Una invitación no es un alta: el canal no se entera hasta que acepte (mig 0041); se
	// entera ella, en su campana (COM-15).
	if m.Pending {
		s.avisarInvitacion(ctx, channelID, m.Subject)
	} else {
		s.publish(ctx, EventChannelMemberAdded, channelID, memberEvent("added", channelID, actor, &m))
	}
	return m, nil
}

// Invitaciones lista las invitaciones pendientes de subject a conversaciones de OTRAS
// organizaciones (mig 0041).
func (s *Service) Invitaciones(ctx context.Context, subject string) ([]domain.Invitacion, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return nil, err
	}
	inv, err := s.pstore.Invitaciones(ctx, subject, t.Slug)
	if err != nil || s.nombresOrgs == nil {
		return inv, err
	}
	// El nombre visible de la organización que invita (decisión del dueño, 17/10): solo
	// de las que invitan a ESTA persona, que es la verja de Sytue ID.
	slugs := make([]string, 0, len(inv))
	for _, i := range inv {
		slugs = append(slugs, i.Organizacion)
	}
	nombres := s.nombresOrgs.Nombres(ctx, slugs)
	for i := range inv {
		inv[i].NombreOrganizacion = nombres[inv[i].Organizacion]
	}
	return inv, nil
}

// AceptarInvitacion entra en la conversación: desde ahora es miembro (externo) y el
// canal se entera como de cualquier alta.
func (s *Service) AceptarInvitacion(ctx context.Context, subject, name string, channelID uuid.UUID) error {
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	anfitrion, err := s.pstore.AceptarInvitacion(ctx, subject, t.Slug, channelID)
	if err != nil {
		return err
	}
	s.publish(tenancy.Into(ctx, tenancy.Tenant{Slug: anfitrion}), EventChannelMemberAdded, channelID,
		memberEvent("added", channelID, subject, &domain.Member{Subject: subject, Name: name, Role: domain.RoleMember, External: true}))
	return nil
}

// RechazarInvitacion la descarta: no entra, y quien invitó no ve nada nuevo.
func (s *Service) RechazarInvitacion(ctx context.Context, subject string, channelID uuid.UUID) error {
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	return s.pstore.RechazarInvitacion(ctx, subject, t.Slug, channelID)
}

// RemoveChannelMember saca a alguien del canal (admin del canal o del team; o uno mismo).
func (s *Service) RemoveChannelMember(ctx context.Context, channelID uuid.UUID, actor, subject string) error {
	if err := s.pstore.RemoveChannelMember(ctx, channelID, actor, subject); err != nil {
		return err
	}
	s.publish(ctx, EventChannelMemberRemoved, channelID,
		memberEvent("removed", channelID, actor, &domain.Member{Subject: subject}))
	return nil
}

// SetChannelMemberRole fija el rol (admin|member) de un miembro del canal.
func (s *Service) SetChannelMemberRole(ctx context.Context, channelID uuid.UUID, actor, subject string, role domain.MemberRole) (domain.Member, error) {
	if !domain.ValidMemberRole(role) {
		return domain.Member{}, domain.ErrInvalidInput
	}
	m, err := s.pstore.SetChannelMemberRole(ctx, channelID, actor, subject, role)
	if err != nil {
		return domain.Member{}, err
	}
	// Mismo frame que el alta (upsert por subject): el cliente repinta la fila con su
	// nuevo rol sin un tercer tipo de evento.
	s.publish(ctx, EventChannelMemberAdded, channelID, memberEvent("added", channelID, actor, &m))
	return m, nil
}

// memberEvent construye el payload de un cambio de membresía de canal: la acción, el
// canal, quién la hizo y el miembro afectado (completo en las altas y cambios de rol,
// solo con su subject en las bajas), para que el cliente reconcilie el roster sin
// re-consultar.
func memberEvent(action string, channelID uuid.UUID, actor string, m *domain.Member) map[string]any {
	ev := map[string]any{
		"action":     action,
		"channel_id": channelID,
		"actor":      actor,
		"subject":    m.Subject,
	}
	if action != "removed" {
		ev["member"] = m
	}
	return ev
}

// OpenDirect abre (o reutiliza) el DM 1:1 del actor con otherSub en el team activo.
func (s *Service) OpenDirect(ctx context.Context, teamID uuid.UUID, actorSub, otherSub string) (domain.Channel, error) {
	otherSub = strings.TrimSpace(otherSub)
	if otherSub == "" {
		return domain.Channel{}, domain.ErrInvalidInput
	}
	return s.store.OpenDirect(ctx, teamID, actorSub, otherSub)
}

// OpenGroupDirect abre (o reutiliza) un DM directo de grupo (o 1:1) del actor con
// otherSubs. El store valida el conjunto (2..8, sin duplicados, todos del team) y
// garantiza la idempotencia por slug determinista del conjunto.
//
// Quien no esté en el team puede ser de OTRA organización (ADR 0040): se confirma
// en Sytue ID y entra como externo. Y si ese mismo grupo ya tiene conversación en
// otra organización donde el actor es el de fuera, se devuelve esa: el mismo grupo
// no puede tener dos según quién escribió primero.
func (s *Service) OpenGroupDirect(ctx context.Context, teamID uuid.UUID, actorSub string, otherSubs []string) (domain.Channel, error) {
	externos, err := s.personasDeFuera(ctx, teamID, otherSubs)
	if err != nil {
		return domain.Channel{}, err
	}
	if len(externos) > 0 {
		if t, terr := tenancy.From(ctx); terr == nil {
			grupo := append([]string{actorSub}, otherSubs...)
			ch, hay, err := s.pstore.DirectoDeFuera(ctx, actorSub, t.Slug, grupo)
			if err != nil || hay {
				return ch, err
			}
			// El grupo ya le había escrito desde otra organización: abrirlo es aceptar.
			if ch, hay, err = s.pstore.AceptarDirectoPendiente(ctx, actorSub, t.Slug, grupo); err != nil || hay {
				return ch, err
			}
		}
	}
	ch, err := s.store.OpenGroupDirect(ctx, teamID, actorSub, otherSubs, externos)
	if err == nil {
		for _, sub := range ch.Invitados {
			s.avisarInvitacion(ctx, ch.ID, sub)
		}
	}
	return ch, err
}

// SetChannelPref fija las preferencias per-usuario de un canal: favorito y política de
// aviso (notifyLevel 'all'|'mentions'|'none'). El `muted` legado lo traduce el store a
// notify_level (fuente de verdad). Valida el nivel si viene explícito.
func (s *Service) SetChannelPref(ctx context.Context, id uuid.UUID, sub, name string, favorite, muted *bool, notifyLevel *string) (domain.Channel, error) {
	if notifyLevel != nil && (*notifyLevel == "" || !domain.ValidNotifyLevel(*notifyLevel)) {
		return domain.Channel{}, domain.ErrInvalidInput
	}
	return s.store.SetChannelPref(ctx, id, sub, name, favorite, muted, notifyLevel)
}

// GetNotifyPrefs devuelve las preferencias de notificación del usuario (palabras clave
// + ventana DND). Delegación sobre el store concreto (como los seguimientos de hilo).
func (s *Service) GetNotifyPrefs(ctx context.Context, sub string) (domain.NotifyPrefs, error) {
	return s.pstore.GetNotifyPrefs(ctx, sub)
}

// PutNotifyPrefs fija las preferencias de notificación del usuario, saneándolas antes de
// persistir (dedupe/recorte de palabras clave, DND acotado a [0..1439]).
func (s *Service) PutNotifyPrefs(ctx context.Context, sub string, p domain.NotifyPrefs) (domain.NotifyPrefs, error) {
	return s.pstore.PutNotifyPrefs(ctx, sub, domain.NormalizeNotifyPrefs(p))
}

// ListJoinableChannels lista canales públicos del team a los que el usuario no pertenece.
func (s *Service) ListJoinableChannels(ctx context.Context, teamID uuid.UUID, sub, query string, limit int) ([]domain.Channel, error) {
	return s.store.ListJoinableChannels(ctx, teamID, sub, query, limit)
}

// LeaveChannel saca al usuario de un canal y anuncia la baja al roster.
func (s *Service) LeaveChannel(ctx context.Context, id uuid.UUID, sub string) error {
	if err := s.store.LeaveChannel(ctx, id, sub); err != nil {
		return err
	}
	s.publish(ctx, EventChannelMemberRemoved, id, memberEvent("removed", id, sub, &domain.Member{Subject: sub}))
	return nil
}

// DeleteChannel borra un canal público o privado (admin del canal o del equipo) y
// lo anuncia a la sala, para que los clientes lo quiten de su barra lateral.
func (s *Service) DeleteChannel(ctx context.Context, id uuid.UUID, actor string) error {
	if err := s.pstore.DeleteChannel(ctx, id, actor); err != nil {
		return err
	}
	s.publish(ctx, EventChannelDeleted, id, map[string]any{"channel_id": id, "actor": actor})
	return nil
}

// HideDirect borra un mensaje directo solo para sub (ver store.HideDirect). No se
// anuncia: a los demás no les cambia nada.
func (s *Service) HideDirect(ctx context.Context, id uuid.UUID, sub string) error {
	return s.pstore.HideDirect(ctx, id, sub)
}

// CanAccessChannel satisface ws.Authorizer: comprueba lectura sobre un canal y
// devuelve la organización en la que se concede. Es la del token salvo en una
// conversación de OTRA organización donde el usuario está como persona de fuera
// (ADR 0040): entonces es la anfitriona, que es donde se difunden sus eventos.
func (s *Service) CanAccessChannel(ctx context.Context, tenant string, channelID uuid.UUID, subject string) (string, bool) {
	if _, err := s.store.GetChannel(tenancy.Into(ctx, tenancy.Tenant{Slug: tenant}), channelID, subject); err == nil {
		return tenant, true
	}
	if s.pstore == nil {
		return "", false
	}
	anfitrion, err := s.pstore.AnfitrionDeCanal(ctx, subject, tenant, channelID)
	if err != nil || anfitrion == "" {
		return "", false
	}
	if _, err := s.store.GetChannel(tenancy.Into(ctx, tenancy.Tenant{Slug: anfitrion}), channelID, subject); err != nil {
		return "", false
	}
	return anfitrion, true
}

// ---------------------------------------------------------------------------
// Categorías del sidebar (Ola 11)
// ---------------------------------------------------------------------------
//
// Estado PER-USUARIO (subject): agrupa/ordena la barra lateral sin afectar a los demás.
// La asignación canal→categoría vive en la membresía (SetChannelCategory) y la lee
// ListChannels; ver mig. 0009.

// ListCategories lista las categorías del sidebar del usuario (ordenadas).
func (s *Service) ListCategories(ctx context.Context, sub string) ([]domain.Category, error) {
	return s.store.ListCategories(ctx, sub)
}

// CreateCategory crea una categoría del sidebar del usuario (valida el nombre).
func (s *Service) CreateCategory(ctx context.Context, sub, name string) (domain.Category, error) {
	if !domain.ValidName(name) {
		return domain.Category{}, domain.ErrInvalidName
	}
	return s.store.CreateCategory(ctx, sub, name)
}

// UpdateCategory renombra, colapsa/expande y/o reordena una categoría del usuario.
// Solo valida el nombre si viene (los demás campos son opcionales).
func (s *Service) UpdateCategory(ctx context.Context, id uuid.UUID, sub string, upd store.UpdateCategoryRecord) (domain.Category, error) {
	if upd.Name != nil && !domain.ValidName(*upd.Name) {
		return domain.Category{}, domain.ErrInvalidName
	}
	return s.store.UpdateCategory(ctx, id, sub, upd)
}

// DeleteCategory borra una categoría del usuario; sus canales caen a la por defecto.
func (s *Service) DeleteCategory(ctx context.Context, id uuid.UUID, sub string) error {
	return s.store.DeleteCategory(ctx, id, sub)
}

// SetChannelCategory asigna el canal a una categoría del usuario (categoryID nil =
// categoría por defecto según kind) y opcionalmente fija su orden.
func (s *Service) SetChannelCategory(ctx context.Context, id uuid.UUID, sub, name string, categoryID *uuid.UUID, sortOrder *int) (domain.Channel, error) {
	return s.store.SetChannelCategory(ctx, id, sub, name, categoryID, sortOrder)
}

// ---------------------------------------------------------------------------
// Borradores (paridad §4)
// ---------------------------------------------------------------------------
//
// Estado PER-USUARIO (subject) y cross-canal: lo escrito y no enviado. El cliente
// conserva su copia en localStorage para restaurarlo sin esperar a la red; esto es lo
// que lo hace existir en el OTRO dispositivo y lo que alimenta la vista «Borradores».

// ListDrafts devuelve los borradores del usuario, del más reciente al más antiguo.
func (s *Service) ListDrafts(ctx context.Context, sub string) ([]domain.Draft, error) {
	return s.store.ListDrafts(ctx, sub)
}

// PutDraft guarda el borrador del usuario en un ámbito de redacción (canal, o hilo si
// parentID no es nil). Un cuerpo en BLANCO lo borra: es lo que hace el composer al
// vaciarse y tras enviar, y tenerlo aquí evita un segundo endpoint para lo mismo. En
// ese caso devuelve el borrador con el cuerpo vacío, que el cliente lee como «ya no
// existe». Un cuerpo demasiado largo se rechaza con el MISMO tope que un mensaje: un
// borrador que no cabe en un mensaje no se podría enviar nunca.
func (s *Service) PutDraft(ctx context.Context, sub string, channelID uuid.UUID, parentID *uuid.UUID, body string) (domain.Draft, error) {
	if s.cfg.MatrixRequired && strings.TrimSpace(body) != "" {
		return domain.Draft{}, domain.ErrEncryptedRequired
	}
	body, err := domain.NormalizeBody(body, s.cfg.MaxMessageBytes)
	if errors.Is(err, domain.ErrEmptyMessage) {
		return domain.Draft{ChannelID: channelID, ParentID: parentID}, s.store.DeleteDraft(ctx, sub, channelID, parentID)
	}
	if err != nil {
		return domain.Draft{}, err
	}
	return s.store.PutDraft(ctx, sub, channelID, parentID, body)
}

// ---------------------------------------------------------------------------
// Mensajes / hilos
// ---------------------------------------------------------------------------

// PostMessageInput son los parámetros para publicar un mensaje.
type PostMessageInput struct {
	ChannelID   uuid.UUID
	ParentID    *uuid.UUID
	Body        string
	MatrixEvent *domain.MatrixEventRef
	// Type es "user" (normal) o "call" (tarjeta de sistema de llamada iniciada).
	// Vacío se normaliza a "user". "call" admite cuerpo vacío.
	Type       string
	Mentions   []string
	FileIDs    []uuid.UUID
	AuthorSub  string
	AuthorName string
	// Priority es "normal" (por defecto), "important" o "urgent" (Ola 12).
	Priority string
	// RequiresAck pide acuse de recibo explícito a los lectores (Ola 12).
	RequiresAck bool
	// Card es la tarjeta del mensaje: encuesta o tarea. nil = mensaje normal. El
	// CUERPO sigue mandando (pregunta/título), así que una tarjeta nunca sustituye
	// al texto: lo acompaña.
	Card *domain.Card
}

// PostMessage valida y publica un mensaje, resolviendo adjuntos de Drive y
// emitiendo el evento de dominio (que difunde el relay a la sala WS).
func (s *Service) PostMessage(ctx context.Context, in PostMessageInput) (domain.Message, error) {
	if in.MatrixEvent != nil {
		return s.postMatrixMessage(ctx, in)
	}
	if s.cfg.MatrixRequired {
		return domain.Message{}, domain.ErrEncryptedRequired
	}
	if !domain.ValidMessageType(in.Type) || !domain.ValidPriority(in.Priority) {
		return domain.Message{}, domain.ErrInvalidInput
	}
	msgType := in.Type
	if msgType == "" {
		msgType = domain.MessageTypeUser
	}

	body, err := domain.NormalizeBody(in.Body, s.cfg.MaxMessageBytes)
	if err != nil && len(in.FileIDs) == 0 && msgType == domain.MessageTypeUser {
		return domain.Message{}, err
	}
	if err != nil {
		// Cuerpo vacío permitido con adjuntos o en un mensaje de sistema (call).
		body = ""
	}

	// Tarjeta (encuesta/tarea): se sanea aquí y el CUERPO es obligatorio —es la
	// pregunta o el título—, aunque el mensaje llevara adjuntos. Una encuesta sin
	// pregunta no la puede leer nadie, y el push y la búsqueda solo ven el cuerpo.
	var card *domain.Card
	if in.Card != nil && in.Card.HasCard() {
		c, cerr := domain.NormalizeCard(*in.Card)
		if cerr != nil {
			return domain.Message{}, cerr
		}
		if body == "" {
			return domain.Message{}, domain.ErrEmptyMessage
		}
		card = &c
	}

	attachments, err := s.resolveAttachments(ctx, in.ChannelID, in.FileIDs)
	if err != nil {
		return domain.Message{}, err
	}

	m, err := s.store.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID:  in.ChannelID,
		ParentID:   in.ParentID,
		AuthorSub:  in.AuthorSub,
		AuthorName: in.AuthorName,
		Body:       body,
		Type:       msgType,
		Mentions:   dedupeNonEmpty(in.Mentions),
		// @channel/@here/@all: se marca el mensaje (mentions_all) y el contador de
		// menciones lo cuenta por consulta indexada; NO se expande a N menciones.
		MentionsAll: domain.HasChannelMention(body),
		Attachments: attachments,
		Priority:    in.Priority,
		RequiresAck: in.RequiresAck,
		Card:        card,
	})
	if err != nil {
		return domain.Message{}, err
	}
	s.publish(ctx, EventMessageCreated, m.ChannelID, m)
	return m, nil
}

// PostCallEnded publica la tarjeta de sistema «llamada finalizada» de un canal (Ola 6,
// follow-up meet→chat): un mensaje type=call cuyo CUERPO lleva la duración en segundos
// (el cliente lo formatea como «Llamada finalizada · N»). Reusa PostMessage —y con él
// createMessageTx, la validación de escritura y el fan-out WS— sin flujo nuevo. El autor
// es el sub que inició la sala (miembro del canal); lo invoca el endpoint interno que
// Sytue Rooms llama al recibir room_finished. El tenant y el acceso los fija el llamante.
func (s *Service) PostCallEnded(ctx context.Context, channelID uuid.UUID, authorSub string, durationSec int64) (domain.Message, error) {
	if authorSub == "" {
		return domain.Message{}, domain.ErrInvalidInput
	}
	if durationSec < 0 {
		durationSec = 0
	}
	// Cuerpo = duración en segundos (no vacío ⇒ pasa NormalizeBody). El tipo `call` ya es
	// válido y admite este cuerpo; una tarjeta call con cuerpo vacío es «unirse», con
	// número es «finalizada». Sin nuevo tipo ni migración del CHECK de messages.type.
	return s.PostMessage(ctx, PostMessageInput{
		ChannelID: channelID,
		AuthorSub: authorSub,
		Type:      domain.MessageTypeCall,
		Body:      strconv.FormatInt(durationSec, 10),
	})
}

// resolveAttachments consulta Drive para denormalizar los metadatos de cada
// fichero adjunto (nombre/tipo/tamaño). Si Drive no está disponible, conserva el
// id (el front puede resolver bajo demanda). Los adjuntos se resuelven EN EL ÁMBITO
// del canal de destino: Drive rechaza (ErrNotFound ⇒ ErrInvalidInput aquí) el
// file_id que no pertenezca a ese canal o que el autor no pueda leer, de modo que no
// se puede colar en un canal el adjunto de otro.
func (s *Service) resolveAttachments(ctx context.Context, channelID uuid.UUID, fileIDs []uuid.UUID) ([]domain.Attachment, error) {
	if len(fileIDs) == 0 {
		return nil, nil
	}
	out := make([]domain.Attachment, 0, len(fileIDs))
	seen := make(map[uuid.UUID]struct{}, len(fileIDs))
	for _, id := range fileIDs {
		if id == uuid.Nil {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		if s.files != nil {
			att, err := s.files.Resolve(ctx, channelID, id)
			if err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					return nil, domain.ErrInvalidInput
				}
				log.From(ctx).Warn("chat: resolver adjunto en Drive", "file_id", id, "error", err)
				out = append(out, domain.Attachment{FileID: id})
				continue
			}
			out = append(out, att)
			continue
		}
		out = append(out, domain.Attachment{FileID: id})
	}
	return out, nil
}

// ListMessages devuelve una página de mensajes raíz de un canal.
func (s *Service) ListMessages(ctx context.Context, channelID uuid.UUID, sub string, before *time.Time, limit int) ([]domain.Message, error) {
	if limit <= 0 {
		limit = s.cfg.MessagePageSize
	}
	return s.store.ListMessages(ctx, channelID, sub, store.MessagePage{Before: before, Limit: limit})
}

// MessagesAround devuelve una ventana de mensajes CENTRADA en messageID (permalink /
// scroll-to-message, Ola 16): N/2 anteriores + el objetivo + N/2 posteriores, en
// orden cronológico ASCENDENTE. Delegación pura sobre el store concreto (fetch-around
// por keyset; no lleva lógica de aplicación que ejercitar con el fake de dataStore).
func (s *Service) MessagesAround(ctx context.Context, channelID uuid.UUID, sub string, messageID uuid.UUID, limit int) ([]domain.Message, uuid.UUID, error) {
	if limit <= 0 {
		limit = s.cfg.MessagePageSize
	}
	return s.pstore.MessagesAround(ctx, channelID, sub, messageID, limit)
}

// DMReceipts devuelve el acuse de lectura de los otros miembros de un DM («Visto»,
// Ola 16). Solo tiene contenido para canales directos (el store lo garantiza);
// delegación pura sobre el store concreto.
func (s *Service) DMReceipts(ctx context.Context, channelID uuid.UUID, sub string) ([]domain.Receipt, error) {
	return s.pstore.DMReceipts(ctx, channelID, sub)
}

// Thread devuelve un mensaje raíz con sus respuestas. Abrirlo marca el hilo como
// visto para el usuario que lo sigue (side effect de ListThread, Ola 9).
func (s *Service) Thread(ctx context.Context, rootID uuid.UUID, sub string) (domain.Message, []domain.Message, error) {
	return s.store.ListThread(ctx, rootID, sub)
}

// ---------------------------------------------------------------------------
// Seguimiento de hilos (Ola 9)
// ---------------------------------------------------------------------------
//
// Delegaciones puras sobre el store CONCRETO (s.pstore, como Store()): no llevan
// lógica de aplicación que ejercitar con el fake de dataStore, así que no engordan
// esa superficie mockeable. El auto-follow al responder lo hace CreateMessage.

// FollowThread hace que el usuario siga un hilo (mensaje raíz).
func (s *Service) FollowThread(ctx context.Context, rootID uuid.UUID, sub string) error {
	return s.pstore.FollowThread(ctx, rootID, sub)
}

// UnfollowThread deja de seguir un hilo.
func (s *Service) UnfollowThread(ctx context.Context, rootID uuid.UUID, sub string) error {
	return s.pstore.UnfollowThread(ctx, rootID, sub)
}

// IsFollowingThread indica si el usuario sigue el hilo (botón Seguir/Siguiendo).
func (s *Service) IsFollowingThread(ctx context.Context, rootID uuid.UUID, sub string) (bool, error) {
	return s.pstore.IsFollowingThread(ctx, rootID, sub)
}

// ListFollowedThreads lista los hilos que el usuario sigue (vista global «Hilos»),
// con no-leídos por hilo y paginación keyset por última actividad.
func (s *Service) ListFollowedThreads(ctx context.Context, sub string, before *time.Time, limit int) ([]domain.ThreadSummary, *time.Time, error) {
	if limit <= 0 {
		limit = s.cfg.MessagePageSize
	}
	return s.pstore.ListFollowedThreads(ctx, sub, before, limit)
}

// EditMessage edita el cuerpo de un mensaje (solo autor) y emite el evento.
func (s *Service) EditMessage(ctx context.Context, id uuid.UUID, actor, body string) (domain.Message, error) {
	if s.cfg.MatrixRequired {
		return domain.Message{}, domain.ErrEncryptedRequired
	}
	body, err := domain.NormalizeBody(body, s.cfg.MaxMessageBytes)
	if err != nil {
		return domain.Message{}, err
	}
	m, err := s.store.EditMessage(ctx, id, actor, body)
	if err != nil {
		return domain.Message{}, err
	}
	s.publish(ctx, EventMessageEdited, m.ChannelID, m)
	return m, nil
}

// DeleteMessage borra un mensaje (autor o admin) y emite el evento.
func (s *Service) DeleteMessage(ctx context.Context, id uuid.UUID, actor string) error {
	channelID, err := s.store.DeleteMessage(ctx, id, actor)
	if err != nil {
		return err
	}
	s.publish(ctx, EventMessageDeleted, channelID, map[string]any{"id": id, "channel_id": channelID})
	return nil
}

// SearchMessages busca mensajes accesibles en un team.
func (s *Service) SearchMessages(ctx context.Context, teamID uuid.UUID, sub, query string, limit int) ([]domain.Message, error) {
	if s.cfg.MatrixRequired {
		return nil, domain.ErrEncryptedRequired
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, domain.ErrInvalidInput
	}
	return s.store.SearchMessages(ctx, teamID, sub, query, limit)
}

// SearchAll busca mensajes en todos los canales accesibles del usuario (búsqueda
// global de Rooms y federada del Hub), cross-team.
func (s *Service) SearchAll(ctx context.Context, sub, query string, limit int) ([]domain.Message, error) {
	if s.cfg.MatrixRequired {
		return nil, domain.ErrEncryptedRequired
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, domain.ErrInvalidInput
	}
	return s.store.SearchAll(ctx, sub, query, limit)
}

// ---------------------------------------------------------------------------
// Reacciones / lectura
// ---------------------------------------------------------------------------

// AddReaction añade una reacción y emite el evento de cambio.
func (s *Service) AddReaction(ctx context.Context, messageID uuid.UUID, sub, emoji string) error {
	if !domain.ValidEmoji(emoji) {
		return domain.ErrInvalidInput
	}
	channelID, err := s.store.AddReaction(ctx, messageID, sub, emoji)
	if err != nil {
		return err
	}
	s.publish(ctx, EventReactionChanged, channelID, reactionEvent("added", messageID, channelID, sub, emoji))
	return nil
}

// RemoveReaction retira una reacción y emite el evento de cambio.
func (s *Service) RemoveReaction(ctx context.Context, messageID uuid.UUID, sub, emoji string) error {
	if !domain.ValidEmoji(emoji) {
		return domain.ErrInvalidInput
	}
	channelID, err := s.store.RemoveReaction(ctx, messageID, sub, emoji)
	if err != nil {
		return err
	}
	s.publish(ctx, EventReactionChanged, channelID, reactionEvent("removed", messageID, channelID, sub, emoji))
	return nil
}

// AckMessage registra el acuse de recibo del usuario sobre un mensaje que lo pide
// (requires_ack) y devuelve el nuevo total de confirmaciones. Idempotente.
// Delegación pura sobre el store concreto (como los seguimientos de hilo).
//
// ponytail: sin fan-out WS del acuse — quien confirma actualiza su estado y el
// contador desde la respuesta; los demás ven «Confirmado por N» al recargar/abrir.
// Emitir un chat.message.ack (como reaction) lo pondría en vivo: follow-up barato.
func (s *Service) AckMessage(ctx context.Context, messageID uuid.UUID, sub string) (int, error) {
	_, count, err := s.pstore.AckMessage(ctx, messageID, sub)
	return count, err
}

// MessageReaders: quién ha visto un mensaje y cuándo, más cuánta gente hay en el
// canal aparte del autor. Ver store/readers.go: el dato sale de `read_state`, que
// guarda «leído hasta aquí» y no una fila por mensaje y persona.
func (s *Service) MessageReaders(
	ctx context.Context, messageID uuid.UUID, sub string,
) ([]store.MessageReader, int, error) {
	return s.pstore.MessageReaders(ctx, messageID, sub)
}

// SetCardVote marca un valor de la tarjeta de un mensaje (opción de una encuesta o
// «hecha» de una tarea) y difunde el RECUENTO ya agregado: quien lo recibe pinta el
// resultado sin releer el mensaje. La validación de qué valores existen la hace el
// store contra la tarjeta guardada, no el cliente.
func (s *Service) SetCardVote(ctx context.Context, messageID uuid.UUID, sub, value string) ([]domain.CardVote, error) {
	channelID, votes, err := s.store.SetCardVote(ctx, messageID, sub, value)
	if err != nil {
		return nil, err
	}
	s.publish(ctx, EventCardChanged, channelID, cardEvent(messageID, channelID, votes))
	return votes, nil
}

// ClearCardVote retira el valor marcado por el usuario y difunde el recuento nuevo.
func (s *Service) ClearCardVote(ctx context.Context, messageID uuid.UUID, sub, value string) ([]domain.CardVote, error) {
	channelID, votes, err := s.store.ClearCardVote(ctx, messageID, sub, value)
	if err != nil {
		return nil, err
	}
	s.publish(ctx, EventCardChanged, channelID, cardEvent(messageID, channelID, votes))
	return votes, nil
}

// cardEvent arma el payload del evento de tarjeta: el recuento COMPLETO del mensaje,
// no el delta. Un delta obligaría a cada cliente a tener el estado previo correcto
// (y a resolver los cruces de dos votos simultáneos); el recuento entero es idempotente.
func cardEvent(messageID, channelID uuid.UUID, votes []domain.CardVote) map[string]any {
	if votes == nil {
		votes = []domain.CardVote{}
	}
	return map[string]any{
		"message_id": messageID,
		"channel_id": channelID,
		"votes":      votes,
	}
}

func reactionEvent(action string, messageID, channelID uuid.UUID, sub, emoji string) map[string]any {
	return map[string]any{
		"action":     action,
		"message_id": messageID,
		"channel_id": channelID,
		"subject":    sub,
		"emoji":      emoji,
	}
}

// MarkRead marca un canal como leído por el usuario.
func (s *Service) MarkRead(ctx context.Context, channelID uuid.UUID, sub string, messageID *uuid.UUID) error {
	return s.store.MarkRead(ctx, channelID, sub, messageID)
}

// MarkUnread deja el canal como no leído a partir de un mensaje (per-usuario; no
// emite evento: solo afecta al badge de quien lo pide).
func (s *Service) MarkUnread(ctx context.Context, messageID uuid.UUID, sub string) error {
	return s.store.MarkUnread(ctx, messageID, sub)
}

// Unread devuelve los contadores de no leídos por canal en un team.
func (s *Service) Unread(ctx context.Context, teamID uuid.UUID, sub string) ([]domain.ReadState, error) {
	return s.store.UnreadByChannel(ctx, teamID, sub)
}

// ---------------------------------------------------------------------------
// Fijados (pinned) / Guardados (saved)
// ---------------------------------------------------------------------------

// PinMessage fija un mensaje y emite el evento de fijado (que el relay difunde a
// la sala del canal para que los paneles de fijados se actualicen en vivo).
func (s *Service) PinMessage(ctx context.Context, messageID uuid.UUID, sub string) (domain.Message, error) {
	m, err := s.store.PinMessage(ctx, messageID, sub)
	if err != nil {
		return domain.Message{}, err
	}
	s.publish(ctx, EventPinChanged, m.ChannelID, pinEvent("pinned", m.ID, m.ChannelID, sub, &m))
	return m, nil
}

// UnpinMessage desfija un mensaje y emite el evento de desfijado.
func (s *Service) UnpinMessage(ctx context.Context, messageID uuid.UUID, sub string) error {
	channelID, err := s.store.UnpinMessage(ctx, messageID, sub)
	if err != nil {
		return err
	}
	s.publish(ctx, EventPinChanged, channelID, pinEvent("unpinned", messageID, channelID, sub, nil))
	return nil
}

// ListPins lista los mensajes fijados de un canal (más reciente primero).
func (s *Service) ListPins(ctx context.Context, channelID uuid.UUID, sub string) ([]domain.Message, error) {
	return s.store.ListPins(ctx, channelID, sub)
}

// pinEvent construye el payload del evento de fijado/desfijado. En «pinned» lleva
// el mensaje completo para que los paneles lo pinten sin re-consultar.
func pinEvent(action string, messageID, channelID uuid.UUID, sub string, m *domain.Message) map[string]any {
	ev := map[string]any{
		"action":     action,
		"message_id": messageID,
		"channel_id": channelID,
		"subject":    sub,
	}
	if m != nil {
		ev["message"] = m
	}
	return ev
}

// SaveMessage guarda un mensaje para el usuario (per-usuario; sin evento WS).
func (s *Service) SaveMessage(ctx context.Context, messageID uuid.UUID, sub string) error {
	return s.store.SaveMessage(ctx, messageID, sub)
}

// UnsaveMessage retira un mensaje de los guardados del usuario.
func (s *Service) UnsaveMessage(ctx context.Context, messageID uuid.UUID, sub string) error {
	return s.store.UnsaveMessage(ctx, messageID, sub)
}

// ListSaved lista los mensajes guardados por el usuario, cross-canal, con
// paginación keyset por fecha de guardado. Devuelve el cursor de la siguiente
// página (o nil si no hay más).
func (s *Service) ListSaved(ctx context.Context, sub string, before *time.Time, limit int) ([]domain.Message, *time.Time, error) {
	if limit <= 0 {
		limit = s.cfg.MessagePageSize
	}
	return s.store.ListSaved(ctx, sub, before, limit)
}

// ---------------------------------------------------------------------------
// Emoji personalizados (Ola 14)
// ---------------------------------------------------------------------------
//
// Estado por TENANT (catálogo acotado). La imagen vive en Sytue Drive (reusa el
// flujo de adjuntos → file_id); el servicio la resuelve en Drive al crear para (a)
// autorizar el acceso del autor al fichero y (b) obtener/validar el content_type
// (solo imágenes). La imagen se sirve luego por el proxy de adjuntos (nosniff).

// ListCustomEmoji lista los emoji personalizados del tenant.
func (s *Service) ListCustomEmoji(ctx context.Context) ([]domain.CustomEmoji, error) {
	return s.pstore.ListCustomEmoji(ctx)
}

// CreateCustomEmoji crea un emoji personalizado: valida el nombre, resuelve el
// fichero en Drive (propagando la identidad del autor) para exigir que sea una
// imagen accesible, y persiste el catálogo con el content_type denormalizado.
func (s *Service) CreateCustomEmoji(ctx context.Context, name string, fileID uuid.UUID, actor string) (domain.CustomEmoji, error) {
	if !domain.ValidEmojiName(name) {
		return domain.CustomEmoji{}, domain.ErrInvalidName
	}
	if fileID == uuid.Nil || s.files == nil {
		return domain.CustomEmoji{}, domain.ErrInvalidInput
	}
	// Sin canal: la imagen del emoji es un activo del TENANT (visible para toda la
	// organización, como su catálogo), no un adjunto confidencial de un canal.
	att, err := s.files.Resolve(ctx, uuid.Nil, fileID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrForbidden) {
			// El autor no puede acceder al fichero (o no existe): entrada inválida.
			return domain.CustomEmoji{}, domain.ErrInvalidInput
		}
		return domain.CustomEmoji{}, err
	}
	if !strings.HasPrefix(att.ContentType, "image/") {
		return domain.CustomEmoji{}, domain.ErrInvalidInput
	}
	return s.pstore.CreateCustomEmoji(ctx, name, fileID, att.ContentType, actor)
}

// DeleteCustomEmoji borra un emoji personalizado (autor o admin del workspace).
func (s *Service) DeleteCustomEmoji(ctx context.Context, name, actor string) error {
	return s.pstore.DeleteCustomEmoji(ctx, name, actor)
}

// dedupeNonEmpty elimina duplicados y vacíos de una lista de subs.
func dedupeNonEmpty(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
