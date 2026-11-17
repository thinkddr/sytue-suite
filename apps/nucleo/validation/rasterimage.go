// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package validation

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Validación de avatares/fotos que viajan como data URI raster (RFC 2397):
// Contactos los incrusta en la PHOTO de una vCard, Sytue ID los guarda en
// user_avatars. Las dos rutas comparten el mismo riesgo (SVG con <script>,
// tipo declarado que no coincide con el contenido, payload sin tope) y el
// mismo límite, así que la validación vive una vez aquí.

// MaxRasterImageBytes acota el tamaño de la imagen DECODIFICADA (no el
// base64). El cliente redimensiona antes de subir; el servidor revalida el
// límite como frontera de confianza. 256 KiB es de sobra para un avatar (el
// cliente apunta a mucho menos, ~64 KiB tras redimensionar a 256px).
const MaxRasterImageBytes = 256 * 1024

// ErrInvalidRasterImage se devuelve cuando la imagen no es un data URI raster
// válido o supera el límite de tamaño.
var ErrInvalidRasterImage = errors.New("validation: imagen inválida")

// rasterImageMIME son los formatos raster admitidos (sin SVG: es texto/vector
// y puede portar script). El sniff de bytes debe coincidir con la etiqueta.
var rasterImageMIME = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
}

// RasterImageDataURI valida un data URI de imagen: prefijo
// data:image/<t>;base64, con <t> en la lista blanca raster, base64
// decodificable, tamaño <= MaxRasterImageBytes y bytes cuyo tipo real (sniff)
// coincide con la etiqueta. Devuelve el data URI normalizado (canónico, sin
// espacios) listo para persistir.
func RasterImageDataURI(dataURI string) (string, error) {
	dataURI = strings.TrimSpace(dataURI)
	const p = "data:"
	if !strings.HasPrefix(strings.ToLower(dataURI), p) {
		return "", fmt.Errorf("%w: no es un data URI", ErrInvalidRasterImage)
	}
	comma := strings.IndexByte(dataURI, ',')
	if comma < 0 {
		return "", fmt.Errorf("%w: data URI sin coma", ErrInvalidRasterImage)
	}
	meta := dataURI[len(p):comma] // p.ej. "image/jpeg;base64"
	payload := dataURI[comma+1:]
	if !strings.Contains(strings.ToLower(meta), ";base64") {
		return "", fmt.Errorf("%w: solo se admite base64", ErrInvalidRasterImage)
	}
	mime := strings.ToLower(strings.TrimSpace(strings.SplitN(meta, ";", 2)[0]))
	if !rasterImageMIME[mime] {
		return "", fmt.Errorf("%w: tipo %q no permitido (solo JPEG/PNG/WebP/GIF)", ErrInvalidRasterImage, mime)
	}
	// El base64 en un data URI puede llegar sin relleno o con saltos si se plegó;
	// se limpian los espacios antes de decodificar.
	clean := strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, payload)
	raw, err := base64.StdEncoding.DecodeString(clean)
	if err != nil {
		return "", fmt.Errorf("%w: base64 ilegible", ErrInvalidRasterImage)
	}
	if len(raw) == 0 {
		return "", fmt.Errorf("%w: imagen vacía", ErrInvalidRasterImage)
	}
	if len(raw) > MaxRasterImageBytes {
		return "", fmt.Errorf("%w: %d bytes supera el máximo de %d", ErrInvalidRasterImage, len(raw), MaxRasterImageBytes)
	}
	// Sniff de bytes: el contenido real debe ser el raster declarado (no un SVG ni
	// texto disfrazado). http.DetectContentType reconoce jpeg/png/webp/gif.
	if sniff := http.DetectContentType(raw); !sameRasterType(sniff, mime) {
		return "", fmt.Errorf("%w: el contenido (%s) no coincide con %s", ErrInvalidRasterImage, sniff, mime)
	}
	return "data:" + mime + ";base64," + clean, nil
}

// sameRasterType compara el tipo detectado con el declarado ignorando parámetros.
func sameRasterType(sniffed, declared string) bool {
	s := strings.ToLower(strings.TrimSpace(strings.SplitN(sniffed, ";", 2)[0]))
	return s == declared
}

// RasterImageBytes decodifica el data URI de una imagen a sus bytes y tipo
// MIME. Se usa para servirla cruda (avatar) sin exponer el data URI entero.
func RasterImageBytes(dataURI string) ([]byte, string, error) {
	norm, err := RasterImageDataURI(dataURI)
	if err != nil {
		return nil, "", err
	}
	comma := strings.IndexByte(norm, ',')
	mime := strings.TrimSuffix(strings.TrimPrefix(norm[:comma], "data:"), ";base64")
	raw, err := base64.StdEncoding.DecodeString(norm[comma+1:])
	if err != nil {
		return nil, "", fmt.Errorf("%w: base64 ilegible", ErrInvalidRasterImage)
	}
	return raw, mime, nil
}
