// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package oauth2 mintea y CACHEA los access token con los que la pasarela habla con el
// SaaS del cliente.
//
// Existe por dos motivos. El primero es que la política del producto es que toda
// integración se autentique por OAuth 2.0 siempre que el SaaS lo permita: un token
// estático no caduca, no se acota por scope, no se revoca sin regenerarlo a mano y
// obliga al administrador a ir a fabricarlo al panel del tercero. El segundo es que los
// dos drivers que ya hablaban OAuth —Microsoft Graph (client_credentials) y Google
// Workspace (JWT bearer, RFC 7523)— duplicaban el mismo POST al token endpoint, el mismo
// parseo y el mismo tratamiento de error, y NINGUNO cacheaba: Graph pedía un token nuevo
// en cada consulta, como admitía su propio comentario.
//
// Invariante que no se toca: el POST al token endpoint sale por `Runtime.HTTP`, el ÚNICO
// cliente de egress de la pasarela. Pedir un token es tráfico saliente hacia un tercero
// como cualquier otro, así que pasa por la misma cadena —destino SSRF-safe, rate-limit
// por organización, auditoría de egress y política— en vez de abrir una puerta paralela.
// El `client_secret` viaja en el CUERPO del formulario, que la auditoría nunca persiste.
package oauth2

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

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
)

const (
	// refreshMargin renueva el token un poco antes de que caduque, para que una petición
	// no salga con un token que expira en vuelo.
	refreshMargin = 30 * time.Second
	// defaultTTL se aplica cuando el proveedor no declara expires_in. Conservador a
	// propósito: más vale pedir de más que usar un token muerto.
	defaultTTL = 5 * time.Minute
	// maxTokenRespBytes acota la respuesta del token endpoint (defensa anti-OOM).
	maxTokenRespBytes = 1 << 20
)

// Cache guarda los access token vivos por clave. Una instancia de la pasarela la comparte
// entre todos los drivers y todas las peticiones: el Runtime se fabrica por petición, así
// que si la caché viviera dentro de él no cachearía nada.
//
// ponytail: sin singleflight. Dos peticiones concurrentes de la misma instancia pueden
// pedir dos tokens al proveedor; los emite sin queja y el último gana. Si algún día el
// volumen lo justifica —o aparece un proveedor que invalide el token anterior al emitir
// uno nuevo—, aquí es donde va.
type Cache struct {
	mu     sync.Mutex
	tokens map[string]entry
}

type entry struct {
	value   string
	expires time.Time
}

// NewCache construye una caché vacía, lista para compartir entre drivers.
func NewCache() *Cache {
	return &Cache{tokens: make(map[string]entry)}
}

// Request describe qué token se pide y a quién.
type Request struct {
	// Key identifica el token en la caché. Debe distinguir TODO lo que cambia el token:
	// organización, instancia y, si el proveedor los diferencia, el sujeto delegado y el
	// scope. Dos credenciales distintas jamás pueden compartir clave.
	Key string
	// TokenURL es el token endpoint del proveedor.
	TokenURL string
	// Form son los parámetros del grant (grant_type, client_id, client_secret,
	// assertion, scope…). Viajan en el cuerpo, nunca en la URL.
	Form url.Values
	// Provider nombra al proveedor en los mensajes de error ("graph", "gws"), para que un
	// fallo diga de quién es sin tener que leer trazas.
	Provider string
}

// Token devuelve un access token válido para la petición dada, del caché si sigue vivo o
// del proveedor si no. `rt` es el Runtime de la instancia: el POST sale por su cliente.
func (c *Cache) Token(ctx context.Context, rt connector.Runtime, req Request) (string, error) {
	if req.Key == "" {
		return "", fmt.Errorf("oauth2: falta la clave de caché del token")
	}
	c.mu.Lock()
	if t, ok := c.tokens[req.Key]; ok && time.Now().Before(t.expires) {
		c.mu.Unlock()
		return t.value, nil
	}
	c.mu.Unlock()

	tok, ttl, err := post(ctx, rt, req)
	if err != nil {
		return "", err
	}
	margin := refreshMargin
	if margin > ttl/2 {
		margin = ttl / 2
	}
	c.mu.Lock()
	c.tokens[req.Key] = entry{value: tok, expires: time.Now().Add(ttl - margin)}
	c.mu.Unlock()
	return tok, nil
}

// Forget descarta el token cacheado de una clave. Lo usa quien recibe un 401 del
// proveedor: el token pudo ser revocado antes de caducar, y sin esto la instancia
// seguiría reintentando con el mismo token muerto hasta que expirase.
func (c *Cache) Forget(key string) {
	c.mu.Lock()
	delete(c.tokens, key)
	c.mu.Unlock()
}

// Fetch mintea un token SIN pasar por la caché. Es para los usos de un solo disparo —un
// job de migración pide un token al empezar y no vuelve—, donde cachear no ahorra nada.
// El camino normal de un driver es Cache.Token.
func Fetch(ctx context.Context, rt connector.Runtime, req Request) (string, error) {
	tok, _, err := post(ctx, rt, req)
	return tok, err
}

// tokenResp es la respuesta del token endpoint (RFC 6749 §5.1 y §5.2).
type tokenResp struct {
	AccessToken      string `json:"access_token"`
	ExpiresIn        int64  `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// post ejecuta el canje contra el token endpoint y devuelve el token y su vigencia.
func post(ctx context.Context, rt connector.Runtime, r Request) (string, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.TokenURL, strings.NewReader(r.Form.Encode()))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Sytue-Integration-Gateway/1")

	resp, err := rt.HTTP.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("%s: token: %w", r.Provider, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxTokenRespBytes))
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenRespBytes))
	if err != nil {
		return "", 0, fmt.Errorf("%s: token: leer respuesta: %w", r.Provider, err)
	}

	var tr tokenResp
	// El cuerpo de un error OAuth2 trae el motivo REAL en error_description, así que se
	// parsea también cuando el status no es 2xx: un «401» a secas no dice si el secreto
	// caducó, si el scope no está consentido o si el client no existe.
	_ = json.Unmarshal(body, &tr)
	if tr.AccessToken == "" {
		if tr.Error != "" {
			return "", 0, fmt.Errorf("%s: el token endpoint devolvió %s: %s",
				r.Provider, tr.Error, truncate(tr.ErrorDescription, 200))
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", 0, fmt.Errorf("%s: el token endpoint respondió HTTP %d", r.Provider, resp.StatusCode)
		}
		return "", 0, fmt.Errorf("%s: el token endpoint no devolvió access_token", r.Provider)
	}

	ttl := defaultTTL
	if tr.ExpiresIn > 0 {
		ttl = time.Duration(tr.ExpiresIn) * time.Second
	}
	return tr.AccessToken, ttl, nil
}

// truncate acota un texto de error ajeno antes de propagarlo.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
