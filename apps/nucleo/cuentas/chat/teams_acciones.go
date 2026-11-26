// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"context"
	"encoding/base64"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
)

// Lo de Teams que es más que leer y escribir texto: abrir un chat nuevo, mandar ficheros
// (subidos al OneDrive de la persona y enlazados en el mensaje, como hace Teams) y bajar
// los que llegan.

// PersonaTeams es como se nombra a alguien al abrir un chat: su id de Microsoft Entra o
// su nombre principal (el correo de la organización).
var PersonaTeams = regexp.MustCompile(`^[^\s/?#@'"\\()]{1,128}@[^\s/?#@'"\\()]{1,255}$`)

func personaTeamsValida(p string) bool {
	_, err := uuid.Parse(p)
	return err == nil || PersonaTeams.MatchString(p)
}

// Chat trae un chat con sus miembros.
func (g *Graph) Chat(ctx context.Context, id string) (ChatGraph, error) {
	if !IDChatTeamsValido(id) {
		return ChatGraph{}, ErrEntrada
	}
	var c ChatGraph
	err := g.c().pedir(ctx, http.MethodGet, g.url("/chats/"+url.PathEscape(id), url.Values{"$expand": {"members"}}), nil, &c)
	return c, err
}

// CrearChat abre un chat 1:1 (con una persona) o de grupo (con varias) en el que está
// yo. En un 1:1, Graph devuelve el que ya hay si lo hay; vale con `Chat.ReadWrite`.
func (g *Graph) CrearChat(ctx context.Context, yo string, personas []string) (ChatGraph, error) {
	if _, err := uuid.Parse(yo); err != nil || len(personas) == 0 || len(personas) > 249 {
		return ChatGraph{}, ErrEntrada
	}
	tipo := "oneOnOne"
	if len(personas) > 1 {
		tipo = "group"
	}
	miembro := func(id string) map[string]any {
		return map[string]any{
			"@odata.type":     "#microsoft.graph.aadUserConversationMember",
			"roles":           []string{"owner"},
			"user@odata.bind": strings.TrimRight(g.Base, "/") + "/users('" + id + "')",
		}
	}
	miembros := []any{miembro(yo)}
	for _, p := range personas {
		if !personaTeamsValida(p) || strings.EqualFold(p, yo) {
			return ChatGraph{}, ErrEntrada
		}
		miembros = append(miembros, miembro(p))
	}
	var c ChatGraph
	if err := g.c().pedir(ctx, http.MethodPost, g.url("/chats", nil), map[string]any{"chatType": tipo, "members": miembros}, &c); err != nil {
		return c, err
	}
	if !IDChatTeamsValido(c.ID) {
		return c, ErrRespuesta
	}
	return c, nil
}

// ----- Ficheros -----

// AdjuntoGraph es un fichero de un mensaje.
type AdjuntoGraph struct {
	ID        string `json:"id"`
	Tipo      string `json:"contentType"`  //nolint:tagliatelle // contrato de Graph
	Enlace    string `json:"contentUrl"`   //nolint:tagliatelle // contrato de Graph
	Nombre    string `json:"name"`         //nolint:tagliatelle // contrato de Graph
	Miniatura string `json:"thumbnailUrl"` //nolint:tagliatelle // contrato de Graph
}

// Referencia dice si es un fichero de OneDrive o SharePoint enlazado (y no una tarjeta).
func (a AdjuntoGraph) Referencia() bool { return a.Tipo == "reference" && a.Enlace != "" }

// ElementoDrive es un fichero de OneDrive.
type ElementoDrive struct {
	ID     string `json:"id"`
	Nombre string `json:"name"`
	Web    string `json:"webUrl"` //nolint:tagliatelle // contrato de Graph
	ETag   string `json:"eTag"`   //nolint:tagliatelle // contrato de Graph
	Tamano int64  `json:"size"`
}

// IDAdjunto es el id con el que el fichero va en un mensaje: el GUID de su `eTag`
// («{GUID},n»), en minúsculas.
func (e ElementoDrive) IDAdjunto() string {
	g, _, _ := strings.Cut(strings.Trim(e.ETag, `"`), ",")
	if u, err := uuid.Parse(strings.Trim(g, "{}")); err == nil {
		return u.String()
	}
	return ""
}

// CarpetaTeams es donde Teams deja en el OneDrive de quien manda los ficheros de los
// chats.
const CarpetaTeams = "Microsoft Teams Chat Files"

// MaxSubidaTeams es lo que admite una subida simple de OneDrive.
const MaxSubidaTeams = 250 << 20

// NombreOneDrive es nombre con lo que OneDrive no admite en un nombre de fichero
// cambiado por «_» (y sin puntos ni espacios en los extremos).
func NombreOneDrive(nombre string) string {
	n := strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`/\:*?"<>|#%`, r) {
			return '_'
		}
		return r
	}, nombre)
	n = strings.Trim(n, " .")
	if r := []rune(n); len(r) > 200 {
		n = string(r[:200])
	}
	return n
}

// SubirOneDrive sube un fichero al OneDrive de la persona, en CarpetaTeams, sin pisar
// otro con el mismo nombre ([NombreOneDrive]). Pide `Files.ReadWrite` (o `.All`): sin
// él, Graph dice 403.
func (g *Graph) SubirOneDrive(ctx context.Context, nombre string, f Fichero) (ElementoDrive, error) {
	nombre = NombreOneDrive(nombre)
	if nombre == "" || f.Tamano > MaxSubidaTeams || f.Abrir == nil {
		return ElementoDrive{}, ErrEntrada
	}
	ruta := "/me/drive/root:/" + url.PathEscape(CarpetaTeams) + "/" + url.PathEscape(nombre) + ":/content"
	var e ElementoDrive
	if err := g.c().pedir(ctx, http.MethodPut, g.url(ruta, url.Values{"@microsoft.graph.conflictBehavior": {"rename"}}), f, &e); err != nil {
		return e, err
	}
	if e.ID == "" || e.IDAdjunto() == "" {
		return e, ErrRespuesta
	}
	if _, err := enlaceHTTPS(e.Web); err != nil {
		return e, ErrRespuesta
	}
	return e, nil
}

// Compartir da permiso de lectura sobre un fichero de la persona a esas personas (por su
// id de Microsoft Entra), sin mandarles correo: lo que hace Teams con los ficheros de un
// chat.
func (g *Graph) Compartir(ctx context.Context, elemento string, personas []string) error {
	if elemento == "" || strings.ContainsAny(elemento, "/?#\r\n") {
		return ErrEntrada
	}
	var quienes []any
	for _, p := range personas {
		if _, err := uuid.Parse(p); err == nil {
			quienes = append(quienes, map[string]string{"objectId": p})
		}
	}
	if len(quienes) == 0 {
		return nil
	}
	cuerpo := map[string]any{"recipients": quienes, "roles": []string{"read"}, "requireSignIn": true, "sendInvitation": false}
	return g.c().pedir(ctx, http.MethodPost, g.url("/me/drive/items/"+url.PathEscape(elemento)+"/invite", nil), cuerpo, nil)
}

// EnlaceOrganizacion es un enlace de lectura para la organización a un fichero de la
// persona: lo de un canal, cuyos miembros no se conocen uno a uno.
func (g *Graph) EnlaceOrganizacion(ctx context.Context, elemento string) (string, error) {
	if elemento == "" || strings.ContainsAny(elemento, "/?#\r\n") {
		return "", ErrEntrada
	}
	var r struct {
		Enlace struct {
			Web string `json:"webUrl"` //nolint:tagliatelle // contrato de Graph
		} `json:"link"`
	}
	if err := g.c().pedir(ctx, http.MethodPost, g.url("/me/drive/items/"+url.PathEscape(elemento)+"/createLink", nil),
		map[string]string{"type": "view", "scope": "organization"}, &r); err != nil {
		return "", err
	}
	return enlaceHTTPS(r.Enlace.Web)
}

// CuerpoConAdjuntos es el de un mensaje con ficheros: el texto (escapado, en HTML) con
// una marca `<attachment>` por fichero, y los ficheros como `reference` con su enlace.
func CuerpoConAdjuntos(texto string, es []ElementoDrive) map[string]any {
	var b strings.Builder
	b.WriteString(strings.ReplaceAll(html.EscapeString(texto), "\n", "<br>"))
	var adj []any // sin ficheros, `attachments` va a null como siempre
	if len(es) > 0 {
		adj = make([]any, 0, len(es))
	}
	for _, e := range es {
		id := e.IDAdjunto()
		b.WriteString(`<attachment id="` + id + `"></attachment>`)
		adj = append(adj, map[string]string{"id": id, "contentType": "reference", "contentUrl": e.Web, "name": e.Nombre})
	}
	return map[string]any{"body": map[string]string{"contentType": "html", "content": b.String()}, "attachments": adj}
}

// EnviarConAdjuntos publica texto con los ficheros ya subidos a OneDrive en remoto (con
// hilo, como respuesta en un canal).
func (g *Graph) EnviarConAdjuntos(ctx context.Context, remoto, hilo, texto string, es []ElementoDrive) (MensajeGraph, error) {
	ruta, err := RutaMensajesTeams(remoto, hilo)
	if err != nil {
		return MensajeGraph{}, err
	}
	if slices.ContainsFunc(es, func(e ElementoDrive) bool { return e.IDAdjunto() == "" || e.Web == "" }) {
		return MensajeGraph{}, ErrEntrada
	}
	var m MensajeGraph
	err = g.c().pedir(ctx, http.MethodPost, g.url(ruta, nil), CuerpoConAdjuntos(texto, es), &m)
	return m, err
}

// IDCompartido es el de la API `shares` de un enlace de OneDrive o SharePoint: «u!» y el
// enlace en base64url sin relleno.
func IDCompartido(enlace string) string {
	return "u!" + base64.RawURLEncoding.EncodeToString([]byte(enlace))
}

// BajarCompartido trae el contenido de un fichero enlazado en un mensaje (`shares`):
// Graph redirige a su servidor de descarga. Pide poder leer los ficheros que se
// comparten con la persona (`Files.Read.All` o `Files.ReadWrite.All`).
func (g *Graph) BajarCompartido(ctx context.Context, enlace string) (Contenido, error) {
	if _, err := enlaceHTTPS(enlace); err != nil {
		return Contenido{}, ErrEntrada
	}
	return g.c().bajar(ctx, g.url("/shares/"+IDCompartido(enlace)+"/driveItem/content", nil))
}

// alojado es un contenido alojado en un mensaje (una imagen pegada): su id en la
// dirección que va en el HTML.
var alojado = regexp.MustCompile(`hostedContents/([A-Za-z0-9=_%+.-]{1,1000})/\$value`)

// Alojados son los ids de los contenidos alojados (imágenes pegadas) de un mensaje.
func (m MensajeGraph) Alojados() []string {
	var out []string
	for _, x := range alojado.FindAllStringSubmatch(m.Body.Content, 20) {
		if !slices.Contains(out, x[1]) {
			out = append(out, x[1])
		}
	}
	return out
}

// BajarAlojado trae un contenido alojado de un mensaje de remoto (una imagen pegada en
// Teams): vale con poder leer el chat.
func (g *Graph) BajarAlojado(ctx context.Context, remoto, mensaje, id string) (Contenido, error) {
	ruta, err := rutaMensaje(remoto, mensaje)
	if err != nil || !alojado.MatchString("hostedContents/"+id+"/$value") {
		return Contenido{}, ErrEntrada
	}
	return g.c().bajar(ctx, g.url(ruta+"/hostedContents/"+id+"/$value", nil))
}
