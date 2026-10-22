// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
)

// # Elegir un fichero en el móvil
//
// En escritorio `ficheros.Elegir` abre el panel del sistema y devuelve rutas. En el
// móvil no hay programa que lo abra: el selector lo da el toolkit (el de documentos de
// Android, el de Archivos de iOS) y devuelve `content://`. Sin nada más, «Abrir un
// archivo», «Escanear», «Adjuntar» o «Insertar imagen» no hacían nada en el teléfono.
// Aquí se abre ese selector y lo elegido se copia a la carpeta temporal de la app, así
// que las doce llamadas a `ficheros.Elegir` reciben rutas como en escritorio sin tocar
// ninguna (elegir_movil.go).
//
// Cuando todo lo que se acepta es vídeo, o todo imagen, el filtro va por TIPO
// (`video/*`, `image/*`) y no por extensión: así el selector abre la GALERÍA —Fotos y
// Vídeos en Android—, que es de donde se sube un vídeo en un teléfono. Por extensión no
// servía: el `mime` de Go en Android no conoce `.mp4` ni `.mov`, y Fyne, sin tipo para
// una extensión, lo abre a «cualquier cosa».
//
// ponytail: uno cada vez (el selector del toolkit no elige varios) y sin cámara directa
// —el `capture="environment"` con el que la web abre la cámara al «Escanear»—; se
// relevan con un selector propio, que pide una clase de Java en la APK.

// copiarElegido deja lo que el selector ha dado en una carpeta temporal propia, con su
// nombre, y devuelve la ruta. El nombre se queda con su última parte.
func copiarElegido(r fyne.URIReadCloser) (string, error) {
	defer func() { _ = r.Close() }()
	dir, err := os.MkdirTemp("", prefijoElegido)
	if err != nil {
		return "", err
	}
	nombre := filepath.Base(strings.ReplaceAll(r.URI().Name(), `\`, "/"))
	if nombre == "." || nombre == ".." || nombre == string(filepath.Separator) || nombre == "" {
		nombre = "fichero"
	}
	ruta := filepath.Join(dir, nombre)
	f, err := os.Create(ruta) //nolint:gosec // G304: dentro de la carpeta recién creada
	if err != nil {
		return "", err
	}
	_, err = io.Copy(f, r)
	if err = errors.Join(err, f.Close()); err != nil {
		return "", errors.Join(err, os.RemoveAll(dir))
	}
	return ruta, nil
}

// tiposMovil es el filtro por tipo que toca a esas extensiones, o nil si mezclan (y
// entonces se filtra por extensión).
func tiposMovil(extensiones []string) []string {
	if len(extensiones) == 0 {
		return nil
	}
	familia := ""
	for _, e := range extensiones {
		f, ok := familias[strings.ToLower(strings.TrimPrefix(e, "."))]
		if !ok || (familia != "" && f != familia) {
			return nil
		}
		familia = f
	}
	return []string{familia + "/*"}
}

// familias son las extensiones que la suite pide al elegir y de qué tipo son.
var familias = map[string]string{
	"mp4": "video", "m4v": "video", "mov": "video", "mkv": "video", "webm": "video", "avi": "video",
	"mpg": "video", "mpeg": "video", "ogv": "video", "3gp": "video",
	"jpg": "image", "jpeg": "image", "png": "image", "webp": "image", "gif": "image", "heic": "image",
	"heif": "image", "avif": "image", "bmp": "image", "tif": "image", "tiff": "image",
}

// prefijoElegido es el de las carpetas temporales de copiarElegido.
const prefijoElegido = "sytue-elegido-"

// SoltarElegido borra la copia que dejó el selector del teléfono, si `ruta` es una; con
// cualquier otra ruta no hace nada. Es para quien sabe cuándo ya no la necesita: un vídeo
// de gigas subido a Cast no puede quedarse duplicado en el teléfono.
func SoltarElegido(ruta string) {
	dir := filepath.Dir(ruta)
	if filepath.Dir(dir) == filepath.Clean(os.TempDir()) && strings.HasPrefix(filepath.Base(dir), prefijoElegido) {
		_ = os.RemoveAll(dir)
	}
}
