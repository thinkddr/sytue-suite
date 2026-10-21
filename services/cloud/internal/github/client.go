// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package github habla con la API de GitHub App de Sytue Cloud (paridad
// Coolify/Vercel): firma un JWT de app (RS256, hand-rolled con crypto/rsa) y lo
// intercambia por un installation access token efímero (~1h, NO se persiste) con
// el que (a) el builder clona repos privados por HTTPS y (b) el notificador
// publica el comentario de preview en el PR.
//
// Clean-room: implementado sobre la API pública REST de GitHub y stdlib
// (crypto/rsa, crypto/x509, encoding/json, net/http). Sin SDK de terceros; sin
// dependencias AGPL.
//
// Anti-SSRF: el host destino es FIJO (api.github.com), nunca controlado por el
// usuario; owner/repo se escapan como segmentos de ruta. El único dato externo
// es el número de instalación/PR (entero) y owner/repo derivados de una repo_url
// ya validada al conectar el proyecto.
package github

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// defaultBaseURL es el host FIJO de la API de GitHub (anti-SSRF: no configurable
// por el usuario). Los tests lo sobreescriben con un httptest.Server.
const defaultBaseURL = "https://api.github.com"

// defaultOAuthBaseURL es el host FIJO del endpoint OAuth de GitHub (distinto de
// la API REST: el canje de code vive en github.com, no en api.github.com).
// Anti-SSRF: no configurable por el usuario; los tests lo sobreescriben.
const defaultOAuthBaseURL = "https://github.com"

// maxInstallationPages acota la paginación de GET /user/installations (100/página
// ⇒ hasta 10k instalaciones), cortando cualquier bucle patológico.
// ponytail: tope duro; nadie con acceso a esa cantidad de instalaciones conecta
// un repo a mano — subir el cap si algún día aparece el caso.
const maxInstallationPages = 100

// apiVersion fija la versión de la API REST de GitHub (cabecera recomendada).
const apiVersion = "2022-11-28"

// userAgent es obligatorio en toda petición a GitHub (si falta, responde 403).
const userAgent = "sytue-cloud"

// errCommentGone señala que el comentario a editar ya no existe (404 en PATCH):
// el llamante recrea uno nuevo.
var errCommentGone = errors.New("github: el comentario ya no existe")

// Client firma JWTs de app e intercambia installation tokens contra la API de
// GitHub. Es seguro para uso concurrente (sin estado mutable salvo el reloj de
// test). Un *Client nil representa "App no configurada": los llamantes lo tratan
// como no-op (degradación limpia).
type Client struct {
	appID        int64
	key          *rsa.PrivateKey
	http         *http.Client
	baseURL      string
	oauthBaseURL string
	now          func() time.Time // inyectable en tests; time.Now en producción
}

// NewClient construye el cliente de GitHub App a partir del App ID y la clave
// privada PEM (RS256). Devuelve (nil, nil) si NINGUNO está configurado (la
// integración queda deshabilitada, no-op); error si solo uno está presente o si
// la clave no es un PEM RSA válido.
func NewClient(appID int64, privateKeyPEM string, opts ...Option) (*Client, error) {
	privateKeyPEM = strings.TrimSpace(privateKeyPEM)
	if appID == 0 && privateKeyPEM == "" {
		//nolint:nilnil // (nil, nil) ES el contrato documentado: integración sin configurar ⇒ no-op, no un error.
		return nil, nil // no configurada: los llamantes degradan a no-op
	}
	if appID == 0 || privateKeyPEM == "" {
		return nil, fmt.Errorf("github: GITHUB_APP_ID y GITHUB_APP_PRIVATE_KEY deben configurarse juntos")
	}
	key, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return nil, err
	}
	c := &Client{
		appID:        appID,
		key:          key,
		baseURL:      defaultBaseURL,
		oauthBaseURL: defaultOAuthBaseURL,
		// Timeouts acotados; host fijo público (sin dial-control anti-SSRF: el
		// destino no es controlable por el tenant).
		http: &http.Client{Timeout: 10 * time.Second},
		now:  time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// Option ajusta un Client recién construido.
type Option func(*Client)

// WithBaseURLs sobreescribe los hosts de API REST y OAuth. Vacío ⇒ se mantiene el
// valor por defecto (api.github.com / github.com). Sirve para tests con httptest y,
// llegado el caso, para apuntar a un GitHub Enterprise Server. Los hosts NO son
// controlables por el tenant (siguen siendo configuración de la plataforma).
func WithBaseURLs(api, oauth string) Option {
	return func(c *Client) {
		if api != "" {
			c.baseURL = api
		}
		if oauth != "" {
			c.oauthBaseURL = oauth
		}
	}
}

// parsePrivateKey acepta una clave RSA en PEM PKCS#1 ("RSA PRIVATE KEY", el
// formato que descarga GitHub) o PKCS#8 ("PRIVATE KEY").
func parsePrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("github: GITHUB_APP_PRIVATE_KEY no es un PEM válido")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	k8, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("github: clave privada de la App no parseable (¿RSA PKCS#1/PKCS#8?): %w", err)
	}
	rsaKey, ok := k8.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("github: la clave privada de la App debe ser RSA")
	}
	return rsaKey, nil
}

// appJWT firma el JWT de la App (RS256): claims iss=appID, iat/exp con vida ≤10
// min (GitHub rechaza exp > iat+10min y iat en el futuro). Hand-rolled sobre
// crypto/rsa (RSASSA-PKCS1-v1_5 + SHA-256) y base64url sin padding.
func (c *Client) appJWT() (string, error) {
	now := c.now()
	// iat 30 s en el pasado absorbe una desviación de reloj entre nosotros y
	// GitHub; exp a 9 min queda holgadamente bajo el tope de 10 min.
	claims := struct {
		Iat int64 `json:"iat"`
		Exp int64 `json:"exp"`
		Iss int64 `json:"iss"`
	}{
		Iat: now.Add(-30 * time.Second).Unix(),
		Exp: now.Add(9 * time.Minute).Unix(),
		Iss: c.appID,
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	// Header fijo RS256; se serializa una sola forma (sin espacios) — es constante.
	signingInput := b64(`{"alg":"RS256","typ":"JWT"}`) + "." + b64(string(claimsJSON))
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("github: firmar JWT de app: %w", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func b64(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

// InstallationToken intercambia el JWT de app por un installation access token
// efímero (~1h) vía POST /app/installations/{id}/access_tokens. El token NO se
// persiste: se usa una vez (clone o comentario) y se descarta.
func (c *Client) InstallationToken(ctx context.Context, installationID int64) (token string, expiresAt time.Time, err error) {
	if installationID <= 0 {
		return "", time.Time{}, fmt.Errorf("github: installation id inválido")
	}
	jwt, err := c.appJWT()
	if err != nil {
		return "", time.Time{}, err
	}
	endpoint := fmt.Sprintf("%s/app/installations/%d/access_tokens", c.baseURL, installationID)
	var out struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := c.doJSON(ctx, http.MethodPost, endpoint, "Bearer "+jwt, nil, &out); err != nil {
		return "", time.Time{}, err
	}
	if out.Token == "" {
		return "", time.Time{}, fmt.Errorf("github: respuesta de installation token sin token")
	}
	return out.Token, out.ExpiresAt, nil
}

// ExchangeUserCode canjea el `code` OAuth (user-to-server) que GitHub adjunta al
// callback de instalación por un user access token, vía
// POST https://github.com/login/oauth/access_token (Accept: application/json).
// El token identifica AL USUARIO que instaló/autorizó la App y es la única forma
// de probar qué instalaciones le pertenecen (ver UserInstallations). El token
// NUNCA se registra ni se propaga en errores. clientID/clientSecret vienen de la
// config del servicio (no se persisten en el Client para no acoplar el canje al
// wiring de arranque de cada binario).
func (c *Client) ExchangeUserCode(ctx context.Context, clientID, clientSecret, code string) (string, error) {
	if clientID == "" || clientSecret == "" {
		return "", fmt.Errorf("github: client_id/client_secret de OAuth no configurados")
	}
	if code == "" {
		return "", fmt.Errorf("github: code de OAuth vacío")
	}
	form := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
	}
	endpoint := c.oauthBaseURL + "/login/oauth/access_token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		// El error de red NO incluye el body (que llevaría client_secret/code).
		return "", fmt.Errorf("github: canjear code OAuth: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("github: canjear code OAuth devolvió %d", resp.StatusCode)
	}
	// GitHub responde 200 incluso ante error lógico (campo `error` en el cuerpo).
	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return "", fmt.Errorf("github: decodificar respuesta OAuth: %w", err)
	}
	if out.Error != "" {
		// Solo el código de error (p.ej. bad_verification_code), nunca el token.
		return "", fmt.Errorf("github: OAuth rechazado: %s", out.Error)
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("github: respuesta OAuth sin access_token")
	}
	return out.AccessToken, nil
}

// UserInstallations lista los IDs de instalación de la App a los que el USUARIO
// (dueño del userToken) tiene acceso, vía GET /user/installations (paginado).
// Es la comprobación de propiedad: solo se puede ligar una instalación que
// aparezca aquí. El userToken viaja en Authorization (Bearer), nunca en logs.
func (c *Client) UserInstallations(ctx context.Context, userToken string) ([]int64, error) {
	if userToken == "" {
		return nil, fmt.Errorf("github: user token vacío")
	}
	var ids []int64
	for page := 1; page <= maxInstallationPages; page++ {
		endpoint := fmt.Sprintf("%s/user/installations?per_page=100&page=%d", c.baseURL, page)
		var out struct {
			TotalCount    int `json:"total_count"`
			Installations []struct {
				ID int64 `json:"id"`
			} `json:"installations"`
		}
		if err := c.doJSON(ctx, http.MethodGet, endpoint, "Bearer "+userToken, nil, &out); err != nil {
			return nil, err
		}
		for _, in := range out.Installations {
			ids = append(ids, in.ID)
		}
		if len(out.Installations) == 0 || len(ids) >= out.TotalCount {
			break
		}
	}
	return ids, nil
}

// UpsertPRComment publica o edita, de forma idempotente, el comentario de
// preview del PR. Si existingID > 0 intenta editarlo (PATCH); si ya no existe
// (404) o no había, crea uno nuevo (POST). Devuelve el id del comentario vivo
// para que el llamante lo persista y lo reutilice en el siguiente re-deploy del
// mismo PR (así no se duplica el comentario).
func (c *Client) UpsertPRComment(ctx context.Context, token, owner, repo string, pr int, existingID int64, body string) (int64, error) {
	if existingID > 0 {
		err := c.patchComment(ctx, token, owner, repo, existingID, body)
		if err == nil {
			return existingID, nil
		}
		if !errors.Is(err, errCommentGone) {
			return 0, err
		}
		// El comentario fue borrado a mano: cae a crear uno nuevo.
	}
	return c.createComment(ctx, token, owner, repo, pr, body)
}

func (c *Client) createComment(ctx context.Context, token, owner, repo string, pr int, body string) (int64, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments",
		c.baseURL, url.PathEscape(owner), url.PathEscape(repo), pr)
	var out struct {
		ID int64 `json:"id"`
	}
	if err := c.doJSON(ctx, http.MethodPost, endpoint, "token "+token,
		map[string]string{"body": body}, &out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

func (c *Client) patchComment(ctx context.Context, token, owner, repo string, commentID int64, body string) error {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/issues/comments/%d",
		c.baseURL, url.PathEscape(owner), url.PathEscape(repo), commentID)
	return c.doJSON(ctx, http.MethodPatch, endpoint, "token "+token,
		map[string]string{"body": body}, nil)
}

// doJSON envía una petición JSON a GitHub con las cabeceras obligatorias y
// decodifica la respuesta 2xx en out (si no es nil). El cuerpo del error se
// trunca: NUNCA registra ni propaga el token (que va solo en Authorization).
func (c *Client) doJSON(ctx context.Context, method, endpoint, authorization string, payload, out any) error {
	var bodyReader io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	req.Header.Set("User-Agent", userAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("github: %s %s: %w", method, redactURL(endpoint), err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		_ = resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusNotFound && method == http.MethodPatch {
		return errCommentGone
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("github: %s %s devolvió %d: %s",
			method, redactURL(endpoint), resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	if out != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
			return fmt.Errorf("github: decodificar respuesta: %w", err)
		}
	}
	return nil
}

// redactURL devuelve la ruta del endpoint sin query (defensa: la URL no lleva
// secretos, pero evitamos filtrar cualquier futuro parámetro sensible en logs).
func redactURL(endpoint string) string {
	if u, err := url.Parse(endpoint); err == nil {
		return u.Path
	}
	return endpoint
}

// ParseRepo extrae owner/repo de una repo_url de GitHub (https o SSH scp-like).
// ok=false si no es un repositorio de github.com bien formado — el llamante lo
// trata como "no publicar comentario" (no todo repo conectado es de GitHub).
func ParseRepo(repoURL string) (owner, repo string, ok bool) {
	repoURL = strings.TrimSpace(repoURL)
	var host, path string
	switch {
	case strings.HasPrefix(repoURL, "git@"):
		// git@github.com:owner/repo(.git)
		rest := repoURL[len("git@"):]
		h, p, found := strings.Cut(rest, ":")
		if !found {
			return "", "", false
		}
		host, path = h, p
	default:
		u, err := url.Parse(repoURL)
		if err != nil {
			return "", "", false
		}
		host, path = u.Hostname(), u.Path
	}
	if !strings.EqualFold(host, "github.com") {
		return "", "", false
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	o, r, found := strings.Cut(path, "/")
	if !found || o == "" || r == "" || strings.Contains(r, "/") {
		return "", "", false
	}
	return o, r, true
}
