// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package sitios implementa el servidor de las webs publicadas (`pages sitios`,
// PAGES-ENCARGO §8.1): sin base de datos, lee el mapa y los ficheros de S3.
package sitios

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/internal/catalogo"
	"git.sytue.eu/sytue/one/services/pages/internal/config"
	"git.sytue.eu/sytue/one/services/pages/internal/mapa"
	"git.sytue.eu/sytue/one/services/pages/internal/textos"
)

//go:embed textos/*.json
var textosFS embed.FS

var reRutaValida = regexp.MustCompile(`^/[a-z0-9/_.-]*$`)

// ServidorSitios agrupa las dependencias necesarias para servir webs de clientes.
type ServidorSitios struct {
	cfg      config.Sitios
	lector   *mapa.Lector
	objetos  *objectstore.Client
	textos   *textos.Catalogo
	catalogo *catalogo.Catalogo
}

// NuevoRouter construye el router endurecido del servidor de sitios (§8.1).
func NuevoRouter(version string, cfg config.Sitios, lector *mapa.Lector, objetos *objectstore.Client) http.Handler {
	catTextos, err := textos.Cargar(textosFS, "textos")
	if err != nil {
		panic(fmt.Sprintf("sitios: cargar textos: %v", err))
	}

	cat, err := catalogo.CargarPorDefecto()
	if err != nil {
		panic(fmt.Sprintf("sitios: cargar catálogo: %v", err))
	}
	s := &ServidorSitios{
		cfg:      cfg,
		lector:   lector,
		objetos:  objetos,
		textos:   catTextos,
		catalogo: cat,
	}

	frameSrc := "https://www.openstreetmap.org"
	if cfg.CastOrigin != "" {
		frameSrc += " " + cfg.CastOrigin
	}
	frameSrc += " https://www.youtube-nocookie.com https://player.vimeo.com"

	csp := fmt.Sprintf(
		"default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; "+
			"font-src 'self'; connect-src 'self'; frame-src %s; form-action 'self'; "+
			"base-uri 'none'; frame-ancestors 'none'; object-src 'none'; upgrade-insecure-requests",
		frameSrc,
	)

	cabecerasDeSitio := httpx.SecurityHeadersConfig{
		HSTS:                      true,
		HSTSMaxAge:                365 * 24 * time.Hour,
		HSTSIncludeSubdomains:     false,
		HSTSPreload:               false,
		FrameOptions:              "DENY",
		ReferrerPolicy:            "strict-origin-when-cross-origin",
		PermissionsPolicy:         "camera=(), microphone=(), geolocation=(), payment=()",
		CrossOriginEmbedderPolicy: "",
		ContentSecurityPolicy:     csp,
	}

	r := httpx.NewRouter(httpx.RouterOptions{
		Security: &cabecerasDeSitio,
	})

	// Sondas internas: accesibles solo directamente (sin X-Forwarded-For)
	salud := httpx.HealthHandler(version)
	r.Get("/healthz", func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("X-Forwarded-For") != "" {
			http.NotFound(w, req)
			return
		}
		salud(w, req)
	})

	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("X-Forwarded-For") != "" {
			http.NotFound(w, req)
			return
		}
		if s.lector != nil && !s.lector.Listo() {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("mapa no listo"))
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	r.Get("/metrics", func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("X-Forwarded-For") != "" {
			http.NotFound(w, req)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// Despacho de todas las peticiones públicas de sitios
	r.Handle("/*", http.HandlerFunc(s.atenderPeticion))

	// Envolver para garantizar que NUNCA se emite Set-Cookie en las webs (§8.1)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Del("Set-Cookie")
		r.ServeHTTP(w, req)
	})
}

func (s *ServidorSitios) atenderPeticion(w http.ResponseWriter, r *http.Request) {
	// Solo GET y HEAD (§8.1)
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	p := r.URL.Path
	// Prohibir prefijos reservados de la plataforma
	if strings.HasPrefix(p, "/_pages") || strings.HasPrefix(p, "/_sytue") {
		http.NotFound(w, r)
		return
	}

	// Validar formato de ruta: solo caracteres admitidos, sin .. ni //
	if !reRutaValida.MatchString(p) || strings.Contains(p, "..") || strings.Contains(p, "//") || strings.Contains(r.URL.RawPath, "%") {
		http.NotFound(w, r)
		return
	}

	// Limpiar host (minúsculas, sin puerto)
	host := strings.ToLower(r.Host)
	if idx := strings.IndexByte(host, ':'); idx != -1 {
		host = host[:idx]
	}

	// Con el dominio de clientes puesto, la dirección vieja, bajo el dominio de la suite,
	// solo redirige a la nueva: ni una página ni una vista previa de un cliente se sirve
	// ya en él (WEB-09).
	if s.cfg.SitesDomainAnterior != "" && strings.HasSuffix(host, "."+s.cfg.SitesDomainAnterior) {
		slug := strings.TrimSuffix(host, "."+s.cfg.SitesDomainAnterior)
		if strings.Contains(slug, ".") {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "https://"+slug+"."+s.cfg.SitesDomain+r.URL.RequestURI(), http.StatusMovedPermanently)
		return
	}

	if s.lector == nil {
		s.responderNoEncontrado(w, "es")
		return
	}

	siteID, site, ok := s.lector.ObtenerPorHost(host)
	if !ok {
		s.responderNoEncontrado(w, "es")
		return
	}

	lang := s.negociarIdioma(r, site.Languages, site.DefaultLanguage)

	// Un sitio nuevo aún no tiene versión live, pero sí puede autorizar una
	// preview. La suspensión bloquea también previews; retirar revoca sus tokens.
	if strings.HasPrefix(p, "/_vista/") && site.Status != "suspended" {
		s.servirVistaPrevia(w, r, siteID, site)
		return
	}

	if s.despacharEstadoSitio(w, site, lang) {
		return
	}

	// Host no canónico: 301 al canónico
	if site.Canonical != "" && host != site.Canonical {
		destino := "https://" + site.Canonical + r.URL.RequestURI()
		http.Redirect(w, r, destino, http.StatusMovedPermanently)
		return
	}

	s.despacharContenido(w, r, siteID, site, lang)
}

func (s *ServidorSitios) despacharEstadoSitio(w http.ResponseWriter, site mapa.SiteMapa, lang string) bool {
	// Estado suspended: 503 con Retry-After ANTES de cualquier regla (§8.1)
	if site.Status == "suspended" {
		w.Header().Set("Retry-After", "3600")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Robots-Tag", "noindex")
		w.WriteHeader(http.StatusServiceUnavailable)
		titulo := s.textos.Texto(lang, "suspended_titulo", nil)
		mensaje := s.textos.Texto(lang, "suspended_mensaje", nil)
		_, _ = fmt.Fprintf(w, "<!DOCTYPE html><html lang=\"%s\"><head><meta charset=\"utf-8\"><title>%s</title></head><body><h1>%s</h1><p>%s</p></body></html>", lang, titulo, titulo, mensaje)
		return true
	}

	// Estado unpublished: 404
	if site.Status == "unpublished" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Robots-Tag", "noindex")
		w.WriteHeader(http.StatusNotFound)
		titulo := s.textos.Texto(lang, "unpublished_titulo", nil)
		mensaje := s.textos.Texto(lang, "unpublished_mensaje", nil)
		_, _ = fmt.Fprintf(w, "<!DOCTYPE html><html lang=\"%s\"><head><meta charset=\"utf-8\"><title>%s</title></head><body><h1>%s</h1><p>%s</p></body></html>", lang, titulo, titulo, mensaje)
		return true
	}

	return false
}

func (s *ServidorSitios) despacharContenido(w http.ResponseWriter, r *http.Request, siteID string, site mapa.SiteMapa, lang string) {
	p := r.URL.Path

	// Raíz /: 302 al idioma negociado
	if p == "/" {
		w.Header().Set("Vary", "Accept-Language")
		http.Redirect(w, r, "/"+lang+"/", http.StatusFound)
		return
	}

	// Recursos estáticos inmutables: /_astro/<nombre>
	if strings.HasPrefix(p, "/_astro/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		s.servirRecurso(w, r, siteID, path.Base(p))
		return
	}

	// Archivos raíz especiales: sitemap.xml, robots.txt
	if p == "/sitemap.xml" || p == "/robots.txt" {
		clave := fmt.Sprintf("shared/sitios/%s/v/%d%s", siteID, site.Number, p)
		w.Header().Set("Cache-Control", "no-cache")
		s.servirObjetoS3(w, r, clave, "", false)
		return
	}

	// Páginas de contenido: exigir barra final en directorios (§8.1)
	if !strings.HasSuffix(p, "/") && !strings.Contains(path.Base(p), ".") {
		http.Redirect(w, r, p+"/", http.StatusMovedPermanently)
		return
	}

	// Servir index.html de la página
	subRuta := strings.TrimPrefix(p, "/")
	if strings.HasSuffix(subRuta, "/") {
		subRuta += "index.html"
	}
	clavePagina := fmt.Sprintf("shared/sitios/%s/v/%d/%s", siteID, site.Number, subRuta)
	w.Header().Set("Cache-Control", "no-cache")

	hRuta := sha256.Sum256([]byte(p))
	etag := fmt.Sprintf(`"v%d-%s"`, site.Number, hex.EncodeToString(hRuta[:8]))
	w.Header().Set("ETag", etag)

	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	s.servirObjetoConFallback(w, r, siteID, site.Number, lang, clavePagina, p)
}

// Los recursos live y preview comparten almacenamiento y las mismas cabeceras
// de descarga segura. El llamador decide acceso y política de caché.
func (s *ServidorSitios) servirRecurso(w http.ResponseWriter, r *http.Request, siteID, nombre string) {
	if strings.HasSuffix(nombre, ".pdf") {
		w.Header().Set("Content-Disposition", "attachment")
		w.Header().Set("Content-Security-Policy", "sandbox")
	}
	clave := fmt.Sprintf("shared/sitios/%s/recursos/%s", siteID, nombre)
	if catKey, conocido := s.catalogo.ClaveRecurso(nombre); conocido {
		clave = catKey
	}
	s.servirObjetoS3(w, r, clave, "", false)
}

func (s *ServidorSitios) servirVistaPrevia(w http.ResponseWriter, r *http.Request, siteID string, site mapa.SiteMapa) {
	resto := strings.TrimPrefix(r.URL.Path, "/_vista/")
	partes := strings.SplitN(resto, "/", 2)
	token := partes[0]
	if token == "" {
		http.NotFound(w, r)
		return
	}

	hToken := sha256.Sum256([]byte(token))
	hashKey := hex.EncodeToString(hToken[:16])

	expiraStr, existe := site.Previews[hashKey]
	if !existe {
		http.NotFound(w, r)
		return
	}

	expira, err := time.Parse(time.RFC3339, expiraStr)
	if err != nil || !time.Now().UTC().Before(expira) {
		http.NotFound(w, r)
		return
	}

	subRuta := ""
	if len(partes) > 1 {
		subRuta = partes[1]
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex")
	if subRuta == "" {
		lang := s.negociarIdioma(r, site.Languages, site.DefaultLanguage)
		w.Header().Set("Vary", "Accept-Language")
		http.Redirect(w, r, "/_vista/"+token+"/"+lang+"/", http.StatusFound)
		return
	}
	if strings.HasPrefix(subRuta, "_astro/") {
		s.servirRecurso(w, r, siteID, path.Base(subRuta))
		return
	}
	if strings.HasSuffix(subRuta, "/") {
		subRuta += "index.html"
	}

	clave := fmt.Sprintf("shared/sitios/%s/vistas/%s/%s", siteID, hashKey, subRuta)
	s.servirObjetoS3(w, r, clave, "", false)
}

func (s *ServidorSitios) servirObjetoConFallback(w http.ResponseWriter, r *http.Request, siteID string, version int, lang, clave, rutaOriginal string) {
	if s.objetos == nil {
		s.responderNoEncontrado(w, lang)
		return
	}

	tenantShared := tenancy.Tenant{Slug: "shared"}
	rc, err := s.objetos.Get(r.Context(), tenantShared, clave)
	if err == nil {
		defer rc.Close()
		s.determinarContentType(w, clave)
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, rc)
		return
	}

	// Comprobar redirecciones de la versión
	claveRed := fmt.Sprintf("shared/sitios/%s/v/%d/_redirecciones.json", siteID, version)
	if rcRed, errRed := s.objetos.Get(r.Context(), tenantShared, claveRed); errRed == nil {
		defer rcRed.Close()
		var wrapper struct {
			Redirects []struct {
				Source string `json:"source"`
				Target string `json:"target"`
				Code   int    `json:"code"`
			} `json:"redirects"`
		}
		if err := json.NewDecoder(rcRed).Decode(&wrapper); err == nil {
			for _, red := range wrapper.Redirects {
				if red.Source == rutaOriginal || red.Source+"/" == rutaOriginal {
					code := http.StatusMovedPermanently
					if red.Code == http.StatusFound {
						code = http.StatusFound
					}
					http.Redirect(w, r, red.Target, code)
					return
				}
			}
		}
	}

	// Servir 404.html de la versión en el idioma negociado
	clave404 := fmt.Sprintf("shared/sitios/%s/v/%d/%s/404.html", siteID, version, lang)
	if rc404, err404 := s.objetos.Get(r.Context(), tenantShared, clave404); err404 == nil {
		defer rc404.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.Copy(w, rc404)
		return
	}

	s.responderNoEncontrado(w, lang)
}

func (s *ServidorSitios) servirObjetoS3(w http.ResponseWriter, r *http.Request, clave, forcedContentType string, is404 bool) {
	if s.objetos == nil {
		http.NotFound(w, r)
		return
	}

	tenantShared := tenancy.Tenant{Slug: "shared"}

	// Soporte de Range si la cabecera está presente (§8.1)
	if rangeH := r.Header.Get("Range"); rangeH != "" {
		obj, err := s.objetos.GetRange(r.Context(), tenantShared, clave, rangeH)
		if err == nil {
			defer obj.Body.Close()
			if obj.ContentRange != "" {
				w.Header().Set("Content-Range", obj.ContentRange)
				w.Header().Set("Content-Length", fmt.Sprintf("%d", obj.Size))
				s.determinarContentType(w, clave)
				w.WriteHeader(http.StatusPartialContent)
			} else {
				w.Header().Set("Content-Length", fmt.Sprintf("%d", obj.Size))
				s.determinarContentType(w, clave)
				w.WriteHeader(http.StatusOK)
			}
			_, _ = io.Copy(w, obj.Body)
			return
		}
	}

	rc, err := s.objetos.Get(r.Context(), tenantShared, clave)
	if err != nil {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Del("Content-Disposition")
		if errors.Is(err, objectstore.ErrNotFound) {
			http.NotFound(w, r)
		} else {
			http.Error(w, "Servicio temporalmente no disponible", http.StatusServiceUnavailable)
		}
		return
	}
	defer rc.Close()

	if forcedContentType != "" {
		w.Header().Set("Content-Type", forcedContentType)
	} else {
		s.determinarContentType(w, clave)
	}

	if is404 {
		w.WriteHeader(http.StatusNotFound)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_, _ = io.Copy(w, rc)
}

func (s *ServidorSitios) determinarContentType(w http.ResponseWriter, clave string) {
	ext := filepath.Ext(clave)
	switch ext {
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".js", ".mjs":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case ".json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".webp":
		w.Header().Set("Content-Type", "image/webp")
	case ".woff2":
		w.Header().Set("Content-Type", "font/woff2")
	case ".pdf":
		w.Header().Set("Content-Type", "application/pdf")
	case ".xml":
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	case ".txt":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	default:
		if ct := mime.TypeByExtension(ext); ct != "" {
			w.Header().Set("Content-Type", ct)
		} else {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
	}
}

func (s *ServidorSitios) responderNoEncontrado(w http.ResponseWriter, lang string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex")
	w.WriteHeader(http.StatusNotFound)
	titulo := s.textos.Texto(lang, "not_found_titulo", nil)
	mensaje := s.textos.Texto(lang, "not_found_mensaje", nil)
	_, _ = fmt.Fprintf(w, "<!DOCTYPE html><html lang=\"%s\"><head><meta charset=\"utf-8\"><title>%s</title></head><body><h1>%s</h1><p>%s</p></body></html>", lang, titulo, titulo, mensaje)
}

func (s *ServidorSitios) negociarIdioma(r *http.Request, disponibles []string, porDefecto string) string {
	if len(disponibles) == 0 {
		return porDefecto
	}
	al := r.Header.Get("Accept-Language")
	if al == "" {
		return porDefecto
	}

	partes := strings.Split(al, ",")
	for _, p := range partes {
		tag := strings.TrimSpace(strings.Split(p, ";")[0])
		tag = strings.ToLower(tag)
		baseLang := strings.Split(tag, "-")[0]

		for _, d := range disponibles {
			if strings.EqualFold(d, tag) || strings.EqualFold(d, baseLang) {
				return d
			}
		}
	}

	return porDefecto
}
