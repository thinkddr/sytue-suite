// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package connectors

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

// OAuth2 client_credentials (RFC 6749 §4.4) para el conector REST: antes de la
// petición de datos se obtiene un access token del token endpoint y se adjunta
// como Bearer. El token se CACHEA en memoria por credencial (con su expires_in) y
// se refresca al caducar o si la petición de datos responde 401.
//
// Nota sobre refresh_token: client_credentials NO emite refresh_token (no hay un
// usuario que delegue acceso); el único "refresco" es volver a pedir un token con
// las mismas credenciales, que es justo lo que hace la caché al expirar. Los flujos
// que sí llevan refresh_token (authorization_code / password) no aplican a un
// conector servidor-a-servidor y por eso no se implementan aquí.
//
// Seguridad: el client_secret y el access token JAMÁS se registran ni se devuelven
// al cliente; el token_url pasa por la barrera SSRF (es una URL externa).

const (
	// oauthTokenSkew: trata el token como caducado un poco antes de expires_in
	// (margen de reloj/latencia), para no usar uno recién expirado.
	oauthTokenSkew = 30 * time.Second
	// oauthDefaultTTL: si el proveedor no envía expires_in, cachea de forma
	// conservadora (se refrescará pronto).
	oauthDefaultTTL = 60 * time.Second
	// oauthMaxTokenBytes: un token endpoint responde un JSON pequeño; acota la lectura.
	oauthMaxTokenBytes = 1 << 20 // 1 MiB
)

// errUnauthorized: la petición de DATOS devolvió 401. Con OAuth2 dispara UN
// reintento con token nuevo (el token pudo caducar antes de tiempo o ser
// revocado). Va envuelto en ErrUpstream: si el 401 persiste, mapea a 502.
var errUnauthorized = errors.New("connectors: 401 del origen externo")

// cachedToken es un access token con su instante de caducidad.
type cachedToken struct {
	accessToken string
	expiresAt   time.Time
}

// valid indica si el token sigue siendo utilizable en 'now' (con margen de reloj).
func (t cachedToken) valid(now time.Time) bool {
	return t.accessToken != "" && now.Add(oauthTokenSkew).Before(t.expiresAt)
}

// tokenCache cachea access tokens de client_credentials, indexados por un HASH de
// las credenciales (token_url|client_id|scope|audience|client_auth|client_secret).
// Misma configuración ⇒ mismo token (correcto: la identidad del token es la de la
// credencial). Rotar el secreto produce una clave nueva; las entradas viejas
// caducan y se barren perezosamente al insertar. Concurrente-seguro (mutex).
//
// ponytail: sin evicción activa ni single-flight; el nº de conexiones OAuth por
// tenant es pequeño y las entradas caducan solas — un sweep al insertar basta.
// Añadir LRU/single-flight si el volumen o el coste del token endpoint lo exigen.
type tokenCache struct {
	mu     sync.Mutex
	tokens map[string]cachedToken
}

func newTokenCache() *tokenCache { return &tokenCache{tokens: make(map[string]cachedToken)} }

// sweepExpired elimina las entradas caducadas. Se llama con el lock tomado.
func (c *tokenCache) sweepExpired(now time.Time) {
	for k, t := range c.tokens {
		if !now.Before(t.expiresAt) {
			delete(c.tokens, k)
		}
	}
}

// oauthCacheKey deriva la clave de caché de las credenciales OAuth2. Es un hash:
// NUNCA guarda el secreto en claro (ni como clave del mapa, ni por tanto en un
// eventual volcado de memoria/diagnóstico del mapa).
func oauthCacheKey(a RestAuth, clientSecret string) string {
	h := sha256.New()
	// Separador NUL entre campos: evita colisiones por concatenación ("ab"+"c" ≠ "a"+"bc").
	for _, s := range []string{a.TokenURL, a.ClientID, a.Scope, a.Audience, a.ClientAuth, clientSecret} {
		_, _ = io.WriteString(h, s)
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// executeRESTAuthed ejecuta la petición de datos REST resolviendo la auth OAuth2 si
// aplica: obtiene el access token (cacheado), lo usa como Bearer y, ante un 401,
// refresca el token UNA vez y reintenta. Para el resto de auth, delega directo.
func (e *Executor) executeRESTAuthed(ctx context.Context, cfg RestConfig, creds Credentials, q RestQuery) (*Result, error) {
	if cfg.Auth.Type != RestAuthOAuth2ClientCreds {
		return executeREST(ctx, e.client, cfg, creds, q, e.limits, "")
	}
	token, err := e.oauthToken(ctx, cfg, creds, false)
	if err != nil {
		return nil, err
	}
	res, err := executeREST(ctx, e.client, cfg, creds, q, e.limits, token)
	if errors.Is(err, errUnauthorized) {
		// El token pudo caducar antes de expires_in o ser revocado: refresca y reintenta.
		token, err = e.oauthToken(ctx, cfg, creds, true)
		if err != nil {
			return nil, err
		}
		res, err = executeREST(ctx, e.client, cfg, creds, q, e.limits, token)
	}
	return res, err
}

// oauthToken devuelve un access token válido para la config OAuth2 dada, usando la
// caché. Si force, ignora la caché (tras un 401) y pide uno nuevo. Concurrente-seguro.
func (e *Executor) oauthToken(ctx context.Context, cfg RestConfig, creds Credentials, force bool) (string, error) {
	key := oauthCacheKey(cfg.Auth, creds.Secret)
	now := time.Now()

	if !force {
		e.tokens.mu.Lock()
		t, ok := e.tokens.tokens[key]
		e.tokens.mu.Unlock()
		if ok && t.valid(now) {
			return t.accessToken, nil
		}
	}

	// Fetch FUERA del lock (I/O de red): distintas conexiones no se serializan. Dos
	// fetches concurrentes de la misma clave son aceptables (raros); el último gana.
	token, ttl, err := fetchClientCredentialsToken(ctx, e.client, cfg.Auth, creds.Secret)
	if err != nil {
		return "", err
	}

	e.tokens.mu.Lock()
	e.tokens.sweepExpired(now)
	e.tokens.tokens[key] = cachedToken{accessToken: token, expiresAt: now.Add(ttl)}
	e.tokens.mu.Unlock()
	return token, nil
}

// fetchClientCredentialsToken pide un access token al token endpoint por
// client_credentials. Pasa el token_url por la barrera SSRF (URL externa). Errores
// claros: destino bloqueado (SSRF), fallo de red (ErrUpstream) y 4xx del endpoint
// (credenciales/scope rechazados ⇒ ErrInvalid/400). NUNCA registra secreto ni token.
func fetchClientCredentialsToken(ctx context.Context, client *http.Client, a RestAuth, clientSecret string) (string, time.Duration, error) {
	// Barrera 1 SSRF sobre el token_url (host externo aportado por el usuario). El
	// cliente re-valida cada redirect (barrera 1) y conecta a la IP validada (barrera 2).
	if err := ssrfhttp.ValidateURL(ctx, a.TokenURL); err != nil {
		return "", 0, err
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	if a.Scope != "" {
		form.Set("scope", a.Scope)
	}
	if a.Audience != "" {
		form.Set("audience", a.Audience)
	}
	// client_secret_post: credenciales en el cuerpo. client_secret_basic: en la
	// cabecera Authorization (se añade más abajo).
	if a.ClientAuth != "basic" {
		form.Set("client_id", a.ClientID)
		form.Set("client_secret", clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, invalid("token_url no es válida")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Sytue-Docs-Connector/1")
	if a.ClientAuth == "basic" {
		// RFC 6749 §2.3.1: client_id y secret se codifican x-www-form-urlencoded
		// antes de formar el par usuario:contraseña de HTTP Basic.
		req.SetBasicAuth(url.QueryEscape(a.ClientID), url.QueryEscape(clientSecret))
	}

	resp, err := client.Do(req)
	if err != nil {
		// El error crudo puede llevar el host: se envuelve en ErrUpstream (la capa
		// HTTP no lo expone; queda solo para logs de servidor).
		return "", 0, fmt.Errorf("%w: token_url: %w", ErrUpstream, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, oauthMaxTokenBytes))
	if err != nil {
		return "", 0, fmt.Errorf("%w: leer respuesta del token: %w", ErrUpstream, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 4xx = credenciales/scope rechazados: es un problema de configuración del
		// usuario ⇒ ErrInvalid (400) con mensaje claro (sin filtrar el cuerpo remoto).
		return "", 0, invalid("el proveedor OAuth2 rechazó la petición de token (HTTP %d)", resp.StatusCode)
	}

	var tr struct {
		AccessToken string          `json:"access_token"`
		ExpiresIn   json.RawMessage `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", 0, fmt.Errorf("%w: la respuesta del token no es JSON", ErrUpstream)
	}
	if tr.AccessToken == "" {
		return "", 0, fmt.Errorf("%w: el token endpoint no devolvió access_token", ErrUpstream)
	}
	ttl := oauthDefaultTTL
	if secs := parseExpiresIn(tr.ExpiresIn); secs > 0 {
		ttl = time.Duration(secs) * time.Second
	}
	return tr.AccessToken, ttl, nil
}

// parseExpiresIn lee expires_in, que distintos proveedores mandan como número
// (3600) o como cadena ("3600"); 0 si falta o no es un entero.
func parseExpiresIn(raw json.RawMessage) int64 {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
