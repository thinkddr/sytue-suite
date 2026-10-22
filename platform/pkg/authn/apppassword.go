// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package authn

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// # Basic con una contraseña de aplicación, para los clientes DAV
//
// El gestor de ficheros de Linux y el proveedor de documentos de Android no saben
// pedir un token OIDC ni renovarlo: entran con usuario y contraseña. La contraseña es
// una de aplicación de Sytue ID (`/api/v1/me/app-passwords`), revocable en «Mi
// cuenta», y aquí se CANJEA en Sytue ID por un access token de la persona
// (`POST /internal/app-passwords/token`), que es lo que el resto del servicio ya
// sabe autorizar. El token no sale del servicio: se pone en la cabecera de la
// petición y se guarda en memoria hasta poco antes de caducar.

// RealmBasic es el nombre que el cliente enseña al pedir la contraseña.
const RealmBasic = "Sytue"

// margenCanje es cuánto antes de caducar se vuelve a canjear: una petición larga
// (una subida) no puede empezar con un token a punto de morir.
const margenCanje = time.Minute

// AppPasswords canjea credenciales Basic por un token de Sytue ID.
type AppPasswords struct {
	url    string // <id interno>/internal/app-passwords/token
	secret string
	http   *http.Client

	mu  sync.Mutex
	hay map[[32]byte]canjeado
}

type canjeado struct {
	token  string
	caduca time.Time
}

// NewAppPasswords prepara el canje contra Sytue ID (`idURL` es su raíz interna y
// `secret` el de su puerta interna, SYTUE_ID_INTERNAL_SECRET). Con cualquiera de los
// dos vacío devuelve nil, y [AppPasswords.Basic] deja pasar sin canjear nada.
func NewAppPasswords(idURL, secret string) *AppPasswords {
	if strings.TrimSpace(idURL) == "" || secret == "" {
		return nil
	}
	return &AppPasswords{
		url:    strings.TrimRight(idURL, "/") + "/internal/app-passwords/token",
		secret: secret,
		http:   &http.Client{Timeout: 10 * time.Second},
		hay:    map[[32]byte]canjeado{},
	}
}

// errCredenciales es la contraseña que Sytue ID no reconoce.
var errCredenciales = errors.New("authn: contraseña de aplicación no válida")

// Basic va DELANTE de RequireAuth: una petición con `Authorization: Basic` sale de
// aquí con su Bearer. Sin cabecera, contesta 401 con el reto Basic, que es lo que un
// cliente DAV necesita para mandar la contraseña. Con nil, no hace nada.
func (a *AppPasswords) Basic(next http.Handler) http.Handler {
	if a == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			retar(w, r)
			return
		}
		usuario, clave, ok := r.BasicAuth()
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		token, err := a.token(r.Context(), usuario, clave)
		if errors.Is(err, errCredenciales) {
			retar(w, r)
			return
		}
		if err != nil {
			httpx.Fail(w, r, httpx.ErrUnavailable.Wrap(err))
			return
		}
		r.Header.Set("Authorization", "Bearer "+token)
		next.ServeHTTP(w, r)
	})
}

func retar(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", `Basic realm="`+RealmBasic+`", charset="UTF-8"`)
	httpx.Fail(w, r, httpx.ErrUnauthorized)
}

// token es el de la caché si sigue vivo; si no, lo canjea. La clave de la caché es el
// hash de usuario y contraseña: en memoria no queda la contraseña en claro.
//
// ponytail: un mapa por réplica que se poda al guardar; las entradas son las
// credenciales DAV vivas de quienes usan esta réplica.
func (a *AppPasswords) token(ctx context.Context, usuario, clave string) (string, error) {
	k := sha256.Sum256([]byte(usuario + "\x00" + clave))
	ahora := time.Now()
	a.mu.Lock()
	c, ok := a.hay[k]
	a.mu.Unlock()
	if ok && ahora.Before(c.caduca) {
		return c.token, nil
	}
	cuerpo, err := json.Marshal(map[string]string{"identity": usuario, "password": clave})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url, bytes.NewReader(cuerpo))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+a.secret)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusBadRequest:
		return "", errCredenciales
	case resp.StatusCode != http.StatusOK:
		return "", errors.New("authn: Sytue ID contesta " + resp.Status + " al canjear la contraseña")
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.AccessToken == "" {
		return "", errors.New("authn: respuesta del canje ilegible")
	}
	c = canjeado{token: out.AccessToken, caduca: ahora.Add(time.Duration(out.ExpiresIn)*time.Second - margenCanje)}
	a.mu.Lock()
	for kk, v := range a.hay {
		if !ahora.Before(v.caduca) {
			delete(a.hay, kk)
		}
	}
	a.hay[k] = c
	a.mu.Unlock()
	return c.token, nil
}
