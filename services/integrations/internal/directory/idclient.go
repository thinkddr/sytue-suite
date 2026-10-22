// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package directory es el cliente de LECTURA del directorio de Sytue ID: usuarios,
// grupos y membresías de una organización, tomados de los endpoints admin
// EXISTENTES de services/id (GET /api/v1/admin/users, /admin/groups, /admin/groups/{id}).
// Es el "estado deseado" que el reconcile-por-diff empuja al SaaS.
//
// Autenticación: client_credentials (RFC 6749 §4.4) como el client `sytue-gateway`
// contra POST /oauth2/token; el access token se cachea por org hasta su caducidad.
// Sytue ID es un servicio INTERNO de la suite, así que este cliente usa un
// *http.Client normal — NO el Runtime.HTTP de la pasarela (ese es SOLO para el
// egress al SaaS de terceros, y su barrera SSRF bloquearía un host interno).
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

// User es un usuario del directorio de la org. Active=false ⇒ el reconcile lo
// desactiva en el SaaS (nunca lo borra).
type User struct {
	ID     string
	Email  string
	Name   string
	Active bool
}

// Group es un grupo del directorio con los ids de sus miembros (usuarios de Sytue).
type Group struct {
	ID          string
	Name        string
	Description string
	Members     []string // ids de usuario de Sytue
}

// Config parametriza el cliente del directorio.
type Config struct {
	// BaseURL es la URL interna de Sytue ID (p. ej. http://id:8081).
	BaseURL string
	// ClientID/ClientSecret son las credenciales del client `sytue-gateway`
	// (grant client_credentials). Las siembra la ola W2A.
	ClientID     string
	ClientSecret string
	// HTTPClient es opcional (por defecto uno con timeout de 15s).
	HTTPClient *http.Client
}

// Client lee el directorio de Sytue ID con tokens client_credentials cacheados
// por org. Concurrente-seguro.
type Client struct {
	base   string
	id     string
	secret string
	http   *http.Client

	mu     sync.Mutex
	tokens map[string]cachedToken // por org
}

type cachedToken struct {
	value   string
	expires time.Time
}

// New construye el cliente. base y las credenciales se toman de la config.
func New(cfg Config) *Client {
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{
		base:   strings.TrimRight(cfg.BaseURL, "/"),
		id:     cfg.ClientID,
		secret: cfg.ClientSecret,
		http:   hc,
		tokens: make(map[string]cachedToken),
	}
}

// ListUsers devuelve TODOS los usuarios de la org, siguiendo la paginación keyset
// (cursor/nextCursor) del endpoint admin. Sin OFFSET: estable a cientos de miles.
func (c *Client) ListUsers(ctx context.Context, org string) ([]User, error) {
	const pageSize = 200 // el máximo que admite el endpoint (1..200).
	var out []User
	cursor := ""
	for {
		var resp struct {
			Items []struct {
				ID     string `json:"id"`
				Email  string `json:"email"`
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"items"`
			//nolint:tagliatelle // es la forma que EMITE el API admin de Sytue ID (camelCase); aquí solo se consume.
			NextCursor string `json:"nextCursor"`
		}
		q := url.Values{}
		q.Set("limit", fmt.Sprint(pageSize))
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		if err := c.getJSON(ctx, org, "/api/v1/admin/users?"+q.Encode(), &resp); err != nil {
			return nil, err
		}
		for _, it := range resp.Items {
			out = append(out, User{
				ID:     it.ID,
				Email:  it.Email,
				Name:   it.Name,
				Active: it.Status == "active",
			})
		}
		if resp.NextCursor == "" {
			return out, nil
		}
		cursor = resp.NextCursor
	}
}

// ListGroups devuelve los grupos de la org con sus miembros. El listado de grupos
// no trae miembros, así que se resuelven con una llamada por grupo.
//
// ponytail: una llamada de miembros por grupo (los grupos por org son pocos). Si
// alguna org tuviera miles de grupos, añadir a services/id un listado con miembros
// embebidos o un endpoint de membresías en bloque.
func (c *Client) ListGroups(ctx context.Context, org string) ([]Group, error) {
	var list struct {
		Groups []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"groups"`
	}
	if err := c.getJSON(ctx, org, "/api/v1/admin/groups", &list); err != nil {
		return nil, err
	}
	out := make([]Group, 0, len(list.Groups))
	for _, g := range list.Groups {
		var detail struct {
			Members []struct {
				UserID string `json:"user_id"`
			} `json:"members"`
		}
		if err := c.getJSON(ctx, org, "/api/v1/admin/groups/"+url.PathEscape(g.ID), &detail); err != nil {
			return nil, err
		}
		members := make([]string, 0, len(detail.Members))
		for _, m := range detail.Members {
			members = append(members, m.UserID)
		}
		out = append(out, Group{ID: g.ID, Name: g.Name, Description: g.Description, Members: members})
	}
	return out, nil
}

// getJSON hace un GET autenticado (Bearer client_credentials de la org) y decodifica
// la respuesta JSON. Un status != 2xx es un error.
func (c *Client) getJSON(ctx context.Context, org, path string, dst any) error {
	tok, err := c.token(ctx, org)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, http.NoBody)
	if err != nil {
		return fmt.Errorf("directory: construir petición: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("directory: GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("directory: GET %s: status %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("directory: decodificar %s: %w", path, err)
	}
	return nil
}

// token devuelve un access token válido para la org, del caché o pidiéndolo por
// client_credentials. Se cachea hasta su caducidad menos un margen de seguridad.
//
// ponytail: el org va como parámetro `organization` del token (así el token queda
// acotado a esa org y los endpoints admin lo derivan de su claim tenant). El
// cableado de esa acotación es contrato de W2A (siembra de `sytue-gateway`).
func (c *Client) token(ctx context.Context, org string) (string, error) {
	c.mu.Lock()
	if t, ok := c.tokens[org]; ok && time.Now().Before(t.expires) {
		c.mu.Unlock()
		return t.value, nil
	}
	c.mu.Unlock()

	tok, ttl, err := postTokenForm(ctx, c.http, c.base, url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.id},
		"client_secret": {c.secret},
		"organization":  {org},
	})
	if err != nil {
		return "", err
	}
	// Margen de seguridad: renovar 30s antes de expirar (o antes si el TTL es corto).
	margin := tokenRefreshMargin
	if margin > ttl/2 {
		margin = ttl / 2
	}
	c.mu.Lock()
	c.tokens[org] = cachedToken{value: tok, expires: time.Now().Add(ttl - margin)}
	c.mu.Unlock()
	return tok, nil
}
