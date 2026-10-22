// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package externo baja lo que está FUERA de la suite: la imagen de una nota de imagen, y
// lo que otra app necesite de una URL que ha puesto una persona.
//
// Un navegador pone solo los topes de un `<img src>`; un cliente propio los tiene que
// poner a mano, y aquí están una vez para todas las apps:
//
//   - solo http y https, también en cada redirección;
//   - sin credenciales: ni la sesión de la suite ni cookies viajan a un tercero;
//   - con tiempo y tamaño máximos, y el cuerpo leído con tope, no entero;
//   - una imagen se DECODIFICA solo si sus dimensiones caben: la cabecera de un PNG de
//     pocos bytes puede pedir gigas de memoria al descomprimirse.
package externo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	// Los formatos de imagen de la web: PNG, JPEG y GIF de la biblioteca estándar, y WebP.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

// Los topes.
const (
	// MaxBytes es lo más que se lee de una respuesta.
	MaxBytes = 16 << 20
	// MaxPixeles es la imagen más grande que se decodifica: 40 megapíxeles, 160 MB en
	// RGBA. Una foto de móvil son 12.
	MaxPixeles = 40_000_000
	// Tiempo es lo más que espera una descarga entera.
	Tiempo = 20 * time.Second
	// maxSaltos son las redirecciones que se siguen.
	maxSaltos = 5
)

// Los fallos que quien llama puede distinguir.
var (
	ErrNoWeb    = errors.New("externo: la dirección no es http ni https")
	ErrGrande   = errors.New("externo: pasa del tope")
	ErrNoImagen = errors.New("externo: no es una imagen que se sepa leer")
)

// cliente es el de todas las descargas: sin tarro de cookies, con su tiempo y sin
// redirecciones fuera de la web.
var cliente = &http.Client{
	Timeout: Tiempo,
	CheckRedirect: func(r *http.Request, previas []*http.Request) error {
		if len(previas) >= maxSaltos {
			return fmt.Errorf("externo: más de %d redirecciones", maxSaltos)
		}
		if !esWeb(r.URL) {
			return ErrNoWeb
		}
		return nil
	},
}

func esWeb(u *url.URL) bool {
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// Bajar trae el cuerpo de una URL http(s) de fuera, hasta `tope` bytes, y su tipo.
func Bajar(ctx context.Context, direccion string, tope int64) ([]byte, string, error) {
	u, err := url.Parse(direccion)
	if err != nil || !esWeb(u) {
		return nil, "", ErrNoWeb
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, "", err
	}
	resp, err := cliente.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("externo: %s responde %d", u.Host, resp.StatusCode)
	}
	if resp.ContentLength > tope {
		return nil, "", ErrGrande
	}
	cuerpo, err := io.ReadAll(io.LimitReader(resp.Body, tope+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(cuerpo)) > tope {
		return nil, "", ErrGrande
	}
	return cuerpo, resp.Header.Get("Content-Type"), nil
}

// Imagen baja y decodifica una imagen. El formato lo dice su contenido, no la extensión
// ni el `Content-Type`, que un servidor puede equivocar; pero una página HTML se
// descarta antes de mirarla.
func Imagen(ctx context.Context, direccion string) (image.Image, error) {
	cuerpo, tipo, err := Bajar(ctx, direccion, MaxBytes)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(tipo, "text/") {
		return nil, ErrNoImagen
	}
	return Decodificar(cuerpo)
}

// Decodificar lee una imagen de unos bytes, mirando antes que sus dimensiones caben.
func Decodificar(b []byte) (image.Image, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, ErrNoImagen
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > MaxPixeles {
		return nil, ErrGrande
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, ErrNoImagen
	}
	return img, nil
}
