// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package directory

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Este fichero mintea los tokens con los que la migración escribe EN LA CUENTA DE
// CADA PERSONA, en vez de en la del admin que lanzó el trabajo.
//
// El mecanismo es el canje RFC 8693 de Sytue ID (token exchange con impersonación
// acotada): el client DELEGABLE de la organización presenta su propio token
// (client_credentials) y recibe otro cuyo `sub` es el usuario objetivo y cuyo
// `act.sub` es el client. Cada canje deja su fila en el audit_log de Sytue ID —
// incluidas las RENOVACIONES, que es exactamente lo que se quiere: la traza dice
// cuántas veces y durante cuánto tiempo se actuó en nombre de alguien.
//
// El token dura 5 MINUTOS y no tiene refresh (es deliberado, domain.ImpersonationTokenTTL
// en services/id). Una migración de un buzón grande dura mucho más, así que aquí
// NO se mintea una vez y se reza: Token() renueva sola cuando le queda poco.

const (
	// tokenRefreshMargin es cuánto ANTES de caducar se renueva. Una subida grande
	// puede empezar con el token casi válido y terminar con él caducado: renovar
	// con margen evita el 401 a mitad de fichero (que costaría reintentarlo entero).
	tokenRefreshMargin = 30 * time.Second
	// grantTokenExchange es el grant_type del canje (RFC 8693 §2.1).
	//nolint:gosec // G101: identificador del estándar, no una credencial.
	grantTokenExchange = "urn:ietf:params:oauth:grant-type:token-exchange"
	// tokenTypeAccess es el URN del tipo de token que se entrega y se pide (§3).
	//nolint:gosec // G101: identificador del estándar, no una credencial.
	tokenTypeAccess = "urn:ietf:params:oauth:token-type:access_token"
)

// DelegationConfig parametriza el minteo de tokens para UN usuario objetivo.
type DelegationConfig struct {
	// BaseURL es la URL interna de Sytue ID (p. ej. http://id:8081).
	BaseURL string
	// ClientID/ClientSecret son las credenciales del client DELEGABLE de la
	// organización (mig 0006: migration_delegates, selladas con la KEK). Son de la
	// org, no globales: un client delegable solo puede actuar sobre miembros de SU
	// organización, y Sytue ID lo impone en el WHERE.
	ClientID     string
	ClientSecret string
	// Subject es el usuario objetivo: su correo o su uuid en Sytue.
	Subject string
	// Audience es el client de la suite al que va dirigido el token (`sytue-drive`,
	// `sytue-contacts`): su verificador solo acepta audiencias de esa allowlist.
	Audience string
	// HTTPClient es opcional (por defecto uno con timeout de 15s). Sytue ID es
	// interno: http.Client normal, NO el Runtime.HTTP de egress (su barrera SSRF
	// bloquearía —con razón— un host interno de la suite).
	HTTPClient *http.Client
	// Now inyecta el reloj (los tests comprueban la renovación sin dormir).
	Now func() time.Time
}

// Delegation mintea y RENUEVA el token de un usuario objetivo. Concurrente-seguro:
// el destino sube ficheros en serie, pero nada impide que un futuro worker
// paralelice y dos escrituras pidan el token a la vez.
type Delegation struct {
	cfg DelegationConfig

	mu      sync.Mutex
	token   string
	expires time.Time
}

// NewDelegation construye el minteador de tokens de un usuario objetivo.
func NewDelegation(cfg DelegationConfig) *Delegation {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &Delegation{cfg: cfg}
}

// Token devuelve un Bearer VÁLIDO AHORA para escribir como el usuario objetivo,
// renovándolo si al cacheado le queda menos que el margen. Se llama en CADA
// petición al destino: es la mitad que impide que un trabajo largo muera a los
// cinco minutos.
//
// El error es LEGIBLE porque acaba en el informe del trabajo: dice de quién no se
// pudo obtener permiso, no un "401" a secas.
func (d *Delegation) Token(ctx context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.token != "" && d.cfg.Now().Before(d.expires) {
		return d.token, nil
	}

	// El token propio del client (client_credentials) es el `subject_token` del
	// canje: prueba QUIÉN pide actuar. No se cachea aparte a propósito — se pide
	// una vez cada renovación (≈ cada 4½ min), que es ruido frente a una migración.
	own, _, err := postTokenForm(ctx, d.cfg.HTTPClient, d.cfg.BaseURL, url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {d.cfg.ClientID},
		"client_secret": {d.cfg.ClientSecret},
	})
	if err != nil {
		return "", fmt.Errorf("no se ha podido obtener permiso para escribir en la cuenta de %s: %w", d.cfg.Subject, err)
	}

	form := url.Values{
		"grant_type":         {grantTokenExchange},
		"client_id":          {d.cfg.ClientID},
		"client_secret":      {d.cfg.ClientSecret},
		"subject_token":      {own},
		"subject_token_type": {tokenTypeAccess},
		"requested_subject":  {d.cfg.Subject},
	}
	if d.cfg.Audience != "" {
		form.Set("audience", d.cfg.Audience)
	}
	tok, ttl, err := postTokenForm(ctx, d.cfg.HTTPClient, d.cfg.BaseURL, form)
	if err != nil {
		return "", fmt.Errorf("no se ha podido obtener permiso para escribir en la cuenta de %s: %w", d.cfg.Subject, err)
	}

	// Margen acotado a la mitad del TTL: con un TTL corto, restar 30 s fijos daría
	// un token ya "caducado" al nacer y un canje por petición.
	margin := tokenRefreshMargin
	if margin > ttl/2 {
		margin = ttl / 2
	}
	d.token, d.expires = tok, d.cfg.Now().Add(ttl-margin)
	return tok, nil
}

// postTokenForm llama al token endpoint de Sytue ID con un formulario y devuelve
// el access token y su vida. Fuente ÚNICA de esa llamada en el paquete (la usan el
// client_credentials del directorio y el canje). Un status != 200 se devuelve con
// el cuerpo acotado: el `error_description` de OAuth2 explica QUÉ falló (sujeto no
// impersonable, client no delegable…) y tiene que llegar al informe del trabajo.
func postTokenForm(ctx context.Context, hc *http.Client, base string, form url.Values) (string, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("directory: construir petición de token: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := hc.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("directory: pedir token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		// El cuerpo de un error OAuth2 trae el motivo REAL en error_description; se
		// extrae para que el informe del trabajo diga «el sujeto solicitado no es
		// impersonable por este client» y no un volcado de JSON.
		var oe struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		_ = json.Unmarshal(body, &oe)
		detail := strings.TrimSpace(oe.Description)
		if detail == "" {
			detail = strings.TrimSpace(string(body))
		}
		if oe.Error != "" {
			detail = detail + " (" + oe.Error + ")"
		}
		return "", 0, fmt.Errorf("petición de token rechazada por Sytue ID (HTTP %d): %s", resp.StatusCode, detail)
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&tr); err != nil {
		return "", 0, fmt.Errorf("directory: decodificar token: %w", err)
	}
	if tr.AccessToken == "" {
		return "", 0, fmt.Errorf("directory: token vacío en la respuesta")
	}
	ttl := time.Duration(tr.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return tr.AccessToken, ttl, nil
}
