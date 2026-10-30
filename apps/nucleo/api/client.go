// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package api es el SDK de los servicios Go de la suite. Este fichero es el núcleo
// HTTP, puerto de packages/api-clients/src/core/http.ts; cada servicio añade su
// cliente encima.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

// Session es lo que el cliente necesita de internal/auth: un token vigente, y poder
// decir «este me lo han rechazado».
type Session interface {
	AccessToken(ctx context.Context) (string, error)
	Invalidate(rechazado string)
}

// Client habla con UN servicio.
type Client struct {
	base    string
	session Session
	http    *http.Client
	// sinQuery recuerda que este servicio no entiende el método QUERY, para no
	// pagar el intento fallido en cada consulta.
	sinQuery atomic.Bool
}

// New crea el cliente de un servicio. session nil es un cliente anónimo; httpc nil usa
// uno con Transporte, que sí tiene plazo.
func New(baseURL string, session Session, httpc *http.Client) *Client {
	if httpc == nil {
		httpc = &http.Client{Transport: Transporte}
	}
	return &Client{base: strings.TrimRight(baseURL, "/"), session: session, http: httpc}
}

// Transporte es el de todas las llamadas a los servicios: el de siempre, con un plazo
// para que EMPIECE la respuesta. Con `http.DefaultClient`, lo que llamaba con
// `context.Background()` (la carpeta de Windows, los instaladores del Hub) esperaba para
// siempre a un servicio colgado (FYS-11). No es `Client.Timeout`, que cortaría también
// una descarga larga que va llegando.
var Transporte http.RoundTripper = func() http.RoundTripper {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return http.DefaultTransport
	}
	t := base.Clone()
	t.ResponseHeaderTimeout = 60 * time.Second
	return t
}()

// maxJSON acota la respuesta JSON que se decodifica: un servicio comprometido podía
// mandar una sin fin y agotar la memoria de la app (FYS-11).
const maxJSON = 32 << 20

// ErrRespuestaGrande es una respuesta JSON de más de maxJSON.
var ErrRespuestaGrande = errors.New("api: la respuesta es demasiado grande")

// Error es una respuesta que no es 2xx.
type Error struct {
	Status int
	URL    string
	Body   string
	// causa es lo que lo provocó cuando no llegó a salir la petición: el
	// `auth.ErrLoginRequired` de un 401 que se sabe sin preguntar (ver token).
	causa error
}

// Unwrap deja ver la causa: `errors.Is(err, auth.ErrLoginRequired)` sigue valiendo.
func (e *Error) Unwrap() error { return e.causa }

// token pide el access token para `destino`. Sin sesión —no hay nada guardado, o Sytue
// ID ya no renueva la que había— la respuesta del servicio sería un 401 seguro, y así
// se devuelve: como el 401 que es. Antes salía el error del llavero tal cual, las
// pantallas lo leían como «no se pudo cargar» y quien no había iniciado sesión veía
// «Comprueba tu conexión» en Calcus, Docus y Diapos, o una libreta vacía en Contacts
// (visto en una ventana real el 2026-09-27).
func (c *Client) token(ctx context.Context, destino string) (string, error) {
	t, err := c.session.AccessToken(ctx)
	if errors.Is(err, auth.ErrLoginRequired) {
		return "", &Error{Status: http.StatusUnauthorized, URL: destino, causa: err}
	}
	return t, err
}

func (c *Client) authorize(req *http.Request) (*http.Request, string, error) {
	ctx := req.Context()
	var token string
	var err error
	if scoped, ok := c.session.(interface {
		AccessTokenScoped(context.Context) (string, *auth.AuthorizationScope, error)
	}); ok {
		var scope *auth.AuthorizationScope
		token, scope, err = scoped.AccessTokenScoped(ctx)
		req = req.WithContext(auth.WithAuthorizationScope(ctx, scope))
		if errors.Is(err, auth.ErrLoginRequired) {
			err = &Error{Status: http.StatusUnauthorized, URL: req.URL.String(), causa: err}
		}
	} else {
		if auth.RequestAuthorizationIntent(ctx) != nil {
			return nil, "", auth.ErrAuthorizationChanged
		}
		token, err = c.token(ctx, req.URL.String())
	}
	if err == nil {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, token, err
}

func validateAuthorizationIntent(req *http.Request) error {
	if intent := auth.RequestAuthorizationIntent(req.Context()); intent != nil {
		scope := auth.RequestAuthorizationScope(req.Context())
		if !intent.Allows(scope, req.Header.Get("Authorization")) {
			return auth.ErrAuthorizationChanged
		}
	}
	return nil
}

func (e *Error) Error() string {
	return fmt.Sprintf("HTTP %d %s en %s", e.Status, http.StatusText(e.Status), e.URL)
}

// cuerpoError es el error de platform/pkg/httpx. Los servicios lo mandan DENTRO de un
// sobre (`{"error": {...}, "request_id": …}`, `httpx.Fail`); se lee también suelto
// porque así lo escriben los dobles de prueba de este paquete, y leer solo lo suelto
// dejaba `Code()` y `Message()` vacíos contra cualquier servicio de verdad.
type cuerpoError struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details"`
	Params  map[string]any  `json:"params"`
}

func (e *Error) cuerpo() cuerpoError {
	var sobre struct {
		Error *cuerpoError `json:"error"`
		cuerpoError
	}
	_ = json.Unmarshal([]byte(e.Body), &sobre)
	if sobre.Error != nil {
		return *sobre.Error
	}
	return sobre.cuerpoError
}

// Code es el `code` del cuerpo de error de platform/pkg/httpx, o "".
func (e *Error) Code() string { return e.cuerpo().Code }

// RetryAfter returns a bounded provider cooldown. It never replays a write.
func (e *Error) RetryAfter() time.Duration {
	seconds, ok := e.cuerpo().Params["retry_after_seconds"].(float64)
	if !ok || seconds <= 0 {
		return 0
	}
	return time.Duration(min(seconds, 86400) * float64(time.Second))
}

// Campos son los errores por campo de una validación (`details.fields` de
// `platform/pkg/validation`): clave del campo → mensaje. Vacío si el servicio no los da.
func (e *Error) Campos() map[string]string {
	var d struct {
		Fields []struct{ Field, Message string } `json:"fields"`
	}
	_ = json.Unmarshal(e.cuerpo().Details, &d)
	out := make(map[string]string, len(d.Fields))
	for _, f := range d.Fields {
		out[f.Field] = f.Message
	}
	return out
}

// Message es el `message` del cuerpo de error de platform/pkg/httpx, o "".
//
// Es el respaldo EN INGLÉS que el servicio manda junto al código, ya con sus
// marcadores sustituidos. Se enseña cuando dice por qué falló mejor que un texto
// nuestro —«esa clave de acceso es el único factor que te queda»— y no hay clave de
// catálogo para ese caso. Quien tenga una traducción para el `Code()`, que use esa.
func (e *Error) Message() string { return e.cuerpo().Message }

// NoActiveOrg: la sesión no tiene organización con derecho de uso. Los diez servicios
// contestan esto a TODO; la shell lo enseña una vez en vez de diez errores distintos.
func (e *Error) NoActiveOrg() bool {
	return e.Status == http.StatusForbidden && e.Code() == "no_active_organization"
}

// Request describe una petición. Body va en memoria porque hay que poder repetirla
// tras un 401; las subidas grandes no pasan por aquí (van con URL prefirmada).
type Request struct {
	Method    string // "" es GET
	Path      string // relativo a la base del servicio
	Query     url.Values
	Header    http.Header
	Body      []byte
	Anonymous bool
}

// maxError acota lo que se guarda del cuerpo de un error.
const maxError = 64 << 10

// Do ejecuta la petición y devuelve la respuesta 2xx; cerrarla es cosa de quien llama.
//
// Ante un 401 renueva el token y lo intenta UNA vez más. Si vuelve a ser 401, se
// devuelve tal cual: el siguiente AccessToken dirá si hay que iniciar sesión.
func (c *Client) Do(ctx context.Context, r Request) (*http.Response, error) {
	for intento := 0; ; intento++ {
		resp, token, err := c.una(ctx, r)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}
		cuerpo, _ := io.ReadAll(io.LimitReader(resp.Body, maxError))
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized && token != "" && intento == 0 {
			c.session.Invalidate(token)
			continue
		}
		return nil, &Error{Status: resp.StatusCode, URL: resp.Request.URL.String(), Body: string(cuerpo)}
	}
}

func (c *Client) una(ctx context.Context, r Request) (*http.Response, string, error) {
	metodo := r.Method
	if metodo == "" {
		metodo = http.MethodGet
	}
	var cuerpo io.Reader
	if r.Body != nil {
		cuerpo = bytes.NewReader(r.Body)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, c.URL(r.Path, r.Query), cuerpo)
	if err != nil {
		return nil, "", err
	}
	for k, v := range r.Header {
		req.Header[k] = v
	}
	var token string
	if !r.Anonymous && c.session == nil && auth.RequestAuthorizationIntent(ctx) != nil {
		return nil, "", auth.ErrAuthorizationChanged
	}
	if !r.Anonymous && c.session != nil {
		if req, token, err = c.authorize(req); err != nil {
			return nil, "", err
		}
		if err := validateAuthorizationIntent(req); err != nil {
			return nil, "", err
		}
	}
	resp, err := c.http.Do(req)
	return resp, token, err
}

// Stream manda un cuerpo que NO cabe en memoria —la subida de un fichero— tal cual llega.
// No reintenta tras un 401: el cuerpo ya se ha leído y no se puede repetir. No hace
// falta: AccessToken renueva ANTES de caducar, así que el token sale vigente.
func (c *Client) Stream(ctx context.Context, metodo, path string, query url.Values, cabeceras http.Header, cuerpo io.Reader, tamano int64) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, metodo, c.URL(path, query), cuerpo)
	if err != nil {
		return nil, err
	}
	req.ContentLength = tamano
	for k, v := range cabeceras {
		req.Header[k] = v
	}
	if c.session == nil && auth.RequestAuthorizationIntent(ctx) != nil {
		return nil, auth.ErrAuthorizationChanged
	}
	if c.session != nil {
		var err error
		req, _, err = c.authorize(req)
		if err != nil {
			return nil, err
		}
		if err := validateAuthorizationIntent(req); err != nil {
			return nil, err
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	detalle, _ := io.ReadAll(io.LimitReader(resp.Body, maxError))
	_ = resp.Body.Close()
	return nil, &Error{Status: resp.StatusCode, URL: resp.Request.URL.String(), Body: string(detalle)}
}

// JSON ejecuta la petición y decodifica la respuesta en T. Un 204 devuelve el cero.
func JSON[T any](ctx context.Context, c *Client, r Request) (T, error) {
	var out T
	r.Header = r.Header.Clone()
	if r.Header == nil {
		r.Header = http.Header{}
	}
	if r.Header.Get("Accept") == "" {
		r.Header.Set("Accept", "application/json")
	}
	resp, err := c.Do(ctx, r)
	if err != nil {
		return out, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNoContent {
		return out, nil
	}
	cuerpo := &io.LimitedReader{R: resp.Body, N: maxJSON + 1}
	err = json.NewDecoder(cuerpo).Decode(&out)
	if cuerpo.N <= 0 {
		return out, ErrRespuestaGrande
	}
	return out, err
}

// PostJSON manda `cuerpo` como JSON y decodifica la respuesta en T.
func PostJSON[T any](ctx context.Context, c *Client, path string, cuerpo any) (T, error) {
	datos, err := json.Marshal(cuerpo)
	if err != nil {
		var cero T
		return cero, err
	}
	return JSON[T](ctx, c, Request{
		Method: http.MethodPost, Path: path, Body: datos,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// Consulta es una lectura con filtros. Va por el método QUERY, con los filtros en el
// cuerpo y no en la URL —que es lo que acaba en los logs de acceso de cualquier proxy—
// y, si el servicio no lo entiende (404, 405, 501), por GET de toda la vida.
func Consulta[T any](ctx context.Context, c *Client, r Request) (T, error) {
	if len(r.Query) == 0 || c.sinQuery.Load() {
		return JSON[T](ctx, c, r)
	}
	porCuerpo := r
	porCuerpo.Method, porCuerpo.Query, porCuerpo.Body = "QUERY", nil, []byte(r.Query.Encode())
	porCuerpo.Header = r.Header.Clone()
	if porCuerpo.Header == nil {
		porCuerpo.Header = http.Header{}
	}
	porCuerpo.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	out, err := JSON[T](ctx, c, porCuerpo)
	var e *Error
	if !errors.As(err, &e) || (e.Status != http.StatusNotFound && e.Status != http.StatusMethodNotAllowed && e.Status != http.StatusNotImplemented) {
		return out, err
	}
	// Solo se recuerda si el GET sí funciona: un 404 de verdad no dice nada del método.
	if out, err = JSON[T](ctx, c, r); err == nil {
		c.sinQuery.Store(true)
	}
	return out, err
}

// URL compone la dirección de una ruta del servicio.
func (c *Client) URL(path string, query url.Values) string {
	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

// WSURL es URL con el esquema de WebSocket (http→ws, https→wss).
func (c *Client) WSURL(path string, query url.Values) string {
	return "ws" + strings.TrimPrefix(c.URL(path, query), "http")
}
