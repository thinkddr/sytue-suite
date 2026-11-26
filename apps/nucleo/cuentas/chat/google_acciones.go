// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Lo de Google Chat que es más que leer y escribir texto: abrir un mensaje directo,
// subir y bajar ficheros, y el nombre de quien escribe por la People API.

// UsuarioChat es una persona en Google Chat: `users/<id>` o, donde la API lo admite (abrir
// un directo, añadir a alguien), `users/<correo>`.
var UsuarioChat = regexp.MustCompile(`^users/(\d{1,40}|[^\s/?#@'"\\]{1,128}@[^\s/?#@'"\\]{1,255})$`)

// BuscarDirecto es el mensaje directo que ya hay con usuario (`spaces.findDirectMessage`);
// si no lo hay, Google contesta 404. Basta con ver los espacios (`chat.spaces.readonly`).
func (g *GoogleChat) BuscarDirecto(ctx context.Context, usuario string) (EspacioChat, error) {
	if !UsuarioChat.MatchString(usuario) {
		return EspacioChat{}, ErrEntrada
	}
	var e EspacioChat
	if err := g.c().pedir(ctx, http.MethodGet, unir(g.Base, "spaces:findDirectMessage", url.Values{"name": {usuario}}), nil, &e); err != nil {
		return e, err
	}
	if !NombreEspacio.MatchString(e.Name) {
		return e, ErrRespuesta
	}
	return e, nil
}

// CrearConversacion crea un mensaje directo (con una persona) o un chat de grupo sin
// nombre (con varias) con quien entró dentro (`spaces.setup`). Pide el permiso
// `chat.spaces.create`, que no está entre los de siempre: sin él, Google contesta 403.
func (g *GoogleChat) CrearConversacion(ctx context.Context, usuarios []string) (EspacioChat, error) {
	if len(usuarios) == 0 || len(usuarios) > 20 {
		return EspacioChat{}, ErrEntrada
	}
	tipo := "DIRECT_MESSAGE"
	if len(usuarios) > 1 {
		tipo = "GROUP_CHAT"
	}
	miembros := make([]any, 0, len(usuarios))
	for _, u := range usuarios {
		if !UsuarioChat.MatchString(u) {
			return EspacioChat{}, ErrEntrada
		}
		miembros = append(miembros, map[string]any{"member": map[string]string{"name": u, "type": "HUMAN"}})
	}
	cuerpo := map[string]any{"space": map[string]string{"spaceType": tipo}, "memberships": miembros}
	var e EspacioChat
	if err := g.c().pedir(ctx, http.MethodPost, unir(g.Base, "spaces:setup", nil), cuerpo, &e); err != nil {
		return e, err
	}
	if !NombreEspacio.MatchString(e.Name) {
		return e, ErrRespuesta
	}
	return e, nil
}

// baseSubida es la de las subidas: `…/upload/v1` para `…/v1`.
func (g *GoogleChat) baseSubida() string {
	if g.BaseSubida != "" {
		return g.BaseSubida
	}
	b := strings.TrimRight(g.Base, "/")
	i := strings.LastIndex(b, "/")
	if i < 0 {
		return b
	}
	return b[:i] + "/upload" + b[i:]
}

// MaxSubidaGoogle es lo más que admite Chat por fichero.
const MaxSubidaGoogle = 200 << 20

// multiparte es un cuerpo `multipart/related` (los metadatos en JSON y el fichero), que
// se arma en cada intento.
type multiparte struct {
	tipo  string
	abrir func() (io.ReadCloser, error)
}

// Subir sube un fichero a un espacio (`media.upload`) y da la referencia con la que va
// en un mensaje. Vale con el permiso de escribir (`chat.messages.create`).
func (g *GoogleChat) Subir(ctx context.Context, espacio, nombre string, f Fichero) (RefAdjuntoGoogle, error) {
	if !NombreEspacio.MatchString(espacio) || strings.TrimSpace(nombre) == "" || f.Tamano > MaxSubidaGoogle || f.Abrir == nil {
		return RefAdjuntoGoogle{}, ErrEntrada
	}
	azar := make([]byte, 12)
	_, _ = rand.Read(azar)
	frontera := "sytue" + hex.EncodeToString(azar)
	meta, err := json.Marshal(map[string]string{"filename": nombre})
	if err != nil {
		return RefAdjuntoGoogle{}, err
	}
	tipo := cmp.Or(f.Tipo, "application/octet-stream")
	if _, _, err := mime.ParseMediaType(tipo); err != nil || strings.ContainsAny(tipo, "\r\n") {
		tipo = "application/octet-stream"
	}
	m := multiparte{tipo: "multipart/related; boundary=" + frontera, abrir: func() (io.ReadCloser, error) {
		rc, err := f.Abrir()
		if err != nil {
			return nil, err
		}
		var pre bytes.Buffer
		pre.WriteString("--" + frontera + "\r\nContent-Type: application/json; charset=UTF-8\r\n\r\n")
		pre.Write(meta)
		pre.WriteString("\r\n--" + frontera + "\r\nContent-Type: " + tipo + "\r\n\r\n")
		post := "\r\n--" + frontera + "--\r\n"
		return leerYCerrar{Reader: io.MultiReader(&pre, rc, strings.NewReader(post)), c: rc}, nil
	}}
	var r struct {
		Ref RefAdjuntoGoogle `json:"attachmentDataRef"` //nolint:tagliatelle // contrato de Google Chat
	}
	destino := unir(g.baseSubida(), espacio+"/attachments:upload", url.Values{"uploadType": {"multipart"}})
	if err := g.c().pedir(ctx, http.MethodPost, destino, m, &r); err != nil {
		return RefAdjuntoGoogle{}, err
	}
	if r.Ref.Recurso == "" && r.Ref.Token == "" {
		return RefAdjuntoGoogle{}, ErrRespuesta
	}
	return r.Ref, nil
}

type leerYCerrar struct {
	io.Reader
	c io.Closer
}

func (l leerYCerrar) Close() error { return l.c.Close() }

// EnviarConAdjuntos publica texto con los ficheros ya subidos (con hilo, como respuesta).
func (g *GoogleChat) EnviarConAdjuntos(ctx context.Context, espacio, hilo, texto string, refs []RefAdjuntoGoogle) (MensajeGoogle, error) {
	ruta, cuerpo, err := EnvioGoogle(espacio, hilo, texto)
	if err != nil {
		return MensajeGoogle{}, err
	}
	if strings.TrimSpace(texto) == "" {
		delete(cuerpo, "text")
	}
	var adj []any
	for _, r := range refs {
		adj = append(adj, map[string]any{"attachmentDataRef": r})
	}
	if len(adj) > 0 {
		cuerpo["attachment"] = adj
	}
	var m MensajeGoogle
	err = g.c().pedir(ctx, http.MethodPost, strings.TrimRight(g.Base, "/")+"/"+ruta, cuerpo, &m)
	return m, err
}

// recursoGoogle es un nombre de recurso de un adjunto que se puede poner en la ruta.
var recursoGoogle = regexp.MustCompile(`^[A-Za-z0-9_.=+/-]+$`)

// Bajar trae el contenido de un fichero subido a Chat (`media.download`) por su
// referencia. Pide poder leer los mensajes (D-22).
func (g *GoogleChat) Bajar(ctx context.Context, recurso string) (Contenido, error) {
	if len(recurso) > 4096 || !recursoGoogle.MatchString(recurso) || strings.Contains(recurso, "..") || strings.HasPrefix(recurso, "/") {
		return Contenido{}, ErrEntrada
	}
	return g.c().bajar(ctx, unir(g.Base, "media/"+recurso, url.Values{"alt": {"media"}}))
}

// PersonasGoogle es la People API de Google: el nombre de quien escribe, que Chat no da
// con la autenticación de la persona (solo `users/<id>`).
type PersonasGoogle struct {
	// Base es la de la API con su versión (`…/v1`).
	Base   string
	Token  Token
	HTTP   HTTPDoer
	Limite int64
}

// PersonaGoogle es lo que se sabe de alguien.
type PersonaGoogle struct {
	Nombre, Correo string
}

// Persona es el nombre y el correo de `users/<id>` (`people.get` de `people/<id>`), si
// Google los enseña a quien pregunta: los de su organización, con el permiso `profile`
// de siempre; si no, contesta 403 o 404.
func (p *PersonasGoogle) Persona(ctx context.Context, usuario string) (PersonaGoogle, error) {
	id, ok := strings.CutPrefix(usuario, "users/")
	if !ok || !SujetoGoogle.MatchString(id) {
		return PersonaGoogle{}, ErrEntrada
	}
	var r struct {
		Nombres []struct {
			Nombre string `json:"displayName"` //nolint:tagliatelle // contrato de People
		} `json:"names"`
		Correos []struct {
			Valor string `json:"value"`
		} `json:"emailAddresses"` //nolint:tagliatelle // contrato de People
	}
	c := cliente{http: p.HTTP, token: p.Token, limite: p.Limite}
	if err := c.pedir(ctx, http.MethodGet, unir(p.Base, "people/"+id, url.Values{"personFields": {"names,emailAddresses"}}), nil, &r); err != nil {
		return PersonaGoogle{}, err
	}
	var out PersonaGoogle
	if len(r.Nombres) > 0 {
		out.Nombre = strings.TrimSpace(r.Nombres[0].Nombre)
	}
	if len(r.Correos) > 0 {
		out.Correo = strings.TrimSpace(r.Correos[0].Valor)
	}
	return out, nil
}
