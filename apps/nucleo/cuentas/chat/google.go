// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ----- Lo movido del servicio de integraciones -----

// Nombres de recursos de Google Chat que se aceptan en una ruta. Eran `roomsSpaceName`
// y `externalGoogleThread`; NombreMensaje y SujetoGoogle (`externalGoogleSubject`, el
// `sub` de OpenID de la persona) van con ellos.
var (
	NombreEspacio = regexp.MustCompile(`^spaces/[A-Za-z0-9_-]+$`)
	NombreHilo    = regexp.MustCompile(`^spaces/[A-Za-z0-9_-]+/threads/[A-Za-z0-9_.-]+$`)
	NombreMensaje = regexp.MustCompile(`^spaces/[A-Za-z0-9_-]+/messages/[A-Za-z0-9_.-]+$`)
	SujetoGoogle  = regexp.MustCompile(`^\d{1,40}$`)
)

// MensajeGoogle es un mensaje de Google Chat tal como llega. Era
// `googleExternalMessage` del servicio de integraciones (por eso sus campos conservan el
// nombre); lo demás se añadió para Sytue Office.
type MensajeGoogle struct {
	Name       string     `json:"name"`
	Text       string     `json:"text"`
	CreateTime time.Time  `json:"createTime"`     //nolint:tagliatelle // contrato de Google Chat
	UpdateTime *time.Time `json:"lastUpdateTime"` //nolint:tagliatelle // contrato de Google Chat
	DeleteTime *time.Time `json:"deleteTime"`     //nolint:tagliatelle // contrato de Google Chat
	Sender     struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"` //nolint:tagliatelle // contrato de Google Chat
		Type        string `json:"type"`
	} `json:"sender"`
	Thread struct {
		Name string `json:"name"`
	} `json:"thread"`
	ThreadReply bool `json:"threadReply"` //nolint:tagliatelle // contrato de Google Chat
	Reactions   []struct {
		Emoji struct {
			Unicode string `json:"unicode"`
		} `json:"emoji"`
		Count int `json:"reactionCount"` //nolint:tagliatelle // contrato de Google Chat
	} `json:"emojiReactionSummaries"` //nolint:tagliatelle // contrato de Google Chat
	Attachment []AdjuntoGoogle `json:"attachment"`
}

// AdjuntoGoogle es un fichero de un mensaje de Google Chat: subido a Chat (con su
// referencia para bajarlo) o de Drive.
type AdjuntoGoogle struct {
	Name        string            `json:"name"`
	ContentName string            `json:"contentName"`                 //nolint:tagliatelle // contrato de Google Chat
	ContentType string            `json:"contentType"`                 //nolint:tagliatelle // contrato de Google Chat
	Datos       *RefAdjuntoGoogle `json:"attachmentDataRef,omitempty"` //nolint:tagliatelle // contrato de Google Chat
	Drive       *struct {
		ID string `json:"driveFileId"` //nolint:tagliatelle // contrato de Google Chat
	} `json:"driveDataRef,omitempty"` //nolint:tagliatelle // contrato de Google Chat
	Source    string `json:"source"`
	Miniatura string `json:"thumbnailUri"` //nolint:tagliatelle // contrato de Google Chat
	Descarga  string `json:"downloadUri"`  //nolint:tagliatelle // contrato de Google Chat
}

// RefAdjuntoGoogle es la referencia de un fichero subido a Chat: con ella se manda en un
// mensaje y se baja (`media.download`).
type RefAdjuntoGoogle struct {
	Recurso string `json:"resourceName"`                    //nolint:tagliatelle // contrato de Google Chat
	Token   string `json:"attachmentUploadToken,omitempty"` //nolint:tagliatelle // contrato de Google Chat
}

// EnvioGoogle prepara el envío de texto a un espacio (y, con hilo, como respuesta en él):
// la ruta relativa a la base de la API y el cuerpo. Comprueba que el hilo es de ese
// espacio. Era el principio de `externalGoogleSend`.
func EnvioGoogle(espacio, hilo, texto string) (ruta string, cuerpo map[string]any, err error) {
	if !NombreEspacio.MatchString(espacio) || hilo != "" && (!NombreHilo.MatchString(hilo) || !strings.HasPrefix(hilo, espacio+"/threads/")) {
		return "", nil, ErrEntrada
	}
	cuerpo = map[string]any{"text": texto}
	q := url.Values{}
	if hilo != "" {
		cuerpo["thread"] = map[string]string{"name": hilo}
		q.Set("messageReplyOption", "REPLY_MESSAGE_OR_FAIL")
	}
	ruta = espacio + "/messages"
	if len(q) > 0 {
		ruta += "?" + q.Encode()
	}
	return ruta, cuerpo, nil
}

// EnlaceMeet comprueba el enlace que devuelve Meet al crear un espacio: https a la
// máquina de las reuniones de Meet, sin credenciales y con un código. Era el final de
// `createRoomsMeeting`.
func EnlaceMeet(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "https" || u.Host != "meet.google.com" || u.User != nil || len(u.Path) < 2 {
		return "", ErrEnlace
	}
	return u.String(), nil
}

// ----- Los clientes -----

// GoogleChat es el cliente de la API de Google Chat con un token de la persona.
type GoogleChat struct {
	// Base es la de la API con su versión (`…/v1`): la pone quien lo usa.
	Base string
	// BaseSubida es la de las subidas (`…/upload/v1`); vacía, la de Base con `/upload`
	// delante de la versión.
	BaseSubida string
	Token      Token
	HTTP       HTTPDoer
	// Limite es lo más que se lee de una respuesta (cero: MaxRespuesta).
	Limite int64
}

func (g *GoogleChat) c() cliente { return cliente{http: g.HTTP, token: g.Token, limite: g.Limite} }

// Pedir es una petición a la API para lo que el cliente no tiene método: ruta relativa a
// Base o dirección absoluta del mismo origen.
func (g *GoogleChat) Pedir(ctx context.Context, metodo, ruta string, cuerpo, out any) error {
	d, err := destino(g.Base, ruta)
	if err != nil {
		return err
	}
	return g.c().pedir(ctx, metodo, d, cuerpo, out)
}

// EspacioChat es un espacio (o un mensaje directo, o un chat de grupo).
type EspacioChat struct {
	Name           string     `json:"name"`
	DisplayName    string     `json:"displayName"`    //nolint:tagliatelle // contrato de Google Chat
	SpaceType      string     `json:"spaceType"`      //nolint:tagliatelle // contrato de Google Chat
	LastActiveTime *time.Time `json:"lastActiveTime"` //nolint:tagliatelle // contrato de Google Chat
	CreateTime     *time.Time `json:"createTime"`     //nolint:tagliatelle // contrato de Google Chat
	Details        struct {
		Description string `json:"description"`
	} `json:"spaceDetails"` //nolint:tagliatelle // contrato de Google Chat
}

// Directo dice si es un mensaje directo o un chat de grupo sin nombre.
func (e EspacioChat) Directo() bool {
	return e.SpaceType == "DIRECT_MESSAGE" || e.SpaceType == "GROUP_CHAT"
}

// MaxEspacios es cuántos espacios se listan como mucho.
const MaxEspacios = 500

// Espacios son los de la persona.
func (g *GoogleChat) Espacios(ctx context.Context) ([]EspacioChat, error) {
	var out []EspacioChat
	token := ""
	for range MaxPaginas {
		var p struct {
			Spaces []EspacioChat `json:"spaces"`
			Next   string        `json:"nextPageToken"` //nolint:tagliatelle // contrato de Google Chat
		}
		q := url.Values{"pageSize": {"100"}}
		if token != "" {
			q.Set("pageToken", token)
		}
		if err := g.c().pedir(ctx, http.MethodGet, unir(g.Base, "spaces", q), nil, &p); err != nil {
			return out, err
		}
		for _, e := range p.Spaces {
			if NombreEspacio.MatchString(e.Name) {
				out = append(out, e)
			}
		}
		if p.Next == "" || p.Next == token || len(out) >= MaxEspacios {
			break
		}
		token = p.Next
	}
	return out, nil
}

func fechaGoogle(t time.Time) string { return strconv.Quote(t.UTC().Format(time.RFC3339Nano)) }

func (g *GoogleChat) listar(ctx context.Context, espacio string, q url.Values) ([]MensajeGoogle, error) {
	if !NombreEspacio.MatchString(espacio) {
		return nil, ErrEntrada
	}
	var p struct {
		Messages []MensajeGoogle `json:"messages"`
	}
	err := g.c().pedir(ctx, http.MethodGet, unir(g.Base, espacio+"/messages", q), nil, &p)
	return p.Messages, err
}

// Mensajes es una página de un espacio, del más nuevo al más viejo, de los anteriores
// a antes (cero: los últimos). Pide leer mensajes, que es un permiso restringido de
// Google (D-22).
func (g *GoogleChat) Mensajes(ctx context.Context, espacio string, antes time.Time, n int) ([]MensajeGoogle, error) {
	q := url.Values{"pageSize": {strconv.Itoa(min(max(n, 1), 1000))}, "orderBy": {"createTime desc"}}
	if !antes.IsZero() {
		q.Set("filter", "createTime < "+fechaGoogle(antes))
	}
	return g.listar(ctx, espacio, q)
}

// Nuevos son los mensajes creados después de desde, del más viejo al más nuevo (Chat no
// tiene `delta`: el sondeo pregunta por fecha).
func (g *GoogleChat) Nuevos(ctx context.Context, espacio string, desde time.Time) ([]MensajeGoogle, error) {
	return g.listar(ctx, espacio, url.Values{"pageSize": {"100"}, "filter": {"createTime > " + fechaGoogle(desde)}})
}

// Hilo son los mensajes de un hilo, del primero al último.
func (g *GoogleChat) Hilo(ctx context.Context, espacio, hilo string) ([]MensajeGoogle, error) {
	if !NombreHilo.MatchString(hilo) || !strings.HasPrefix(hilo, espacio+"/threads/") {
		return nil, ErrEntrada
	}
	return g.listar(ctx, espacio, url.Values{"pageSize": {"1000"}, "filter": {"thread.name = " + hilo}})
}

// Mensaje trae un mensaje por su nombre (`spaces/…/messages/…`).
func (g *GoogleChat) Mensaje(ctx context.Context, nombre string) (MensajeGoogle, error) {
	if !NombreMensaje.MatchString(nombre) {
		return MensajeGoogle{}, ErrEntrada
	}
	var m MensajeGoogle
	err := g.c().pedir(ctx, http.MethodGet, unir(g.Base, nombre, nil), nil, &m)
	return m, err
}

// Enviar publica texto en un espacio; con hilo, como respuesta en él.
func (g *GoogleChat) Enviar(ctx context.Context, espacio, hilo, texto string) (MensajeGoogle, error) {
	ruta, cuerpo, err := EnvioGoogle(espacio, hilo, texto)
	if err != nil {
		return MensajeGoogle{}, err
	}
	var m MensajeGoogle
	err = g.c().pedir(ctx, http.MethodPost, strings.TrimRight(g.Base, "/")+"/"+ruta, cuerpo, &m)
	return m, err
}

// MiembroGoogle es la pertenencia de alguien a un espacio.
type MiembroGoogle struct {
	Name   string `json:"name"`
	Role   string `json:"role"`
	Member struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"` //nolint:tagliatelle // contrato de Google Chat
		Type        string `json:"type"`
	} `json:"member"`
}

// Miembros son los de un espacio.
func (g *GoogleChat) Miembros(ctx context.Context, espacio string) ([]MiembroGoogle, error) {
	if !NombreEspacio.MatchString(espacio) {
		return nil, ErrEntrada
	}
	var p struct {
		Memberships []MiembroGoogle `json:"memberships"`
	}
	err := g.c().pedir(ctx, http.MethodGet, unir(g.Base, espacio+"/members", url.Values{"pageSize": {"1000"}}), nil, &p)
	return p.Memberships, err
}

// Meet es el cliente de la API REST de Google Meet.
type Meet struct {
	// Base es la de la API con su versión (`…/v2`).
	Base  string
	Token Token
	HTTP  HTTPDoer
	// Limite es lo más que se lee de una respuesta (cero: MaxRespuesta).
	Limite int64
}

// Crear crea un espacio de reunión (`spaces.create`) y da su enlace (D-21: se entra con
// Meet). Un enlace que no es de Meet es [ErrEnlace].
func (m *Meet) Crear(ctx context.Context) (string, error) {
	var r struct {
		URI string `json:"meetingUri"` //nolint:tagliatelle // contrato de Google Meet
	}
	if err := (cliente{http: m.HTTP, token: m.Token, limite: m.Limite}).pedir(ctx, http.MethodPost, unir(m.Base, "spaces", nil), map[string]any{}, &r); err != nil {
		return "", err
	}
	return EnlaceMeet(r.URI)
}

// UsuarioGoogle es quien entró, según OpenID Connect.
type UsuarioGoogle struct {
	Sujeto string `json:"sub"`
	Correo string `json:"email"`
	Nombre string `json:"name"`
}

// Recurso es el nombre de la persona en Google Chat (`users/<sub>`).
func (u UsuarioGoogle) Recurso() string { return "users/" + u.Sujeto }

// Usuario pregunta quién es a destino (el `userinfo` de OpenID Connect de Google). Un
// `sub` que no es de Google es [ErrIdentidad]. Sin token (la pasarela), lo pone el
// transporte.
func Usuario(ctx context.Context, h HTTPDoer, t Token, destino string) (UsuarioGoogle, error) {
	var u UsuarioGoogle
	if err := (cliente{http: h, token: t, limite: 1 << 20}).pedir(ctx, http.MethodGet, destino, nil, &u); err != nil {
		return u, err
	}
	if !SujetoGoogle.MatchString(u.Sujeto) {
		return u, ErrIdentidad
	}
	return u, nil
}
