// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package logoorg trae el logotipo de la organización desde Sytue ID para incrustarlo
// en los documentos que llevan su marca.
//
// El logo vive en Sytue ID y este servicio NO lo guarda: lo pide, servidor a servidor, al
// endpoint que ID ya publica sin sesión (`GET /branding/{slug}/logo.png`), que sirve un
// PNG reencuadrado y recodificado por ID, nunca el fichero que subió nadie. No viaja
// ninguna credencial —es el mismo recurso que pinta la barra de cada app—, así que no hay
// token que pueda filtrarse ni audiencia que equivocar. Lo que sí se cuida aquí:
//
//   - De quién es: el slug es SIEMPRE el de la organización de la sesión (lo pone quien
//     llama desde `tenancy`), se valida y la caché va por slug: una organización no puede
//     pedir ni recibir el logo de otra.
//   - Adónde se pide: a la base configurada, sin seguir redirecciones.
//   - Qué se acepta: `image/png`, hasta 512 KiB y 4 megapíxeles; se DECODIFICA y se vuelve a
//     codificar aquí, así que lo que se incrusta son píxeles nuestros. Un SVG (con o sin
//     guiones), un HTML o cualquier otra cosa no decodifica como PNG y se descarta, diga lo
//     que diga su cabecera.
//   - Cada cuánto: caché en memoria por slug, revalidada con el ETag de ID (su hash del
//     logo) cada cinco minutos; la versión que ve el documento es el hash de lo incrustado.
package logoorg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	maxBytes   = 512 << 10 // lo que se acepta leer de ID
	maxLado    = 4096      // px por lado
	maxPixeles = 4 << 20   // tope de la imagen decodificada (bomba de descompresión)
	vidaCache  = 5 * time.Minute
	maxEntrada = 2000 // organizaciones en caché; al pasarlo se vacía entera
	plazo      = 5 * time.Second
)

// ErrSinLogo: la organización no tiene logotipo propio (o el slug no es válido).
var ErrSinLogo = errors.New("logoorg: sin logotipo")

// ErrNoValido: ID contestó algo que no es un PNG aceptable. No se incrusta nada.
var ErrNoValido = errors.New("logoorg: respuesta no válida")

// Logo es el logotipo listo para incrustar.
type Logo struct {
	PNG   []byte // recodificado aquí
	Hash  string // versión: sha256 (16 hex) de PNG
	Ancho int
	Alto  int
}

var reSlug = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

type entrada struct {
	logo   *Logo // nil = la organización no tiene
	etag   string
	hastaT time.Time
}

// Cliente pide y cachea logotipos. El cero no vale: usar Nuevo.
type Cliente struct {
	base  string
	http  *http.Client
	ahora func() time.Time

	mu    sync.Mutex
	cache map[string]entrada
}

// Nuevo crea el cliente contra la base de Sytue ID («https://id.sytue.eu» o su URL
// interna). No sigue redirecciones: la respuesta es la de esa base o ninguna.
func Nuevo(base string) *Cliente {
	return &Cliente{
		base:  strings.TrimRight(base, "/"),
		ahora: time.Now,
		cache: map[string]entrada{},
		http: &http.Client{
			Timeout:       plazo,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// Logo devuelve el logotipo de la organización `slug`. ErrSinLogo si no tiene; ErrNoValido
// si ID contesta algo inaceptable. Si ID no responde y hay un logo en caché, se sirve ese:
// un documento no debe quedarse sin su marca por un parpadeo de red.
func (c *Cliente) Logo(ctx context.Context, slug string) (*Logo, error) {
	if c == nil || c.base == "" || !reSlug.MatchString(slug) {
		return nil, ErrSinLogo
	}
	c.mu.Lock()
	e, hay := c.cache[slug]
	c.mu.Unlock()
	if hay && c.ahora().Before(e.hastaT) {
		return resultado(e)
	}
	nueva, err := c.pedir(ctx, slug, e)
	if err != nil {
		if hay && e.logo != nil && !errors.Is(err, ErrNoValido) {
			return e.logo, nil
		}
		return nil, err
	}
	nueva.hastaT = c.ahora().Add(vidaCache)
	c.mu.Lock()
	if len(c.cache) >= maxEntrada {
		c.cache = map[string]entrada{}
	}
	c.cache[slug] = nueva
	c.mu.Unlock()
	return resultado(nueva)
}

func resultado(e entrada) (*Logo, error) {
	if e.logo == nil {
		return nil, ErrSinLogo
	}
	return e.logo, nil
}

func (c *Cliente) pedir(ctx context.Context, slug string, previa entrada) (entrada, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/branding/"+url.PathEscape(slug)+"/logo.png", http.NoBody)
	if err != nil {
		return entrada{}, err
	}
	if previa.logo != nil && previa.etag != "" {
		req.Header.Set("If-None-Match", previa.etag)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return entrada{}, fmt.Errorf("logoorg: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	switch res.StatusCode {
	case http.StatusNotFound:
		return entrada{}, nil // sin logo: se recuerda, para no preguntar en cada documento
	case http.StatusNotModified:
		if previa.logo != nil {
			return entrada{logo: previa.logo, etag: previa.etag}, nil
		}
		return entrada{}, fmt.Errorf("%w: 304 sin copia", ErrNoValido)
	case http.StatusOK:
	default:
		// Incluye cualquier 3xx: no se siguen redirecciones.
		return entrada{}, fmt.Errorf("logoorg: estado %d", res.StatusCode)
	}
	if tipo, _, _ := strings.Cut(res.Header.Get("Content-Type"), ";"); strings.TrimSpace(strings.ToLower(tipo)) != "image/png" {
		return entrada{}, fmt.Errorf("%w: tipo %q", ErrNoValido, res.Header.Get("Content-Type"))
	}
	crudo, err := io.ReadAll(io.LimitReader(res.Body, maxBytes+1))
	if err != nil {
		return entrada{}, fmt.Errorf("logoorg: %w", err)
	}
	if len(crudo) > maxBytes {
		return entrada{}, fmt.Errorf("%w: más de %d bytes", ErrNoValido, maxBytes)
	}
	logo, err := recodificar(crudo)
	if err != nil {
		return entrada{}, err
	}
	return entrada{logo: logo, etag: res.Header.Get("ETag")}, nil
}

// recodificar valida que `crudo` es un PNG de tamaño razonable y lo vuelve a codificar:
// lo que sale son solo píxeles (ni metadatos, ni trozos ajenos, ni nada que no sea imagen).
func recodificar(crudo []byte) (*Logo, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(crudo))
	if err != nil {
		return nil, fmt.Errorf("%w: no es un PNG", ErrNoValido)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxLado || cfg.Height > maxLado || cfg.Width*cfg.Height > maxPixeles {
		return nil, fmt.Errorf("%w: %d×%d px", ErrNoValido, cfg.Width, cfg.Height)
	}
	img, err := png.Decode(bytes.NewReader(crudo))
	if err != nil {
		return nil, fmt.Errorf("%w: PNG ilegible", ErrNoValido)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, fmt.Errorf("logoorg: %w", err)
	}
	suma := sha256.Sum256(b.Bytes())
	return &Logo{PNG: b.Bytes(), Hash: hex.EncodeToString(suma[:8]), Ancho: cfg.Width, Alto: cfg.Height}, nil
}
