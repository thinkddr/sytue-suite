// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package scimout es el cliente SCIM 2.0 (RFC 7644) SALIENTE del Sytue Integration
// Gateway y su motor de reconciliación por diff. El cliente aprovisiona usuarios y
// grupos EN el SaaS de terceros (create/replace/patch/deactivate), adopta recursos
// preexistentes por el filtro `userName eq` (en vez de duplicarlos) y desactiva
// —nunca borra— a quien se da de baja en Sytue.
//
// Todo el egress sale por el *http.Client de la pasarela (Runtime.HTTP): valida
// SSRF, rate-limitea por org:instance, audita e INYECTA el Bearer del SaaS desde
// el vault. El cliente NUNCA construye su propio http.Client ni ve el token.
package scimout

import (
	"bytes"
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

// Schemas SCIM 2.0 (RFC 7643/7644).
const (
	schemaUser    = "urn:ietf:params:scim:schemas:core:2.0:User"
	schemaGroup   = "urn:ietf:params:scim:schemas:core:2.0:Group"
	schemaPatchOp = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
	contentType   = "application/scim+json"
)

// ErrConflict indica un 409 del SaaS (p. ej. userName ya existe): el llamante debe
// adoptar el recurso por filtro en vez de crear.
var ErrConflict = errors.New("scimout: conflicto (409)")

// UserInput es el estado deseado de un usuario a empujar al SaaS.
type UserInput struct {
	UserName   string
	ExternalID string
	Formatted  string
	GivenName  string
	FamilyName string
	Email      string
	Active     bool
}

// GroupInput es el estado deseado de un grupo a empujar. MemberIDs son ids
// REMOTOS (del SaaS) de los miembros, ya resueltos por el reconcile.
type GroupInput struct {
	DisplayName string
	ExternalID  string
	MemberIDs   []string
}

// Client habla SCIM 2.0 con un SaaS a través del *http.Client de la pasarela.
type Client struct {
	http *http.Client
	base string
	// sleep espera entre reintentos (inyectable en tests para no dormir de verdad).
	sleep func(time.Duration)
	// maxRetries es el número de reintentos ante 429/5xx transitorio.
	maxRetries int
	// maxBackoff acota lo que se respeta de Retry-After (defensa ante valores absurdos).
	maxBackoff time.Duration
}

// NewClient construye el cliente sobre el http.Client de egress (Runtime.HTTP) y la
// URL base SCIM del SaaS (p. ej. https://saas.example.com/scim/v2).
func NewClient(hc *http.Client, base string) *Client {
	return &Client{
		http:       hc,
		base:       strings.TrimRight(base, "/"),
		sleep:      time.Sleep,
		maxRetries: 3,
		maxBackoff: 30 * time.Second,
	}
}

// --- Usuarios ----------------------------------------------------------------

// FindUser busca un usuario por userName (adopción de preexistentes). Devuelve el
// id remoto y found=false si no existe.
func (c *Client) FindUser(ctx context.Context, userName string) (id string, found bool, err error) {
	return c.findByFilter(ctx, "/Users", `userName eq "`+escapeFilterValue(userName)+`"`)
}

// CreateUser crea el usuario y devuelve su id remoto. ErrConflict si ya existía.
func (c *Client) CreateUser(ctx context.Context, u UserInput) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/Users", userResource(u), &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// ReplaceUser reemplaza (PUT) el recurso completo del usuario.
func (c *Client) ReplaceUser(ctx context.Context, id string, u UserInput) error {
	return c.doJSON(ctx, http.MethodPut, "/Users/"+url.PathEscape(id), userResource(u), nil)
}

// SetUserActive activa/desactiva un usuario por PATCH (RFC 7644 §3.5.2). El
// desaprovisionamiento es SIEMPRE desactivación, nunca borrado.
func (c *Client) SetUserActive(ctx context.Context, id string, active bool) error {
	patch := map[string]any{
		"schemas": []string{schemaPatchOp},
		"Operations": []map[string]any{
			{"op": "replace", "path": "active", "value": active},
		},
	}
	return c.doJSON(ctx, http.MethodPatch, "/Users/"+url.PathEscape(id), patch, nil)
}

// --- Grupos ------------------------------------------------------------------

// FindGroup busca un grupo por displayName (adopción de preexistentes).
func (c *Client) FindGroup(ctx context.Context, displayName string) (id string, found bool, err error) {
	return c.findByFilter(ctx, "/Groups", `displayName eq "`+escapeFilterValue(displayName)+`"`)
}

// CreateGroup crea el grupo (con sus miembros) y devuelve su id remoto.
func (c *Client) CreateGroup(ctx context.Context, g GroupInput) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/Groups", groupResource(g), &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// ReplaceGroup reemplaza (PUT) el grupo completo, incluida su membresía.
func (c *Client) ReplaceGroup(ctx context.Context, id string, g GroupInput) error {
	return c.doJSON(ctx, http.MethodPut, "/Groups/"+url.PathEscape(id), groupResource(g), nil)
}

// DeleteGroup borra un grupo del SaaS (un grupo es estructura, no una identidad:
// al desaparecer en Sytue se elimina en el SaaS). Un 404 se trata como éxito.
func (c *Client) DeleteGroup(ctx context.Context, id string) error {
	err := c.doJSON(ctx, http.MethodDelete, "/Groups/"+url.PathEscape(id), nil, nil)
	if err != nil && errors.Is(err, errNotFound) {
		return nil
	}
	return err
}

// --- Serialización de recursos ----------------------------------------------

func userResource(u UserInput) map[string]any {
	active := u.Active
	res := map[string]any{
		"schemas":  []string{schemaUser},
		"userName": u.UserName,
		"active":   active,
	}
	if u.ExternalID != "" {
		res["externalId"] = u.ExternalID
	}
	if u.Formatted != "" || u.GivenName != "" || u.FamilyName != "" {
		res["name"] = map[string]any{
			"formatted":  u.Formatted,
			"givenName":  u.GivenName,
			"familyName": u.FamilyName,
		}
	}
	if u.Email != "" {
		res["emails"] = []map[string]any{{"value": u.Email, "primary": true}}
	}
	return res
}

func groupResource(g GroupInput) map[string]any {
	members := make([]map[string]any, 0, len(g.MemberIDs))
	for _, m := range g.MemberIDs {
		members = append(members, map[string]any{"value": m})
	}
	res := map[string]any{
		"schemas":     []string{schemaGroup},
		"displayName": g.DisplayName,
		"members":     members,
	}
	if g.ExternalID != "" {
		res["externalId"] = g.ExternalID
	}
	return res
}

// --- Transporte --------------------------------------------------------------

var errNotFound = errors.New("scimout: recurso no encontrado (404)")

// listResponse es la envoltura de un GET con filtro (RFC 7644 §3.4.2).
type listResponse struct {
	Resources []struct {
		ID string `json:"id"`
	} `json:"Resources"`
}

// findByFilter ejecuta GET {path}?filter=... y devuelve el id del primer recurso.
func (c *Client) findByFilter(ctx context.Context, path, filter string) (string, bool, error) {
	q := url.Values{}
	q.Set("filter", filter)
	q.Set("count", "1")
	var lr listResponse
	if err := c.doJSON(ctx, http.MethodGet, path+"?"+q.Encode(), nil, &lr); err != nil {
		return "", false, err
	}
	if len(lr.Resources) == 0 || lr.Resources[0].ID == "" {
		return "", false, nil
	}
	return lr.Resources[0].ID, true, nil
}

// doJSON ejecuta una petición SCIM con reintento ante 429 (respetando Retry-After)
// y 5xx transitorio. Un 409 devuelve ErrConflict; un 404, errNotFound; otro 4xx es
// un error permanente. Si out != nil, decodifica el cuerpo 2xx en out.
func (c *Client) doJSON(ctx context.Context, method, path string, body, out any) error {
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("scimout: serializar cuerpo: %w", err)
		}
		payload = b
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		var rdr io.Reader
		if payload != nil {
			rdr = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
		if err != nil {
			return fmt.Errorf("scimout: construir petición: %w", err)
		}
		if payload != nil {
			req.Header.Set("Content-Type", contentType)
		}
		req.Header.Set("Accept", contentType)

		resp, err := c.http.Do(req)
		if err != nil {
			// Error de transporte: no reintentamos aquí (la pasarela ya audita y el
			// outbox reintentará el trabajo entero); propagar.
			return fmt.Errorf("scimout: %s %s: %w", method, path, err)
		}

		switch {
		case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode >= 500:
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
			lastErr = fmt.Errorf("scimout: %s %s: status %d", method, path, resp.StatusCode)
			drainClose(resp)
			if attempt == c.maxRetries {
				return lastErr
			}
			c.sleep(c.backoff(attempt, retryAfter))
			continue

		case resp.StatusCode == http.StatusConflict:
			drainClose(resp)
			return fmt.Errorf("%w: %s %s", ErrConflict, method, path)

		case resp.StatusCode == http.StatusNotFound:
			drainClose(resp)
			return fmt.Errorf("%w: %s %s", errNotFound, method, path)

		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			if out != nil {
				dec := json.NewDecoder(resp.Body)
				if err := dec.Decode(out); err != nil && !errors.Is(err, io.EOF) {
					drainClose(resp)
					return fmt.Errorf("scimout: decodificar respuesta de %s %s: %w", method, path, err)
				}
			}
			drainClose(resp)
			return nil

		default:
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			drainClose(resp)
			return fmt.Errorf("scimout: %s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(b)))
		}
	}
	return lastErr
}

// backoff elige la espera: respeta Retry-After si viene (acotado), si no, backoff
// exponencial (1s, 2s, 4s…). ponytail: sin jitter; añadir si varias réplicas
// martillean el mismo SaaS y sincronizan sus reintentos.
func (c *Client) backoff(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		if retryAfter > c.maxBackoff {
			return c.maxBackoff
		}
		return retryAfter
	}
	d := time.Second << attempt
	if d > c.maxBackoff {
		return c.maxBackoff
	}
	return d
}

// parseRetryAfter interpreta la cabecera Retry-After como segundos (delta) o como
// fecha HTTP. Devuelve 0 si está ausente o es inválida.
func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// escapeFilterValue escapa las comillas de un valor dentro de un filtro SCIM.
func escapeFilterValue(v string) string {
	return strings.ReplaceAll(v, `"`, `\"`)
}

// drainClose consume y cierra el cuerpo para permitir la reutilización de la conexión.
func drainClose(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
}
