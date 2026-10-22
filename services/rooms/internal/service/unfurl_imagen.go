// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/miniatura"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// Proxy de la imagen de una vista previa de enlace.
//
// EL PROBLEMA QUE RESUELVE. Hasta ahora la `og:image` se devolvía al cliente con
// su URL original y el navegador la pedía directamente al host remoto. Eso
// significa que, por cada mensaje con vista previa, **la dirección IP de cada
// persona que lee el canal** llegaba a un tercero cualquiera —el que hubiera
// elegido quien pegó el enlace—, junto con la hora a la que lo leyó. Sin
// consentimiento, sin aviso y sin forma de evitarlo. El lector de correo de la
// casa hace justo lo contrario y lo hace bien: bloquea el contenido remoto y
// pide permiso.
//
// Y NO SE PAGA CON PEOR EXPERIENCIA, que era el motivo de no proxiar. Al pasar
// por aquí la imagen se reduce a `imagenMaxLado`: una portada de dos megas se
// convierte en decenas de kilobytes. El navegador descarga MENOS que antes, la
// vista previa aparece antes, y deja de romperse cuando el host remoto está
// lento o caído.
//
// POR QUÉ NO ES UN PROXY ABIERTO. No se acepta una URL por parámetro —eso
// convertiría el servicio en un buscador de recursos ajenos a petición de
// cualquiera—. Se acepta el HASH de un enlace, y solo se sirve si ese enlace ya
// está en la caché de vistas previas de este tenant: es decir, si alguien lo
// pegó de verdad en una conversación a la que tiene acceso. La caché de unfurl
// es la lista blanca; no hay otra puerta de entrada.
const (
	// imagenMaxLado es el lado máximo de la miniatura servida. La vista previa se
	// pinta pequeña; 512 deja margen para pantallas 2× sin acercarse al peso del
	// original, igual que la miniatura de Drive.
	imagenMaxLado = 512
	// imagenMaxBytes acota lo que se descarga del host remoto ANTES de decodificar.
	// Es la primera barrera: un fichero enorme se corta aquí aunque sus dimensiones
	// declaradas sean plausibles. La segunda —dimensiones imposibles— la pone
	// miniatura.Render.
	imagenMaxBytes = 8 << 20 // 8 MiB
	// imagenTTL es lo que vive la miniatura en la caché efímera. Más corto que el
	// TTL del unfurl a propósito: si el original cambia, se regenera antes.
	imagenTTL = 6 * time.Hour
	// imagenTipoSalida: JPEG para todo. La vista previa es una foto de portada, no
	// un logotipo con transparencia, y JPEG pesa bastante menos.
	imagenTipoSalida = "image/jpeg"
)

// HashDeEnlace es el identificador público de una vista previa. Reusa
// domain.HashToken, que es EXACTAMENTE el que el store guarda en `url_hash`:
// con dos funciones distintas, el proxy nunca encontraría la fila y la vista
// previa se quedaría sin imagen sin que nada fallara.
func HashDeEnlace(rawURL string) string {
	return domain.HashToken(strings.TrimSpace(rawURL))
}

// ImagenDeUnfurl devuelve la miniatura de la vista previa identificada por
// `hash`, junto con su content-type y su ETag.
//
// Devuelve domain.ErrNotFound si ese hash no corresponde a ningún enlace que
// este tenant tenga en caché, o si el enlace no traía imagen. Nunca sale a
// buscar una URL que no esté ya guardada.
func (s *Service) ImagenDeUnfurl(ctx context.Context, hash string) ([]byte, string, string, error) {
	hash = strings.ToLower(strings.TrimSpace(hash))
	if len(hash) != 64 {
		return nil, "", "", domain.ErrInvalidInput
	}

	// LISTA BLANCA: el enlace tiene que estar ya en la caché de vistas previas.
	// Aquí es donde se corta el proxy abierto.
	u, ok, err := s.pstore.GetUnfurlByHash(ctx, hash)
	if err != nil {
		return nil, "", "", err
	}
	if !ok || strings.TrimSpace(u.ImageURL) == "" {
		return nil, "", "", domain.ErrNotFound
	}

	// Caché efímera, DESPUÉS de la lista blanca y CON EL TENANT EN LA CLAVE. Las
	// dos cosas importan y por el mismo motivo: el hash es solo de la URL, así
	// que dos organizaciones que peguen el mismo enlace comparten hash. Si se
	// consultara la caché antes de comprobar la fila, o con una clave sin tenant,
	// bastaría con adivinar un hash para saltarse la lista blanca entera y hacer
	// que el servidor sirviera una imagen por un enlace que nadie pegó aquí.
	//
	// Es Dragonfly, o sea almacenamiento efímero, que es donde el invariante de
	// la casa permite meter cosas: una miniatura perdida se vuelve a generar del
	// origen sin que nadie note nada.
	t, _ := tenancy.From(ctx)
	claveCache := "rooms:unfurl:img:" + t.Slug + ":" + hash
	if s.efimero != nil && s.efimero.Enabled() {
		if val, ok, err := s.efimero.Get(ctx, claveCache); err == nil && ok && val != "" {
			return []byte(val), imagenTipoSalida, `"` + hash + `"`, nil
		}
	}

	// Barrera SSRF: el host de la imagen puede no ser el del enlace, y una
	// redirección puede llevar a cualquier sitio. Se valida igual que el unfurl.
	if err := validateOutgoingURL(ctx, u.ImageURL); err != nil {
		return nil, "", "", domain.ErrNotFound
	}

	datos, err := s.descargarImagen(ctx, u.ImageURL)
	if err != nil {
		return nil, "", "", domain.ErrNotFound
	}
	thumb, err := miniatura.Render(datos, imagenMaxLado, imagenTipoSalida)
	if err != nil {
		// Lo que había en esa URL no es una imagen que sepamos decodificar. Es un
		// dato de un tercero, no un fallo nuestro: hueco, no error de servidor.
		return nil, "", "", domain.ErrNotFound
	}

	if s.efimero != nil && s.efimero.Enabled() {
		// Best effort: si la caché falla se sirve igual y se regenerará.
		_ = s.efimero.SetTTL(ctx, claveCache, string(thumb), imagenTTL)
	}
	return thumb, imagenTipoSalida, `"` + hash + `"`, nil
}

// descargarImagen trae la imagen con el cliente SSRF-safe y el cuerpo acotado.
func (s *Service) descargarImagen(ctx context.Context, rawURL string) ([]byte, error) {
	client := s.unfurl
	if client == nil {
		client = unfurlClient()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "image/*")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("service: la imagen respondió " + resp.Status)
	}
	// +1 para detectar el desbordamiento sin confiar en Content-Length, que el
	// remoto puede mentir o no enviar.
	datos, err := io.ReadAll(io.LimitReader(resp.Body, imagenMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(datos) > imagenMaxBytes {
		return nil, errors.New("service: imagen demasiado grande")
	}
	return datos, nil
}
