// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package videos mide un vídeo del equipo para llevarlo a la fototeca de Sytue Office
// (FO-09): su duración, sus lados y un fotograma de póster, con lo que haya. Lo usan
// Photos, al enlazar o copiar un vídeo, y el Hub, al traer los de una copia de Sytue
// («Traer desde Sytue», `nucleo/local/traer`), que así entran igual.
//
// Con lo que ya usa Cast: la duración y los lados de un WebM y de un MP4/MOV se leen en
// Go (`video.Abrir`, `conversor.SondearMP4`); lo demás, con el descodificador del sistema
// si quien llama lo ha registrado (`video/sistema`) o con el ffprobe del equipo. El
// póster sale del reproductor propio para WebM (`video.Fotograma`), del ffmpeg del
// equipo (`local/conversor`) o de la miniatura que da el sistema (`local/videoteca`: la
// del gestor de ficheros, Quick Look, `MediaMetadataRetriever`).
package videos

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/video"
	"git.sytue.eu/sytue/one/apps/nucleo/local/conversor"
	"git.sytue.eu/sytue/one/apps/nucleo/local/videoteca"
)

// Medido es lo que se sabe de un vídeo. Lo que no se ha podido saber va a cero (sin
// póster, la fototeca pone uno neutro).
type Medido struct {
	Poster      []byte
	Duracion    time.Duration
	Ancho, Alto int
}

// Medir saca la duración, los lados y un póster del vídeo de ruta, con lo que haya.
func Medir(ctx context.Context, ruta string) Medido {
	var m Medido
	if f, err := os.Open(ruta); err == nil { //nolint:gosec // G304: un vídeo que la persona lleva a la fototeca
		if w, err := video.Abrir(f); err == nil {
			m.Duracion = w.Duracion
			if v := w.Pista(video.TipoVideo); v != nil {
				m.Ancho, m.Alto = v.Ancho, v.Alto
			}
		}
		_ = f.Close()
	}
	if m.Duracion == 0 || m.Ancho == 0 {
		if in, err := conversor.SondearMP4(ruta); err == nil {
			m.completar(in.Duracion, in.Ancho, in.Alto)
		}
	}
	if (m.Duracion == 0 || m.Ancho == 0) && video.HaySistema() {
		if s, err := video.AbrirConSistema(ruta); err == nil {
			in := s.Info()
			m.completar(in.Duracion, in.Ancho, in.Alto)
			_ = s.Close()
		}
	}
	herr, hayFFmpeg := conversor.Buscar("")
	if (m.Duracion == 0 || m.Ancho == 0) && hayFFmpeg == nil {
		if in, err := herr.Sondear(ctx, ruta); err == nil {
			m.completar(in.Duracion, in.Ancho, in.Alto)
		}
	}
	m.Poster = poster(ctx, ruta, m.Duracion, herr, hayFFmpeg == nil)
	return m
}

func (m *Medido) completar(d time.Duration, ancho, alto int) {
	if m.Duracion == 0 {
		m.Duracion = d
	}
	if m.Ancho == 0 || m.Alto == 0 {
		m.Ancho, m.Alto = ancho, alto
	}
}

// poster es un fotograma del principio (a un segundo, que el primero suele ser negro),
// en JPEG.
func poster(ctx context.Context, ruta string, dur time.Duration, h conversor.Herramientas, hayFFmpeg bool) []byte {
	en := time.Duration(0)
	if dur > 3*time.Second {
		en = time.Second
	}
	if f, err := os.Open(ruta); err == nil { //nolint:gosec // G304: como arriba
		img, err := video.Fotograma(f, en)
		_ = f.Close()
		if err == nil {
			if b, err := aJPEG(img); err == nil {
				return b
			}
		}
	}
	if hayFFmpeg {
		dir, err := os.MkdirTemp("", "sytue-poster-")
		if err == nil {
			defer func() { _ = os.RemoveAll(dir) }()
			salida := filepath.Join(dir, "fotograma-poster.jpg")
			c, fin := context.WithTimeout(ctx, 30*time.Second)
			err = h.Fotograma(c, ruta, salida, en)
			fin()
			if err == nil {
				if b, err := os.ReadFile(salida); err == nil && len(b) > 0 { //nolint:gosec // G304: nuestro temporal
					return b
				}
			}
		}
	}
	if b, _, err := videoteca.MiniaturaDelSistema(ctx, ruta); err == nil {
		return b
	}
	return nil
}

func aJPEG(img image.Image) ([]byte, error) {
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
