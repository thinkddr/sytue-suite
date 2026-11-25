// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package buildcache

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Scaleway es la API REST de Scaleway que usa la caché (Account v3, Registry v1 e IAM
// v1alpha1), escrita a mano con net/http como el cliente IAM del controller: la auth es la
// cabecera X-Auth-Token con la secret key de plataforma, que tiene derecho IAM.
type Scaleway struct {
	base   string // https://api.scaleway.com
	region string // fr-par
	token  string
	orgID  string
	hc     *http.Client
}

// NewScaleway construye el cliente. base es la raíz de la API (`https://api.scaleway.com`).
func NewScaleway(base, region, token, orgID string) *Scaleway {
	return &Scaleway{
		base: strings.TrimRight(base, "/"), region: region, token: token, orgID: orgID,
		hc: &http.Client{Timeout: 15 * time.Second},
	}
}

var _ API = (*Scaleway)(nil)

// do ejecuta una petición JSON. body/out nil se omiten.
func (c *Scaleway) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("buildcache: marshal: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return fmt.Errorf("buildcache: petición: %w", err)
	}
	req.Header.Set("X-Auth-Token", c.token) // nunca en logs
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("buildcache: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("buildcache: %s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("buildcache: decode: %w", err)
		}
	}
	return nil
}

type conNombre struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Endpoint string `json:"endpoint,omitempty"`
}

// porNombre es el de nombre EXACTO de una lista (los filtros `name` de Scaleway son de
// coincidencia parcial).
func porNombre(l []conNombre, name string) (conNombre, bool) {
	for _, x := range l {
		if x.Name == name {
			return x, true
		}
	}
	return conNombre{}, false
}

// EnsureProject implementa API.
func (c *Scaleway) EnsureProject(ctx context.Context, name string) (string, error) {
	var list struct {
		Projects []conNombre `json:"projects"`
	}
	q := url.Values{"organization_id": {c.orgID}, "name": {name}, "page_size": {"100"}}
	if err := c.do(ctx, http.MethodGet, "/account/v3/projects", q, nil, &list); err != nil {
		return "", err
	}
	if p, ok := porNombre(list.Projects, name); ok {
		return p.ID, nil
	}
	var p conNombre
	body := map[string]string{"name": name, "organization_id": c.orgID, "description": "Sytue Cloud: caché de builds de un tenant"}
	if err := c.do(ctx, http.MethodPost, "/account/v3/projects", nil, body, &p); err != nil {
		return "", err
	}
	if p.ID == "" {
		return "", fmt.Errorf("buildcache: proyecto creado sin id")
	}
	return p.ID, nil
}

// EnsureRegistryNamespace implementa API. El espacio de nombres es PRIVADO.
func (c *Scaleway) EnsureRegistryNamespace(ctx context.Context, projectID, name string) (string, error) {
	ruta := "/registry/v1/regions/" + url.PathEscape(c.region) + "/namespaces"
	var list struct {
		Namespaces []conNombre `json:"namespaces"`
	}
	q := url.Values{"project_id": {projectID}, "name": {name}, "page_size": {"100"}}
	if err := c.do(ctx, http.MethodGet, ruta, q, nil, &list); err != nil {
		return "", err
	}
	if ns, ok := porNombre(list.Namespaces, name); ok {
		return ns.Endpoint, nil
	}
	var ns conNombre
	body := map[string]any{"name": name, "project_id": projectID, "is_public": false, "description": "Sytue Cloud: caché de builds"}
	if err := c.do(ctx, http.MethodPost, ruta, nil, body, &ns); err != nil {
		return "", err
	}
	if ns.Endpoint == "" {
		return "", fmt.Errorf("buildcache: espacio de nombres creado sin endpoint")
	}
	return ns.Endpoint, nil
}

// EnsureApplication implementa API.
func (c *Scaleway) EnsureApplication(ctx context.Context, name string) (string, error) {
	var list struct {
		Applications []conNombre `json:"applications"`
	}
	q := url.Values{"organization_id": {c.orgID}, "name": {name}, "page_size": {"100"}}
	if err := c.do(ctx, http.MethodGet, "/iam/v1alpha1/applications", q, nil, &list); err != nil {
		return "", err
	}
	if a, ok := porNombre(list.Applications, name); ok {
		return a.ID, nil
	}
	var a conNombre
	body := map[string]string{"name": name, "organization_id": c.orgID, "description": "Sytue Cloud: credencial de la caché de builds de un tenant"}
	if err := c.do(ctx, http.MethodPost, "/iam/v1alpha1/applications", nil, body, &a); err != nil {
		return "", err
	}
	if a.ID == "" {
		return "", fmt.Errorf("buildcache: application creada sin id")
	}
	return a.ID, nil
}

// EnsureProjectPolicy implementa API: una sola regla, con `project_ids` y nunca con
// `organization_id`, que es lo que acota la key al proyecto de caché.
func (c *Scaleway) EnsureProjectPolicy(ctx context.Context, name, appID, projectID, permissionSet string) error {
	var list struct {
		Policies []conNombre `json:"policies"`
	}
	q := url.Values{"organization_id": {c.orgID}, "application_ids": {appID}, "page_size": {"100"}}
	if err := c.do(ctx, http.MethodGet, "/iam/v1alpha1/policies", q, nil, &list); err != nil {
		return err
	}
	if _, ok := porNombre(list.Policies, name); ok {
		return nil
	}
	body := map[string]any{
		"name": name, "organization_id": c.orgID, "application_id": appID,
		"description": "Sytue Cloud: solo el registry del proyecto de caché del tenant",
		"rules":       []map[string]any{{"permission_set_names": []string{permissionSet}, "project_ids": []string{projectID}}},
	}
	return c.do(ctx, http.MethodPost, "/iam/v1alpha1/policies", nil, body, nil)
}

// RotateAPIKey implementa API: borra las keys de la application y acuña una.
func (c *Scaleway) RotateAPIKey(ctx context.Context, appID string) (string, string, error) {
	var list struct {
		APIKeys []struct {
			AccessKey string `json:"access_key"`
		} `json:"api_keys"`
	}
	if err := c.do(ctx, http.MethodGet, "/iam/v1alpha1/api-keys", url.Values{"application_id": {appID}, "page_size": {"100"}}, nil, &list); err != nil {
		return "", "", err
	}
	for _, k := range list.APIKeys {
		if k.AccessKey == "" {
			continue
		}
		if err := c.do(ctx, http.MethodDelete, "/iam/v1alpha1/api-keys/"+url.PathEscape(k.AccessKey), nil, nil, nil); err != nil {
			return "", "", err
		}
	}
	var key struct {
		AccessKey string `json:"access_key"`
		SecretKey string `json:"secret_key"`
	}
	body := map[string]string{"application_id": appID, "description": "Sytue Cloud: caché de builds"}
	if err := c.do(ctx, http.MethodPost, "/iam/v1alpha1/api-keys", nil, body, &key); err != nil {
		return "", "", err
	}
	if key.AccessKey == "" || key.SecretKey == "" {
		return "", "", fmt.Errorf("buildcache: api-key sin access/secret")
	}
	return key.AccessKey, key.SecretKey, nil
}
