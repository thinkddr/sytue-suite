// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// UNFURL de enlaces (Ola 16): cuando un cliente pide la vista previa de una URL
// http/https (GET /unfurl?url=), se obtienen sus metadatos OpenGraph UNA vez y se
// cachean por tenant (tabla link_unfurls). Peticiones posteriores reusan la fila sin
// re-fetch mientras siga fresca (TTL). El fetch remoto REUSA la barrera SSRF de la
// Ola 15 (validateOutgoingURL + el dialer de ssrfSafeTransport): jamás llega a un
// destino privado/loopback/link-local/ULA/CGNAT, ni siquiera vía redirect.
//
// IMAGEN REMOTA (og:image): NO se descarga aquí. Se devuelve la URL al cliente, que
// la pinta con referrerpolicy=no-referrer (sin filtrar el Referer ni credenciales) y
// loading=lazy. No hay proxy de imagen (evita coste de banda/almacenamiento a
// escala); a cambio, el navegador del usuario contacta el host de la imagen. Solo se
// aceptan URLs de imagen http/https (nunca data:/javascript:).

const (
	unfurlMaxRespBytes = 512 << 10      // cap del HTML leído (el <head> cabe de sobra)
	unfurlTTL          = 24 * time.Hour // frescura de la caché antes de re-fetch
	unfurlMaxRedirects = 5              // saltos permitidos (cada uno re-validado por el dialer)
	unfurlUserAgent    = "Sytue-Rooms-Unfurl/1"
)

// unfurlClient construye el cliente HTTP del unfurl: MISMA barrera de dialer que los
// webhooks salientes (ssrfSafeTransport), pero SIGUE redirects acotados. Es seguro
// porque el dialer re-resuelve y valida las IPs en CADA salto (barrera 2), y además
// re-validamos la URL de cada redirect (barrera 1) — un redirect a un destino privado
// se corta igual que la petición original.
func unfurlClient() *http.Client {
	return &http.Client{
		Timeout: outgoingTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= unfurlMaxRedirects {
				return errors.New("unfurl: demasiados redirects")
			}
			if err := validateOutgoingURL(req.Context(), req.URL.String()); err != nil {
				return err
			}
			return nil
		},
		Transport: ssrfSafeTransport(),
	}
}

// Unfurl devuelve la vista previa (OpenGraph) de una URL http/https. Sirve la caché
// si está fresca; si no, valida (barrera SSRF 1), la obtiene con el cliente SSRF-safe,
// parsea los metadatos OG y cachea el resultado (incluido el NEGATIVO: una URL sin
// metadatos se recuerda para no re-fetchearla). Nunca contacta destinos privados.
func (s *Service) Unfurl(ctx context.Context, rawURL string) (domain.LinkUnfurl, error) {
	rawURL = strings.TrimSpace(rawURL)
	if !domain.ValidCallbackURL(rawURL) { // http/https + host (reusa el validador de Ola 15)
		return domain.LinkUnfurl{}, domain.ErrInvalidInput
	}
	// Caché fresca: no re-fetch (dedupe a escala; indexado por url_hash).
	if cached, ok, err := s.pstore.GetUnfurl(ctx, rawURL); err != nil {
		return domain.LinkUnfurl{}, err
	} else if ok && time.Since(cached.FetchedAt) < unfurlTTL {
		return cached, nil
	}
	// Barrera SSRF 1: valida esquema + que TODAS las IPs del host sean públicas.
	if err := validateOutgoingURL(ctx, rawURL); err != nil {
		return domain.LinkUnfurl{}, domain.ErrInvalidInput
	}
	u := s.fetchUnfurl(ctx, rawURL)
	// Cachea SIEMPRE (también el negativo): evita re-fetch dentro del TTL. Best-effort:
	// un fallo de caché no impide devolver el resultado ya obtenido.
	if err := s.pstore.UpsertUnfurl(ctx, u); err != nil {
		return u, nil //nolint:nilerr // best-effort: un fallo de cache no impide devolver el unfurl ya obtenido
	}
	return u, nil
}

// fetchUnfurl hace UNA petición GET SSRF-safe, exige Content-Type text/html, lee el
// cuerpo con tope y parsea los metadatos OG. Devuelve un unfurl (posiblemente vacío)
// con URL/FetchedAt fijados; nunca error (un fallo remoto = unfurl vacío cacheado).
func (s *Service) fetchUnfurl(ctx context.Context, rawURL string) domain.LinkUnfurl {
	empty := domain.LinkUnfurl{URL: rawURL, FetchedAt: time.Now()}
	client := s.unfurl
	if client == nil {
		client = unfurlClient()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return empty
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("User-Agent", unfurlUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return empty
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return empty
	}
	// Solo HTML: no parseamos imágenes/binarios ni descargamos recursos ajenos.
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(strings.ToLower(ct), "text/html") {
		return empty
	}
	u := domain.ParseOpenGraph(io.LimitReader(resp.Body, unfurlMaxRespBytes))
	u.URL = rawURL
	u.FetchedAt = time.Now()
	u.ImageURL = safeImageURL(u.ImageURL)
	return u
}

// safeImageURL solo deja pasar URLs de imagen http/https absolutas (nunca data:/
// javascript:/relativas sin esquema): el navegador del cliente las cargará
// directamente, así que se filtran esquemas peligrosos aquí. Vacío si no es válida.
func safeImageURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return raw
}
