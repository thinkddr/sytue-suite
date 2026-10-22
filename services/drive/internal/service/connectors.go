// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// Google Drive y OneDrive son almacenes conectados por CONSENTIMIENTO: el token
// OAuth vive en el servicio de integraciones (que ya sabe pedir consentimiento,
// guardar el refresh token sellado y revocar), y Drive lo pide por el endpoint
// interno cuando lo necesita. Aquí no hay credenciales de proveedor: solo el
// secreto compartido entre los dos servicios.

// ErrConnectorsOff se devuelve cuando Drive no tiene el secreto interno del
// servicio de integraciones: las clases por consentimiento no están.
var ErrConnectorsOff = errors.New("service: el almacenamiento conectado por consentimiento no está configurado")

// ExternalInstance es una instancia de Google Drive u OneDrive del tenant.
type ExternalInstance struct {
	ID             string `json:"id"`
	TemplateKey    string `json:"template_key"`
	Name           string `json:"name"`
	Connected      bool   `json:"connected"`
	OAuthAvailable bool   `json:"oauth_available"`
}

// ExternalAccounts carries actionable provider readiness without credentials.
type ExternalAccounts struct {
	Items     []ExternalInstance `json:"items"`
	Providers map[string]bool    `json:"providers"`
}

type integrationsClient struct {
	base, secret string
	http         *http.Client

	mu    sync.Mutex
	cache map[string]tokenCacheado // instancia → token
}

type tokenCacheado struct {
	token   string
	expires time.Time
}

func newIntegrationsClient(base, secret string) *integrationsClient {
	if strings.TrimSpace(base) == "" || secret == "" {
		return nil
	}
	return &integrationsClient{
		base: strings.TrimRight(base, "/"), secret: secret,
		http:  &http.Client{Timeout: 15 * time.Second},
		cache: map[string]tokenCacheado{},
	}
}

func (c *integrationsClient) get(ctx context.Context, ruta string, q url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+ruta+"?"+q.Encode(), http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("X-Sytue-Internal", c.secret)
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("integraciones: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if res.StatusCode != http.StatusOK {
		var e struct {
			Error struct{ Message string } `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		if e.Error.Message == "" {
			e.Error.Message = res.Status
		}
		return fmt.Errorf("integraciones: %s", e.Error.Message)
	}
	return json.Unmarshal(body, out)
}

// Instances lista las instancias de almacenamiento del tenant.
func (c *integrationsClient) Instances(ctx context.Context, t tenancy.Tenant) (ExternalAccounts, error) {
	var out ExternalAccounts
	if err := c.get(ctx, "/internal/v1/storage/instances", url.Values{"tenant": {t.Slug}}, &out); err != nil {
		return out, err
	}
	return out, nil
}

// Token devuelve un access token vigente de la instancia, de la caché mientras
// le quede más de un minuto.
func (c *integrationsClient) Token(ctx context.Context, t tenancy.Tenant, instance string) (string, error) {
	c.mu.Lock()
	e, ok := c.cache[instance]
	c.mu.Unlock()
	if ok && time.Until(e.expires) > time.Minute {
		return e.token, nil
	}
	var out struct {
		AccessToken string    `json:"access_token"`
		ExpiresAt   time.Time `json:"expires_at"`
	}
	if err := c.get(ctx, "/internal/v1/storage/token", url.Values{"tenant": {t.Slug}, "instance": {instance}}, &out); err != nil {
		return "", err
	}
	exp := out.ExpiresAt
	if exp.IsZero() {
		exp = time.Now().Add(30 * time.Minute)
	}
	c.mu.Lock()
	c.cache[instance] = tokenCacheado{token: out.AccessToken, expires: exp}
	c.mu.Unlock()
	return out.AccessToken, nil
}
