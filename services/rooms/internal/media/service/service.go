// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package service orquesta la lógica de aplicación de Sytue Rooms: combina el
// store (estado canónico en PostgreSQL) con el SFU LiveKit (rooms + minteo de
// JWT de acceso) y publica eventos de dominio (outbox sobre PostgreSQL) que el
// Hub propaga como intents cross-app (Chat/Calendar). El api_secret de LiveKit
// nunca sale del servidor; el JWT de sala se mintea TRAS validar el JWT de
// Sytue ID y autorizar al usuario.
package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/livekit"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/store"
)

// eventChannel es el canal lógico de eventos de Meet (events.Channel).
const eventChannel events.Channel = "meet"

// EventPublisher publica eventos de dominio (outbox). La implementación real es
// events.PublishTx sobre el *pg.Pool.
type EventPublisher interface {
	Publish(ctx context.Context, ev events.Event) error
}

// Store es la superficie de persistencia que consume el servicio.
type Store = store.Store

// NewStore construye el store de Meet sobre el pool de PostgreSQL.
func NewStore(pool *pg.Pool) *Store { return store.New(pool) }

// dataStore es la superficie mínima de persistencia que consume el Service. La
// satisface *store.Store (producción) y un fake en tests, permitiendo ejercitar
// la lógica sin PostgreSQL. No cambia la firma pública de New.
type dataStore interface {
	CreateRoom(ctx context.Context, rec store.NewRoomRecord) (domain.Room, error)
	FindRoomBySource(ctx context.Context, kind domain.SourceKind, refID string) (domain.Room, bool, error)
	GetRoom(ctx context.Context, id uuid.UUID) (domain.Room, error)
	GetRoomByName(ctx context.Context, name string) (domain.Room, error)
	ListRooms(ctx context.Context, status domain.RoomStatus, limit int, sujeto string) ([]domain.Room, error)
	ListParticipants(ctx context.Context, roomID uuid.UUID, onlyPresent bool) ([]domain.Participant, error)
	ListRecordings(ctx context.Context, roomID uuid.UUID) ([]domain.Recording, error)
	DeleteRecording(ctx context.Context, roomID, recordingID uuid.UUID) error
	CloseRoom(ctx context.Context, id uuid.UUID) (domain.Room, error)
	SetRoomLocked(ctx context.Context, id uuid.UUID, locked bool) (domain.Room, error)
	Knock(ctx context.Context, roomID uuid.UUID, identity, name string) (domain.Admission, error)
	SetAdmission(ctx context.Context, roomID uuid.UUID, identity string, admitido bool) error
	IsAdmitted(ctx context.Context, roomID uuid.UUID, identity string) (bool, error)
	ListKnocks(ctx context.Context, roomID uuid.UUID) ([]domain.Admission, error)
	MarkRoomStarted(ctx context.Context, name, sid string, at time.Time) error
	MarkRoomEnded(ctx context.Context, name string, at time.Time) error
	RecordJoin(ctx context.Context, in store.RecordJoinInput) error
	RecordLeave(ctx context.Context, name, sid string, at time.Time) error
	UpsertRecording(ctx context.Context, roomName, egressID string, status domain.RecordingStatus, storageKey string, size, duration int64, ended *time.Time) error
}

// roomProvisioner es la superficie mínima del SFU LiveKit que consume el Service.
type roomProvisioner interface {
	CreateRoom(ctx context.Context, req livekit.CreateRoomRequest) (livekit.Room, error)
	DeleteRoom(ctx context.Context, name string) error
	RemoveParticipant(ctx context.Context, room, identity string) error
	GetParticipant(ctx context.Context, room, identity string) (livekit.ParticipantInfo, error)
	MutePublishedTrack(ctx context.Context, room, identity, trackSID string, muted bool) error
	SetCanPublish(ctx context.Context, room, identity string, canPublish bool) error
	SetMediaPermissions(ctx context.Context, room, identity string, canPublish, canPublishData bool) error
	StartRoomComposite(ctx context.Context, sala, layout, ruta string, dest livekit.DestinoS3) (livekit.EgressInfo, error)
	StopEgress(ctx context.Context, egressID string) (livekit.EgressInfo, error)
}

// almacenGrabaciones es lo único que el servicio necesita del Object Storage:
// firmar una descarga temporal. No se sirve el fichero por aquí —una grabación
// de una hora son cientos de megas y pasarlos por el proceso significa un pod
// ocupado por descarga—; el navegador va directo al bucket con una URL que
// caduca.
type almacenGrabaciones interface {
	PresignGet(ctx context.Context, t tenancy.Tenant, key string, expiry time.Duration) (*url.URL, error)
	// Delete borra el fichero del bucket. Hace falta para que una grabación se
	// pueda SUPRIMIR: sin esto el objeto se queda ahí para siempre —no hay
	// expiración que lo alcance— y el derecho de supresión del RGPD no se puede
	// ejercer sobre la conversación entera de un equipo.
	Delete(ctx context.Context, t tenancy.Tenant, key string) error
}

// Service implementa los casos de uso de Meet.
type Service struct {
	cfg    config.Config
	store  dataStore
	lk     roomProvisioner
	signer *livekit.Signer
	pub    EventPublisher
	// chatNotifier avisa a Sytue Rooms cuando una sala ligada a un canal termina, para
	// publicar la tarjeta «Llamada finalizada» (follow-up meet→chat). nil = deshabilitado
	// (sin CHAT_INTERNAL_URL/SECRET). Lo inyecta main.go vía SetMessagingNotifier.
	chatNotifier MessagingNotifier
	// docGate decide quién puede entrar a una sala nacida en un documento: la ACL de
	// Sytue Docs. nil = sin verja (sin DOCS_URL) y las salas de documento quedan cerradas.
	docGate DocumentGate
	// calGate dice quién está invitado a la reunión de un evento de Calendar. nil = nadie
	// entra por eso (sí quien la convocó, quien ya estuvo y quien trae la clave).
	calGate CalendarGate
	// objetos firma las descargas de grabaciones. nil = sin destino configurado
	// (misma condición que `PuedeGrabar`), y entonces no hay nada que descargar.
	objetos                 almacenGrabaciones
	encryption              EncryptionGateway
	documentEncryptionReady bool
}

// New construye el servicio.
func New(cfg config.Config, st *Store, lk *livekit.Client, signer *livekit.Signer, pub EventPublisher) *Service {
	return &Service{cfg: cfg, store: st, lk: lk, signer: signer, pub: pub}
}

// SetObjectStore inyecta el cliente de Object Storage que firma las descargas de
// grabaciones. Se llama una vez al arrancar; nil deja la descarga deshabilitada.
func (s *Service) SetObjectStore(a almacenGrabaciones) { s.objetos = a }

// SetMessagingNotifier inyecta el notificador meet→chat (tarjeta «llamada finalizada»). Se
// llama una vez al arrancar; nil deja el aviso deshabilitado.
func (s *Service) SetMessagingNotifier(n MessagingNotifier) { s.chatNotifier = n }

// SetCalendarGate inyecta la sonda de invitados de Calendar. Se llama una vez al arrancar.
func (s *Service) SetCalendarGate(g CalendarGate) { s.calGate = g }

// SetDocumentGate inyecta la verja de documentos (Sytue Docs). Se llama una vez al arrancar.
func (s *Service) SetDocumentGate(g DocumentGate) { s.docGate = g }

// requireDocument autoriza el acceso a lo que nace en un documento: lo decide SU ACL, no
// el rol en el tenant ni haber creado o participado antes. Fail-closed.
func (s *Service) requireDocument(ctx context.Context, docID string) error {
	if s.docGate == nil {
		return domain.ErrPermission
	}
	return s.docGate.CanAccessDocument(ctx, docID)
}

// publish emite un evento de dominio best-effort (no rompe la operación).
func (s *Service) publish(ctx context.Context, eventType string, payload any) {
	if s.pub == nil {
		return
	}
	t, _ := tenancy.From(ctx)
	ev, err := events.NewEvent(eventChannel, eventType, t.Slug, payload)
	if err != nil {
		log.From(ctx).Warn("meet: construir evento", "type", eventType, "error", err)
		return
	}
	if err := s.pub.Publish(ctx, ev); err != nil {
		log.From(ctx).Warn("meet: publicar evento", "type", eventType, "error", err)
	}
}

// cargaDeCierre construye el payload de «sala terminada» CON su origen.
//
// El origen va aquí porque el aviso tiene que llegar al canal de chat del que
// salió la llamada: sin él, el fan-out no sabe a quién avisar y el banner de
// «llamada en curso» se queda colgado en la pantalla de todos hasta que alguien
// recarga. Es simétrico a lo que ya hacía el evento de creación.
func cargaDeCierre(room domain.Room) map[string]any {
	carga := map[string]any{"room_id": room.ID, "name": room.Name}
	if src := domain.SourceDeMetadata(room.Metadata); src.IsCrossApp() {
		carga["source_kind"] = string(src.Kind)
		carga["source_ref_id"] = src.RefID
	}
	return carga
}

// ---------------------------------------------------------------------------
// Salas
// ---------------------------------------------------------------------------

// CreateRoomInput son los datos de creación de una sala.
type CreateRoomInput struct {
	Title           string
	Name            string // opcional; si vacío se genera uno opaco
	CreatedBy       string
	MaxParticipants int
	// Intent identifica el origen cross-app (p. ej. "chat", "calendar") para
	// que el Hub propague el evento meet.room.created a la app de origen.
	Intent string
	// Source enlaza la sala con el recurso de origen (evento de Calendar o canal
	// de Chat) para reconciliación bidireccional. Si Kind está vacío, se asume
	// SourceDirect. Cuando es cross-app, RefID es obligatorio.
	Source   domain.RoomSource
	Metadata map[string]any
}

// CreateRoom crea la sala en PostgreSQL y aprovisiona la room en LiveKit. La
// creación en LiveKit es best-effort/idempotente: la sala también se materializa
// con el primer room_started si LiveKit no estuviera disponible al crear.
func (s *Service) CreateRoom(ctx context.Context, in CreateRoomInput) (domain.Room, error) {
	if !domain.ValidTitle(in.Title) {
		return domain.Room{}, domain.ErrInvalidName
	}
	maxPart := in.MaxParticipants
	if maxPart <= 0 || maxPart > s.cfg.MaxParticipants {
		maxPart = s.cfg.MaxParticipants
	}
	emptyTimeout := int(s.cfg.RoomEmptyTimeout.Seconds())

	// Normaliza el origen: por defecto directo. Si es cross-app debe validar
	// (RefID presente); en caso contrario se rechaza para no crear enlaces rotos.
	src := in.Source
	if src.Kind == "" {
		src.Kind = domain.SourceDirect
	}
	if !src.Valid() {
		return domain.Room{}, domain.ErrInvalidName
	}
	// El intent se deriva del origen si no se pasó explícitamente.
	// LA SALA DE UN CANAL LA CREA QUIEN ESTÁ EN EL CANAL. Esta verja estaba solo en
	// CreateRoomFromSource; por aquí —POST /rooms con `source`— cualquiera de la
	// organización creaba la sala de un canal ajeno, y como creador pasaba luego
	// requireRoomAccess sin preguntar al canal: en cuanto alguien del canal llamaba,
	// from-source le devolvía ESA sala, con el intruso de anfitrión.
	if src.IsCrossApp() {
		if err := s.requireChannelSource(ctx, src.Kind, src.RefID, in.CreatedBy); err != nil {
			return domain.Room{}, err
		}
	}
	if in.Intent == "" && src.IsCrossApp() {
		in.Intent = string(src.Kind)
	}

	t, err := tenancy.From(ctx)
	if err != nil {
		return domain.Room{}, err
	}
	// El nombre técnico SIEMPRE lleva delante el tenant REAL (el del contexto, no el que
	// propusiera el llamante): es lo que permite atribuir un webhook del SFU a su tenant
	// sin depender de nada más. Ver roomName.
	in.Name = roomName(t.Slug, in.Name)
	if !domain.ValidRoomName(in.Name) {
		return domain.Room{}, domain.ErrInvalidName
	}
	// Los metadatos del cliente, sin las claves que decide el servicio: de `source`
	// depende a qué canal o documento pertenece la sala (y por tanto quién entra), y
	// de `sytue_tenant`, a qué organización se apunta lo que avise LiveKit.
	meta := make(map[string]any, len(in.Metadata)+3)
	for k, v := range in.Metadata {
		if !metadatoReservado[k] {
			meta[k] = v
		}
	}
	if in.Intent != "" {
		meta["intent"] = in.Intent
	}
	// Enlace bidireccional: guardamos la referencia de origen en la metadata de
	// la sala. El Hub la usa para notificar a la app de origen (Calendar/Chat) la
	// URL de la sala, cerrando el intent cross-app "crear sala desde evento".
	if src.IsCrossApp() {
		meta["source"] = map[string]any{"kind": string(src.Kind), "ref_id": src.RefID, "title": src.Title}
	}
	// Embebemos el tenant en la metadata de la room de LiveKit para poder
	// resolverlo desde los webhooks (que son globales del despliegue LiveKit y
	// no portan contexto de Sytue ID). La metadata es opaca para LiveKit.
	meta["sytue_tenant"] = t.Slug

	room, err := s.store.CreateRoom(ctx, store.NewRoomRecord{
		Name:                in.Name,
		Title:               in.Title,
		CreatedBy:           in.CreatedBy,
		MaxParticipants:     maxPart,
		EmptyTimeoutSeconds: emptyTimeout,
		Metadata:            meta,
	})
	if err != nil {
		return domain.Room{}, err
	}

	// Aprovisiona la room en LiveKit (idempotente). No fallar si LiveKit no
	// responde: la room se crea sola al unirse el primer participante.
	metaJSON, _ := json.Marshal(meta)
	if _, err := s.lk.CreateRoom(ctx, livekit.CreateRoomRequest{
		Name:            room.Name,
		EmptyTimeout:    u32(emptyTimeout),
		MaxParticipants: u32(maxPart),
		Metadata:        string(metaJSON),
	}); err != nil {
		log.From(ctx).Warn("meet: crear room en LiveKit (se materializará al unirse)", "room", room.Name, "error", err)
	}

	payload := map[string]any{
		"room_id": room.ID, "name": room.Name, "title": room.Title, "intent": in.Intent,
	}
	if src.IsCrossApp() {
		// El Hub enruta este evento a la app de origen con la referencia y la URL
		// de señalización para que enlace la sala en el evento/canal.
		payload["source_kind"] = string(src.Kind)
		payload["source_ref_id"] = src.RefID
		payload["livekit_url"] = s.cfg.LiveKitURL
		// Quién la abrió: en un documento, la auditoría de la sala.
		payload["created_by"] = in.CreatedBy
	}
	s.publish(ctx, "meet.room.created", payload)
	return room, nil
}

// metadatoReservado son las claves de los metadatos de una sala que escribe el
// servicio y nunca el cliente.
var metadatoReservado = map[string]bool{"source": true, "sytue_tenant": true}

// CreateRoomFromSource crea una sala enlazada a un recurso de otra app (evento
// de Sytue Calendar o canal de Sytue Rooms), materializando el intent cross-app
// "crear sala desde evento". Es un envoltorio de CreateRoom que fija el origen y
// hereda el título del recurso cuando no se pasa uno propio. La sala emite el
// evento meet.room.created con la referencia de origen para el enlace inverso.
func (s *Service) CreateRoomFromSource(ctx context.Context, kind domain.SourceKind, refID, title, createdBy string, maxParticipants int) (domain.Room, error) {
	if !kind.IsCrossAppKind() {
		return domain.Room{}, domain.ErrInvalidName
	}
	src := domain.RoomSource{Kind: kind, RefID: refID, Title: title}
	if !src.Valid() {
		return domain.Room{}, domain.ErrInvalidName
	}
	// Verja de ORIGEN: crear la sala de un canal ajeno era la vía de escalada. Como
	// resolveRole da HOST al creador (identity == room.CreatedBy), quien creaba la sala
	// del canal de otros entraba con RoomAdmin en LiveKit y podía silenciar y expulsar
	// a los miembros legítimos de su propia conversación. Se exige poder LEER el canal.
	if err := s.requireChannelSource(ctx, kind, refID, createdBy); err != nil {
		return domain.Room{}, err
	}
	// Idempotencia por origen: aquí kind ya es cross-app y refID no vacío (garantía
	// de los guards anteriores). Si ya existe una sala viva enlazada a este
	// (kind, ref_id) la devolvemos en vez de crear una duplicada, de modo que el
	// Hub (o cualquier reintento del intent) pueda llamar repetidamente sin
	// acumular una sala por evento/canal. Mueve la dedup de sala-por-canal desde
	// el BFF a Go.
	// ponytail: find-then-create no es transaccional; dos from-source EXACTAMENTE
	// simultáneos aún podrían crear 2 salas. Upgrade path: índice único parcial
	// sobre (tenant, metadata->'source'->>'kind', metadata->'source'->>'ref_id')
	// WHERE status <> 'ended'. No lo añadimos ahora (riesgo con datos existentes).
	if existing, found, err := s.store.FindRoomBySource(ctx, kind, refID); err != nil {
		return domain.Room{}, err
	} else if found {
		// La reunión de un evento es suelta: la devuelve (y con ella la clave de su
		// enlace) solo a quien ya puede entrar. Conocer el UID del evento no basta.
		if kind == domain.SourceCalendar {
			if err := s.requireRoomAccessDe(ctx, existing); err != nil {
				return domain.Room{}, err
			}
		}
		return existing, nil
	}
	return s.CreateRoom(ctx, CreateRoomInput{
		Title:           title,
		CreatedBy:       createdBy,
		MaxParticipants: maxParticipants,
		Source:          src,
	})
}

// GetRoom devuelve una sala a quien puede entrar en ella (requireRoomAccess): la de
// un canal, a quien está en el canal; la de un documento, a quien puede verlo. Es
// por donde pasan la ficha de la sala y todas las acciones de moderación.
func (s *Service) GetRoom(ctx context.Context, id uuid.UUID) (domain.Room, error) {
	room, err := s.store.GetRoom(ctx, id)
	if err != nil {
		return domain.Room{}, err
	}
	if err := s.requireRoomAccessDe(ctx, room); err != nil {
		return domain.Room{}, err
	}
	return room, nil
}

// RoomBySource devuelve la sala VIVA enlazada a un recurso de origen cross-app
// (evento de Calendar o canal de Chat) por su (kind, ref_id) SIN crearla — lo que
// necesita Sytue Rooms para su indicador de «llamada en curso» al sondear un canal.
// Reusa el mismo lookup que CreateRoomFromSource emplea para reutilizar. Devuelve
// ErrNotFound si no hay ninguna sala viva para ese origen (sin efectos laterales).
func (s *Service) RoomBySource(ctx context.Context, kind domain.SourceKind, refID string) (domain.Room, error) {
	if !kind.IsCrossAppKind() {
		return domain.Room{}, domain.ErrInvalidName
	}
	// Mirar la sala de un documento es mirar el documento: sin acceso, ni existe.
	if kind == domain.SourceDocument {
		if err := s.requireDocument(ctx, refID); err != nil {
			return domain.Room{}, domain.ErrNotFound
		}
	}
	room, found, err := s.store.FindRoomBySource(ctx, kind, refID)
	if err != nil {
		return domain.Room{}, err
	}
	if !found {
		return domain.Room{}, domain.ErrNotFound
	}
	// «¿Hay llamada en este canal?» solo se le contesta a quien está en el canal: a
	// los demás, lo mismo que si no la hubiera.
	if err := s.requireRoomAccessDe(ctx, room); err != nil {
		return domain.Room{}, domain.ErrNotFound
	}
	return room, nil
}

// ListRooms lista las salas de la organización A LAS QUE QUIEN PREGUNTA TIENE ACCESO.
//
// Antes devolvía todas: con sus metadatos iba el canal de cada llamada, también el de
// los canales privados y los mensajes directos de otros. Ahora, la sala de un canal
// solo la ve quien administra la organización, quien la creó o quien está en ese
// canal. Una reunión suelta, solo quien la convocó o ya estuvo dentro —ni siquiera
// quien administra (decisión del dueño, 2026-10-16)—: ese filtro va en el SQL del
// store, para que las de otros no gasten el tope de la lista.
func (s *Service) ListRooms(ctx context.Context, status domain.RoomStatus, limit int) ([]domain.Room, error) {
	c, _ := authn.FromContext(ctx)
	rooms, err := s.store.ListRooms(ctx, status, limit, c.Subject)
	if err != nil {
		return nil, err
	}
	esAdmin := c.HasAnyRole("admin", "owner")
	t, _ := tenancy.From(ctx)
	enCanal := map[string]bool{} // una pregunta a la mensajería por canal, no por sala
	visibles := rooms[:0]
	for _, r := range rooms {
		// Las salas de documento no se listan: su título es el del documento y solo se
		// llega a ellas desde él (by-source, con la ACL del documento).
		if documentRef(r) != "" {
			continue
		}
		canal := chatChannelRef(r)
		if canal == "" { // suelta: ya la filtró el store
			visibles = append(visibles, r)
			continue
		}
		if r.Status == domain.RoomEnded && (esAdmin || (c.Subject != "" && c.Subject == r.CreatedBy)) {
			visibles = append(visibles, r)
			continue
		}
		puede, visto := enCanal[canal]
		if !visto {
			// Fail-closed, como al entrar: si no se puede preguntar, no se enseña.
			if s.chatNotifier != nil && c.Subject != "" && t.Slug != "" {
				puede, err = s.chatNotifier.ChannelAccess(ctx, t.Slug, canal, c.Subject)
				puede = puede && err == nil
			}
			enCanal[canal] = puede
		}
		if puede {
			visibles = append(visibles, r)
		}
	}
	return visibles, nil
}

// ListParticipants lista los participantes de una sala.
func (s *Service) ListParticipants(ctx context.Context, roomID uuid.UUID, onlyPresent bool) ([]domain.Participant, error) {
	if err := s.requireRoomAccessFromCtx(ctx, roomID); err != nil {
		return nil, err
	}
	return s.store.ListParticipants(ctx, roomID, onlyPresent)
}

// ListRecordings lista los metadatos de grabación de una sala.
func (s *Service) ListRecordings(ctx context.Context, roomID uuid.UUID) ([]domain.Recording, error) {
	if err := s.requireRoomAccessFromCtx(ctx, roomID); err != nil {
		return nil, err
	}
	return s.store.ListRecordings(ctx, roomID)
}

// CloseRoom finaliza una sala explícitamente y la cierra en LiveKit.
func (s *Service) CloseRoom(ctx context.Context, id uuid.UUID) (domain.Room, error) {
	if s.encryption != nil {
		if err := s.encryption.CloseCallEncryption(ctx, id); err != nil {
			return domain.Room{}, err
		}
	}
	room, err := s.store.CloseRoom(ctx, id)
	if err != nil {
		return domain.Room{}, err
	}
	if err := s.lk.DeleteRoom(ctx, room.Name); err != nil {
		log.From(ctx).Warn("meet: cerrar room en LiveKit", "room", room.Name, "error", err)
	}
	s.publish(ctx, "meet.room.closed", cargaDeCierre(room))
	return room, nil
}

// RemoveParticipant expulsa a un participante de una sala (moderación de host).
func (s *Service) RemoveParticipant(ctx context.Context, roomID uuid.UUID, identity string) error {
	room, err := s.store.GetRoom(ctx, roomID)
	if err != nil {
		return err
	}
	if s.encryption != nil {
		if err := s.encryption.RevokeCallMember(ctx, roomID, identity, true); err != nil {
			return err
		}
	}
	if err := s.lk.RemoveParticipant(ctx, room.Name, identity); err != nil && !errors.Is(err, livekit.ErrParticipantNotFound) {
		return fmt.Errorf("meet: expulsar participante: %w", err)
	}
	s.publish(ctx, "meet.participant.removed", map[string]any{"room_id": room.ID, "identity": identity})
	return nil
}

// MuteParticipant silencia las pistas de audio de un participante (moderación de
// host). Es lo que hace falta cuando alguien deja el micrófono abierto: cortar
// el audio sin echarlo de la reunión, que es lo único que había hasta ahora.
//
// Solo se tocan las pistas de AUDIO: silenciar la cámara de alguien desde fuera
// no resuelve ningún problema de la reunión y sí sorprende a quien la tiene
// puesta. Para lo demás está expulsar.
func (s *Service) MuteParticipant(ctx context.Context, roomID uuid.UUID, identity string) error {
	room, err := s.store.GetRoom(ctx, roomID)
	if err != nil {
		return err
	}
	if s.encryption != nil {
		name, err := s.encryption.CallTransportName(ctx, roomID)
		if err != nil {
			return err
		}
		room.Name = name
	}
	silenciadas, err := s.silenciar(ctx, room.Name, identity)
	if err != nil {
		return err
	}
	// Que no hubiera nada que silenciar no es un error: quien ya estaba callado
	// sigue callado, y devolver un fallo haría que la interfaz mintiera.
	if silenciadas == 0 {
		return nil
	}
	s.publish(ctx, "meet.participant.muted", map[string]any{
		"room_id": room.ID, "identity": identity, "tracks": silenciadas,
	})
	return nil
}

// silenciar corta las pistas de audio ABIERTAS de un participante y devuelve
// cuántas ha cerrado. Vive aparte porque lo usan silenciar a uno y a todos, y dos
// copias de este bucle acabarían divergiendo en qué cuenta como «ya callado».
func (s *Service) silenciar(ctx context.Context, sala, identity string) (int, error) {
	info, err := s.lk.GetParticipant(ctx, sala, identity)
	if err != nil {
		return 0, fmt.Errorf("meet: leer participante: %w", err)
	}
	n := 0
	for _, tr := range info.Tracks {
		if tr.Type != "AUDIO" || tr.Muted {
			continue
		}
		if err := s.lk.MutePublishedTrack(ctx, sala, identity, tr.SID, true); err != nil {
			return n, fmt.Errorf("meet: silenciar pista: %w", err)
		}
		n++
	}
	return n, nil
}

// SetCanPublish retira o devuelve la palabra a un participante.
//
// Al retirarla se le silencia ADEMÁS: quitarle el permiso no apaga lo que ya
// estaba sonando —el SFU deja de aceptar pistas nuevas, no corta las vivas— y el
// micrófono abierto seguiría abierto hasta que el otro lado hiciera algo. Sin
// esto, el botón parecería no funcionar justo en el caso que lo justifica.
func (s *Service) SetCanPublish(ctx context.Context, roomID uuid.UUID, identity string, canPublish bool) error {
	room, err := s.store.GetRoom(ctx, roomID)
	if err != nil {
		return err
	}
	if s.encryption != nil {
		return s.encryption.SetCallPublication(ctx, roomID, identity, canPublish)
	}
	if err := s.lk.SetCanPublish(ctx, room.Name, identity, canPublish); err != nil {
		return fmt.Errorf("meet: cambiar el permiso de publicar: %w", err)
	}
	if !canPublish {
		if _, err := s.silenciar(ctx, room.Name, identity); err != nil {
			log.From(ctx).Warn("meet: retirada la palabra, pero la pista seguía viva",
				"room", room.Name, "identity", logsafe.Text(identity), "error", err)
		}
	}
	evento := "meet.participant.speech_granted"
	if !canPublish {
		evento = "meet.participant.speech_revoked"
	}
	s.publish(ctx, evento, map[string]any{"room_id": room.ID, "identity": identity})
	return nil
}

// MuteAll silencia a todo el que esté dentro MENOS a quien lo pide.
//
// Quien lo pide se queda con el micro como lo tenía: es quien dirige la reunión y
// acaba de pedir silencio para poder hablar. Silenciarle a él también convertiría
// el botón en una trampa.
//
// Un participante que falle NO detiene al resto: en una sala de treinta personas,
// que alguien se haya ido justo ahora —y su lectura devuelva 404— no puede dejar a
// los otros veintinueve con el micro abierto. Se devuelve cuántos se silenciaron.
func (s *Service) MuteAll(ctx context.Context, roomID uuid.UUID, excepto string) (int, error) {
	room, err := s.store.GetRoom(ctx, roomID)
	if err != nil {
		return 0, err
	}
	if s.encryption != nil {
		name, err := s.encryption.CallTransportName(ctx, roomID)
		if err != nil {
			return 0, err
		}
		room.Name = name
	}
	presentes, err := s.store.ListParticipants(ctx, roomID, true)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, p := range presentes {
		if p.Identity == "" || p.Identity == excepto {
			continue
		}
		n, err := s.silenciar(ctx, room.Name, p.Identity)
		if err != nil {
			log.From(ctx).Warn("meet: silenciar a todos: uno falló",
				"room", room.Name, "identity", p.Identity, "error", err)
			continue
		}
		if n > 0 {
			total++
		}
	}
	if total > 0 {
		s.publish(ctx, "meet.participants.muted", map[string]any{
			"room_id": room.ID, "count": total, "by": excepto,
		})
	}
	return total, nil
}

// ---------------------------------------------------------------------------
// Grabación (egress)
// ---------------------------------------------------------------------------

// ErrGrabacionNoConfigurada se devuelve cuando falta el destino de las
// grabaciones. Es un centinela y no un error genérico para que la interfaz pueda
// decir «esta instalación no tiene grabación» en vez de «algo salió mal».
var ErrGrabacionNoConfigurada = fmt.Errorf("meet: no hay destino configurado para las grabaciones")

// LockRoom cierra o reabre la puerta de la sala. Devuelve la sala actualizada.
//
// No echa a quien ya está dentro: bloquear es «que no entre nadie más», no «que
// se vaya todo el mundo». Para eso están expulsar y cerrar la sala, que son otras
// dos acciones con otras consecuencias.
func (s *Service) LockRoom(ctx context.Context, roomID uuid.UUID, locked bool) (domain.Room, error) {
	room, err := s.store.SetRoomLocked(ctx, roomID, locked)
	if err != nil {
		return domain.Room{}, err
	}
	evento := "meet.room.unlocked"
	if locked {
		evento = "meet.room.locked"
	}
	s.publish(ctx, evento, map[string]any{"room_id": room.ID, "name": room.Name})
	return room, nil
}

// Knock llama a la puerta de una sala bloqueada.
//
// Solo tiene sentido con la sala CERRADA: si está abierta se devuelve un error
// de dominio para que la interfaz no ofrezca llamar a una puerta que no existe
// —y para que un cliente desactualizado no llene la cola de gente que en
// realidad podía pasar—.
func (s *Service) Knock(ctx context.Context, roomID uuid.UUID, identity, name string) (domain.Admission, error) {
	room, err := s.store.GetRoom(ctx, roomID)
	if err != nil {
		return domain.Admission{}, err
	}
	if !room.IsJoinable() {
		return domain.Admission{}, domain.ErrRoomClosed
	}
	if !room.Locked {
		return domain.Admission{}, domain.ErrRoomOpen
	}
	// El acceso a la sala se comprueba IGUAL que al entrar: llamar a la puerta de
	// una reunión de un canal del que no eres miembro no es «esperar», es
	// enterarse de que esa reunión existe.
	if err := s.requireRoomAccess(ctx, room, identity, false); err != nil {
		return domain.Admission{}, err
	}
	a, err := s.store.Knock(ctx, roomID, identity, name)
	if err != nil {
		return domain.Admission{}, err
	}
	s.publish(ctx, "meet.knock", map[string]any{
		"room_id": room.ID, "name": room.Name, "identity": identity,
	})
	return a, nil
}

// Admit abre la puerta a una persona; Deny la retira de la cola.
func (s *Service) Admit(ctx context.Context, roomID uuid.UUID, identity string, admitido bool) error {
	room, err := s.store.GetRoom(ctx, roomID)
	if err != nil {
		return err
	}
	if admitido && s.encryption != nil {
		if err := s.encryption.ReadmitCallMember(ctx, roomID, identity); err != nil {
			return err
		}
	}
	if err := s.store.SetAdmission(ctx, roomID, identity, admitido); err != nil {
		return fmt.Errorf("meet: responder a la puerta: %w", err)
	}
	evento := "meet.knock.denied"
	if admitido {
		evento = "meet.knock.admitted"
	}
	s.publish(ctx, evento, map[string]any{"room_id": room.ID, "identity": identity})
	return nil
}

// ListKnocks devuelve quién espera en la puerta, por orden de llegada.
func (s *Service) ListKnocks(ctx context.Context, roomID uuid.UUID) ([]domain.Admission, error) {
	return s.store.ListKnocks(ctx, roomID)
}

// ErrYaSeEstaGrabando evita arrancar un segundo egress sobre la misma sala:
// serían dos ficheros a medias, dos facturas y ningún acta completa.
var ErrYaSeEstaGrabando = fmt.Errorf("meet: esta sala ya se está grabando")

// StartRecording arranca la grabación compuesta de una sala.
//
// Devuelve el identificador del egress, que es lo único con lo que se puede
// parar después. El fichero NO existe todavía cuando esto vuelve: el egress lo
// sube al terminar y el resultado llega por webhook, que es quien completa
// `recordings_meta` con tamaño y duración.
func (s *Service) StartRecording(ctx context.Context, roomID uuid.UUID) (string, error) {
	if s.encryption != nil {
		return "", domain.ErrEncryptedOperation
	}
	// Las DOS condiciones, y la segunda no sobra: `PuedeGrabar` solo mira que la
	// configuración esté puesta, mientras que `objetos` solo se inyecta si el
	// bucket respondió al arrancar. Con el nombre bien escrito y el bucket sin
	// crear, el egress entraría en la sala, avisaría a todo el mundo de que se
	// está grabando y fallaría al subir: se habría grabado a la gente para nada.
	if !s.cfg.PuedeGrabar() || s.objetos == nil {
		return "", ErrGrabacionNoConfigurada
	}
	room, err := s.store.GetRoom(ctx, roomID)
	if err != nil {
		return "", err
	}
	if room.Status == domain.RoomEnded {
		return "", domain.ErrRoomClosed
	}
	// Una sala, una grabación. Se mira lo que ya hay antes de pedir otra.
	if grabaciones, err := s.store.ListRecordings(ctx, roomID); err == nil {
		for _, g := range grabaciones {
			if g.Status == domain.RecordingStarted {
				return "", ErrYaSeEstaGrabando
			}
		}
	}

	t, _ := tenancy.From(ctx)
	// Dos claves para el mismo fichero, y la diferencia importa: al egress se le
	// da la ruta ENTERA dentro del bucket —el prefijo del tenant delante, para que
	// una política pueda acotar por él—, mientras que en la base se guarda la
	// relativa, que es la que espera `objectstore` (él vuelve a anteponer el
	// prefijo al firmar). Guardar la entera hacía que la descarga buscara en
	// `tenants/acme/tenants/acme/...` y devolviera un 404 desconcertante.
	// El nombre de la sala + el instante evitan que dos grabaciones se pisen.
	relativa := fmt.Sprintf("%s/%d.mp4", room.Name, time.Now().UTC().Unix())
	ruta := t.StoragePrefix() + relativa
	info, err := s.lk.StartRoomComposite(ctx, room.Name, s.cfg.RecordingLayout, ruta, livekit.DestinoS3{
		AccessKey:      s.cfg.RecordingAccessKey,
		Secret:         s.cfg.RecordingSecretKey,
		Region:         s.cfg.RecordingRegion,
		Bucket:         s.cfg.RecordingBucket,
		Endpoint:       s.cfg.RecordingEndpointURL(),
		ForcePathStyle: true,
	})
	if err != nil {
		return "", fmt.Errorf("meet: arrancar la grabación: %w", err)
	}

	// Se anota YA, sin esperar al webhook: si el proceso se cae entre medias, la
	// grabación existe en LiveKit y sin esta fila nadie sabría que hay un egress
	// corriendo —ni podría pararlo—.
	if err := s.store.UpsertRecording(ctx, room.Name, info.EgressID,
		domain.RecordingStarted, relativa, 0, 0, nil); err != nil {
		log.From(ctx).Warn("meet: anotar la grabación recién arrancada",
			"egress", info.EgressID, "error", err)
	}

	// El aviso NO es cortesía: grabar a alguien sin decírselo es el problema, no
	// el fichero. Va como evento de dominio para que llegue por el mismo camino
	// que el resto —y quede en el outbox, que es la prueba de que se avisó—.
	s.publish(ctx, "meet.recording.started", map[string]any{
		"room_id": room.ID, "name": room.Name, "egress_id": info.EgressID,
	})
	return info.EgressID, nil
}

// StopRecording detiene la grabación en curso de una sala.
//
// Que LiveKit responda con error no significa que se siga grabando: el egress
// pudo terminar solo —la sala se vació, el proceso murió—. En ese caso se marca
// como terminada igualmente, porque dejarla como «grabando» para siempre haría
// que nadie pudiera volver a grabar esa sala.
func (s *Service) StopRecording(ctx context.Context, roomID uuid.UUID) error {
	room, err := s.store.GetRoom(ctx, roomID)
	if err != nil {
		return err
	}
	grabaciones, err := s.store.ListRecordings(ctx, roomID)
	if err != nil {
		return err
	}
	var egressID string
	for _, g := range grabaciones {
		if g.Status == domain.RecordingStarted {
			egressID = g.EgressID
			break
		}
	}
	if egressID == "" {
		return domain.ErrNotFound
	}

	if _, err := s.lk.StopEgress(ctx, egressID); err != nil {
		log.From(ctx).Warn("meet: parar el egress (¿ya había terminado?)",
			"egress", egressID, "error", err)
	}
	ahora := time.Now().UTC()
	if err := s.store.UpsertRecording(ctx, room.Name, egressID,
		domain.RecordingCompleted, "", 0, 0, &ahora); err != nil {
		return fmt.Errorf("meet: cerrar la grabación: %w", err)
	}
	s.publish(ctx, "meet.recording.stopped", map[string]any{
		"room_id": room.ID, "name": room.Name, "egress_id": egressID,
	})
	return nil
}

// ---------------------------------------------------------------------------
// Minteo del JWT de LiveKit (acceso a sala)
// ---------------------------------------------------------------------------

// JoinResult es el resultado de unirse a una sala: el JWT de LiveKit y la URL de
// señalización que el cliente web usa para conectarse al SFU.
type JoinResult struct {
	Room        domain.Room            `json:"room"`
	Token       string                 `json:"token"`
	LiveKitURL  string                 `json:"livekit_url"`
	Identity    string                 `json:"identity"`
	Role        domain.Role            `json:"role"`
	ExpiresAtMS int64                  `json:"expires_at_ms"`
	JoinID      string                 `json:"join_id,omitempty"`
	Encryption  *domain.CallEncryption `json:"encryption,omitempty"`
}

// JoinRoom autoriza al usuario y mintea su JWT de LiveKit con los grants del rol.
// Se invoca SOLO tras validar el JWT de Sytue ID (middleware authn). El creador
// de la sala obtiene rol host; el resto, el rol solicitado (acotado a speaker/
// viewer salvo que el solicitante sea admin/owner del tenant).
func (s *Service) JoinRoom(ctx context.Context, roomID uuid.UUID, identity, displayName string, requested domain.Role, isAdmin bool) (JoinResult, error) {
	room, err := s.store.GetRoom(ctx, roomID)
	if err != nil {
		return JoinResult{}, err
	}
	ctx, err = s.encryptedRoomAccess(ctx, room, identity, isAdmin)
	if err != nil {
		return JoinResult{}, err
	}
	isAdmin = currentDocumentAdministrator(ctx, room, isAdmin)
	if !room.IsJoinable() {
		return JoinResult{}, domain.ErrRoomClosed
	}
	// Sala bloqueada: no entra NADIE nuevo. El anfitrión y los admin sí, y no es
	// un privilegio de cortesía: quien la cerró se quedaría fuera al recargar la
	// página, y entonces ya no podría reabrirla ni echar a quien está dentro.
	// Y pasa también quien tenga admisión concedida: es la sala de espera, que
	// abre a UNO sin reabrirle a todo el mundo.
	if room.Locked && !isAdmin && identity != room.CreatedBy {
		admitido, err := s.store.IsAdmitted(ctx, roomID, identity)
		if err != nil {
			return JoinResult{}, err
		}
		if !admitido {
			return JoinResult{}, domain.ErrRoomLocked
		}
	}

	role := resolveRole(room, identity, requested, isAdmin)
	binding, joinID, ttl, err := s.prepareEncryptedJoin(ctx, room, identity, role)
	if err != nil {
		return JoinResult{}, err
	}

	canPublish := role.CanPublish()
	canSubscribe := true
	canData := true
	roomAdmin := role.IsAdmin()
	if binding != nil {
		canPublish = false
		canData = false
		roomAdmin = false
	}
	grant := livekit.VideoGrant{
		Room:           room.Name,
		RoomJoin:       true,
		RoomAdmin:      roomAdmin,
		CanPublish:     &canPublish,
		CanSubscribe:   &canSubscribe,
		CanPublishData: &canData,
	}

	livekitRoom := room.Name
	if binding != nil {
		livekitRoom = binding.LiveKitRoom
		grant.Room = livekitRoom
	}
	partMeta, _ := json.Marshal(callParticipantMetadata{Role: string(role), JoinID: joinID, MatrixDeviceID: matrixDevice(ctx)})
	token, err := s.signer.AccessToken(livekit.AccessTokenOptions{
		Identity: identity,
		Name:     displayName,
		Room:     livekitRoom,
		Grant:    grant,
		Metadata: string(partMeta),
		TTL:      ttl,
	})
	if err != nil {
		return JoinResult{}, fmt.Errorf("meet: mintear JWT de LiveKit: %w", err)
	}

	s.publish(ctx, "meet.participant.joining", map[string]any{
		"room_id": room.ID, "identity": identity, "role": string(role),
	})

	return JoinResult{
		Room:        room,
		Token:       token,
		LiveKitURL:  s.cfg.LiveKitURL,
		Identity:    identity,
		Role:        role,
		ExpiresAtMS: time.Now().Add(ttl).UnixMilli(),
		JoinID:      joinID, Encryption: binding,
	}, nil
}

// requireRoomAccess autoriza el acceso a una sala. Sin esta verja, `GetRoom` solo
// filtraba por (tenant, id), así que CUALQUIER usuario autenticado de la organización
// podía entrar a la videollamada de un canal privado o de un DM y recibir audio y
// vídeo en vivo: el permiso de la llamada se evaluaba en el ámbito TENANT de Meet
// mientras su padre —el canal— se gobierna por el roster en Chat. Y el id de sala no
// había que adivinarlo (ListRooms, RoomBySource y el BFF de Rooms lo daban).
//
// Es la misma clase de fallo que los adjuntos de canal (commit 16c3b70) y se resuelve
// igual: preguntando al dueño del roster en CADA acceso, de modo que salir del canal
// revoca la llamada al instante y sin ACL replicada aquí.
//
// Active channel calls always consult the current channel ACL. Historical
// participation grants access only to ended-call metadata, never live media.
func (s *Service) requireRoomAccess(ctx context.Context, room domain.Room, subject string, isAdmin bool) error {
	// Sala de un documento: manda la ACL del documento, también sobre el administrador, el
	// creador y quien ya participó. Quitar a alguien del documento le cierra la llamada.
	if docID := documentRef(room); docID != "" {
		return s.requireDocument(ctx, docID)
	}
	if chatChannelRef(room) == "" {
		return s.requireReunionSuelta(ctx, room, subject)
	}
	if room.Status == domain.RoomEnded && (isAdmin || (subject != "" && subject == room.CreatedBy)) {
		return nil
	}
	if room.Status == domain.RoomEnded && subject != "" {
		// ponytail: se recorre el roster de la sala (decenas de filas como máximo);
		// no compensa un método de store solo para el EXISTS.
		parts, err := s.store.ListParticipants(ctx, room.ID, false)
		if err != nil {
			return err
		}
		for _, p := range parts {
			if p.Identity == subject {
				return nil
			}
		}
	}
	// Sala de un canal de Chat: el roster vive allí, así que se pregunta a Chat.
	if s.chatNotifier == nil || subject == "" {
		return domain.ErrPermission
	}
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	ok, err := s.chatNotifier.ChannelAccess(ctx, t.Slug, chatChannelRef(room), subject)
	if err != nil || !ok {
		// Fail-closed: no se sirve una llamada privada si no se puede comprobar.
		return domain.ErrPermission
	}
	return nil
}

// requireReunionSuelta: una reunión que no cuelga de un canal ni de un documento
// —directa o de un evento de Calendar— la abren quien la convocó, quien ya estuvo
// dentro y quien trae la clave de su enlace (decidido por el dueño el 2026-10-16:
// «Invitados y quien tenga el enlace»). Ni el rol de administrador abre la puerta: los
// invitados de un evento la tienen en el enlace del evento.
func (s *Service) requireReunionSuelta(ctx context.Context, room domain.Room, subject string) error {
	if subject != "" && subject == room.CreatedBy {
		return nil
	}
	if clave := claveDe(ctx); clave != "" && room.LinkKey != "" &&
		subtle.ConstantTimeCompare([]byte(clave), []byte(room.LinkKey)) == 1 {
		return nil
	}
	if subject == "" {
		return domain.ErrPermission
	}
	// La de un evento: sus invitados, que tienen el enlace en su agenda —también el de
	// antes de que llevara clave—.
	if src := domain.SourceDeMetadata(room.Metadata); src.Kind == domain.SourceCalendar &&
		s.calGate != nil && s.calGate.EsInvitado(ctx, src.RefID) {
		return nil
	}
	// ponytail: se recorre el roster (decenas de filas como mucho), igual que arriba.
	parts, err := s.store.ListParticipants(ctx, room.ID, false)
	if err != nil {
		return err
	}
	for _, p := range parts {
		if p.Identity == subject {
			return nil
		}
	}
	return domain.ErrPermission
}

// CabeceraClave es la cabecera en la que el BFF y las apps nativas mandan la clave del
// enlace (va en una cabecera y no en la ruta: la ruta acaba en los registros de acceso).
const CabeceraClave = "X-Sytue-Room-Key"

type claveKey struct{}

// ConClave deja en el contexto la clave del enlace de reunión que trae la petición
// (cabecera X-Sytue-Room-Key).
func ConClave(ctx context.Context, clave string) context.Context {
	return context.WithValue(ctx, claveKey{}, strings.TrimSpace(clave))
}

func claveDe(ctx context.Context) string {
	c, _ := ctx.Value(claveKey{}).(string)
	return c
}

// requireChannelSource autoriza OPERAR sobre el origen de una sala antes de que la
// sala exista: si el origen es un canal de Chat, el sujeto debe poder LEER ese canal.
// Se aplica al crear o resolver una sala por (kind, ref_id), que es por donde se
// escalaba a HOST creando la sala del canal de otros. Fail-closed.
func (s *Service) requireChannelSource(ctx context.Context, kind domain.SourceKind, refID, subject string) error {
	if kind == domain.SourceDocument && s.encryption != nil {
		room := domain.Room{Metadata: map[string]any{"source": map[string]any{"kind": "document", "ref_id": refID}}}
		_, err := s.encryptedDocumentContext(ctx, room, subject)
		return err
	}
	if kind == domain.SourceDocument {
		return s.requireDocument(ctx, refID)
	}
	if kind != domain.SourceChat {
		// `calendar`: sin sonda hermana todavía (ver nota ponytail de requireRoomAccess).
		return nil
	}
	if s.chatNotifier == nil || subject == "" {
		return domain.ErrPermission
	}
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	ok, err := s.chatNotifier.ChannelAccess(ctx, t.Slug, refID, subject)
	if err != nil || !ok {
		return domain.ErrPermission
	}
	return nil
}

// requireRoomAccessFromCtx aplica requireRoomAccess resolviendo la identidad del
// llamante desde los claims del JWT verificado que el middleware dejó en el contexto
// (mismo patrón que la verja de calendar). Lo usan las lecturas que no reciben el
// sujeto por parámetro: el roster de participantes y los metadatos de grabación.
func (s *Service) requireRoomAccessFromCtx(ctx context.Context, roomID uuid.UUID) error {
	room, err := s.store.GetRoom(ctx, roomID)
	if err != nil {
		return err
	}
	return s.requireRoomAccessDe(ctx, room)
}

// requireRoomAccessDe es requireRoomAccess sobre una sala ya leída, con la identidad
// y el rol de la sesión.
func (s *Service) requireRoomAccessDe(ctx context.Context, room domain.Room) error {
	c, _ := authn.FromContext(ctx)
	_, err := s.encryptedRoomAccess(ctx, room, c.Subject, c.HasAnyRole("admin", "owner"))
	return err
}

// resolveRole decide el rol efectivo: el creador o un admin/owner del tenant
// puede ser host; los demás quedan acotados a speaker/viewer.
func resolveRole(room domain.Room, identity string, requested domain.Role, isAdmin bool) domain.Role {
	if identity != "" && identity == room.CreatedBy {
		return domain.RoleHost
	}
	if requested == domain.RoleHost && !isAdmin {
		return domain.RoleSpeaker
	}
	if !domain.ValidRole(requested) {
		return domain.RoleSpeaker
	}
	return requested
}

// ---------------------------------------------------------------------------
// Webhooks de LiveKit -> PostgreSQL
// ---------------------------------------------------------------------------

// HandleWebhookEvent resuelve el tenant del evento (embebido en la metadata de
// la room) y procesa el webhook bajo el contexto de ese tenant. Los webhooks de
// LiveKit son globales del despliegue y no portan contexto de Sytue ID; el
// tenant viaja en la metadata `sytue_tenant` que fijamos al crear la room.
func (s *Service) HandleWebhookEvent(ctx context.Context, ev livekit.WebhookEvent) error {
	slug := tenantFromEvent(ev)
	if slug == "" {
		// La metadata falta más a menudo de lo que parece, y su ausencia era silenciosa:
		// los eventos de egress nunca la traen, y una room de LiveKit puede existir SIN
		// la nuestra — si `CreateRoom` no llegó a completarse, o si la sala caducó por
		// empty_timeout y LiveKit la recreó sola al volver a entrar alguien. En ese
		// caso el `room_finished` se descartaba con un WARN y un 200: la sala quedaba
		// «activa» para siempre en PostgreSQL y el canal nunca recibía su tarjeta de
		// llamada finalizada. El nombre de la room SÍ viaja siempre y es único, así que
		// se resuelve por él vía la función acotada del store.
		slug = tenantFromRoomName(roomNameFromEvent(ev))
	}
	if slug == "" {
		// Sin tenant identificable no podemos aplicar RLS; ignoramos el evento
		// (p. ej. rooms creadas fuera de Sytue Rooms). No es un error reintentar.
		log.From(ctx).Warn("meet: webhook sin tenant en metadata; ignorado", "event", logsafe.Text(ev.Event))
		return nil
	}
	t, err := tenancy.New(slug)
	if err != nil {
		log.From(ctx).Warn("meet: tenant inválido en metadata de webhook", "slug", logsafe.Text(slug), "error", logsafe.Text(err))
		return nil
	}
	return s.HandleWebhook(tenancy.Into(ctx, t), ev)
}

// tenantFromEvent extrae el slug de tenant de la metadata de la room/egress.
func tenantFromEvent(ev livekit.WebhookEvent) string {
	if ev.Room != nil {
		if slug := tenantFromMetadata(ev.Room.Metadata); slug != "" {
			return slug
		}
	}
	return ""
}

// roomNameFromEvent devuelve el nombre de la room del evento, venga en la room o en la
// información de egress. Es el único identificador que TODOS los eventos portan.
func roomNameFromEvent(ev livekit.WebhookEvent) string {
	if ev.Room != nil && ev.Room.Name != "" {
		return ev.Room.Name
	}
	if ev.EgressInfo != nil {
		return ev.EgressInfo.RoomName
	}
	return ""
}

// tenantFromMetadata lee el campo sytue_tenant de una metadata JSON opaca.
func tenantFromMetadata(meta string) string {
	if meta == "" {
		return ""
	}
	var m struct {
		Tenant string `json:"sytue_tenant"`
	}
	if err := json.Unmarshal([]byte(meta), &m); err != nil {
		return ""
	}
	return m.Tenant
}

// HandleWebhook procesa un evento de LiveKit ya verificado (firma comprobada en
// la capa HTTP) bajo el tenant ya presente en el contexto. Actualiza el estado
// canónico en PostgreSQL y emite los eventos de dominio correspondientes. Es
// idempotente frente a reentregas.
func (s *Service) HandleWebhook(ctx context.Context, ev livekit.WebhookEvent) error {
	switch ev.Event {
	case livekit.EventRoomStarted:
		if ev.Room == nil {
			return nil
		}
		if err := s.store.MarkRoomStarted(ctx, ev.Room.Name, ev.Room.SID, eventTime(ev.CreatedAt)); err != nil {
			return err
		}
		s.publish(ctx, "meet.room.started", map[string]any{"name": ev.Room.Name, "sid": ev.Room.SID})

	case livekit.EventRoomFinished:
		return s.handleCallFinished(ctx, ev)

	case livekit.EventParticipantJoined:
		return s.handleCallJoined(ctx, ev)

	case livekit.EventParticipantLeft:
		return s.handleCallLeft(ctx, ev)

	case livekit.EventEgressStarted, livekit.EventEgressUpdated, livekit.EventEgressEnded:
		return s.handleEgress(ctx, ev)
	}
	return nil
}

// notifyCallEnded avisa a Sytue Rooms de que la sala `room` (ya en su foto pre-cierre)
// terminó, para publicar la tarjeta «Llamada finalizada · duración» en el canal ligado.
// Solo aplica a salas con origen chat; best-effort (registra el fallo, no rompe el
// webhook, para no provocar reintentos de LiveKit que dupliquen la tarjeta). El dedupe
// ante reentregas lo hace el llamante por el status previo de la sala.
func (s *Service) notifyCallEnded(ctx context.Context, room domain.Room, endedAt time.Time) {
	if s.chatNotifier == nil {
		return
	}
	channelID := chatChannelRef(room)
	if channelID == "" {
		return // no es una sala ligada a un canal de chat
	}
	var durationSec int64
	if room.StartedAt != nil {
		durationSec = int64(endedAt.Sub(*room.StartedAt).Seconds())
	}
	if durationSec < 0 {
		durationSec = 0
	}
	// El tenant viene del contexto (ya acotado por HandleWebhookEvent), no del round-trip
	// al store: Chat lo necesita para fijar RLS en su endpoint interno.
	t, _ := tenancy.From(ctx)
	if err := s.chatNotifier.CallEnded(ctx, CallEndedInput{
		Tenant:      t.Slug,
		ChannelID:   channelID,
		AuthorSub:   room.CreatedBy,
		DurationSec: durationSec,
	}); err != nil {
		log.From(ctx).Warn("meet: avisar a chat de llamada finalizada", "room", room.Name, "error", err)
	}
}

// chatChannelRef devuelve el id del canal de chat ligado a la sala (source.ref_id) si su
// origen es chat; "" en caso contrario. Lee la metadata JSON opaca (map[string]any).
func chatChannelRef(room domain.Room) string {
	src, ok := room.Metadata["source"].(map[string]any)
	if !ok {
		return ""
	}
	if kind, _ := src["kind"].(string); kind != string(domain.SourceChat) {
		return ""
	}
	ref, _ := src["ref_id"].(string)
	return ref
}

// handleEgress mapea los eventos de grabación (egress) a recordings_meta.
func (s *Service) handleEgress(ctx context.Context, ev livekit.WebhookEvent) error {
	eg := ev.EgressInfo
	if eg == nil {
		return nil
	}
	status := domain.RecordingStarted
	var ended *time.Time
	if ev.Event == livekit.EventEgressEnded {
		t := eventTime(ev.CreatedAt)
		ended = &t
		if eg.Status == "EGRESS_FAILED" || eg.Status == "EGRESS_ABORTED" {
			status = domain.RecordingFailed
		} else {
			status = domain.RecordingCompleted
		}
	}
	var storageKey string
	var size, duration int64
	if f := firstFile(eg); f != nil {
		storageKey = f.Filename
		if storageKey == "" {
			storageKey = f.Location
		}
		// El egress informa de la ruta CON el prefijo del tenant (es la que le
		// dimos), y `Location` puede venir incluso como URL completa. En la base
		// se guarda siempre la relativa: si aquí se cuela la absoluta, el upsert
		// pisa la que escribió `StartRecording` y la descarga deja de encontrar
		// el fichero, pero solo al terminar de grabar —o sea, en producción—.
		tn, _ := tenancy.From(ctx)
		storageKey = claveRelativa(tn, storageKey)
		size = int64(f.Size)
		duration = int64(f.Duration)
	}
	if err := s.store.UpsertRecording(ctx, eg.RoomName, eg.EgressID, status, storageKey, size, duration, ended); err != nil {
		return err
	}
	s.publish(ctx, "meet.recording."+string(status), map[string]any{
		"name": eg.RoomName, "egress_id": eg.EgressID,
	})
	return nil
}

// claveRelativa reduce lo que informa el egress a la clave que espera
// `objectstore`: sin esquema ni host si vino como URL, y sin el prefijo del
// tenant, que el propio cliente vuelve a anteponer al firmar.
func claveRelativa(t tenancy.Tenant, s string) string {
	if i := strings.Index(s, "://"); i >= 0 {
		resto := s[i+3:]
		// Tras el host va el bucket y luego la clave; se descartan los dos
		// primeros segmentos solo si de verdad hay algo detrás.
		if j := strings.Index(resto, "/"); j >= 0 {
			s = resto[j+1:]
			if pre := t.StoragePrefix(); !strings.HasPrefix(s, pre) {
				if k := strings.Index(s, "/"); k >= 0 && strings.HasPrefix(s[k+1:], pre) {
					s = s[k+1:]
				}
			}
		}
	}
	s = strings.TrimPrefix(s, "/")
	return strings.TrimPrefix(s, t.StoragePrefix())
}

// RecordingURL firma una descarga temporal de una grabación terminada.
//
// Solo de las TERMINADAS: mientras el egress graba, el objeto o no existe o está
// a medias, y una URL que devuelve un fichero truncado es peor que un error.
func (s *Service) RecordingURL(ctx context.Context, roomID, recordingID uuid.UUID) (string, error) {
	if s.objetos == nil {
		return "", ErrGrabacionNoConfigurada
	}
	grabaciones, err := s.store.ListRecordings(ctx, roomID)
	if err != nil {
		return "", err
	}
	for _, g := range grabaciones {
		if g.ID != recordingID {
			continue
		}
		if g.Status != domain.RecordingCompleted || g.StorageKey == "" {
			return "", domain.ErrNotFound
		}
		t, _ := tenancy.From(ctx)
		u, err := s.objetos.PresignGet(ctx, t, g.StorageKey, descargaVigencia)
		if err != nil {
			return "", fmt.Errorf("meet: firmar la descarga: %w", err)
		}
		return u.String(), nil
	}
	return "", domain.ErrNotFound
}

// DeleteRecording borra una grabación: primero el fichero del bucket y solo
// después la fila.
//
// EL ORDEN NO ES INDIFERENTE. Al revés —fila y luego objeto— un fallo a mitad
// deja en el bucket un fichero que ya no aparece en ninguna consulta: nadie lo
// puede borrar porque nadie sabe que está, y sigue siendo la conversación de un
// equipo. Así, un fallo deja la grabación entera y se puede reintentar.
//
// Borrar algo que ya no está NO es un error: esta llamada se repite (un
// reintento, dos pestañas) y S3 responde igual, así que la fila se va también.
func (s *Service) DeleteRecording(ctx context.Context, roomID, recordingID uuid.UUID) error {
	if s.objetos == nil {
		return ErrGrabacionNoConfigurada
	}
	grabaciones, err := s.store.ListRecordings(ctx, roomID)
	if err != nil {
		return err
	}
	for _, g := range grabaciones {
		if g.ID != recordingID {
			continue
		}
		t, _ := tenancy.From(ctx)
		if g.StorageKey != "" {
			if err := s.objetos.Delete(ctx, t, g.StorageKey); err != nil {
				return fmt.Errorf("meet: borrar el fichero de la grabación: %w", err)
			}
		}
		return s.store.DeleteRecording(ctx, roomID, recordingID)
	}
	return domain.ErrNotFound
}

// descargaVigencia: lo justo para pulsar el enlace y que empiece la descarga.
// Una URL firmada es un permiso al portador; cuanto menos viva, mejor.
const descargaVigencia = 10 * time.Minute

// firstFile devuelve el primer fichero resultante de un egress (file o fileResults).
func firstFile(eg *livekit.WebhookEgress) *livekit.WebhookEgressFile {
	if eg.File != nil {
		return eg.File
	}
	if len(eg.FileResults) > 0 {
		return &eg.FileResults[0]
	}
	return nil
}

// roleFromMetadata extrae el rol embebido en la metadata del participante (el
// que minteamos en JoinRoom). Por defecto speaker.
func roleFromMetadata(meta string) domain.Role {
	if meta == "" {
		return domain.RoleSpeaker
	}
	var m struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal([]byte(meta), &m); err == nil && domain.ValidRole(domain.Role(m.Role)) {
		return domain.Role(m.Role)
	}
	return domain.RoleSpeaker
}

// eventTime convierte el createdAt (segundos epoch) de un webhook a time.Time UTC.
func eventTime(epochSec livekit.Int64) time.Time {
	if epochSec <= 0 {
		return time.Now().UTC()
	}
	return time.Unix(int64(epochSec), 0).UTC()
}

// roomName compone el nombre técnico de una room: el slug del tenant, un guion bajo y
// una parte opaca e impredecible. El guion bajo separa sin ambigüedad porque un slug de
// tenant solo admite [a-z0-9-] (tenancy.ValidSlug), así que nunca contiene uno.
//
// El prefijo NO es decorativo: los webhooks de LiveKit son globales del despliegue y no
// portan contexto de Sytue ID. Antes el tenant se leía de la metadata de la room, que
// falta cada vez que LiveKit crea la room por su cuenta (la sala caducó por
// empty_timeout y alguien vuelve a entrar), y el respaldo era una función SQL
// SECURITY DEFINER que NUNCA pudo funcionar: las tablas llevan FORCE ROW LEVEL
// SECURITY, así que su dueño también queda filtrado y devolvía cero filas. El aviso se
// descartaba con un WARN y un 200: la sala quedaba «activa» para siempre y el canal
// nunca recibía su tarjeta de llamada finalizada.
//
// `propuesto` se conserva como sufijo, nunca como prefijo: si el llamante pudiera fijar
// el principio del nombre, podría atribuir sus salas a OTRO tenant.
func roomName(tenantSlug, propuesto string) string {
	if propuesto == "" {
		b := make([]byte, 9)
		_, _ = rand.Read(b)
		propuesto = hex.EncodeToString(b)
	}
	return tenantSlug + "_" + propuesto
}

// tenantFromRoomName devuelve el slug de tenant embebido en el nombre de una room, o ""
// si el nombre no lo lleva (rooms creadas antes de este formato, o ajenas a Sytue).
func tenantFromRoomName(name string) string {
	slug, _, ok := strings.Cut(name, "_")
	if !ok || !tenancy.ValidSlug(slug) {
		return ""
	}
	return slug
}

// u32 acota un entero de configuración no-negativo al rango uint32 que exige
// la API de LiveKit, evitando el overflow int→uint32 (gosec G115).
func u32(n int) uint32 {
	if n < 0 {
		return 0
	}
	if uint64(n) > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(n) //nolint:gosec // G115: acotado a [0, MaxUint32] por los guards.
}
