// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package directory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
)

// Este fichero implementa manifest.IDApps sobre el mismo cliente del directorio: la
// materialización de una instancia (OLA 3) da de alta la identidad SALIENTE en Sytue ID
// —un SP SAML o un client OIDC— usando la API admin de W2A con el MISMO token
// client_credentials de `sytue-gateway` que ya se cachea por org aquí (sin un segundo
// flujo de token). Sytue ID es interno: se usa el http.Client normal, no el egress SSRF.

// CreateSAMLSP registra un SP SAML en la org (Sytue ID como IdP). idp_initiated=true y
// enabled=true para que el tile del portal lance el SSO IdP-initiated en 1 clic.
func (c *Client) CreateSAMLSP(ctx context.Context, org string, in manifest.SAMLSPInput) (manifest.SAMLSP, error) {
	body := map[string]any{
		"name":          in.Name,
		"entity_id":     in.EntityID,
		"acs_url":       in.ACSURL,
		"idp_initiated": true,
		"enabled":       true,
	}
	if in.NameIDFormat != "" {
		body["name_id_format"] = in.NameIDFormat
	}
	var resp struct {
		ID          string `json:"id"`
		InitiateURL string `json:"initiate_url"`
		MetadataURL string `json:"idp_metadata_url"`
	}
	tok, err := c.token(ctx, org)
	if err != nil {
		return manifest.SAMLSP{}, err
	}
	if err := c.doJSON(ctx, http.MethodPost, c.base+"/api/v1/admin/apps/saml-sps", tok, body, &resp); err != nil {
		return manifest.SAMLSP{}, err
	}
	if resp.ID == "" {
		return manifest.SAMLSP{}, fmt.Errorf("directory: Sytue ID no devolvió id del SP SAML")
	}
	return manifest.SAMLSP{ID: resp.ID, InitiateURL: resp.InitiateURL, MetadataURL: resp.MetadataURL}, nil
}

// DeleteSAMLSP borra un SP SAML de la org (reversión/rollback). Un 404 se trata como
// éxito: el objetivo del borrado —que el SP no exista— ya se cumple (idempotente).
func (c *Client) DeleteSAMLSP(ctx context.Context, org, spID string) error {
	tok, err := c.token(ctx, org)
	if err != nil {
		return err
	}
	return c.doJSON(ctx, http.MethodDelete, c.base+"/api/v1/admin/apps/saml-sps/"+url.PathEscape(spID), tok, nil, nil)
}

// CreateOIDCClient registra un client OIDC para la app por el registro dinámico (RFC
// 7591): (1) emite un token de acceso inicial acotado a la org con el token de
// `sytue-gateway`; (2) se registra en /oauth2/register con ese token. Devuelve
// client_id/secret (el secret se emite una vez y lo custodia el SaaS).
func (c *Client) CreateOIDCClient(ctx context.Context, org string, in manifest.OIDCClientInput) (manifest.OIDCClient, error) {
	tok, err := c.token(ctx, org)
	if err != nil {
		return manifest.OIDCClient{}, err
	}
	var iat struct {
		InitialAccessToken string `json:"initial_access_token"`
	}
	if err := c.doJSON(ctx, http.MethodPost, c.base+"/api/v1/admin/apps/dcr-tokens", tok, map[string]any{}, &iat); err != nil {
		return manifest.OIDCClient{}, err
	}
	if iat.InitialAccessToken == "" {
		return manifest.OIDCClient{}, fmt.Errorf("directory: Sytue ID no devolvió token de registro OIDC")
	}
	// El registro se hace SIEMPRE contra el endpoint interno (c.base), no contra el
	// registration_endpoint público de la respuesta: la pasarela llama a id por su red
	// interna.
	body := map[string]any{
		"redirect_uris":              in.RedirectURIs,
		"client_name":                in.Name,
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"scope":                      "openid profile email offline_access",
		"token_endpoint_auth_method": "client_secret_basic",
	}
	var reg struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := c.doJSON(ctx, http.MethodPost, c.base+"/oauth2/register", iat.InitialAccessToken, body, &reg); err != nil {
		return manifest.OIDCClient{}, err
	}
	if reg.ClientID == "" {
		return manifest.OIDCClient{}, fmt.Errorf("directory: el registro OIDC no devolvió client_id")
	}
	return manifest.OIDCClient{ClientID: reg.ClientID, ClientSecret: reg.ClientSecret}, nil
}

// doJSON ejecuta una petición autenticada (Bearer explícito) con cuerpo JSON opcional y
// decodifica la respuesta en dst (si no es nil). Acepta 2xx; un 404 en DELETE es éxito
// (borrado idempotente). Cualquier otro status es error con el cuerpo recortado.
func (c *Client) doJSON(ctx context.Context, method, endpoint, bearer string, body, dst any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("directory: serializar %s %s: %w", method, endpoint, err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return fmt.Errorf("directory: construir %s %s: %w", method, endpoint, err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("directory: %s %s: %w", method, endpoint, err)
	}
	defer resp.Body.Close()
	if method == http.MethodDelete && resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("directory: %s %s: status %d: %s", method, endpoint, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if dst == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("directory: decodificar %s %s: %w", method, endpoint, err)
	}
	return nil
}
