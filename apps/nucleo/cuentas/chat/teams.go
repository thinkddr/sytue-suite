// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
)

// ----- Lo movido del servicio de integraciones (external_chat_teams.go) -----

// MensajeGraph es un `chatMessage` de Microsoft Graph tal como llega, con lo que se usa
// de él. Era `teamsExternalMessage` del servicio de integraciones (por eso sus campos
// conservan el nombre); lo demás se añadió para Sytue Office.
type MensajeGraph struct {
	ID       string     `json:"id"`
	Created  time.Time  `json:"createdDateTime"`      //nolint:tagliatelle // contrato de Graph
	Modified *time.Time `json:"lastModifiedDateTime"` //nolint:tagliatelle // contrato de Graph
	Edited   *time.Time `json:"lastEditedDateTime"`   //nolint:tagliatelle // contrato de Graph
	Deleted  *time.Time `json:"deletedDateTime"`      //nolint:tagliatelle // contrato de Graph
	// IsDeleted es el de la vista previa de un chat (`chatMessageInfo`), que no trae
	// `deletedDateTime`.
	IsDeleted bool   `json:"isDeleted"`   //nolint:tagliatelle // contrato de Graph
	ReplyTo   string `json:"replyToId"`   //nolint:tagliatelle // contrato de Graph
	Type      string `json:"messageType"` //nolint:tagliatelle // contrato de Graph
	ChatID    string `json:"chatId"`      //nolint:tagliatelle // contrato de Graph
	From      struct {
		User struct {
			ID   string `json:"id"`
			Name string `json:"displayName"` //nolint:tagliatelle // contrato de Graph
		} `json:"user"`
		Application *struct {
			ID   string `json:"id"`
			Name string `json:"displayName"` //nolint:tagliatelle // contrato de Graph
		} `json:"application"`
	} `json:"from"`
	Body struct {
		Type    string `json:"contentType"` //nolint:tagliatelle // contrato de Graph
		Content string `json:"content"`
	} `json:"body"`
	Reactions []ReaccionGraph `json:"reactions"`
	// Replies son las respuestas de un mensaje de canal pedido con `$expand=replies`.
	Replies []MensajeGraph `json:"replies"`
	// Attachments son los ficheros del mensaje (los de OneDrive o SharePoint van como
	// `reference`, con su enlace).
	Attachments []AdjuntoGraph `json:"attachments"`
}

// ReaccionGraph es una reacción a un mensaje.
type ReaccionGraph struct {
	Tipo string    `json:"reactionType"`    //nolint:tagliatelle // contrato de Graph
	En   time.Time `json:"createdDateTime"` //nolint:tagliatelle // contrato de Graph
	User struct {
		User struct {
			ID   string `json:"id"`
			Name string `json:"displayName"` //nolint:tagliatelle // contrato de Graph
		} `json:"user"`
	} `json:"user"`
}

// Texto es el cuerpo en texto: el HTML de Teams pasa por `htmltext` (sin etiquetas ni
// guiones, sin las imágenes que rastrean).
func (m MensajeGraph) Texto() string {
	if m.Body.Type == "html" {
		return htmltext.Text(m.Body.Content)
	}
	return m.Body.Content
}

// Borrado dice si el mensaje está borrado (en la lista o en la vista previa).
func (m MensajeGraph) Borrado() bool { return m.Deleted != nil || m.IsDeleted }

// Cambiado es la última vez que cambió: al editarlo, al borrarlo o, si no, al crearlo.
func (m MensajeGraph) Cambiado() time.Time {
	t := m.Created
	for _, x := range []*time.Time{m.Modified, m.Edited, m.Deleted} {
		if x != nil && x.After(t) {
			t = *x
		}
	}
	return t
}

// IDChatTeamsValido dice si id es un chat de Teams que se puede poner en una ruta. Era
// `externalTeamsChatID`.
func IDChatTeamsValido(id string) bool {
	return id != "" && len(id) <= 512 && !strings.ContainsAny(id, "/?#\r\n\x00")
}

// IDMensajeTeamsValido es lo mismo para un mensaje. Era `externalTeamsMessageID`.
func IDMensajeTeamsValido(id string) bool {
	return id != "" && len(id) <= 128 && !strings.ContainsAny(id, "/?#\r\n\x00")
}

// IDCanalTeamsValido dice si remoto es `<equipo>/<canal>` con el equipo un GUID. Era
// `validRoomsTeamsID`.
func IDCanalTeamsValido(remoto string) bool {
	team, channel, ok := strings.Cut(remoto, "/")
	_, err := uuid.Parse(team)
	return ok && err == nil && channel != "" && len(channel) <= 512 && !strings.ContainsAny(channel, "/?#\r\n\x00")
}

// PrefijoChat es el de los chats en un identificador remoto: `chat/<id>`; un canal de
// equipo es `<equipo>/<canal>`.
const PrefijoChat = "chat/"

// RemotoChat es el remoto de un chat.
func RemotoChat(id string) string { return PrefijoChat + id }

// EsChat dice si remoto es un chat (y no un canal de equipo) y da su id.
func EsChat(remoto string) (string, bool) { return strings.CutPrefix(remoto, PrefijoChat) }

// RutaMensajesTeams es la ruta de Graph de los mensajes de remoto (un chat o un canal)
// y, con hilo, la de las respuestas a ese mensaje (solo en canales: los chats no tienen
// hilos). Era `externalTeamsPath`.
func RutaMensajesTeams(remoto, hilo string) (string, error) {
	if chat, ok := EsChat(remoto); ok {
		if !IDChatTeamsValido(chat) || hilo != "" {
			return "", ErrEntrada
		}
		return "/chats/" + url.PathEscape(chat) + "/messages", nil
	}
	if !IDCanalTeamsValido(remoto) {
		return "", ErrEntrada
	}
	team, channel, _ := strings.Cut(remoto, "/")
	path := "/teams/" + url.PathEscape(team) + "/channels/" + url.PathEscape(channel) + "/messages"
	if hilo != "" {
		if !IDMensajeTeamsValido(hilo) {
			return "", ErrEntrada
		}
		path += "/" + url.PathEscape(hilo) + "/replies"
	}
	return path, nil
}

// CuerpoTexto es el cuerpo de un mensaje de texto de Graph.
func CuerpoTexto(texto string) map[string]any {
	return map[string]any{"body": map[string]string{"contentType": "text", "content": texto}}
}

// ----- El cliente -----

// Graph es el cliente de Teams en Microsoft Graph con un token delegado de la persona.
type Graph struct {
	// Base es la de Graph con su versión (`…/v1.0`): la pone quien lo usa.
	Base  string
	Token Token
	HTTP  HTTPDoer
	// Limite es lo más que se lee de una respuesta (cero: MaxRespuesta).
	Limite int64
}

func (g *Graph) c() cliente { return cliente{http: g.HTTP, token: g.Token, limite: g.Limite} }

func (g *Graph) url(ruta string, q url.Values) string { return unir(g.Base, ruta, q) }

// Pedir es una petición a Graph para lo que el cliente no tiene método: ruta relativa a
// Base (con su consulta) o dirección absoluta del mismo origen.
func (g *Graph) Pedir(ctx context.Context, metodo, ruta string, cuerpo, out any) error {
	d, err := destino(g.Base, ruta)
	if err != nil {
		return err
	}
	return g.c().pedir(ctx, metodo, d, cuerpo, out)
}

// UsuarioGraph es quien entró.
type UsuarioGraph struct {
	ID     string `json:"id"`
	Nombre string `json:"displayName"`       //nolint:tagliatelle // contrato de Graph
	Correo string `json:"mail"`              //nolint:tagliatelle // contrato de Graph
	UPN    string `json:"userPrincipalName"` //nolint:tagliatelle // contrato de Graph
}

// Yo es quien entró (`/me`).
func (g *Graph) Yo(ctx context.Context) (UsuarioGraph, error) {
	var u UsuarioGraph
	if err := g.c().pedir(ctx, http.MethodGet, g.url("/me", url.Values{"$select": {"id,displayName,mail,userPrincipalName"}}), nil, &u); err != nil {
		return u, err
	}
	if _, err := uuid.Parse(u.ID); err != nil {
		return u, ErrIdentidad
	}
	return u, nil
}

// MiembroGraph es una persona de un chat o de un canal.
type MiembroGraph struct {
	ID      string   `json:"id"`
	Nombre  string   `json:"displayName"` //nolint:tagliatelle // contrato de Graph
	Usuario string   `json:"userId"`      //nolint:tagliatelle // contrato de Graph
	Correo  string   `json:"email"`
	Roles   []string `json:"roles"`
}

// ChatGraph es un chat (1:1, de grupo o de una reunión).
//
//nolint:revive // va como MiembroGraph o MensajeGraph, y Graph ya es el cliente; local/chat lo usa con este nombre
type ChatGraph struct {
	ID        string         `json:"id"`
	Tema      string         `json:"topic"`
	Tipo      string         `json:"chatType"`        //nolint:tagliatelle // contrato de Graph
	Inquilino string         `json:"tenantId"`        //nolint:tagliatelle // contrato de Graph
	Creado    time.Time      `json:"createdDateTime"` //nolint:tagliatelle // contrato de Graph
	Miembros  []MiembroGraph `json:"members"`
	// Ultimo es la vista previa del último mensaje (`$expand=lastMessagePreview`).
	Ultimo *MensajeGraph `json:"lastMessagePreview"` //nolint:tagliatelle // contrato de Graph
	Vista  *VistaChat    `json:"viewpoint"`
}

// VistaChat es lo del chat que es de quien mira: si lo ocultó y hasta cuándo lo leyó.
type VistaChat struct {
	Oculto bool       `json:"isHidden"`                //nolint:tagliatelle // contrato de Graph
	Leido  *time.Time `json:"lastMessageReadDateTime"` //nolint:tagliatelle // contrato de Graph
}

type paginaGraph[T any] struct {
	Valor     []T    `json:"value"`
	Siguiente string `json:"@odata.nextLink"`  //nolint:tagliatelle // contrato de Graph
	Delta     string `json:"@odata.deltaLink"` //nolint:tagliatelle // contrato de Graph
}

// todas sigue las páginas de un listado de Graph dentro de su origen.
func todas[T any](ctx context.Context, g *Graph, destino string, tope int) ([]T, error) {
	var out []T
	for range MaxPaginas {
		var p paginaGraph[T]
		if err := g.c().pedir(ctx, http.MethodGet, destino, nil, &p); err != nil {
			return out, err
		}
		out = append(out, p.Valor...)
		if p.Siguiente == "" || len(out) >= tope {
			break
		}
		if !MismoOrigen(g.Base, p.Siguiente) {
			return out, ErrRespuesta
		}
		destino = p.Siguiente
	}
	return out, nil
}

// MaxChats es cuántos chats se listan como mucho.
const MaxChats = 500

// Chats son los de la persona con sus miembros y la vista previa de su último mensaje.
// Si Graph no acepta pedir las dos cosas a la vez, se piden solo los miembros.
func (g *Graph) Chats(ctx context.Context) ([]ChatGraph, error) {
	cs, err := todas[ChatGraph](ctx, g, g.url("/me/chats", url.Values{"$top": {"50"}, "$expand": {"members,lastMessagePreview"}}), MaxChats)
	if Estado(err) == http.StatusBadRequest {
		cs, err = todas[ChatGraph](ctx, g, g.url("/me/chats", url.Values{"$top": {"50"}, "$expand": {"members"}}), MaxChats)
	}
	return cs, err
}

// PaginaChats es una página de chats con sus miembros, desde el `$skiptoken` de la
// anterior (vacío: la primera), y el enlace de la siguiente tal como lo da Graph (quien
// lo siga, lo comprueba).
func (g *Graph) PaginaChats(ctx context.Context, salto string) ([]ChatGraph, string, error) {
	q := url.Values{"$top": {"50"}, "$expand": {"members"}}
	if salto != "" {
		q.Set("$skiptoken", salto)
	}
	var p paginaGraph[ChatGraph]
	err := g.c().pedir(ctx, http.MethodGet, g.url("/me/chats", q), nil, &p)
	return p.Valor, p.Siguiente, err
}

// PaginaMensajes es una página de los mensajes de remoto (o de las respuestas a hilo,
// en un canal) de n en n, desde el `$skiptoken` de la anterior, y el enlace de la
// siguiente tal como lo da Graph.
func (g *Graph) PaginaMensajes(ctx context.Context, remoto, hilo, salto string, n int) ([]MensajeGraph, string, error) {
	ruta, err := RutaMensajesTeams(remoto, hilo)
	if err != nil {
		return nil, "", err
	}
	q := url.Values{"$top": {strconv.Itoa(min(max(n, 1), 50))}}
	if salto != "" {
		q.Set("$skiptoken", salto)
	}
	var p paginaGraph[MensajeGraph]
	err = g.c().pedir(ctx, http.MethodGet, g.url(ruta, q), nil, &p)
	return p.Valor, p.Siguiente, err
}

// ChatsRecientes es una página de chats del de mensaje más reciente al más viejo, con
// la vista previa de ese mensaje: lo que mira el sondeo para saber dónde hay algo nuevo
// sin pedir los mensajes de cada chat.
func (g *Graph) ChatsRecientes(ctx context.Context) ([]ChatGraph, error) {
	var p paginaGraph[ChatGraph]
	err := g.c().pedir(ctx, http.MethodGet, g.url("/me/chats", url.Values{
		"$top": {"50"}, "$expand": {"lastMessagePreview"}, "$orderby": {"lastMessagePreview/createdDateTime desc"},
	}), nil, &p)
	return p.Valor, err
}

func fechaGraph(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

// MensajesChat es una página de un chat, del más nuevo al más viejo, de los anteriores a
// antes (cero: los últimos).
func (g *Graph) MensajesChat(ctx context.Context, chat string, antes time.Time, n int) ([]MensajeGraph, error) {
	ruta, err := RutaMensajesTeams(RemotoChat(chat), "")
	if err != nil {
		return nil, err
	}
	q := url.Values{"$top": {strconv.Itoa(min(max(n, 1), 50))}, "$orderby": {"createdDateTime desc"}}
	if !antes.IsZero() {
		q.Set("$filter", "createdDateTime lt "+fechaGraph(antes))
	}
	var p paginaGraph[MensajeGraph]
	err = g.c().pedir(ctx, http.MethodGet, g.url(ruta, q), nil, &p)
	return p.Valor, err
}

// CambiosChat son los mensajes de un chat creados, editados o borrados después de desde
// (Graph no da `delta` de los chats: se pregunta por fecha), del más nuevo al más viejo.
func (g *Graph) CambiosChat(ctx context.Context, chat string, desde time.Time) ([]MensajeGraph, error) {
	ruta, err := RutaMensajesTeams(RemotoChat(chat), "")
	if err != nil {
		return nil, err
	}
	return todas[MensajeGraph](ctx, g, g.url(ruta, url.Values{
		"$top": {"50"}, "$orderby": {"lastModifiedDateTime desc"}, "$filter": {"lastModifiedDateTime gt " + fechaGraph(desde)},
	}), 200)
}

// Enviar publica texto en remoto (un chat o un canal); con hilo, como respuesta a ese
// mensaje de canal.
func (g *Graph) Enviar(ctx context.Context, remoto, hilo, texto string) (MensajeGraph, error) {
	ruta, err := RutaMensajesTeams(remoto, hilo)
	if err != nil {
		return MensajeGraph{}, err
	}
	var m MensajeGraph
	err = g.c().pedir(ctx, http.MethodPost, g.url(ruta, nil), CuerpoTexto(texto), &m)
	return m, err
}

// rutaMensaje es la de un mensaje (de chat o de canal).
func rutaMensaje(remoto, mensaje string) (string, error) {
	ruta, err := RutaMensajesTeams(remoto, "")
	if err != nil || !IDMensajeTeamsValido(mensaje) {
		return "", ErrEntrada
	}
	return ruta + "/" + url.PathEscape(mensaje), nil
}

// Mensaje trae un mensaje.
func (g *Graph) Mensaje(ctx context.Context, remoto, mensaje string) (MensajeGraph, error) {
	ruta, err := rutaMensaje(remoto, mensaje)
	if err != nil {
		return MensajeGraph{}, err
	}
	var m MensajeGraph
	err = g.c().pedir(ctx, http.MethodGet, g.url(ruta, nil), nil, &m)
	return m, err
}

// Editar cambia el texto de un mensaje propio de un chat (en los canales pide un
// permiso de administrador, y no se ofrece).
func (g *Graph) Editar(ctx context.Context, remoto, mensaje, texto string) error {
	if _, ok := EsChat(remoto); !ok {
		return ErrEntrada
	}
	ruta, err := rutaMensaje(remoto, mensaje)
	if err != nil {
		return err
	}
	return g.c().pedir(ctx, http.MethodPatch, g.url(ruta, nil), CuerpoTexto(texto), nil)
}

// Borrar borra un mensaje propio de un chat (`softDelete` a nombre de yo).
func (g *Graph) Borrar(ctx context.Context, yo, remoto, mensaje string) error {
	if _, err := uuid.Parse(yo); err != nil {
		return ErrEntrada
	}
	if _, ok := EsChat(remoto); !ok {
		return ErrEntrada
	}
	ruta, err := rutaMensaje(remoto, mensaje)
	if err != nil {
		return err
	}
	return g.c().pedir(ctx, http.MethodPost, g.url("/users/"+url.PathEscape(yo)+ruta+"/softDelete", nil), nil, nil)
}

// MarcarLeido da el chat por leído para yo.
func (g *Graph) MarcarLeido(ctx context.Context, chat, yo, inquilino string) error {
	if !IDChatTeamsValido(chat) {
		return ErrEntrada
	}
	cuerpo := map[string]any{"user": map[string]string{"id": yo, "tenantId": inquilino}}
	return g.c().pedir(ctx, http.MethodPost, g.url("/chats/"+url.PathEscape(chat)+"/markChatReadForUser", nil), cuerpo, nil)
}

// EquipoGraph es un equipo del que la persona es miembro.
type EquipoGraph struct {
	ID          string `json:"id"`
	Nombre      string `json:"displayName"` //nolint:tagliatelle // contrato de Graph
	Descripcion string `json:"description"`
}

// CanalGraph es un canal de un equipo.
type CanalGraph struct {
	ID          string    `json:"id"`
	Nombre      string    `json:"displayName"` //nolint:tagliatelle // contrato de Graph
	Descripcion string    `json:"description"`
	Tipo        string    `json:"membershipType"`  //nolint:tagliatelle // contrato de Graph
	Creado      time.Time `json:"createdDateTime"` //nolint:tagliatelle // contrato de Graph
}

// Equipos son los de la persona (`/me/joinedTeams`).
func (g *Graph) Equipos(ctx context.Context) ([]EquipoGraph, error) {
	es, err := todas[EquipoGraph](ctx, g, g.url("/me/joinedTeams", url.Values{"$select": {"id,displayName,description"}}), 200)
	return slices.DeleteFunc(es, func(e EquipoGraph) bool { _, err := uuid.Parse(e.ID); return err != nil }), err
}

// Canales son los de un equipo.
func (g *Graph) Canales(ctx context.Context, equipo string) ([]CanalGraph, error) {
	if _, err := uuid.Parse(equipo); err != nil {
		return nil, ErrEntrada
	}
	cs, err := todas[CanalGraph](ctx, g, g.url("/teams/"+url.PathEscape(equipo)+"/channels", url.Values{"$select": {"id,displayName,description,membershipType,createdDateTime"}}), 500)
	return slices.DeleteFunc(cs, func(c CanalGraph) bool { return !IDCanalTeamsValido(equipo + "/" + c.ID) }), err
}

// MensajesCanal es una página de raíces de un canal con sus respuestas, de la más nueva
// a la más vieja, y el enlace de la siguiente (más vieja). Con siguiente, sigue ese.
func (g *Graph) MensajesCanal(ctx context.Context, remoto, siguiente string) ([]MensajeGraph, string, error) {
	destino := siguiente
	if destino == "" {
		ruta, err := RutaMensajesTeams(remoto, "")
		if err != nil {
			return nil, "", err
		}
		if _, ok := EsChat(remoto); ok {
			return nil, "", ErrEntrada
		}
		destino = g.url(ruta, url.Values{"$top": {"50"}, "$expand": {"replies"}})
	} else if !MismoOrigen(g.Base, destino) {
		return nil, "", ErrEntrada
	}
	var p paginaGraph[MensajeGraph]
	if err := g.c().pedir(ctx, http.MethodGet, destino, nil, &p); err != nil {
		return nil, "", err
	}
	if p.Siguiente != "" && !MismoOrigen(g.Base, p.Siguiente) {
		return p.Valor, "", ErrRespuesta
	}
	return p.Valor, p.Siguiente, nil
}

// Respuestas son las de un mensaje de canal.
func (g *Graph) Respuestas(ctx context.Context, remoto, raiz string) ([]MensajeGraph, error) {
	ruta, err := RutaMensajesTeams(remoto, raiz)
	if err != nil {
		return nil, err
	}
	return todas[MensajeGraph](ctx, g, g.url(ruta, url.Values{"$top": {"50"}}), 1000)
}

// DeltaCanal son los cambios de las raíces de un canal (`messages/delta`): con enlace
// vacío empieza por lo cambiado después de desde; si no, sigue ese `deltaLink`. Devuelve
// los cambios y el `deltaLink` de la próxima vez.
func (g *Graph) DeltaCanal(ctx context.Context, remoto, enlace string, desde time.Time) ([]MensajeGraph, string, error) {
	destino := enlace
	if destino == "" {
		ruta, err := RutaMensajesTeams(remoto, "")
		if err != nil {
			return nil, "", err
		}
		if _, ok := EsChat(remoto); ok {
			return nil, "", ErrEntrada
		}
		destino = g.url(ruta+"/delta", url.Values{"$top": {"50"}, "$filter": {"lastModifiedDateTime gt " + fechaGraph(desde)}})
	} else if !MismoOrigen(g.Base, destino) {
		return nil, "", ErrEntrada
	}
	var out []MensajeGraph
	for range MaxPaginas {
		var p paginaGraph[MensajeGraph]
		if err := g.c().pedir(ctx, http.MethodGet, destino, nil, &p); err != nil {
			return out, "", err
		}
		out = append(out, p.Valor...)
		switch {
		case p.Delta != "":
			if !MismoOrigen(g.Base, p.Delta) {
				return out, "", ErrRespuesta
			}
			return out, p.Delta, nil
		case p.Siguiente != "" && MismoOrigen(g.Base, p.Siguiente):
			destino = p.Siguiente
		default:
			return out, "", ErrRespuesta
		}
	}
	return out, "", ErrRespuesta
}

// Reaccionar pone o quita una reacción (un emoji, o los clásicos de Teams: `like`…).
func (g *Graph) Reaccionar(ctx context.Context, remoto, mensaje, tipo string, poner bool) error {
	ruta, err := rutaMensaje(remoto, mensaje)
	if err != nil || strings.TrimSpace(tipo) == "" {
		return ErrEntrada
	}
	accion := "/unsetReaction"
	if poner {
		accion = "/setReaction"
	}
	return g.c().pedir(ctx, http.MethodPost, g.url(ruta+accion, nil), map[string]string{"reactionType": tipo}, nil)
}

// MiembrosDe son los de un chat o un canal.
func (g *Graph) MiembrosDe(ctx context.Context, remoto string) ([]MiembroGraph, error) {
	ruta, err := RutaMensajesTeams(remoto, "")
	if err != nil {
		return nil, err
	}
	return todas[MiembroGraph](ctx, g, g.url(strings.TrimSuffix(ruta, "/messages")+"/members", nil), 1000)
}

// CrearReunion crea una reunión en línea de Teams a nombre de la persona y da su enlace
// para entrar (D-21: se entra con la app oficial).
func (g *Graph) CrearReunion(ctx context.Context, asunto string, inicio, fin time.Time) (string, error) {
	var r struct {
		Enlace string `json:"joinWebUrl"` //nolint:tagliatelle // contrato de Graph
	}
	cuerpo := map[string]string{"subject": asunto, "startDateTime": fechaGraph(inicio), "endDateTime": fechaGraph(fin)}
	if err := g.c().pedir(ctx, http.MethodPost, g.url("/me/onlineMeetings", nil), cuerpo, &r); err != nil {
		return "", err
	}
	return enlaceHTTPS(r.Enlace)
}

// PresenciaGraph es la presencia de alguien.
type PresenciaGraph struct {
	ID             string `json:"id"`
	Disponibilidad string `json:"availability"`
	Actividad      string `json:"activity"`
}

// MaxPresencias es cuántas personas admite Graph por consulta.
const MaxPresencias = 650

// Presencias es la presencia de esas personas (por su id de Microsoft Entra).
func (g *Graph) Presencias(ctx context.Context, ids []string) ([]PresenciaGraph, error) {
	var validos []string
	for _, id := range ids {
		if _, err := uuid.Parse(id); err == nil {
			validos = append(validos, id)
		}
	}
	var out []PresenciaGraph
	for len(validos) > 0 {
		n := min(len(validos), MaxPresencias)
		var p paginaGraph[PresenciaGraph]
		if err := g.c().pedir(ctx, http.MethodPost, g.url("/communications/getPresencesByUserId", nil), map[string]any{"ids": validos[:n]}, &p); err != nil {
			return out, err
		}
		out = append(out, p.Valor...)
		validos = validos[n:]
	}
	return out, nil
}
