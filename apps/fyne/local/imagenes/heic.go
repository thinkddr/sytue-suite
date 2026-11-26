// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package imagenes

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/carrete"
	"git.sytue.eu/sytue/one/apps/nucleo/local/conversor"
)

// # HEIC
//
// No hay decodificador de HEIC en Go puro con una licencia que admita CONVENTIONS §12:
// los que hay (`gen2brain/heic`, `jdeng/goheif`, `strukturag/libheif`) envuelven libheif y
// libde265, que son LGPL-3.0, y HEVC no tiene otra implementación libre permisiva. Así que
// una foto HEIC se pasa a JPEG con lo que ya trae el SISTEMA, como hace Cast con el vídeo
// (un proceso aparte, nunca enlazado), y lo que entra en la fototeca o en la nota es ese
// JPEG:
//
//  1. el del propio sistema (`carrete.Convertir`: Image I/O en iOS y macOS, el
//     decodificador de Android 9+), que conserva EXIF y GPS;
//  2. en macOS sin ese puente, `sips` (viene con el sistema);
//  3. en Linux, `heif-convert` (libheif-examples) si está instalado;
//  4. y si no, el `ffmpeg` que la persona tenga (el mismo que busca Cast,
//     `conversor.Buscar`), que no copia los metadatos: la foto entra con la fecha del
//     fichero.
//
// Sin nada de eso (Windows sin ffmpeg, un Linux pelado), una foto HEIC no se puede leer y
// se dice ([ErrSinHEIC]).

// ErrSinHEIC: este equipo no tiene con qué convertir una foto HEIC.
var ErrSinHEIC = errors.New("imagenes: este equipo no sabe abrir fotos HEIC (instala ffmpeg o libheif)")

// EsHEIC dice si un nombre o un tipo es HEIC o HEIF.
func EsHEIC(nombreOTipo string) bool {
	s := strings.ToLower(nombreOTipo)
	return strings.HasSuffix(s, ".heic") || strings.HasSuffix(s, ".heif") || s == "image/heic" || s == "image/heif" ||
		s == "image/heic-sequence" || s == "image/heif-sequence"
}

// buscarEjecutable es exec.LookPath; las pruebas lo cambian.
var buscarEjecutable = exec.LookPath

// HayConversorHEIC dice si alguna de las cuatro vías existe en este equipo, sin
// convertir nada (para no ofrecer lo que luego fallará).
func HayConversorHEIC() bool {
	switch runtime.GOOS {
	case "ios", "android":
		return carrete.Disponible()
	case "darwin":
		if _, err := buscarEjecutable("sips"); err == nil {
			return true
		}
	case "linux":
		if _, err := buscarEjecutable("heif-convert"); err == nil {
			return true
		}
	}
	h, err := conversor.Buscar("")
	return err == nil && h.Hay()
}

// Convertir deja en un temporal la foto HEIC de ruta en JPEG y dice dónde; `limpiar` lo
// borra y hay que llamarla también con error.
func Convertir(ctx context.Context, ruta string) (string, func(), error) {
	if jpeg, limpiar, err := carrete.Convertir(ctx, ruta); err == nil {
		return jpeg, limpiar, nil
	} else if limpiar != nil {
		limpiar()
	}
	dir, err := os.MkdirTemp("", "sytue-heic-*")
	if err != nil {
		return "", func() {}, err
	}
	limpiar := func() { _ = os.RemoveAll(dir) }
	salida := filepath.Join(dir, strings.TrimSuffix(filepath.Base(ruta), filepath.Ext(ruta))+".jpg")
	for _, orden := range ordenesHEIC(ruta, salida) {
		cmd := exec.CommandContext(ctx, orden[0], orden[1:]...) //nolint:gosec // ejecutables del sistema hallados en el PATH
		if err := cmd.Run(); err != nil {
			continue
		}
		if fi, err := os.Stat(salida); err == nil && fi.Size() > 0 {
			return salida, limpiar, nil
		}
	}
	limpiar()
	return "", func() {}, ErrSinHEIC
}

// AJPEG convierte los bytes de una HEIC que llega de fuera («Abrir con», una dirección)
// y devuelve el nombre con `.jpg` y los bytes del JPEG.
func AJPEG(ctx context.Context, nombre string, datos []byte) (string, []byte, error) {
	dir, err := os.MkdirTemp("", "sytue-heic-*")
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	ext := filepath.Ext(nombre)
	if !EsHEIC(ext) {
		ext = ".heic" // los conversores miran la extensión
	}
	entrada := filepath.Join(dir, "entrada"+ext)
	if err := os.WriteFile(entrada, datos, 0o600); err != nil {
		return "", nil, err
	}
	jpeg, limpiar, err := Convertir(ctx, entrada)
	defer limpiar()
	if err != nil {
		return "", nil, err
	}
	salida, err := os.ReadFile(jpeg) //nolint:gosec // el temporal que acaba de escribir la conversión
	if err != nil {
		return "", nil, err
	}
	return strings.TrimSuffix(filepath.Base(nombre), filepath.Ext(nombre)) + ".jpg", salida, nil
}

// ordenesHEIC son las órdenes que se prueban, en orden, con la ruta del ejecutable ya
// resuelta; las que no están en este equipo no salen.
func ordenesHEIC(entrada, salida string) [][]string {
	var out [][]string
	if runtime.GOOS == "darwin" {
		if p, err := buscarEjecutable("sips"); err == nil {
			out = append(out, []string{p, "-s", "format", "jpeg", entrada, "--out", salida})
		}
	}
	if runtime.GOOS == "linux" {
		if p, err := buscarEjecutable("heif-convert"); err == nil {
			out = append(out, []string{p, "-q", "92", entrada, salida})
		}
	}
	if h, err := conversor.Buscar(""); err == nil && h.Hay() {
		out = append(out, []string{
			h.FFmpeg, "-hide_banner", "-nostdin", "-y", "-loglevel", "error",
			"-i", entrada, "-frames:v", "1", "-q:v", "2", "-f", "image2", "-c:v", "mjpeg", salida,
		})
	}
	return out
}
