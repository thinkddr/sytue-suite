// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// SytueCloud es el destino de las aplicaciones. Escribe por la API pública de
// Sytue Cloud, así que la migración hereda las cuotas del plan de la
// organización, su facturación y su auditoría: una migración no puede colar
// treinta proyectos por una puerta que a una persona le habría dicho que no.
type SytueCloud struct {
	base  string
	token TokenSource
	hc    *http.Client

	mu      sync.Mutex
	bySlug  map[string]string // slug → id de proyecto
	loaded  bool
	buckets map[string]string // projectID → id del servicio de storage
}

// NewSytueCloud construye el cliente. hc es opcional.
func NewSytueCloud(baseURL string, token TokenSource, hc *http.Client) *SytueCloud {
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Minute}
	}
	return &SytueCloud{
		base: strings.TrimRight(baseURL, "/"), token: token, hc: hc,
		bySlug: map[string]string{}, buckets: map[string]string{},
	}
}

// EnsureProject crea el proyecto de una aplicación, o devuelve el que ya existe.
func (s *SytueCloud) EnsureProject(ctx context.Context, a CloudApp) (string, error) {
	slug := cloudSlug(a.Name)
	if slug == "" {
		return "", fmt.Errorf("%w: %q no da un nombre de proyecto válido en Sytue Cloud (solo letras, números y guiones)",
			ErrUnimportable, a.Name)
	}
	if err := s.load(ctx); err != nil {
		return "", err
	}
	s.mu.Lock()
	if id, ok := s.bySlug[slug]; ok {
		s.mu.Unlock()
		return id, nil
	}
	s.mu.Unlock()

	body := map[string]any{
		"slug":           slug,
		"name":           a.Name,
		"instance_size":  instanceSizeFor(a.CPU, a.MemoryMB),
		"instance_count": instanceCountFor(a.MinInstances),
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := s.call(ctx, http.MethodPost, "/api/v1/projects", body, &created); err != nil {
		return "", err
	}
	if created.ID == "" {
		return "", fmt.Errorf("migrate: Sytue Cloud no devolvió el id del proyecto creado")
	}
	s.mu.Lock()
	s.bySlug[slug] = created.ID
	s.mu.Unlock()
	return created.ID, nil
}

// load trae los proyectos que la organización ya tiene (una vez por trabajo).
func (s *SytueCloud) load(ctx context.Context) error {
	s.mu.Lock()
	done := s.loaded
	s.mu.Unlock()
	if done {
		return nil
	}
	var list struct {
		Projects []struct {
			ID   string `json:"id"`
			Slug string `json:"slug"`
		} `json:"projects"`
	}
	if err := s.call(ctx, http.MethodGet, "/api/v1/projects", nil, &list); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range list.Projects {
		s.bySlug[p.Slug] = p.ID
	}
	s.loaded = true
	return nil
}

// SetEnv fija las variables del proyecto. La API acepta una por petición: se
// mandan todas y un fallo suelto no se traga el resto, porque una aplicación con
// nueve variables de diez arranca y se diagnostica; con ninguna, no.
func (s *SytueCloud) SetEnv(ctx context.Context, projectID string, vars []CloudVar) error {
	path := "/api/v1/projects/" + url.PathEscape(projectID) + "/env"
	var firstErr error
	for _, v := range vars {
		body := map[string]any{"key": v.Name, "value": v.Value, "is_secret": v.Secret}
		if err := s.call(ctx, http.MethodPut, path, body, nil); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// AddDomain añade un dominio propio al proyecto.
func (s *SytueCloud) AddDomain(ctx context.Context, projectID, host string) error {
	path := "/api/v1/projects/" + url.PathEscape(projectID) + "/domains"
	return s.call(ctx, http.MethodPost, path, map[string]any{"hostname": host}, nil)
}

// EnsureBucket aprovisiona el Storage gestionado del proyecto, o devuelve el que
// ya tiene.
//
// Se pregunta al destino, no solo a la memoria de este proceso: reanudar un
// trabajo arranca un cliente nuevo, y sin esta consulta cada reanudación
// aprovisionaría OTRO bucket —un recurso de más en la factura por cada corte de
// red—.
func (s *SytueCloud) EnsureBucket(ctx context.Context, projectID string, b CloudBucket) (string, error) {
	s.mu.Lock()
	if id, ok := s.buckets[projectID]; ok {
		s.mu.Unlock()
		return id, nil
	}
	s.mu.Unlock()

	var existing struct {
		Services []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"services"`
	}
	path := "/api/v1/projects/" + url.PathEscape(projectID) + "/services"
	if err := s.call(ctx, http.MethodGet, path, nil, &existing); err == nil {
		for _, svc := range existing.Services {
			if svc.Kind == "storage" {
				s.mu.Lock()
				s.buckets[projectID] = svc.ID
				s.mu.Unlock()
				return svc.ID, nil
			}
		}
	}

	body := map[string]any{"kind": "storage", "name": cloudSlug(b.Name)}
	var created struct {
		ID string `json:"id"`
	}
	if err := s.call(ctx, http.MethodPost, path, body, &created); err != nil {
		return "", err
	}
	s.mu.Lock()
	s.buckets[projectID] = created.ID
	s.mu.Unlock()
	return created.ID, nil
}

// UploadObject sube un objeto al Storage del proyecto por URL prefirmada. Los
// bytes NO pasan por la API de Cloud: van directos al Object Storage, que es
// para lo que existe la URL prefirmada.
func (s *SytueCloud) UploadObject(ctx context.Context, projectID string, o CloudObject, body io.Reader) error {
	var signed struct {
		URL string `json:"url"`
	}
	path := "/api/v1/projects/" + url.PathEscape(projectID) + "/storage/presign"
	if err := s.call(ctx, http.MethodPost, path, map[string]any{"key": o.Key, "method": "PUT"}, &signed); err != nil {
		return err
	}
	if signed.URL == "" {
		return fmt.Errorf("migrate: Sytue Cloud no devolvió la URL de subida de %q", o.Key)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, signed.URL, body)
	if err != nil {
		return err
	}
	if o.ContentType != "" {
		req.Header.Set("Content-Type", o.ContentType)
	}
	if o.Size > 0 {
		req.ContentLength = o.Size
	}
	resp, err := s.hc.Do(req)
	if err != nil {
		return fmt.Errorf("migrate: subir %q al almacenamiento de Sytue Cloud: %w", o.Key, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("migrate: el almacenamiento rechazó %q (HTTP %d)", o.Key, resp.StatusCode)
	}
	return nil
}

// EnsureCronJob recrea una tarea programada como servicio cron del proyecto.
func (s *SytueCloud) EnsureCronJob(ctx context.Context, projectID string, j CloudJob) (string, error) {
	name := cloudSlug(j.Name)
	if name == "" {
		return "", fmt.Errorf("%w: %q no da un nombre de tarea válido", ErrUnimportable, j.Name)
	}
	config := map[string]string{"schedule": j.Schedule, "url": j.TargetURL}
	if j.Method != "" {
		config["method"] = j.Method
	}
	if len(j.Body) > 0 {
		config["body"] = string(j.Body)
		config["content_type"] = "application/json"
	}
	var created struct {
		ID string `json:"id"`
	}
	path := "/api/v1/projects/" + url.PathEscape(projectID) + "/services"
	body := map[string]any{"kind": "cron", "name": name, "config": config}
	if err := s.call(ctx, http.MethodPost, path, body, &created); err != nil {
		return "", err
	}
	return created.ID, nil
}

// slugInvalid son los caracteres que no puede llevar el nombre de un proyecto:
// el slug es su subdominio público, así que tiene que ser una etiqueta DNS.
var slugInvalid = regexp.MustCompile(`[^a-z0-9-]+`)

// cloudSlug convierte el nombre del origen en un slug DNS válido de Sytue Cloud
// (minúsculas, hasta 40 caracteres, sin empezar ni acabar en guion).
func cloudSlug(name string) string {
	s := slugInvalid.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
	s = strings.Trim(s, "-")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-")
	}
	return s
}

// instanceSizeFor traduce los límites del origen al tamaño de Sytue Cloud más
// cercano POR ENCIMA. Quedarse corto convertiría la migración en una caída de
// rendimiento que nadie relaciona con la mudanza; sobrar un escalón cuesta unos
// céntimos y se corrige desde el panel con el uso real delante.
func instanceSizeFor(cpu float64, memoryMB int) string {
	switch {
	case memoryMB <= 512 && cpu <= 1:
		return "basic"
	case memoryMB <= 1024 && cpu <= 1:
		return "standard"
	case memoryMB <= 2048 && cpu <= 2:
		return "pro"
	default:
		return "pro-xl"
	}
}

// instanceCountFor respeta el mínimo de instancias del origen. Cero significa
// «escala a cero», que en Sytue Cloud es una instancia: un servicio que no puede
// atender la primera petición no está migrado.
func instanceCountFor(minInstances int) int {
	if minInstances < 1 {
		return 1
	}
	if minInstances > 10 {
		return 10
	}
	return minInstances
}

// call emite una petición JSON autenticada a Sytue Cloud.
func (s *SytueCloud) call(ctx context.Context, method, path string, body, dst any) error {
	var payload io.Reader = http.NoBody
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(raw)
	}
	token, err := s.token(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, s.base+path, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.hc.Do(req)
	if err != nil {
		return fmt.Errorf("migrate: %s %s: %w", method, redactPath(s.base+path), err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("migrate: Sytue Cloud rechazó la escritura (HTTP %d): esa persona necesita rol de administrador en la organización de Cloud para crear proyectos", resp.StatusCode)
	case resp.StatusCode == http.StatusPaymentRequired:
		// La cuota del plan no se arregla reintentando: hay que subir de plan o
		// migrar menos aplicaciones, y quien decide eso es la organización.
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%w: el plan de Sytue Cloud de la organización no da para más: %s",
			ErrUnimportable, strings.TrimSpace(string(msg)))
	case resp.StatusCode == http.StatusConflict:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%w: ese nombre ya está ocupado en Sytue Cloud: %s",
			ErrUnimportable, strings.TrimSpace(string(msg)))
	case resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnprocessableEntity:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%w: Sytue Cloud no acepta esta configuración: %s",
			ErrUnimportable, strings.TrimSpace(string(msg)))
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &suiteError{status: resp.StatusCode, msg: strings.TrimSpace(string(msg)), service: "Sytue Cloud"}
	}
	if dst == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(dst); err != nil {
		return fmt.Errorf("migrate: respuesta de Sytue Cloud ilegible: %w", err)
	}
	return nil
}

var _ CloudDestination = (*SytueCloud)(nil)
