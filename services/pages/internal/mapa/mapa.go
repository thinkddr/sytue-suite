// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package mapa implementa el mapa host → sitio/versión de las webs publicadas
// (shared/mapa/hosts.json, PAGES-ENCARGO §8.2): lo escribe la API y lo leen la API
// y `pages sitios`.
package mapa

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

// ClaveHostsJSON es la ruta de hosts.json en el almacén de objetos compartido.
const ClaveHostsJSON = "shared/mapa/hosts.json"

// HostsMapa es el documento completo generado en S3.
type HostsMapa struct {
	Version     int                 `json:"version"`
	GeneratedAt time.Time           `json:"generated_at"`
	Sites       map[string]SiteMapa `json:"sites"`
	Hosts       map[string]string   `json:"hosts"`
}

// SiteMapa representa la configuración activa de un sitio en el mapa.
type SiteMapa struct {
	Tenant          string            `json:"tenant"`
	Slug            string            `json:"slug"`
	Number          int               `json:"number"`
	Status          string            `json:"status"` // "live", "unpublished", "suspended"
	Canonical       string            `json:"canonical"`
	Languages       []string          `json:"languages"`
	DefaultLanguage string            `json:"default_language"`
	Previews        map[string]string `json:"previews,omitempty"` // sha256(token)[:32] -> RFC3339
}

// StoreReader define las lecturas de base de datos requeridas para regenerar el mapa.
type StoreReader interface {
	ObtenerDatosParaMapa(ctx context.Context) ([]store.DatosSitioMapa, []store.DatosPreviewMapa, []store.DatosDominioMapa, []store.DatosRetiredSlugMapa, error)
}

// ObjectStore define las operaciones de S3 necesarias para el mapa.
type ObjectStore interface {
	Put(ctx context.Context, t tenancy.Tenant, key string, r io.Reader, size int64, opts objectstore.PutOptions) error
	Get(ctx context.Context, t tenancy.Tenant, key string) (io.ReadCloser, error)
}

// Escritor regenera el mapa a partir de la base y lo escribe en S3 si ha cambiado.
type Escritor struct {
	store       StoreReader
	objetos     ObjectStore
	sitesDomain string
	lastHash    string
	mu          sync.Mutex
}

// NuevoEscritor construye el escritor del mapa.
func NuevoEscritor(st StoreReader, objetos ObjectStore, sitesDomain string) *Escritor {
	return &Escritor{
		store:       st,
		objetos:     objetos,
		sitesDomain: sitesDomain,
	}
}

// Regenerar recalcula el mapa desde la base de datos y lo actualiza en S3 si cambió.
func (e *Escritor) Regenerar(ctx context.Context) error {
	if e.store == nil || e.objetos == nil {
		return nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	sitios, previews, dominios, retiredSlugs, err := e.store.ObtenerDatosParaMapa(ctx)
	if err != nil {
		return fmt.Errorf("mapa: obtener datos para mapa: %w", err)
	}

	mapa := construirMapa(sitios, previews, dominios, retiredSlugs, e.sitesDomain)

	content := struct {
		Version int                 `json:"version"`
		Sites   map[string]SiteMapa `json:"sites"`
		Hosts   map[string]string   `json:"hosts"`
	}{
		Version: 1,
		Sites:   mapa.Sites,
		Hosts:   mapa.Hosts,
	}

	contentBytes, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("mapa: serializar contenido para hash: %w", err)
	}

	h := sha256.Sum256(contentBytes)
	hashStr := hex.EncodeToString(h[:])
	if hashStr == e.lastHash {
		return nil
	}

	mapa.GeneratedAt = time.Now().UTC()
	b, err := json.Marshal(mapa)
	if err != nil {
		return fmt.Errorf("mapa: serializar hosts.json: %w", err)
	}

	tenantShared := tenancy.Tenant{Slug: "shared"}
	if err := e.objetos.Put(ctx, tenantShared, ClaveHostsJSON, bytes.NewReader(b), int64(len(b)), objectstore.PutOptions{
		ContentType: "application/json; charset=utf-8",
	}); err != nil {
		return fmt.Errorf("mapa: escribir %s en S3: %w", ClaveHostsJSON, err)
	}

	e.lastHash = hashStr
	return nil
}

func construirMapa(sitios []store.DatosSitioMapa, previews []store.DatosPreviewMapa, dominios []store.DatosDominioMapa, retiredSlugs []store.DatosRetiredSlugMapa, sitesDomain string) HostsMapa {
	mapa := HostsMapa{
		Version:     1,
		GeneratedAt: time.Now().UTC(),
		Sites:       make(map[string]SiteMapa, len(sitios)),
		Hosts:       make(map[string]string),
	}

	previewsPorSitio := make(map[string]map[string]string)
	for _, p := range previews {
		sID := p.SiteID.String()
		if previewsPorSitio[sID] == nil {
			previewsPorSitio[sID] = make(map[string]string)
		}
		previewsPorSitio[sID][p.PreviewKey] = p.ExpiresAt.UTC().Format(time.RFC3339)
	}

	// Lo único que puede ser el canónico de un sitio, además de su host por slug: un
	// dominio ACTIVO de ese mismo sitio. `sites.canonical_host` fue texto libre del
	// navegador (WEB-01): un sitio que escribía ahí el host de otro se quedaba con él.
	activos := make(map[string]bool, len(dominios))
	for _, d := range dominios {
		if d.Status == "active" {
			activos[d.SiteID.String()+"|"+d.Host] = true
		}
	}

	for _, s := range sitios {
		sID := s.ID.String()
		hostSlug := s.Slug + "." + sitesDomain
		canonical := hostSlug
		if s.CanonicalHost != nil && activos[sID+"|"+*s.CanonicalHost] {
			canonical = *s.CanonicalHost
		}

		status := "live"
		if s.Status == "suspended" {
			status = "suspended"
		} else if s.PublishedNumber == nil {
			status = "unpublished"
		}

		number := 0
		if s.PublishedNumber != nil {
			number = *s.PublishedNumber
		}

		var langs struct {
			Default string   `json:"default"`
			Others  []string `json:"others"`
		}
		if len(s.LanguagesRaw) > 0 {
			_ = json.Unmarshal(s.LanguagesRaw, &langs)
		}
		if langs.Default == "" {
			langs.Default = "es"
		}
		allLangs := append([]string{langs.Default}, langs.Others...)

		mapa.Sites[sID] = SiteMapa{
			Tenant:          s.Tenant,
			Slug:            s.Slug,
			Number:          number,
			Status:          status,
			Canonical:       canonical,
			Languages:       allLangs,
			DefaultLanguage: langs.Default,
			Previews:        previewsPorSitio[sID],
		}

		mapa.Hosts[hostSlug] = sID
	}

	// Un host entra por su fila de `domains` (única entre verificados y activos), nunca
	// por el canónico de un sitio, y no pisa a uno que ya esté: el slug es de su sitio.
	for _, d := range dominios {
		sID := d.SiteID.String()
		if _, ok := mapa.Sites[sID]; !ok {
			continue
		}
		if _, ya := mapa.Hosts[d.Host]; !ya {
			mapa.Hosts[d.Host] = sID
		}
	}

	for _, r := range retiredSlugs {
		sID := r.SiteID.String()
		if _, ok := mapa.Sites[sID]; ok {
			hostRetirado := r.Slug + "." + sitesDomain
			if _, existe := mapa.Hosts[hostRetirado]; !existe {
				mapa.Hosts[hostRetirado] = sID
			}
		}
	}

	return mapa
}

// Lector lee el mapa de S3 con una caché en memoria y refresco periódico.
type Lector struct {
	objetos        ObjectStore
	refresco       time.Duration
	mu             sync.RWMutex
	mapa           HostsMapa
	listo          bool
	ultimoRefresco time.Time
}

// NuevoLector construye el lector del mapa.
func NuevoLector(objetos ObjectStore, refresco time.Duration) *Lector {
	if refresco <= 0 {
		refresco = 10 * time.Second
	}
	return &Lector{
		objetos:  objetos,
		refresco: refresco,
		mapa: HostsMapa{
			Version: 1,
			Sites:   make(map[string]SiteMapa),
			Hosts:   make(map[string]string),
		},
	}
}

// Refrescar lee de S3 la última versión del mapa.
func (l *Lector) Refrescar(ctx context.Context) error {
	if l.objetos == nil {
		l.mu.Lock()
		l.listo = true
		l.mu.Unlock()
		return nil
	}

	tenantShared := tenancy.Tenant{Slug: "shared"}
	rc, err := l.objetos.Get(ctx, tenantShared, ClaveHostsJSON)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			// NoSuchKey = mapa vacío (todo 404) y listo (§8.1)
			l.mu.Lock()
			l.mapa = HostsMapa{
				Version:     1,
				GeneratedAt: time.Now().UTC(),
				Sites:       make(map[string]SiteMapa),
				Hosts:       make(map[string]string),
			}
			l.listo = true
			l.ultimoRefresco = time.Now()
			l.mu.Unlock()
			return nil
		}
		// Error de lectura: conserva el último bueno; si no hay ninguno, listo sigue en false
		return fmt.Errorf("mapa: error leyendo %s: %w", ClaveHostsJSON, err)
	}
	defer rc.Close()

	var m HostsMapa
	if err := json.NewDecoder(rc).Decode(&m); err != nil {
		return fmt.Errorf("mapa: decodificar %s: %w", ClaveHostsJSON, err)
	}

	l.mu.Lock()
	l.mapa = m
	l.listo = true
	l.ultimoRefresco = time.Now()
	l.mu.Unlock()

	return nil
}

// IniciarBucle arranca la gorutina de refresco periódico del mapa.
func (l *Lector) IniciarBucle(ctx context.Context) {
	_ = l.Refrescar(ctx)
	go func() {
		ticker := time.NewTicker(l.refresco)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = l.Refrescar(ctx)
			}
		}
	}()
}

// ObtenerPorHost busca un host y devuelve el ID del sitio y su configuración en el mapa.
func (l *Lector) ObtenerPorHost(host string) (string, SiteMapa, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	siteID, ok := l.mapa.Hosts[host]
	if !ok {
		return "", SiteMapa{}, false
	}
	site, ok := l.mapa.Sites[siteID]
	return siteID, site, ok
}

// ObtenerPorID busca un sitio por su ID en el mapa.
func (l *Lector) ObtenerPorID(siteID string) (SiteMapa, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	site, ok := l.mapa.Sites[siteID]
	return site, ok
}

// Listo indica si el lector ha completado al menos una lectura exitosa o segura de S3.
func (l *Lector) Listo() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.listo
}
