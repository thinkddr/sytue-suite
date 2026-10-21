// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package client is a thin HTTP client for the Sytue Cloud control-plane API
// (services/cloud/openapi.yaml). It carries the Bearer token, timeouts and
// context, decodes typed entities and surfaces a detectable ErrNotFound (404)
// so Terraform Read can reconcile drift by removing deleted resources.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrNotFound is returned when the API responds 404, letting resource Read
// remove the resource from state instead of erroring.
var ErrNotFound = errors.New("sytuecloud: not found")

// Client talks to the Sytue Cloud REST API under <BaseURL>/api/v1.
type Client struct {
	BaseURL string // e.g. https://cloud-api.sytue.eu (no trailing slash, no /api/v1)
	Token   string
	HTTP    *http.Client
}

// New builds a Client with sane timeouts. TLS uses the platform defaults (no
// InsecureSkipVerify): the invariants forbid disabling verification.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// APIError is a typed HTTP error carrying the status and decoded body.
type APIError struct {
	Status int
	Code   string
	Msg    string
}

func (e *APIError) Error() string {
	if e.Code != "" || e.Msg != "" {
		return fmt.Sprintf("sytuecloud: HTTP %d: %s %s", e.Status, e.Code, e.Msg)
	}
	return fmt.Sprintf("sytuecloud: HTTP %d", e.Status)
}

// do issues a request with the Bearer token and JSON body/out. A 404 maps to
// ErrNotFound; any other status >= 400 becomes an *APIError. out may be nil.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	if c.Token == "" {
		return errors.New("sytuecloud: missing token")
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+"/api/v1"+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode >= 400 {
		ae := &APIError{Status: resp.StatusCode}
		var wrap struct {
			Error struct {
				Code, Message string
			} `json:"error"`
		}
		if json.Unmarshal(data, &wrap) == nil {
			ae.Code, ae.Msg = wrap.Error.Code, wrap.Error.Message
		}
		// ponytail: si el cuerpo no encaja en el sobre {error:{code,message}} NO se
		// vuelca crudo — un 4xx de PUT /env podría reflejar el valor rechazado (un
		// secreto) y los diagnostics de Terraform NO respetan Sensitive. Error() ya
		// renderiza "HTTP <status>" con Msg vacío; el detalle va a tflog (opt-in).
		return ae
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// --- Entities (subset of openapi.yaml used by the provider) ---

type Project struct {
	ID             string `json:"id,omitempty"`
	Slug           string `json:"slug,omitempty"`
	Name           string `json:"name,omitempty"`
	RepoURL        string `json:"repo_url,omitempty"`
	GitProvider    string `json:"git_provider,omitempty"`
	ProductionRef  string `json:"production_ref,omitempty"`
	Framework      string `json:"framework,omitempty"`
	BuildStrategy  string `json:"build_strategy,omitempty"`
	RootDir        string `json:"root_dir,omitempty"`
	BuildCommand   string `json:"build_command,omitempty"`
	OutputDir      string `json:"output_dir,omitempty"`
	InstallCommand string `json:"install_command,omitempty"`
}

type EnvVar struct {
	ID        string `json:"id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Key       string `json:"key,omitempty"`
	Value     string `json:"value,omitempty"`
	IsSecret  bool   `json:"is_secret,omitempty"`
	Scope     string `json:"scope,omitempty"`
}

type Domain struct {
	ID          string `json:"id,omitempty"`
	ProjectID   string `json:"project_id,omitempty"`
	Hostname    string `json:"hostname,omitempty"`
	Status      string `json:"status,omitempty"`
	VerifyToken string `json:"verify_token,omitempty"`
}

type ManagedService struct {
	ID        string            `json:"id,omitempty"`
	ProjectID string            `json:"project_id,omitempty"`
	Kind      string            `json:"kind,omitempty"`
	Name      string            `json:"name,omitempty"`
	Status    string            `json:"status,omitempty"`
	Config    map[string]string `json:"config,omitempty"`
}

// --- Projects ---

func (c *Client) CreateProject(ctx context.Context, p Project) (*Project, error) {
	var out Project
	// POST /projects returns the Project object directly (responses.Project).
	err := c.do(ctx, http.MethodPost, "/projects", p, &out)
	return &out, err
}

func (c *Client) GetProject(ctx context.Context, id string) (*Project, error) {
	var out Project
	err := c.do(ctx, http.MethodGet, "/projects/"+id, nil, &out)
	return &out, err
}

func (c *Client) UpdateProject(ctx context.Context, id string, p Project) (*Project, error) {
	var out Project
	err := c.do(ctx, http.MethodPatch, "/projects/"+id, p, &out)
	return &out, err
}

func (c *Client) DeleteProject(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/projects/"+id, nil, nil)
}

func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	var out struct {
		Projects []Project `json:"projects"`
	}
	err := c.do(ctx, http.MethodGet, "/projects", nil, &out)
	return out.Projects, err
}

// --- Env vars (per-key PUT upsert; delete by env id; read the set) ---

func (c *Client) PutEnv(ctx context.Context, projectID string, e EnvVar) (*EnvVar, error) {
	var out EnvVar
	err := c.do(ctx, http.MethodPut, "/projects/"+projectID+"/env", e, &out)
	return &out, err
}

func (c *Client) ListEnv(ctx context.Context, projectID string) ([]EnvVar, error) {
	var out struct {
		Env []EnvVar `json:"env"`
	}
	err := c.do(ctx, http.MethodGet, "/projects/"+projectID+"/env", nil, &out)
	return out.Env, err
}

func (c *Client) DeleteEnv(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/env/"+id, nil, nil)
}

// --- Domains (list + create + delete; no single GET, no update) ---

func (c *Client) CreateDomain(ctx context.Context, projectID, hostname string) (*Domain, error) {
	var out struct {
		Domain Domain `json:"domain"`
	}
	err := c.do(ctx, http.MethodPost, "/projects/"+projectID+"/domains", map[string]string{"hostname": hostname}, &out)
	return &out.Domain, err
}

func (c *Client) ListDomains(ctx context.Context, projectID string) ([]Domain, error) {
	var out struct {
		Domains []Domain `json:"domains"`
	}
	err := c.do(ctx, http.MethodGet, "/projects/"+projectID+"/domains", nil, &out)
	return out.Domains, err
}

func (c *Client) DeleteDomain(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/domains/"+id, nil, nil)
}

// --- Managed services (create + single GET + delete; no update) ---

func (c *Client) CreateService(ctx context.Context, projectID string, s ManagedService) (*ManagedService, error) {
	var out ManagedService
	err := c.do(ctx, http.MethodPost, "/projects/"+projectID+"/services", s, &out)
	return &out, err
}

func (c *Client) GetService(ctx context.Context, id string) (*ManagedService, error) {
	var out ManagedService
	err := c.do(ctx, http.MethodGet, "/services/"+id, nil, &out)
	return &out, err
}

func (c *Client) DeleteService(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/services/"+id, nil, nil)
}
