// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package chat son los clientes de las cuentas de mensajería de terceros que hablan por
// HTTP con un token OAuth: Microsoft Teams (Microsoft Graph), Google Chat y Google Meet
// (apps/fyne/PLAN-EDICION-LOCAL.md §6.14, RO-01).
//
// Lo comparten el servicio de integraciones (las conversaciones externas de Sytue
// Rooms, por la pasarela) y Sytue Office (`local/chat`, desde el dispositivo):
//
//   - lo que era del servidor y se movió aquí sin cambiarlo (R-NO-COPIAR): el mensaje de
//     Graph y el de Google Chat tal como llegan, las comprobaciones de los
//     identificadores que acaban en una ruta, la ruta de los mensajes de un chat o de un
//     canal de Teams, el envío a Google Chat y el enlace que devuelve Meet;
//   - los clientes [Graph], [GoogleChat], [Meet], [Slack] y [PersonasGoogle], con el
//     transporte INYECTADO ([HTTPDoer]: el portero de red en el dispositivo, el runtime
//     de la pasarela con sus defensas en el servidor) y la dirección base también: este
//     paquete no fija ningún servidor, quien lo usa dice a cuál habla.
//
// El token es de quien usa el cliente: en el dispositivo, el de la persona ([Token]); en
// la pasarela, ninguno (Token nil), porque lo inyecta el runtime desde el vault y el
// cliente no lo ve. Con [PedirJSON] la pasarela hace por este mismo transporte lo que no
// es de mensajería.
//
// Ningún error lleva el cuerpo de la respuesta del proveedor ni el token, y una página
// siguiente solo se sigue si es del mismo origen que la base ([MismoOrigen]): el token
// no sale nunca hacia otro sitio.
package chat

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// HTTPDoer es el transporte: `*http.Client` lo cumple.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Token es de dónde sale el de acceso. Invalidar lo tira cuando el proveedor contesta
// 401 (caducó antes de lo que dijo o se revocó): la siguiente petición lo renueva.
type Token interface {
	Acceso(ctx context.Context) (string, error)
	Invalidar()
}

// TokenFijo es un token que no se renueva (las pruebas, o quien ya lo trae).
type TokenFijo string

// Acceso es el token.
func (t TokenFijo) Acceso(context.Context) (string, error) { return string(t), nil }

// Invalidar no hace nada: no hay con qué renovarlo.
func (TokenFijo) Invalidar() {}

// ErrEntrada es un identificador que no es del proveedor: no se llega a llamar a la red.
var ErrEntrada = errors.New("chat: identificador no válido para el proveedor")

// ErrRespuesta es una respuesta del proveedor que no se entiende o no es lo esperado.
var ErrRespuesta = errors.New("chat: el proveedor devolvió una respuesta ilegible")

// ErrEnlace es un enlace de reunión que el proveedor dio y no es el suyo (o no es
// https): una [ErrRespuesta] que la pasarela distingue.
var ErrEnlace = fmt.Errorf("%w: el enlace de la reunión no es válido", ErrRespuesta)

// ErrIdentidad es una identidad de quien entró que no tiene la forma del proveedor: una
// [ErrRespuesta] que la pasarela distingue.
var ErrIdentidad = fmt.Errorf("%w: la identidad de la cuenta no es válida", ErrRespuesta)

// ErrProveedor es una respuesta que no es 2xx, sin su cuerpo.
type ErrProveedor struct {
	Estado int
	// Reintentar es el Retry-After de un 429 (o de un 503).
	Reintentar time.Duration
}

func (e *ErrProveedor) Error() string {
	switch e.Estado {
	case http.StatusUnauthorized:
		return "chat: el proveedor no acepta la sesión (401); vuelve a entrar en la cuenta"
	case http.StatusForbidden:
		return "chat: la cuenta no tiene permiso para esto (403)"
	case http.StatusTooManyRequests:
		return "chat: el proveedor pide esperar antes de seguir (429)"
	}
	return fmt.Sprintf("chat: el proveedor respondió %d", e.Estado)
}

// Estado es el código HTTP de un [ErrProveedor] (0 si err no lo es).
func Estado(err error) int {
	var e *ErrProveedor
	if errors.As(err, &e) {
		return e.Estado
	}
	return 0
}

// Espera es el Retry-After de un [ErrProveedor] (0 si no lo hay).
func Espera(err error) time.Duration {
	var e *ErrProveedor
	if errors.As(err, &e) {
		return e.Reintentar
	}
	return 0
}

// MismoOrigen dice si enlace (una página siguiente, un `deltaLink`) es del mismo
// esquema, máquina y puerto que base y cuelga de su ruta: solo así se sigue con el token.
// El puerto por defecto del esquema cuenta como no ponerlo.
func MismoOrigen(base, enlace string) bool {
	b, err1 := url.Parse(base)
	e, err2 := url.Parse(enlace)
	if err1 != nil || err2 != nil || e.User != nil || e.Fragment != "" {
		return false
	}
	return b.Scheme == e.Scheme && (e.Scheme == "https" || e.Scheme == "http") && strings.EqualFold(b.Hostname(), e.Hostname()) &&
		puerto(b) == puerto(e) && strings.HasPrefix(e.Path, strings.TrimRight(b.Path, "/")+"/")
}

func puerto(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "http" {
		return "80"
	}
	return "443"
}

// destino es ruta (relativa a base, con su consulta) como dirección; una absoluta solo
// vale si es del mismo origen que base.
func destino(base, ruta string) (string, error) {
	if strings.Contains(ruta, "://") {
		if !MismoOrigen(base, ruta) {
			return "", ErrEntrada
		}
		return ruta, nil
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(ruta, "/"), nil
}

// MaxPaginas es cuántas páginas se siguen como mucho en un listado.
const MaxPaginas = 20

// MaxRespuesta es lo más que se lee de una respuesta.
const MaxRespuesta = 8 << 20

// cliente hace las peticiones con el token y repite una vez tras un 401. Sin token, la
// credencial la pone el transporte (la pasarela) y no hay nada que renovar.
type cliente struct {
	http  HTTPDoer
	token Token
	// limite es lo más que se lee de una respuesta (cero: MaxRespuesta).
	limite int64
}

// Fichero es un cuerpo que no es JSON (un fichero que se sube): Abrir se llama en cada
// intento, porque tras un 401 se manda otra vez.
type Fichero struct {
	Tipo   string
	Tamano int64
	Abrir  func() (io.ReadCloser, error)
}

func (c cliente) pedir(ctx context.Context, metodo, destino string, cuerpo, out any) error {
	resp, err := c.abrir(ctx, metodo, destino, cuerpo)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	limite := cmp.Or(c.limite, MaxRespuesta)
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, min(limite, 1<<20)))
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, limite)).Decode(out); err != nil {
		return ErrRespuesta
	}
	return nil
}

// abrir hace la petición y da la respuesta si es 2xx (quien la recibe la cierra); tras
// un 401 tira el token y repite una vez.
func (c cliente) abrir(ctx context.Context, metodo, destino string, cuerpo any) (*http.Response, error) {
	resp, err := c.una(ctx, metodo, destino, cuerpo)
	if Estado(err) == http.StatusUnauthorized && c.token != nil {
		// El token caducó antes de tiempo o se revocó: se tira y se pide otro una vez.
		c.token.Invalidar()
		resp, err = c.una(ctx, metodo, destino, cuerpo)
	}
	return resp, err
}

func (c cliente) una(ctx context.Context, metodo, destino string, cuerpo any) (*http.Response, error) {
	if c.http == nil {
		return nil, errors.New("chat: cliente sin transporte")
	}
	var r io.Reader = http.NoBody
	tipo := ""
	largo := int64(-1)
	switch v := cuerpo.(type) {
	case nil:
	case url.Values:
		r, tipo = strings.NewReader(v.Encode()), "application/x-www-form-urlencoded"
	case Fichero:
		rc, err := v.Abrir()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		r, tipo, largo = rc, cmp.Or(v.Tipo, "application/octet-stream"), v.Tamano
	case multiparte:
		rc, err := v.abrir()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		r, tipo = rc, v.tipo
	default:
		b, err := json.Marshal(cuerpo)
		if err != nil {
			return nil, err
		}
		r, tipo = bytes.NewReader(b), "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, metodo, destino, r)
	if err != nil {
		return nil, err
	}
	if largo >= 0 {
		req.ContentLength = largo
	}
	req.Header.Set("Accept", "application/json")
	if c.token != nil {
		tok, err := c.token.Acceso(ctx)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	if tipo != "" {
		req.Header.Set("Content-Type", tipo)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		resp.Body.Close()
		e := &ErrProveedor{Estado: resp.StatusCode}
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
			e.Reintentar = time.Duration(min(s, 86400)) * time.Second
		}
		return nil, e
	}
	return resp, nil
}

// PedirJSON hace una petición JSON a destino SIN token (la credencial, si hace falta, la
// pone el transporte) y decodifica la respuesta en out (nil: se descarta), leyendo como
// mucho limite (cero: MaxRespuesta). Es el transporte de la pasarela también para lo que
// no es de mensajería; destino lo comprueba quien llama.
func PedirJSON(ctx context.Context, h HTTPDoer, metodo, destino string, cuerpo, out any, limite int64) error {
	return cliente{http: h, limite: limite}.pedir(ctx, metodo, destino, cuerpo, out)
}

// Contenido es el cuerpo de una descarga.
type Contenido struct {
	io.ReadCloser
	Tipo string
}

// bajar da el cuerpo de un GET a destino (quien lo recibe lo cierra).
func (c cliente) bajar(ctx context.Context, destino string) (Contenido, error) {
	resp, err := c.abrir(ctx, http.MethodGet, destino, nil) //nolint:bodyclose // el cuerpo es el Contenido: lo cierra quien lo recibe
	if err != nil {
		return Contenido{}, err
	}
	return Contenido{ReadCloser: resp.Body, Tipo: resp.Header.Get("Content-Type")}, nil
}

// unir es base más ruta y consulta.
func unir(base, ruta string, q url.Values) string {
	s := strings.TrimRight(base, "/") + "/" + strings.TrimLeft(ruta, "/")
	if len(q) > 0 {
		s += "?" + q.Encode()
	}
	return s
}

// enlaceHTTPS comprueba un enlace que el proveedor da para abrirlo fuera (el de una
// reunión, el de un fichero): https, con máquina y sin credenciales.
func enlaceHTTPS(crudo string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(crudo))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return "", ErrEnlace
	}
	return u.String(), nil
}
