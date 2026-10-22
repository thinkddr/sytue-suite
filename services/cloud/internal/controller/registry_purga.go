// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos (thinkddr)

package controller

import (
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

// purgeBatch: proyectos borrados que se purgan por vuelta.
const purgeBatch = 20

// reconcilePurge borra del registry las imágenes de los proyectos borrados y lo
// anota (purged_at). Hasta entonces su slug sigue retenido: si el registry no
// deja borrar, el slug no se libera nunca, que es fallar cerrado (CLOUD-08).
func (c *Controller) reconcilePurge(ctx context.Context) error {
	if c.purgeRepo == nil {
		return nil
	}
	pend, err := c.store.ListProjectsToPurge(ctx, purgeBatch)
	if err != nil {
		return err
	}
	for _, p := range pend {
		// La misma organización volvió a crear el slug: esas imágenes ya son del
		// proyecto nuevo. Se da por purgado sin tocar nada.
		if !p.SlugVivo {
			if err := c.purgeRepo(ctx, p.Slug); err != nil {
				return fmt.Errorf("purgar imágenes de %s: %w", p.Slug, err)
			}
		}
		if err := c.store.MarkProjectPurged(ctx, p.ID); err != nil {
			return err
		}
	}
	return nil
}

// registryPurger borra todas las etiquetas de `<RegistryURL>/<slug>` con la API
// de distribución v2 (lista de etiquetas, digest de cada una y DELETE del
// manifiesto). Habla Basic y, si el registry pide un token (Scaleway, Docker
// Hub), el reto Bearer estándar.
type registryPurger struct {
	base, namespace, user, pass string
	http                        *http.Client
}

func newRegistryPurger(cfg config.Config) func(context.Context, string) error {
	raw := strings.TrimSuffix(cfg.RegistryURL, "/")
	if raw == "" {
		return nil
	}
	scheme := "https://"
	if strings.HasPrefix(raw, "http://") {
		scheme = "http://"
	}
	raw = strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	host, ns, _ := strings.Cut(raw, "/")
	r := &registryPurger{
		base: scheme + host, namespace: ns, user: cfg.RegistryUsername,
		pass: cfg.RegistryPassword, http: &http.Client{Timeout: 30 * time.Second},
	}
	return r.purge
}

func (r *registryPurger) repo(slug string) string {
	if r.namespace == "" {
		return slug
	}
	return r.namespace + "/" + slug
}

func (r *registryPurger) purge(ctx context.Context, slug string) error {
	repo := r.repo(slug)
	resp, err := r.do(ctx, http.MethodGet, "/v2/"+repo+"/tags/list", "")
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()
		return nil // nunca se construyó nada: nada que borrar
	}
	var tags struct {
		Tags []string `json:"tags"`
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&tags)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || err != nil {
		return fmt.Errorf("listar etiquetas: HTTP %d: %w", resp.StatusCode, err)
	}
	const accept = "application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json, " +
		"application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.docker.distribution.manifest.v2+json"
	for _, tag := range tags.Tags {
		head, err := r.do(ctx, http.MethodHead, "/v2/"+repo+"/manifests/"+url.PathEscape(tag), accept)
		if err != nil {
			return err
		}
		_ = head.Body.Close()
		digest := head.Header.Get("Docker-Content-Digest")
		if head.StatusCode == http.StatusNotFound {
			continue // otra etiqueta con el mismo digest ya lo borró
		}
		if head.StatusCode != http.StatusOK || digest == "" {
			return fmt.Errorf("digest de %s: HTTP %d", tag, head.StatusCode)
		}
		del, err := r.do(ctx, http.MethodDelete, "/v2/"+repo+"/manifests/"+digest, "")
		if err != nil {
			return err
		}
		_ = del.Body.Close()
		if del.StatusCode != http.StatusAccepted && del.StatusCode != http.StatusOK && del.StatusCode != http.StatusNotFound {
			return fmt.Errorf("borrar %s: HTTP %d", tag, del.StatusCode)
		}
	}
	return nil
}

// do hace la petición con Basic y, ante un 401 con reto Bearer, pide el token
// al realm y repite. ponytail: un token por petición; con cientos de etiquetas
// convendría reutilizarlo.
func (r *registryPurger) do(ctx context.Context, method, path, accept string) (*http.Response, error) {
	send := func(auth string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, method, r.base+path, http.NoBody)
		if err != nil {
			return nil, err
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		if auth != "" {
			req.Header.Set("Authorization", auth)
		} else if r.user != "" {
			req.SetBasicAuth(r.user, r.pass)
		}
		return r.http.Do(req)
	}
	resp, err := send("")
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		return resp, err
	}
	reto := resp.Header.Get("WWW-Authenticate")
	_ = resp.Body.Close()
	tok, err := r.token(ctx, reto)
	if err != nil {
		return nil, err
	}
	return send("Bearer " + tok)
}

func (r *registryPurger) token(ctx context.Context, reto string) (string, error) {
	scheme, params, ok := strings.Cut(reto, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", fmt.Errorf("el registry no acepta las credenciales")
	}
	kv := map[string]string{}
	for _, p := range strings.Split(params, ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(p), "=")
		kv[k] = strings.Trim(v, `"`)
	}
	u, err := url.Parse(kv["realm"])
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("reto del registry sin realm")
	}
	q := u.Query()
	for _, k := range []string{"service", "scope"} {
		if kv[k] != "" {
			q.Set(k, kv[k])
		}
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return "", err
	}
	if r.user != "" {
		req.SetBasicAuth(r.user, r.pass)
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var t struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&t); err != nil || resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token del registry: HTTP %d", resp.StatusCode)
	}
	if t.Token != "" {
		return t.Token, nil
	}
	return t.AccessToken, nil
}
