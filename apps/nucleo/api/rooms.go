// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Rooms es el cliente del chat de Sytue Rooms (services/rooms, `/api/v1`).
type Rooms struct {
	c *Client

	// conn es el tiempo real que tiene abierto Listen, por donde sale Typing.
	mu   sync.Mutex
	conn *websocket.Conn
}

// NewRooms crea el cliente sobre la raíz del servicio (ServiceURL(apex, "rooms")).
func NewRooms(servicio string, s Session, h *http.Client) *Rooms {
	return &Rooms{c: New(servicio+"/api/v1", s, h)}
}

// RoomsTeam es un equipo: el espacio donde viven los canales.
type RoomsTeam struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
	// Role es el de QUIEN PREGUNTA en el equipo: "admin" o "member".
	Role string `json:"role,omitempty"`
}

// RoomsChannel es un canal o una conversación directa (Kind "direct").
type RoomsChannel struct {
	ID     string `json:"id"`
	TeamID string `json:"team_id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Topic  string `json:"topic,omitempty"`
	// Purpose es «de qué va este canal», lo que se lee al explorarlos.
	Purpose  string `json:"purpose,omitempty"`
	Kind     string `json:"kind"`
	Archived bool   `json:"archived"`
	Muted    bool   `json:"muted,omitempty"`
	// Peers son los nombres de los OTROS miembros de una conversación directa.
	Peers []string `json:"peers,omitempty"`
	// PeerSubs son sus identificadores, en el mismo orden que Peers.
	PeerSubs []string `json:"peer_subs,omitempty"`
	// Favorite, NotifyLevel ("all", "mentions" o "none"), CategoryID y SortOrder son
	// de quien mira: el servicio los pone solo al listar sus canales.
	Favorite    bool   `json:"favorite,omitempty"`
	NotifyLevel string `json:"notify_level,omitempty"`
	CategoryID  string `json:"category_id,omitempty"`
	SortOrder   int    `json:"sort_order,omitempty"`
	// External es una conversación de OTRA organización en la que se está como
	// persona de fuera (ADR 0040). Solo la pone ExternalChannels.
	External bool `json:"external,omitempty"`
}

// Direct dice si es una conversación directa y no un canal.
func (c RoomsChannel) Direct() bool { return c.Kind == "direct" }

// Title es cómo se llama en la lista: un directo, por las personas que están en él.
func (c RoomsChannel) Title() string {
	if c.Direct() && len(c.Peers) > 0 {
		return strings.Join(c.Peers, ", ")
	}
	return c.Name
}

// RoomsMessage es un mensaje de un canal.
type RoomsMessage struct {
	MatrixEvent          *RoomsMatrixEventRef `json:"matrix_event,omitempty"`
	EncryptionError      bool                 `json:"-"`
	Encryption           string               `json:"encryption,omitempty"`
	IdentityVerified     bool                 `json:"-"`
	AuthenticityVerified bool                 `json:"-"`
	ID                   string               `json:"id"`
	ChannelID            string               `json:"channel_id"`
	ParentID             *string              `json:"parent_id,omitempty"`
	AuthorSub            string               `json:"author_sub"`
	AuthorName           string               `json:"author_name,omitempty"`
	Body                 string               `json:"body"`
	Type                 string               `json:"type,omitempty"`
	ReplyCount           int                  `json:"reply_count,omitempty"`
	EditedAt             *time.Time           `json:"edited_at,omitempty"`
	CreatedAt            time.Time            `json:"created_at"`
	// Mentions son los `sub` de las personas @mencionadas.
	Mentions []string `json:"mentions,omitempty"`
	// Priority es "", "important" o "urgent"; IsBot, un webhook o una integración.
	Priority  string          `json:"priority,omitempty"`
	IsBot     bool            `json:"is_bot,omitempty"`
	Reactions []RoomsReaction `json:"reactions,omitempty"`
	// RequiresAck es que el autor pide acuse; AckCount, cuántos lo han confirmado, y
	// Acked, si quien mira ya lo hizo.
	RequiresAck bool `json:"requires_ack,omitempty"`
	AckCount    int  `json:"ack_count,omitempty"`
	Acked       bool `json:"acked,omitempty"`
	// MentionsAll es que menciona a todo el canal (@channel, @here o @all).
	MentionsAll bool              `json:"mentions_all,omitempty"`
	Attachments []RoomsAttachment `json:"attachments,omitempty"`
	Card        *RoomsCard        `json:"card,omitempty"`
}

// RoomsReaction es un emoji puesto en un mensaje: cuántas veces y quién.
type RoomsReaction struct {
	Emoji string   `json:"emoji"`
	Count int      `json:"count"`
	Subs  []string `json:"subs,omitempty"`
}

// RoomsCategory es una categoría personalizada del carril de quien mira.
type RoomsCategory struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
	Collapsed bool   `json:"collapsed"`
}

// RoomsUnread es lo que hay sin leer en un canal.
type RoomsUnread struct {
	ChannelID string `json:"channel_id"`
	Unread    int    `json:"unread"`
	Mentions  int    `json:"mentions"`
}

// Teams lista los equipos de la persona; el servicio crea el de su organización si no tiene.
func (r *Rooms) Teams(ctx context.Context) ([]RoomsTeam, error) {
	out, err := JSON[struct {
		Teams []RoomsTeam `json:"teams"`
	}](ctx, r.c, Request{Path: "/teams"})
	return out.Teams, err
}

// Channels lista los canales y directos de un equipo en los que está la persona.
func (r *Rooms) Channels(ctx context.Context, equipo string) ([]RoomsChannel, error) {
	out, err := JSON[struct {
		Channels []RoomsChannel `json:"channels"`
	}](ctx, r.c, Request{Path: "/teams/" + url.PathEscape(equipo) + "/channels"})
	return out.Channels, err
}

// ExternalChannels son las conversaciones de OTRAS organizaciones en las que se está
// como persona de fuera (ADR 0040), con lo pendiente de cada una.
func (r *Rooms) ExternalChannels(ctx context.Context) ([]RoomsChannel, []RoomsUnread, error) {
	out, err := JSON[struct {
		Channels []RoomsChannel `json:"channels"`
		Unread   []RoomsUnread  `json:"unread"`
	}](ctx, r.c, Request{Path: "/me/external-channels"})
	return out.Channels, out.Unread, err
}

// RoomsInvitacion es la invitación de otra organización a un canal, un DM o un grupo:
// la gente de fuera acepta antes de entrar (decidido por el dueño el 2026-10-16).
type RoomsInvitacion struct {
	Organizacion        string `json:"organization"`
	NombreOrganizacion  string `json:"organization_name"`
	Canal               string `json:"channel_id"`
	Nombre              string `json:"name"`
	Clase               string `json:"kind"`
	NombreDeQuienInvita string `json:"invited_by_name"`
}

// Invitaciones son las invitaciones pendientes de quien mira.
func (r *Rooms) Invitaciones(ctx context.Context) ([]RoomsInvitacion, error) {
	out, err := JSON[struct {
		Invitations []RoomsInvitacion `json:"invitations"`
	}](ctx, r.c, Request{Path: "/me/invitations"})
	return out.Invitations, err
}

// ResponderInvitacion acepta (entra) o rechaza una invitación.
func (r *Rooms) ResponderInvitacion(ctx context.Context, canal string, aceptar bool) error {
	ruta := "/me/invitations/" + url.PathEscape(canal)
	if aceptar {
		return r.sinCuerpo(ctx, http.MethodPost, ruta+"/accept")
	}
	return r.sinCuerpo(ctx, http.MethodDelete, ruta)
}

// Categories son las categorías personalizadas del carril de quien mira.
func (r *Rooms) Categories(ctx context.Context) ([]RoomsCategory, error) {
	out, err := JSON[struct {
		Categories []RoomsCategory `json:"categories"`
	}](ctx, r.c, Request{Path: "/me/categories"})
	return out.Categories, err
}

// SetFavorite marca o desmarca un canal como favorito de quien mira.
func (r *Rooms) SetFavorite(ctx context.Context, canal string, favorito bool) error {
	_, err := PostJSON[RoomsChannel](ctx, r.c, "/channels/"+url.PathEscape(canal)+"/prefs", map[string]bool{"favorite": favorito})
	return err
}

// SetNotifyLevel fija cuándo avisa un canal a quien mira: "all", "mentions" o "none"
// (que es silenciarlo).
func (r *Rooms) SetNotifyLevel(ctx context.Context, canal, nivel string) error {
	_, err := PostJSON[RoomsChannel](ctx, r.c, "/channels/"+url.PathEscape(canal)+"/prefs", map[string]string{"notify_level": nivel})
	return err
}

// SetChannelCategory mueve el canal a una categoría del carril de quien mira; "" lo
// devuelve a su sección por defecto.
func (r *Rooms) SetChannelCategory(ctx context.Context, canal, categoria string) error {
	_, err := PostJSON[RoomsChannel](ctx, r.c, "/channels/"+url.PathEscape(canal)+"/category", map[string]string{"category_id": categoria})
	return err
}

// ArchiveChannel archiva un canal. Solo lo hace quien lo administra; el servicio lo
// comprueba.
func (r *Rooms) ArchiveChannel(ctx context.Context, canal string) error {
	return r.parchear(ctx, "/channels/"+url.PathEscape(canal), map[string]bool{"archived": true})
}

// DeleteChannel borra un canal público o privado. Solo lo hace quien lo administra
// (el canal o el equipo); un directo no se borra así, se oculta con HideDirect.
func (r *Rooms) DeleteChannel(ctx context.Context, canal string) error {
	return r.sinCuerpo(ctx, http.MethodDelete, "/channels/"+url.PathEscape(canal))
}

// HideDirect borra un mensaje directo solo para quien mira: sale de su carril y se le
// esconde lo anterior; los demás lo conservan. Si alguien vuelve a escribir, reaparece.
func (r *Rooms) HideDirect(ctx context.Context, canal string) error {
	return r.sinCuerpo(ctx, http.MethodPost, "/channels/"+url.PathEscape(canal)+"/hide")
}

// RoomsCategoryPatch es lo que cambia de una categoría; lo que va a nil no se toca.
type RoomsCategoryPatch struct {
	Name      *string `json:"name,omitempty"`
	SortOrder *int    `json:"sort_order,omitempty"`
	Collapsed *bool   `json:"collapsed,omitempty"`
}

// CreateCategory crea una categoría del carril de quien mira, al final del orden.
func (r *Rooms) CreateCategory(ctx context.Context, nombre string) (RoomsCategory, error) {
	return PostJSON[RoomsCategory](ctx, r.c, "/me/categories", map[string]string{"name": nombre})
}

// UpdateCategory renombra, pliega o reordena una categoría.
func (r *Rooms) UpdateCategory(ctx context.Context, id string, cambio RoomsCategoryPatch) error {
	return r.parchear(ctx, "/me/categories/"+url.PathEscape(id), cambio)
}

// DeleteCategory borra una categoría; sus canales vuelven a su sección por defecto.
func (r *Rooms) DeleteCategory(ctx context.Context, id string) error {
	return r.sinCuerpo(ctx, http.MethodDelete, "/me/categories/"+url.PathEscape(id))
}

// RoomsScheduled es un mensaje programado de quien mira.
type RoomsScheduled struct {
	ID         string    `json:"id"`
	ChannelID  string    `json:"channel_id"`
	ParentID   *string   `json:"parent_id,omitempty"`
	AuthorSub  string    `json:"author_sub"`
	AuthorName string    `json:"author_name,omitempty"`
	Body       string    `json:"body"`
	FileIDs    []string  `json:"file_ids,omitempty"`
	SendAt     time.Time `json:"send_at"`
}

// Scheduled son los mensajes programados de quien mira, de todos sus canales.
func (r *Rooms) Scheduled(ctx context.Context) ([]RoomsScheduled, error) {
	out, err := JSON[struct {
		Scheduled []RoomsScheduled `json:"scheduled"`
	}](ctx, r.c, Request{Path: "/me/scheduled"})
	return out.Scheduled, err
}

// CancelScheduled cancela un programado: no llega a enviarse.
func (r *Rooms) CancelScheduled(ctx context.Context, id string) error {
	return r.sinCuerpo(ctx, http.MethodDelete, "/scheduled/"+url.PathEscape(id))
}

// RoomsDraft es lo escrito y no enviado en un canal o, con ParentID, en un hilo.
type RoomsDraft struct {
	ChannelID string    `json:"channel_id"`
	ParentID  *string   `json:"parent_id,omitempty"`
	Body      string    `json:"body"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Drafts son los borradores de quien mira, de todos sus canales e hilos.
func (r *Rooms) Drafts(ctx context.Context) ([]RoomsDraft, error) {
	out, err := JSON[struct {
		Drafts []RoomsDraft `json:"drafts"`
	}](ctx, r.c, Request{Path: "/me/drafts"})
	return out.Drafts, err
}

// PutDraft guarda el borrador de un canal o, con `raiz`, de un hilo; un texto en
// blanco lo borra.
func (r *Rooms) PutDraft(ctx context.Context, canal, raiz, texto string) error {
	d := RoomsDraft{ChannelID: canal, Body: texto}
	if raiz != "" {
		d.ParentID = &raiz
	}
	return r.conCuerpo(ctx, http.MethodPut, "/me/drafts", struct {
		ChannelID string  `json:"channel_id"`
		ParentID  *string `json:"parent_id,omitempty"`
		Body      string  `json:"body"`
	}{d.ChannelID, d.ParentID, d.Body}) // sin `updated_at`: lo pone el servidor
}

// parchear es un PATCH con cuerpo JSON cuya respuesta no hace falta.
func (r *Rooms) parchear(ctx context.Context, ruta string, cuerpo any) error {
	return r.conCuerpo(ctx, http.MethodPatch, ruta, cuerpo)
}

// conCuerpo es una petición con cuerpo JSON cuya respuesta no hace falta.
func (r *Rooms) conCuerpo(ctx context.Context, metodo, ruta string, cuerpo any) error {
	datos, err := json.Marshal(cuerpo)
	if err != nil {
		return err
	}
	resp, err := r.c.Do(ctx, Request{
		Method: metodo, Path: ruta, Body: datos, Header: http.Header{"Content-Type": {"application/json"}},
	})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// React pone (poner true) o quita la reacción de quien mira en un mensaje.
func (r *Rooms) React(ctx context.Context, mensaje, emoji string, poner bool) error {
	metodo := http.MethodPut
	if !poner {
		metodo = http.MethodDelete
	}
	return r.sinCuerpo(ctx, metodo, "/messages/"+url.PathEscape(mensaje)+"/reactions/"+url.PathEscape(emoji))
}

// RoomsThread es un hilo: la raíz, sus respuestas en orden de lectura y si quien mira
// lo sigue.
type RoomsThread struct {
	Root      RoomsMessage   `json:"root"`
	Replies   []RoomsMessage `json:"replies"`
	Following bool           `json:"following"`
}

// Thread es un mensaje raíz con sus respuestas. Abrirlo lo da por visto.
func (r *Rooms) Thread(ctx context.Context, raiz string) (RoomsThread, error) {
	return JSON[RoomsThread](ctx, r.c, Request{Path: "/messages/" + url.PathEscape(raiz) + "/thread"})
}

// Typing avisa por el tiempo real de que se está escribiendo en un canal
// (`sendTyping`). Es efímero: sin la conexión de Listen abierta no hace nada.
func (r *Rooms) Typing(ctx context.Context, canal string) error {
	r.mu.Lock()
	conn := r.conn
	r.mu.Unlock()
	if conn == nil {
		return nil
	}
	return wsjson.Write(ctx, conn, map[string]string{"type": "typing", "channel_id": canal})
}

// RoomsThreadSummary es un hilo que sigue quien pregunta: la vista global «Hilos».
type RoomsThreadSummary struct {
	Root          RoomsMessage `json:"root"`
	LastActivity  time.Time    `json:"last_activity"`
	UnreadReplies int          `json:"unread_replies"`
}

// MyThreads son los hilos que sigue quien pregunta, del de actividad más reciente al
// más antiguo, y el cursor de la página siguiente ("" si no hay más). `antes` es ese
// cursor, o "" para la primera.
func (r *Rooms) MyThreads(ctx context.Context, antes string) ([]RoomsThreadSummary, string, error) {
	q := url.Values{}
	if antes != "" {
		q.Set("before", antes)
	}
	out, err := JSON[struct {
		Threads    []RoomsThreadSummary `json:"threads"`
		NextBefore string               `json:"next_before"`
	}](ctx, r.c, Request{Path: "/me/threads", Query: q})
	return out.Threads, out.NextBefore, err
}

// Follow empieza (seguir true) o deja de seguir el hilo de un mensaje.
func (r *Rooms) Follow(ctx context.Context, raiz string, seguir bool) error {
	return r.sinCuerpo(ctx, ponerOQuitar(seguir), "/messages/"+url.PathEscape(raiz)+"/follow")
}

// Reply publica una respuesta en el hilo de un mensaje.
func (r *Rooms) Reply(ctx context.Context, raiz, texto string) (RoomsMessage, error) {
	return PostJSON[RoomsMessage](ctx, r.c, "/messages/"+url.PathEscape(raiz)+"/replies", map[string]string{"body": texto})
}

// Edit cambia el texto de un mensaje propio y devuelve el mensaje ya editado.
func (r *Rooms) Edit(ctx context.Context, mensaje, texto string) (RoomsMessage, error) {
	datos, err := json.Marshal(map[string]string{"body": texto})
	if err != nil {
		return RoomsMessage{}, err
	}
	return JSON[RoomsMessage](ctx, r.c, Request{
		Method: http.MethodPatch, Path: "/messages/" + url.PathEscape(mensaje), Body: datos,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// Delete borra un mensaje propio.
func (r *Rooms) Delete(ctx context.Context, mensaje string) error {
	return r.sinCuerpo(ctx, http.MethodDelete, "/messages/"+url.PathEscape(mensaje))
}

// Pin fija (poner true) o desfija un mensaje en su canal.
func (r *Rooms) Pin(ctx context.Context, mensaje string, poner bool) error {
	return r.sinCuerpo(ctx, ponerOQuitar(poner), "/messages/"+url.PathEscape(mensaje)+"/pin")
}

// Save guarda (poner true) o quita de guardados un mensaje, para quien mira.
func (r *Rooms) Save(ctx context.Context, mensaje string, poner bool) error {
	return r.sinCuerpo(ctx, ponerOQuitar(poner), "/messages/"+url.PathEscape(mensaje)+"/save")
}

// MarkUnread deja el canal del mensaje sin leer a partir de él.
func (r *Rooms) MarkUnread(ctx context.Context, mensaje string) error {
	return r.sinCuerpo(ctx, http.MethodPost, "/messages/"+url.PathEscape(mensaje)+"/unread")
}

// Pins son los mensajes fijados de un canal.
func (r *Rooms) Pins(ctx context.Context, canal string) ([]RoomsMessage, error) {
	out, err := JSON[struct {
		Pins []RoomsMessage `json:"pins"`
	}](ctx, r.c, Request{Path: "/channels/" + url.PathEscape(canal) + "/pins"})
	return out.Pins, err
}

// Saved es la primera página de los mensajes que quien mira ha guardado.
func (r *Rooms) Saved(ctx context.Context) ([]RoomsMessage, error) {
	out, err := JSON[struct {
		Messages []RoomsMessage `json:"messages"`
	}](ctx, r.c, Request{Path: "/me/saved"})
	return out.Messages, err
}

func ponerOQuitar(poner bool) string {
	if poner {
		return http.MethodPost
	}
	return http.MethodDelete
}

// sinCuerpo es una petición que contesta 204.
func (r *Rooms) sinCuerpo(ctx context.Context, metodo, ruta string) error {
	resp, err := r.c.Do(ctx, Request{Method: metodo, Path: ruta})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// Search busca en todos los canales a los que se tiene acceso (`from:`, `in:`,
// `before:`, `after:` y `has:file` se pasan tal cual). Va por QUERY, con GET de
// respaldo.
func (r *Rooms) Search(ctx context.Context, q string, limite int) ([]RoomsMessage, error) {
	out, err := Consulta[struct {
		Messages []RoomsMessage `json:"messages"`
	}](ctx, r.c, Request{Path: "/search", Query: url.Values{"q": {q}, "limit": {strconv.Itoa(limite)}}})
	return out.Messages, err
}

// Around es la ventana de un canal CENTRADA en un mensaje, en orden de lectura (el
// permalink). Si el mensaje era una respuesta, la ventana se ancla en su raíz y
// `raiz` dice cuál es, para abrir ese hilo.
func (r *Rooms) Around(ctx context.Context, canal, mensaje string, limite int) (ms []RoomsMessage, raiz string, err error) {
	out, err := JSON[struct {
		Messages     []RoomsMessage `json:"messages"`
		ThreadRootID string         `json:"thread_root_id"`
	}](ctx, r.c, Request{
		Path:  "/channels/" + url.PathEscape(canal) + "/messages/around",
		Query: url.Values{"message": {mensaje}, "limit": {strconv.Itoa(limite)}},
	})
	return out.Messages, out.ThreadRootID, err
}

// CreateChannel crea un canal en el equipo. `privado` lo hace privado; `tema` vacío,
// sin tema. El servicio valida el nombre y lo pasa a slug: un 409 es que ya existe, un
// 400 que el nombre no vale y un 403 que no se puede crear.
func (r *Rooms) CreateChannel(ctx context.Context, equipo, nombre, tema string, privado bool) (RoomsChannel, error) {
	kind := "public"
	if privado {
		kind = "private"
	}
	return PostJSON[RoomsChannel](ctx, r.c, "/teams/"+url.PathEscape(equipo)+"/channels", struct {
		Name  string `json:"name"`
		Kind  string `json:"kind"`
		Topic string `json:"topic,omitempty"`
	}{nombre, kind, tema})
}

// Browse son los canales públicos del equipo a los que aún no se pertenece.
func (r *Rooms) Browse(ctx context.Context, equipo, q string) ([]RoomsChannel, error) {
	out, err := JSON[struct {
		Channels []RoomsChannel `json:"channels"`
	}](ctx, r.c, Request{Path: "/teams/" + url.PathEscape(equipo) + "/channels/browse", Query: url.Values{"q": {q}}})
	return out.Channels, err
}

// Join une a quien mira a un canal público.
func (r *Rooms) Join(ctx context.Context, canal string) (RoomsChannel, error) {
	return JSON[RoomsChannel](ctx, r.c, Request{Method: http.MethodPost, Path: "/channels/" + url.PathEscape(canal) + "/join"})
}

// OpenDirect abre (o recupera) la conversación directa con esas personas.
func (r *Rooms) OpenDirect(ctx context.Context, equipo string, personas []string) (RoomsChannel, error) {
	return PostJSON[RoomsChannel](ctx, r.c, "/teams/"+url.PathEscape(equipo)+"/dms", map[string][]string{"subjects": personas})
}

// RoomsMember es alguien de un equipo o de un canal.
type RoomsMember struct {
	Subject string `json:"subject"`
	Name    string `json:"name,omitempty"`
	Role    string `json:"role"`
	// External es de otra organización (ADR 0040); solo en el roster de un canal.
	External bool `json:"external,omitempty"`
	// Pending: de fuera y aún sin aceptar la invitación; no es miembro todavía.
	Pending bool `json:"pending,omitempty"`
}

// RoomsPerson es alguien de CUALQUIER organización encontrado por su @usuario o su
// correo en Sytue ID (ADR 0040).
type RoomsPerson struct {
	UserID   string `json:"user_id"`
	Name     string `json:"name"`
	Username string `json:"username,omitempty"`
}

// TeamMembers es la gente del equipo.
func (r *Rooms) TeamMembers(ctx context.Context, equipo string) ([]RoomsMember, error) {
	out, err := JSON[struct {
		Members []RoomsMember `json:"members"`
	}](ctx, r.c, Request{Path: "/teams/" + url.PathEscape(equipo) + "/members"})
	return out.Members, err
}

// ChannelMembers es el roster de un canal; el `role` es el rol en el CANAL.
func (r *Rooms) ChannelMembers(ctx context.Context, canal string) ([]RoomsMember, error) {
	out, err := JSON[struct {
		Members []RoomsMember `json:"members"`
	}](ctx, r.c, Request{Path: "/channels/" + url.PathEscape(canal) + "/members"})
	return out.Members, err
}

// AddChannelMember añade a alguien al canal (del equipo o de fuera) y devuelve cómo
// quedó en el roster.
func (r *Rooms) AddChannelMember(ctx context.Context, canal, persona, nombre string) (RoomsMember, error) {
	return PostJSON[RoomsMember](ctx, r.c, "/channels/"+url.PathEscape(canal)+"/members", struct {
		Subject string `json:"subject"`
		Name    string `json:"name,omitempty"`
	}{persona, nombre})
}

// RemoveChannelMember saca a alguien del canal.
func (r *Rooms) RemoveChannelMember(ctx context.Context, canal, persona string) error {
	return r.sinCuerpo(ctx, http.MethodDelete, "/channels/"+url.PathEscape(canal)+"/members/"+url.PathEscape(persona))
}

// SetChannelMemberRole hace a alguien "admin" o "member" del canal.
func (r *Rooms) SetChannelMemberRole(ctx context.Context, canal, persona, rol string) (RoomsMember, error) {
	datos, err := json.Marshal(map[string]string{"role": rol})
	if err != nil {
		return RoomsMember{}, err
	}
	return JSON[RoomsMember](ctx, r.c, Request{
		Method: http.MethodPatch, Path: "/channels/" + url.PathEscape(canal) + "/members/" + url.PathEscape(persona),
		Body: datos, Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// ResolvePerson busca a alguien por su @usuario o su correo. Un 404 es que no existe,
// un 429 que se ha preguntado demasiado y un 400 que lo escrito no vale.
func (r *Rooms) ResolvePerson(ctx context.Context, consulta string) (RoomsPerson, error) {
	return PostJSON[RoomsPerson](ctx, r.c, "/people:resolve", map[string]string{"query": consulta})
}

// Unread devuelve lo pendiente de cada canal del equipo.
func (r *Rooms) Unread(ctx context.Context, equipo string) ([]RoomsUnread, error) {
	out, err := JSON[struct {
		Channels []RoomsUnread `json:"channels"`
	}](ctx, r.c, Request{Path: "/teams/" + url.PathEscape(equipo) + "/unread"})
	return out.Channels, err
}

// Messages devuelve una página de un canal EN ORDEN DE LECTURA —el más viejo primero—. El
// servicio los da al revés (el más nuevo primero, que es como se pagina hacia atrás).
// antes cero es la página más reciente.
func (r *Rooms) Messages(ctx context.Context, canal string, antes time.Time, limite int) ([]RoomsMessage, error) {
	q := url.Values{"limit": {strconv.Itoa(limite)}}
	if !antes.IsZero() {
		q.Set("before", antes.UTC().Format(time.RFC3339Nano))
	}
	out, err := JSON[struct {
		Messages []RoomsMessage `json:"messages"`
	}](ctx, r.c, Request{Path: "/channels/" + url.PathEscape(canal) + "/messages", Query: q})
	slices.Reverse(out.Messages)
	return out.Messages, err
}

// Post publica un mensaje en un canal.
func (r *Rooms) Post(ctx context.Context, canal, texto string) (RoomsMessage, error) {
	return PostJSON[RoomsMessage](ctx, r.c, "/channels/"+url.PathEscape(canal)+"/messages", map[string]string{"body": texto})
}

// MarkRead da el canal por leído hasta ahora.
func (r *Rooms) MarkRead(ctx context.Context, canal string) error {
	return r.sinCuerpo(ctx, http.MethodPost, "/channels/"+url.PathEscape(canal)+"/read")
}

// RoomsEvent es una trama del tiempo real: qué ha pasado (`message`, `message_edited`,
// `typing`…, los `Server*` de services/rooms/internal/ws/protocol.go) y en qué canal.
type RoomsEvent struct {
	Type      string `json:"type"`
	ChannelID string `json:"channel_id"`
	// Subject y Name son quién: los trae, por ejemplo, «escribiendo».
	Subject string `json:"subject,omitempty"`
	Name    string `json:"name,omitempty"`
	// Payload es el recurso afectado tal cual lo serializa el servicio: en `message`,
	// el mensaje (con su `parent_id` si es una respuesta).
	Payload json.RawMessage `json:"payload,omitempty"`
}

// RoomsReady es la primera trama: la conexión está aceptada.
const RoomsReady = "ready"

// Listen abre el WebSocket de Rooms, se suscribe a esos canales y entrega cada trama
// hasta que el contexto muera o la conexión caiga; vuelve siempre con el porqué. No
// reconecta: eso lo decide quien llama.
//
// En el escritorio el token va en la cabecera; en la web no hay token que poner —lo pone
// el BFF, que reconoce a la persona por la cookie— ni forma de poner cabeceras.
func (r *Rooms) Listen(ctx context.Context, canales []string, alLlegar func(RoomsEvent)) error {
	token := ""
	if r.c.session != nil {
		t, err := r.c.token(ctx, r.c.base)
		if err != nil {
			return err
		}
		token = t
	}
	// `/ws` cuelga de la raíz del servicio, no de `/api/v1` como todo lo demás.
	destino := strings.TrimSuffix(r.c.WSURL("", nil), "/api/v1") + "/ws"
	conn, resp, err := websocket.Dial(ctx, destino, opcionesWS(r.c.http, token))
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("rooms: tiempo real: %w", err)
	}
	r.mu.Lock()
	r.conn = conn
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		if r.conn == conn {
			r.conn = nil
		}
		r.mu.Unlock()
		_ = conn.CloseNow()
	}()
	// Una trama lleva el mensaje entero; con el tope por defecto (32 KiB) uno largo
	// cerraba la conexión, y reconectar volvía a recibirlo.
	conn.SetReadLimit(1 << 20)
	for _, canal := range canales {
		if err := wsjson.Write(ctx, conn, map[string]string{"type": "subscribe", "channel_id": canal}); err != nil {
			return fmt.Errorf("rooms: tiempo real: %w", err)
		}
	}
	for {
		var ev RoomsEvent
		if err := wsjson.Read(ctx, conn, &ev); err != nil {
			return fmt.Errorf("rooms: tiempo real: %w", err)
		}
		alLlegar(ev)
	}
}
