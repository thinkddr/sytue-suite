// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package federation implementa el lado Relying Party (RP) de Sytue ID actuando
// de broker OIDC: descubre la configuración de un IdP upstream (Google Workspace,
// Microsoft EntraID, cualquier OIDC), intercambia el código por tokens y VALIDA el
// id_token (firma vía JWKS del upstream, iss, aud, exp/iat, nonce) para autenticar
// a un usuario contra ese IdP externo.
//
// Clean-room sobre OIDC Core 1.0 / Discovery (RFC 8414). REUSA el verificador JOSE
// de Sytue ID (oidc.Verify: allowlist RS256 — rechaza alg=none —, kid, iss, aud,
// exp/iat), el documento de discovery (oidc.DiscoveryDocument) y la barrera SSRF
// de doble capa de la suite (platform/pkg/ssrfhttp): TODA llamada de red a un
// destino que aporta el administrador (issuer/token/userinfo/jwks) se encauza por
// ahí, de modo que nunca alcanza una IP interna/metadata ni por redirect ni por
// DNS-rebinding, y solo por https.
package federation

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwk"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
)

// Errores públicos del broker (envueltos; no filtran detalle del upstream).
var (
	ErrDiscovery = errors.New("federation: discovery del IdP falló")
	ErrExchange  = errors.New("federation: intercambio de código falló")
	ErrIDToken   = errors.New("federation: id_token inválido")
	ErrScheme    = errors.New("federation: solo se admite https")
)

// maxBody acota el cuerpo de las respuestas del upstream (discovery/token/userinfo)
// para no consumir memoria sin límite ante un endpoint malicioso.
const maxBody = 1 << 20 // 1 MiB

// jwksTTL es la vida de una entrada de JWKS en caché. Corta: las claves del
// upstream rotan; además, ante un kid desconocido se fuerza un refetch.
const jwksTTL = 15 * time.Minute

// insecureAllowed permite issuers/endpoints por http SOLO en tests herméticos
// (httptest sirve por http en loopback). En producción SIEMPRE se exige https.
var insecureAllowed = false

// SetInsecureForTest habilita http (loopback) para tests herméticos y devuelve el
// restaurador. EXCLUSIVO para tests; jamás en producción.
func SetInsecureForTest() (restore func()) {
	prev := insecureAllowed
	insecureAllowed = true
	return func() { insecureAllowed = prev }
}

// Endpoints son los puntos del IdP upstream resueltos por discovery.
type Endpoints struct {
	Authorization string
	Token         string
	Userinfo      string
	JWKS          string
	PKCE          bool // el upstream anuncia code_challenge_method S256
}

// Broker es el cliente RP: un http.Client SSRF-safe y una caché de JWKS.
type Broker struct {
	client *http.Client
	mu     sync.Mutex
	jwks   map[string]cachedSet
}

type cachedSet struct {
	set     jwk.Set
	fetched time.Time
}

// New construye un Broker con el cliente HTTP SSRF-safe compartido (barrera 2 en
// el dialer + redirects re-validados por la barrera 1). timeout 15s, 5 redirects.
func New() *Broker {
	return &Broker{
		client: ssrfhttp.NewClient(15*time.Second, 5),
		jwks:   make(map[string]cachedSet),
	}
}

// checkURL aplica la barrera 1 (esquema + IP pública) ANTES de conectar y exige
// https (salvo el hook de test). La barrera 2 (dialer) la re-aplica en el connect.
func (b *Broker) checkURL(ctx context.Context, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: URL ilegible", ErrScheme)
	}
	if u.Scheme != "https" && (!insecureAllowed || u.Scheme != "http") {
		return fmt.Errorf("%w: esquema %q", ErrScheme, u.Scheme)
	}
	return ssrfhttp.ValidateURL(ctx, rawURL)
}

// getJSON hace GET a un endpoint del upstream (ya SSRF-validado) y decodifica el
// JSON con un tope de cuerpo. auth, si no está vacío, va como Bearer (userinfo).
func (b *Broker) getJSON(ctx context.Context, rawURL, auth string, out any) error {
	if err := b.checkURL(ctx, rawURL); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", "Bearer "+auth)
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("estado %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(out)
}

// Discover resuelve <issuer>/.well-known/openid-configuration (OIDC Discovery),
// valida que el issuer del documento coincide con el configurado (evita que un
// discovery apunte a endpoints de otro emisor) y que authorization/token/jwks son
// https y públicos. Reusa oidc.DiscoveryDocument para decodificar.
func (b *Broker) Discover(ctx context.Context, issuer string) (Endpoints, error) {
	issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
	if issuer == "" {
		return Endpoints{}, fmt.Errorf("%w: issuer vacío", ErrDiscovery)
	}
	wellKnown := issuer + "/.well-known/openid-configuration"
	var doc oidc.DiscoveryDocument
	if err := b.getJSON(ctx, wellKnown, "", &doc); err != nil {
		return Endpoints{}, fmt.Errorf("%w: %w", ErrDiscovery, err)
	}
	// El issuer del documento DEBE coincidir con el configurado (OIDC Discovery
	// §4.3): así el authorization/token no puede desviarse a otro emisor.
	if strings.TrimRight(doc.Issuer, "/") != issuer {
		return Endpoints{}, fmt.Errorf("%w: issuer del documento (%q) ≠ configurado", ErrDiscovery, doc.Issuer)
	}
	for label, ep := range map[string]string{
		"authorization_endpoint": doc.AuthorizationEndpoint,
		"token_endpoint":         doc.TokenEndpoint,
		"jwks_uri":               doc.JWKSURI,
	} {
		if ep == "" {
			return Endpoints{}, fmt.Errorf("%w: falta %s", ErrDiscovery, label)
		}
		if err := b.checkURL(ctx, ep); err != nil {
			return Endpoints{}, fmt.Errorf("%w: %s: %w", ErrDiscovery, label, err)
		}
	}
	if doc.UserinfoEndpoint != "" {
		if err := b.checkURL(ctx, doc.UserinfoEndpoint); err != nil {
			return Endpoints{}, fmt.Errorf("%w: userinfo_endpoint: %w", ErrDiscovery, err)
		}
	}
	return Endpoints{
		Authorization: doc.AuthorizationEndpoint,
		Token:         doc.TokenEndpoint,
		Userinfo:      doc.UserinfoEndpoint,
		JWKS:          doc.JWKSURI,
		PKCE:          containsFold(doc.CodeChallengeMethodsSupported, "S256"),
	}, nil
}

// tokenResponse es la respuesta del token endpoint (RFC 6749 §4.1.4 + OIDC).
type tokenResponse struct {
	IDToken          string `json:"id_token"`
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// exchange canjea el código de autorización por tokens en el token endpoint del
// upstream (grant_type=authorization_code). Autenticación de client por
// client_secret_post (o PKCE puro si no hay secreto). SSRF-safe.
func (b *Broker) exchange(ctx context.Context, p domain.IdentityProvider, code, redirectURI, verifier string) (tokenResponse, error) {
	if err := b.checkURL(ctx, p.TokenEndpoint); err != nil {
		return tokenResponse{}, err
	}
	form := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURI},
		"client_id":    {p.ClientID},
	}
	if p.ClientSecret != "" {
		form.Set("client_secret", p.ClientSecret)
	}
	if verifier != "" {
		form.Set("code_verifier", verifier)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return tokenResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	var tr tokenResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&tr); err != nil {
		return tokenResponse{}, fmt.Errorf("%w: respuesta ilegible", ErrExchange)
	}
	if tr.Error != "" {
		return tokenResponse{}, fmt.Errorf("%w: %s", ErrExchange, tr.Error)
	}
	if tr.IDToken == "" {
		return tokenResponse{}, fmt.Errorf("%w: sin id_token", ErrExchange)
	}
	return tr, nil
}

// keySet devuelve el JWKS del upstream desde caché (o lo refetch-ea si expiró o si
// force). SSRF-safe: jwk.Fetch usa el mismo http.Client con la barrera de dialer.
func (b *Broker) keySet(ctx context.Context, jwksURI string, force bool) (jwk.Set, error) {
	if !force {
		b.mu.Lock()
		c, ok := b.jwks[jwksURI]
		b.mu.Unlock()
		if ok && time.Since(c.fetched) < jwksTTL {
			return c.set, nil
		}
	}
	if err := b.checkURL(ctx, jwksURI); err != nil {
		return nil, err
	}
	set, err := jwk.Fetch(ctx, jwksURI, jwk.WithHTTPClient(b.client))
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	b.jwks[jwksURI] = cachedSet{set: set, fetched: time.Now()}
	b.mu.Unlock()
	return set, nil
}

// verifyIDToken valida el id_token del upstream: firma con el JWKS del provider,
// iss=provider.Issuer, aud=provider.ClientID, exp/iat (reusa oidc.Verify), y el
// nonce contra el esperado (anti-replay). Ante un fallo que pudiera deberse a
// rotación de clave (kid nuevo), refetch-ea el JWKS y reintenta UNA vez.
func (b *Broker) verifyIDToken(ctx context.Context, p domain.IdentityProvider, rawIDToken, nonce string) (domain.FederatedClaims, error) {
	set, err := b.keySet(ctx, p.JWKSURI, false)
	if err != nil {
		return domain.FederatedClaims{}, fmt.Errorf("%w: JWKS", ErrIDToken)
	}
	tok, verr := oidc.Verify(rawIDToken, p.Issuer, p.ClientID, set)
	if verr != nil {
		// Posible rotación de clave: refetch del JWKS y un único reintento.
		if set2, ferr := b.keySet(ctx, p.JWKSURI, true); ferr == nil {
			tok, verr = oidc.Verify(rawIDToken, p.Issuer, p.ClientID, set2)
		}
	}
	if verr != nil {
		return domain.FederatedClaims{}, fmt.Errorf("%w: %w", ErrIDToken, verr)
	}
	// nonce: DEBE coincidir con el emitido en /start (anti-replay del id_token).
	gotNonce, _ := claimString(tok.PrivateClaims(), "nonce")
	if subtle.ConstantTimeCompare([]byte(gotNonce), []byte(nonce)) != 1 {
		return domain.FederatedClaims{}, fmt.Errorf("%w: nonce no coincide", ErrIDToken)
	}
	claims := tok.PrivateClaims()
	fc := domain.FederatedClaims{Subject: tok.Subject()}
	fillClaims(&fc, p, claims)
	return fc, nil
}

// Authenticate ejecuta el canje de código + validación del id_token + extracción
// de atributos mapeados. Si el email no viene en el id_token y hay userinfo, lo
// completa desde userinfo (verificando que el sub coincide). Devuelve los claims
// federados ya validados y mapeados.
func (b *Broker) Authenticate(ctx context.Context, p domain.IdentityProvider, code, redirectURI, verifier, nonce string) (domain.FederatedClaims, error) {
	tr, err := b.exchange(ctx, p, code, redirectURI, verifier)
	if err != nil {
		return domain.FederatedClaims{}, err
	}
	fc, err := b.verifyIDToken(ctx, p, tr.IDToken, nonce)
	if err != nil {
		return domain.FederatedClaims{}, err
	}
	// Fallback a userinfo solo si falta el email y el upstream lo ofrece.
	if fc.Email == "" && p.UserinfoEndpoint != "" && tr.AccessToken != "" {
		var ui map[string]any
		if uerr := b.getJSON(ctx, p.UserinfoEndpoint, tr.AccessToken, &ui); uerr == nil {
			// El sub de userinfo DEBE coincidir con el del id_token (OIDC §5.3.2):
			// evita mezclar identidades de dos sujetos.
			if s, _ := claimString(ui, "sub"); s == fc.Subject {
				fillClaims(&fc, p, ui)
			}
		}
	}
	return fc, nil
}

// fillClaims rellena email/email_verified/name/groups de fc desde un mapa de
// claims (id_token o userinfo), respetando el mapeo de atributos del provider. No
// sobrescribe un valor ya presente (id_token gana sobre userinfo).
func fillClaims(fc *domain.FederatedClaims, p domain.IdentityProvider, claims map[string]any) {
	if fc.Email == "" {
		if v, ok := claimString(claims, p.ClaimName("email")); ok {
			fc.Email = domain.NormalizeEmail(v)
		}
	}
	if !fc.EmailVerified {
		fc.EmailVerified = claimBool(claims, "email_verified")
	}
	if fc.Name == "" {
		if v, ok := claimString(claims, p.ClaimName("name")); ok {
			fc.Name = v
		}
	}
	if len(fc.Groups) == 0 {
		fc.Groups = claimStrings(claims, p.ClaimName("groups"))
	}
}

func claimString(claims map[string]any, key string) (string, bool) {
	v, ok := claims[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// claimBool acepta email_verified como booleano o como la cadena "true" (algunos
// IdP lo serializan como string).
func claimBool(claims map[string]any, key string) bool {
	switch v := claims[key].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	default:
		return false
	}
}

func claimStrings(claims map[string]any, key string) []string {
	v, ok := claims[key]
	if !ok {
		return nil
	}
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		return strings.Fields(t)
	default:
		return nil
	}
}

func containsFold(ss []string, want string) bool {
	for _, s := range ss {
		if strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}
