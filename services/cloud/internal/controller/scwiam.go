// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

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

	"git.sytue.eu/sytue/one/services/cloud/internal/config"
)

// IAMClient es la superficie mínima de Scaleway IAM que Sytue Cloud necesita para
// acuñar credenciales S3 PERSISTENTES y ACOTADAS por-tenant. Scaleway Object
// Storage es S3-compatible: IAM emite applications + API keys (identidad+clave),
// y el ALCANCE a un solo bucket lo impone una bucket policy (ver bucketPolicyJSON),
// porque los permission sets de Scaleway IAM solo se acotan a un proyecto, NUNCA a
// un bucket. Un fake la implementa en tests: no hace falta un Scaleway vivo.
type IAMClient interface {
	// EnsureApplication busca o crea (idempotente) una application IAM por nombre y
	// devuelve su id. Los nombres NO son únicos en Scaleway, así que crear siempre
	// filtraría duplicados en los reintentos del reconcile: se busca-y-crea.
	EnsureApplication(ctx context.Context, name string) (appID string, err error)
	// RotateAPIKey borra las API keys existentes de la application y acuña una
	// nueva, devolviendo su access/secret key (el secret solo se muestra al crear).
	// Rotar (no solo crear) mantiene los reintentos idempotentes: queda válida
	// exactamente la key devuelta, sin acumular secretos huérfanos.
	RotateAPIKey(ctx context.Context, appID string) (accessKey, secretKey string, err error)
	// DeleteApplication elimina la application llamada name y sus API keys
	// (idempotente: si no existe ⇒ nil). Se usa al desaprovisionar el storage.
	DeleteApplication(ctx context.Context, name string) error
}

// scwIAMClient es la implementación REST (hand-rolled con net/http) contra la API
// de Scaleway IAM v1alpha1. La auth es solo la cabecera X-Auth-Token con el secret
// key de plataforma (con derecho IAM); no requiere ningún SDK.
type scwIAMClient struct {
	baseURL string
	token   string
	orgID   string
	hc      *http.Client
}

// newSCWIAMClient construye el cliente REST de Scaleway IAM.
func newSCWIAMClient(cfg config.Config) *scwIAMClient {
	return &scwIAMClient{
		baseURL: strings.TrimRight(cfg.SCWIAMURL, "/"),
		token:   cfg.SCWSecretKey,
		orgID:   cfg.SCWOrgID,
		hc:      &http.Client{Timeout: 15 * time.Second},
	}
}

// do ejecuta una petición JSON contra la API IAM. body/out nil se omiten.
func (c *scwIAMClient) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("controller: IAM marshal: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return fmt.Errorf("controller: IAM request: %w", err)
	}
	// Secreto de plataforma con derecho IAM: cabecera X-Auth-Token (nunca en logs).
	req.Header.Set("X-Auth-Token", c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("controller: IAM %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("controller: IAM %s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("controller: IAM decode: %w", err)
		}
	}
	return nil
}

type scwApp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// EnsureApplication implementa IAMClient.
func (c *scwIAMClient) EnsureApplication(ctx context.Context, name string) (string, error) {
	var list struct {
		Applications []scwApp `json:"applications"`
	}
	q := url.Values{"organization_id": {c.orgID}, "name": {name}, "page_size": {"100"}}
	if err := c.do(ctx, http.MethodGet, "/applications", q, nil, &list); err != nil {
		return "", err
	}
	// El filtro name de Scaleway es de coincidencia parcial: exige match exacto.
	for _, a := range list.Applications {
		if a.Name == name {
			return a.ID, nil
		}
	}
	var created scwApp
	body := map[string]string{
		"name":            name,
		"organization_id": c.orgID,
		"description":     "Sytue Cloud identidad de storage por-tenant",
	}
	if err := c.do(ctx, http.MethodPost, "/applications", nil, body, &created); err != nil {
		return "", err
	}
	if created.ID == "" {
		return "", fmt.Errorf("controller: IAM application creada sin id")
	}
	return created.ID, nil
}

// RotateAPIKey implementa IAMClient.
func (c *scwIAMClient) RotateAPIKey(ctx context.Context, appID string) (string, string, error) {
	if err := c.deleteKeys(ctx, appID); err != nil {
		return "", "", err
	}
	var key struct {
		AccessKey string `json:"access_key"`
		SecretKey string `json:"secret_key"`
	}
	body := map[string]string{
		"application_id": appID,
		"description":    "Sytue Cloud credencial S3 de tenant",
	}
	if err := c.do(ctx, http.MethodPost, "/api-keys", nil, body, &key); err != nil {
		return "", "", err
	}
	if key.AccessKey == "" || key.SecretKey == "" {
		return "", "", fmt.Errorf("controller: IAM api-key sin access/secret")
	}
	return key.AccessKey, key.SecretKey, nil
}

// deleteKeys borra todas las API keys de una application (best-effort acotado a
// una página; con la rotación una application tiene a lo sumo 1 key).
func (c *scwIAMClient) deleteKeys(ctx context.Context, appID string) error {
	var list struct {
		APIKeys []struct {
			AccessKey string `json:"access_key"`
		} `json:"api_keys"`
	}
	q := url.Values{"application_id": {appID}, "page_size": {"100"}}
	if err := c.do(ctx, http.MethodGet, "/api-keys", q, nil, &list); err != nil {
		return err
	}
	for _, k := range list.APIKeys {
		if k.AccessKey == "" {
			continue
		}
		if err := c.do(ctx, http.MethodDelete, "/api-keys/"+url.PathEscape(k.AccessKey), nil, nil, nil); err != nil {
			return err
		}
	}
	return nil
}

// DeleteApplication implementa IAMClient. Borra las keys ANTES que la application
// (no se confía en un borrado en cascada para no filtrar credenciales).
func (c *scwIAMClient) DeleteApplication(ctx context.Context, name string) error {
	var list struct {
		Applications []scwApp `json:"applications"`
	}
	q := url.Values{"organization_id": {c.orgID}, "name": {name}, "page_size": {"100"}}
	if err := c.do(ctx, http.MethodGet, "/applications", q, nil, &list); err != nil {
		return err
	}
	for _, a := range list.Applications {
		if a.Name != name {
			continue
		}
		if err := c.deleteKeys(ctx, a.ID); err != nil {
			return err
		}
		if err := c.do(ctx, http.MethodDelete, "/applications/"+url.PathEscape(a.ID), nil, nil, nil); err != nil {
			return err
		}
	}
	return nil
}

// bucketPolicyJSON construye una bucket policy S3 que concede a la application IAM
// appID acceso S3 a EXACTAMENTE bucket (y sus objetos) y a nada más. Esto es lo
// que ACOTA de verdad la credencial del tenant a su propio bucket: como los
// permission sets de Scaleway IAM no expresan alcance por-bucket, se decide NO
// adjuntar una policy IAM amplia (ObjectStorage de proyecto entero) — la bucket
// policy es toda la autorización. Resource solo <bucket> y <bucket>/*, Principal
// solo application_id:<appID>: jamás un recurso o principal comodín.
func bucketPolicyJSON(bucket, appID string) (string, error) {
	type statement struct {
		Sid       string              `json:"Sid"`
		Effect    string              `json:"Effect"`
		Principal map[string][]string `json:"Principal"`
		Action    []string            `json:"Action"`
		Resource  []string            `json:"Resource"`
	}
	policy := struct {
		Version   string      `json:"Version"`
		Statement []statement `json:"Statement"`
	}{
		Version: "2023-04-17",
		Statement: []statement{{
			Sid:       "sytue-tenant-bucket-scope",
			Effect:    "Allow",
			Principal: map[string][]string{"SCW": {"application_id:" + appID}},
			Action:    []string{"s3:*"},
			Resource:  []string{bucket, bucket + "/*"},
		}},
	}
	b, err := json.Marshal(policy)
	if err != nil {
		return "", fmt.Errorf("controller: construir bucket policy: %w", err)
	}
	return string(b), nil
}
