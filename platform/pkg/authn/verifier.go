// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package authn

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// defaultAllowedAlgs es la allowlist de algoritmos de firma aceptados por
// defecto: asimétricos robustos. Se excluye deliberadamente "none" y los HMAC
// simétricos (HS*) para evitar confusión de algoritmo con claves públicas.
var defaultAllowedAlgs = []jwa.SignatureAlgorithm{
	jwa.RS256, jwa.RS384, jwa.RS512,
	jwa.ES256, jwa.ES384, jwa.ES512,
	jwa.PS256, jwa.PS384, jwa.PS512,
	jwa.EdDSA,
}

// VerifierConfig configura el verificador de tokens de Sytue ID.
type VerifierConfig struct {
	// Issuer es el issuer OIDC esperado (p. ej. https://id.sytue.eu).
	Issuer string
	// JWKSURL es el endpoint JWKS de Sytue ID. Si está vacío, se deriva del
	// issuer como <issuer>/.well-known/jwks.json (ajustable por servicio).
	JWKSURL string
	// Audience es la audiencia esperada (el client_id del servicio/BFF). Si
	// está vacía, no se valida la audiencia.
	Audience string
	// Audiences es una allowlist de audiencias aceptables: el token es válido si
	// SU audiencia contiene AL MENOS UNA. Úsala en servicios de suite (p. ej. Hub)
	// a los que la Sytue Shell llama con el token del app actual (sytue-one,
	// sytue-mail, …). Si está definida, tiene prioridad sobre Audience.
	Audiences []string
	// ClockSkew tolerado en la validación temporal (por defecto 30s).
	ClockSkew time.Duration
	// RefreshInterval mínimo entre refrescos del JWKS (por defecto 15m).
	RefreshInterval time.Duration
	// InitialRefreshTimeout acota cuánto puede esperar el arranque a que Sytue
	// ID publique el primer JWKS (por defecto 30s). Evita CrashLoop durante un
	// rollout simultáneo sin ocultar una indisponibilidad sostenida del IdP.
	InitialRefreshTimeout time.Duration
	// InitialRetryInterval es la pausa inicial entre intentos (por defecto
	// 250ms); crece exponencialmente hasta 2s.
	InitialRetryInterval time.Duration
	// AllowedAlgs es la allowlist de algoritmos de firma aceptados. Si está
	// vacía se usa defaultAllowedAlgs (asimétricos; nunca "none" ni HS*).
	AllowedAlgs []jwa.SignatureAlgorithm
	// RequireKID exige que el JWS traiga un "kid" en la cabecera (por defecto true),
	// lo que fuerza la selección explícita de clave del JWKS (evita ambigüedad).
	RequireKID *bool
}

// Verifier valida JWT de Sytue ID contra su JWKS, con caché y rotación de claves.
type Verifier struct {
	cfg         VerifierConfig
	jwksURL     string
	cache       *jwk.Cache
	clockSkew   time.Duration
	allowedAlgs map[jwa.SignatureAlgorithm]struct{}
	requireKID  bool
}

// NewVerifier crea un Verifier y arranca el cacheo/refresco del JWKS. El
// contexto controla el ciclo de vida del refresco en segundo plano.
func NewVerifier(ctx context.Context, cfg VerifierConfig) (*Verifier, error) {
	if cfg.Issuer == "" {
		return nil, fmt.Errorf("authn: issuer obligatorio")
	}
	jwksURL := cfg.JWKSURL
	if jwksURL == "" {
		jwksURL = cfg.Issuer + "/.well-known/jwks.json"
	}
	if cfg.ClockSkew <= 0 {
		cfg.ClockSkew = 30 * time.Second
	}
	if cfg.RefreshInterval <= 0 {
		cfg.RefreshInterval = 15 * time.Minute
	}
	if cfg.InitialRefreshTimeout <= 0 {
		cfg.InitialRefreshTimeout = 30 * time.Second
	}
	if cfg.InitialRetryInterval <= 0 {
		cfg.InitialRetryInterval = 250 * time.Millisecond
	}

	algs := cfg.AllowedAlgs
	if len(algs) == 0 {
		algs = defaultAllowedAlgs
	}
	allowed := make(map[jwa.SignatureAlgorithm]struct{}, len(algs))
	for _, a := range algs {
		if a == jwa.NoSignature {
			return nil, fmt.Errorf("authn: el algoritmo \"none\" no está permitido")
		}
		allowed[a] = struct{}{}
	}
	requireKID := true
	if cfg.RequireKID != nil {
		requireKID = *cfg.RequireKID
	}

	cache := jwk.NewCache(ctx)
	if err := cache.Register(jwksURL, jwk.WithMinRefreshInterval(cfg.RefreshInterval)); err != nil {
		return nil, fmt.Errorf("authn: registrar JWKS: %w", err)
	}
	// Los Deployments se crean en paralelo y Sytue ID puede necesitar unos
	// segundos para migrar/publicar sus claves. Esperar aquí conserva el fail
	// closed (ningún servicio queda Ready sin JWKS) sin generar CrashLoopBackOff.
	if err := refreshInitialJWKS(ctx, cache, jwksURL, cfg.InitialRefreshTimeout, cfg.InitialRetryInterval); err != nil {
		return nil, fmt.Errorf("authn: carga inicial de JWKS desde %s: %w", jwksURL, err)
	}

	return &Verifier{
		cfg:         cfg,
		jwksURL:     jwksURL,
		cache:       cache,
		clockSkew:   cfg.ClockSkew,
		allowedAlgs: allowed,
		requireKID:  requireKID,
	}, nil
}

func refreshInitialJWKS(
	ctx context.Context,
	cache *jwk.Cache,
	url string,
	timeout time.Duration,
	retryInterval time.Duration,
) error {
	refreshCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	delay := retryInterval
	const maxDelay = 2 * time.Second
	var lastErr error
	for {
		if _, err := cache.Refresh(refreshCtx, url); err == nil {
			return nil
		} else {
			lastErr = err
		}

		timer := time.NewTimer(delay)
		select {
		case <-refreshCtx.Done():
			timer.Stop()
			return fmt.Errorf("agotado el tiempo de espera: %w", lastErr)
		case <-timer.C:
		}
		if delay < maxDelay {
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}
}

// Verify valida un token serializado y devuelve los claims extraídos.
//
// Defensa en profundidad frente a confusión de algoritmo (alg confusion) y
// tokens sin firmar:
//  1. Se inspecciona la cabecera JWS y se exige que "alg" esté en la allowlist
//     (asimétricos; nunca "none" ni HS*) y, si RequireKID, que traiga "kid".
//  2. jwt.Parse valida firma (contra el JWKS), issuer, audiencia y ventana
//     temporal (exp/nbf/iat) con la tolerancia de reloj configurada.
func (v *Verifier) Verify(ctx context.Context, raw string) (Claims, error) {
	return v.verify(ctx, raw)
}

// verify es Verify admitiendo, además de las audiencias configuradas, las `extra`.
// Solo la usa RequireGoogleAPIs: fuera de las rutas que la piden, un token con otra
// audiencia sigue siendo inválido.
func (v *Verifier) verify(ctx context.Context, raw string, extra ...string) (Claims, error) {
	if err := v.checkHeader(raw); err != nil {
		return Claims{}, err
	}

	set, err := v.cache.Get(ctx, v.jwksURL)
	if err != nil {
		return Claims{}, fmt.Errorf("authn: obtener JWKS: %w", err)
	}

	opts := []jwt.ParseOption{
		// El KeySet selecciona la clave por "kid"; el algoritmo ya se validó en
		// checkHeader contra la allowlist (defensa frente a alg confusion).
		jwt.WithKeySet(set),
		jwt.WithValidate(true),
		jwt.WithIssuer(v.cfg.Issuer),
		jwt.WithAcceptableSkew(v.clockSkew),
		// Exigir presencia de exp para que ningún token sea eterno.
		jwt.WithRequiredClaim(jwt.ExpirationKey),
	}
	// Con allowlist (o con audiencias extra) se valida manualmente tras el parseo
	// (intersección); con audiencia única y sin extras se delega en la librería.
	if len(v.cfg.Audiences) == 0 && v.cfg.Audience != "" && len(extra) == 0 {
		opts = append(opts, jwt.WithAudience(v.cfg.Audience))
	}

	tok, err := jwt.ParseString(raw, opts...)
	if err != nil {
		return Claims{}, fmt.Errorf("authn: token inválido: %w", err)
	}
	if permitidas := v.audiences(); len(permitidas) > 0 && len(extra)+len(v.cfg.Audiences) > 0 &&
		!anyAudienceMatches(tok.Audience(), append(slices.Clone(permitidas), extra...)) {
		return Claims{}, fmt.Errorf("authn: audiencia %v no está en la allowlist", tok.Audience())
	}
	return claimsFromToken(tok), nil
}

// audiences son las audiencias propias configuradas: la allowlist o la única.
// Vacío = el verificador no valida audiencia (desarrollo).
func (v *Verifier) audiences() []string {
	if len(v.cfg.Audiences) > 0 {
		return v.cfg.Audiences
	}
	if v.cfg.Audience != "" {
		return []string{v.cfg.Audience}
	}
	return nil
}

// anyAudienceMatches devuelve true si alguna audiencia del token está en la allowlist.
func anyAudienceMatches(tokenAud, allowed []string) bool {
	for _, a := range tokenAud {
		for _, w := range allowed {
			if a == w {
				return true
			}
		}
	}
	return false
}

// checkHeader parsea la parte JWS (sin verificar firma) para validar el
// algoritmo y el kid ANTES de la verificación criptográfica. Rechaza tokens
// multi-firma y algoritmos fuera de la allowlist.
func (v *Verifier) checkHeader(raw string) error {
	msg, err := jws.Parse([]byte(raw))
	if err != nil {
		return fmt.Errorf("authn: JWS ilegible: %w", err)
	}
	sigs := msg.Signatures()
	if len(sigs) != 1 {
		return fmt.Errorf("authn: se esperaba exactamente una firma, hay %d", len(sigs))
	}
	hdr := sigs[0].ProtectedHeaders()
	alg := hdr.Algorithm()
	if _, ok := v.allowedAlgs[alg]; !ok {
		return fmt.Errorf("authn: algoritmo de firma no permitido: %q", alg)
	}
	if v.requireKID && hdr.KeyID() == "" {
		return fmt.Errorf("authn: falta el identificador de clave (kid)")
	}
	return nil
}

// claimsFromToken extrae los claims de Sytue ID de un token validado.
func claimsFromToken(tok jwt.Token) Claims {
	c := Claims{
		Subject:  tok.Subject(),
		Issuer:   tok.Issuer(),
		Audience: tok.Audience(),
		Emitido:  tok.IssuedAt(),
		Expira:   tok.Expiration(),
	}
	if v, ok := tok.Get("email"); ok {
		if s, ok := v.(string); ok {
			c.Email = s
		}
	}
	if v, ok := tok.Get("name"); ok {
		if s, ok := v.(string); ok {
			c.Name = s
		}
	}
	if v, ok := tok.Get("mailbox"); ok {
		if s, ok := v.(string); ok {
			c.Mailbox = s
		}
	}
	// Tenant: claim "tenant" preferente, "org" como alternativa.
	if v, ok := tok.Get("tenant"); ok {
		if s, ok := v.(string); ok {
			c.Tenant = s
		}
	}
	if c.Tenant == "" {
		if v, ok := tok.Get("org"); ok {
			if s, ok := v.(string); ok {
				c.Tenant = s
			}
		}
	}
	if v, ok := tok.Get("mail_domains"); ok {
		c.MailDomains = toStringSlice(v)
	}
	if v, ok := tok.Get("mailboxes"); ok {
		c.Mailboxes = toStringSlice(v)
	}
	// `act` (RFC 8693): el token lo usa OTRO en nombre del sujeto. Ante una forma que
	// no se sepa leer, delegado igual: lo que se decide con esto es qué NO se le abre.
	if v, ok := tok.Get("act"); ok {
		c.Delegado = true
		if m, ok := v.(map[string]any); ok {
			c.Actor, _ = m["sub"].(string)
		}
	}
	planClaimsFromToken(tok, &c)
	if v, ok := tok.Get("roles"); ok {
		c.Roles = toStringSlice(v)
	}
	if v, ok := tok.Get("groups"); ok {
		c.Groups = toStringSlice(v)
	}
	if v, ok := tok.Get("scope"); ok {
		c.Scopes = toStringSlice(v)
	} else if v, ok := tok.Get("scp"); ok {
		c.Scopes = toStringSlice(v)
	}
	return c
}

// planClaimsFromToken lee la clase de la organización, su plan y las cuotas que
// concede. Aparte de claimsFromToken porque son cuatro claims que llegan juntos
// y un token sin ellos —anterior a la distinción— es perfectamente válido.
func planClaimsFromToken(tok jwt.Token, c *Claims) {
	if v, ok := tok.Get("org_kind"); ok {
		if s, ok := v.(string); ok {
			c.OrgKind = s
		}
	}
	if v, ok := tok.Get("plan"); ok {
		if s, ok := v.(string); ok {
			c.Plan = s
		}
	}
	if v, ok := tok.Get("storage_bytes"); ok {
		c.StorageBytes = toInt64(v)
	}
	if v, ok := tok.Get("mailbox_bytes"); ok {
		c.MailboxBytes = toInt64(v)
	}
}
