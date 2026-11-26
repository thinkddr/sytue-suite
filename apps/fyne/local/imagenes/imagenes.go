// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package imagenes son los formatos de imagen que Sytue Office lee además de los de la web
// (PNG, JPEG y GIF de la biblioteca estándar, y WebP, que trae `nucleo/miniatura`): los
// de las fotos del teléfono, de los escáneres y de las capturas. Lo usan Photos y las
// notas de imagen de Notepads, que así leen lo mismo.
//
//   - AVIF, con `gen2brain/avif` (libavif y dav1d compilados a WASM, sin CGO), TIFF, con
//     `x/image/tiff`, y BMP, con `x/image/bmp`: se registran en `image.Decode` al importar
//     el paquete, y con ellos `miniatura.Cabecera`, `Decodificar` y `Render` los leen como
//     los demás.
//   - HEIC y HEIF no tienen decodificador en Go con una licencia admitida: se pasan a JPEG
//     con lo que ya tiene el sistema ([Convertir], [AJPEG]; heic.go).
//
// El tipo de un `.bmp` se registra además en `mime` como `image/bmp`: la tabla del sistema
// no lo trae en todas partes (el móvil no tiene, y algunos Linux dicen `image/x-ms-bmp`),
// y quien lo importa compara el tipo por la extensión con el que dice la cabecera.
package imagenes

import (
	"mime"

	// AVIF: libavif + dav1d en WASM, sin CGO (si el sistema tiene libavif, la usa).
	_ "github.com/gen2brain/avif"
	// BMP y TIFF, en el mismo x/image que ya está en el módulo (capturas de Windows,
	// escáneres, capturas de macOS).
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"

	// WebP: ya lo registra `miniatura`; aquí por si quien importa no la usa.
	_ "golang.org/x/image/webp"
)

// TipoBMP es el tipo con que se nombra un BMP: el que pone `image.RegisterFormat` («bmp»).
const TipoBMP = "image/bmp"

func init() {
	// Solo falla con una extensión sin punto o un tipo mal formado.
	_ = mime.AddExtensionType(".bmp", TipoBMP)
}

// EsBMP dice si un tipo es BMP, también con los nombres que le dan algunos sistemas
// (`image/x-ms-bmp`, el de Android y de Debian viejos; `image/x-bmp`).
func EsBMP(tipo string) bool {
	switch tipo {
	case TipoBMP, "image/x-ms-bmp", "image/x-bmp":
		return true
	}
	return false
}
