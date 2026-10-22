// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package offline

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// En GOOS=js este fichero no existe a propósito: ahí net/http es fetch(), y fetch ya
// pasa por el service worker de la app, que hace exactamente esto.

// Las reglas son las de worker.ts, literales. Un test las busca en aquel fichero.
var (
	// Lo que NUNCA se guarda: sesión, tokens, tickets, y todo lo que es una
	// conversación en vivo y no un dato.
	prefijosPrivados = []string{
		"/auth/", "/api/token", "/api/session", "/api/intent", "/api/presign",
		"/api/ws-ticket", "/api/collab", "/api/call", "/api/conference", "/api/ai",
	}
	// Ficheros: solo se guardan si la persona lo pidió (`?offline=1`).
	prefijosDeFichero = []string{
		"/api/content", "/api/shared-content", "/api/version-content", "/api/zip", "/api/attachment",
	}
	// El prefijo de la app (`/mail/api/…` → `/api/…`): RE_BASE_DE_APP, con el lookahead
	// vuelto grupo porque RE2 no tiene lookaheads.
	reBase = regexp.MustCompile(`^/[a-z-]+(/(?:auth|api|_astro)/)`)
)

// MaxCacheBytes es el tope por respuesta (`MAX_CACHE_BYTES`).
const MaxCacheBytes = 50 << 20

func sinBase(ruta string) string { return reBase.ReplaceAllString(ruta, "$1") }

func tienePrefijo(ruta string, prefijos []string) bool {
	for _, p := range prefijos {
		if strings.HasPrefix(ruta, p) {
			return true
		}
	}
	return false
}

// Transport es `networkFirst`: intenta la red y guarda lo que contesta; si la red falla
// y había copia, sirve la copia marcada con `X-Sytue-Offline: cache`. Solo para GET y
// solo con sesión: sin dueño no hay a nombre de quién guardar.
type Transport struct {
	Base    http.RoundTripper // nil es http.DefaultTransport
	Store   *Store
	Owner   func() string // el `sub` de la sesión, o "" sin ella
	Product string
	Monitor *Monitor // opcional
	// Private añade prefijos que este servicio no quiere ver guardados. Los de
	// worker.ts son rutas del BFF; una API de servicio llamada en directo puede tener
	// las suyas.
	Private []string
}

type respuestaGuardada struct {
	Status int         `json:"status"`
	Header http.Header `json:"header"`
	Body   []byte      `json:"body"` // ponytail: base64 dentro de JSON, +33 %. Basta hasta que se guarden ficheros grandes
}

// RoundTrip implementa http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	owner := ""
	if t.Owner != nil {
		owner = t.Owner()
	}
	ruta := sinBase(req.URL.Path)
	// `Cache-Control: no-store` en la petición: es una descarga (una exportación) que
	// quien llama no quiere en la caché del sistema ni servida vieja sin red.
	if req.Method != http.MethodGet || owner == "" || req.Header.Get("Cache-Control") == "no-store" ||
		tienePrefijo(ruta, prefijosPrivados) || tienePrefijo(ruta, t.Private) {
		return base.RoundTrip(req)
	}
	espacio, id := espacioHTTP(t.Product), req.URL.String()

	resp, err := base.RoundTrip(req)
	if err != nil {
		// Que quien llama cancele no es quedarse sin red.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		var g respuestaGuardada
		if ok, _ := t.Store.Read(owner, espacio, id, &g); !ok {
			return nil, err
		}
		t.informar(Offline)
		g.Header.Set("X-Sytue-Offline", "cache")
		return &http.Response{
			Status: http.StatusText(g.Status), StatusCode: g.Status, Header: g.Header,
			Body: io.NopCloser(bytes.NewReader(g.Body)), ContentLength: int64(len(g.Body)),
			Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1, Request: req,
		}, nil
	}
	t.informar(Online)
	if !guardable(resp, req) {
		return resp, nil
	}
	// Se lee hasta un byte más del tope. Si cabe, se guarda; si no, no se guarda, y
	// en los dos casos quien llama recibe el cuerpo ENTERO.
	leido, errLeer := io.ReadAll(io.LimitReader(resp.Body, MaxCacheBytes+1))
	if errLeer != nil || len(leido) > MaxCacheBytes {
		resp.Body = struct {
			io.Reader
			io.Closer
		}{io.MultiReader(bytes.NewReader(leido), errorTras(resp.Body, errLeer)), resp.Body}
		return resp, nil
	}
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(leido))
	// No poder guardar no estropea la respuesta: se entrega igual.
	_ = t.Store.Write(owner, espacio, id, respuestaGuardada{resp.StatusCode, resp.Header, leido})
	return resp, nil
}

func (t *Transport) informar(s State) {
	if t.Monitor != nil {
		t.Monitor.Report(s)
	}
}

// errorTras sigue leyendo de r, salvo que la lectura ya hubiera fallado: entonces
// devuelve ESE error en vez de hacer como que el cuerpo acabó bien.
func errorTras(r io.Reader, err error) io.Reader {
	if err == nil {
		return r
	}
	return lectorRoto{err}
}

type lectorRoto struct{ err error }

func (l lectorRoto) Read([]byte) (int, error) { return 0, l.err }

// guardable es `cacheable(response, request, true)`.
func guardable(resp *http.Response, req *http.Request) bool {
	if resp.StatusCode != http.StatusOK || req.Header.Get("Range") != "" {
		return false
	}
	ruta, q := sinBase(req.URL.Path), req.URL.Query()
	pedidoOffline := q.Get("offline") == "1"
	esFichero := tienePrefijo(ruta, prefijosDeFichero) || (ruta == "/api/attachments" && q.Has("index"))
	if esFichero && !pedidoOffline {
		return false
	}
	// Un fichero marcado para uso sin conexión y sin tamaño declarado no se guarda:
	// permitirlo haría ficticio el tope, y una respuesta chunked podría llenar el disco.
	if pedidoOffline && resp.ContentLength <= 0 {
		return false
	}
	return resp.ContentLength <= MaxCacheBytes
}
