// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"os"
	"strings"
	"sync"

	"git.sytue.eu/sytue/one/apps/fyne/video"
	"git.sytue.eu/sytue/one/apps/nucleo/local/conversor"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
)

// ajustesCast son los de esta app, en claro en `<ajustes>/cast.json`.
type ajustesCast struct {
	// FFmpeg es la ruta que puso la persona (el ejecutable o su carpeta); vacía, el PATH.
	FFmpeg string `json:"ffmpeg,omitempty"`
}

// herramientas guarda el ffmpeg encontrado: buscarlo es mirar el disco, y la videoteca
// pregunta a menudo. Se vuelve a buscar al cambiar la ruta en la app.
type herramientas struct {
	ruta string // del fichero de ajustes

	mu    sync.Mutex
	aj    ajustesCast
	h     conversor.Herramientas
	err   error
	cods  map[string]bool
	hecho bool
}

func nuevasHerramientas(ruta string) *herramientas {
	t := &herramientas{ruta: ruta}
	_ = espacio.LeerJSON(ruta, &t.aj)
	return t
}

// Actuales devuelve el ffmpeg en uso (o ErrSinFFmpeg).
func (t *herramientas) Actuales() (conversor.Herramientas, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.hecho {
		t.buscar()
	}
	return t.h, t.err
}

// buscar, con t.mu tomado.
func (t *herramientas) buscar() {
	t.hecho = true
	t.h, t.err = conversor.Buscar(t.aj.FFmpeg)
	t.cods = nil
	if t.err == nil {
		if err := t.h.LeerVersion(context.Background()); err != nil {
			t.h, t.err = conversor.Herramientas{}, err
			return
		}
		t.cods, _ = t.h.Codificadores(context.Background())
	}
}

// Codificadores son los de este ffmpeg; nil si no se sabe.
func (t *herramientas) Codificadores() map[string]bool {
	_, _ = t.Actuales()
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cods
}

// Configurada es la ruta puesta por la persona.
func (t *herramientas) Configurada() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.aj.FFmpeg
}

// Configurar guarda una ruta nueva y vuelve a buscar.
func (t *herramientas) Configurar(ruta string) (conversor.Herramientas, error) {
	t.mu.Lock()
	t.aj.FFmpeg = strings.TrimSpace(ruta)
	_ = espacio.EscribirJSON(t.ruta, t.aj)
	t.buscar()
	h, err := t.h, t.err
	t.mu.Unlock()
	return h, err
}

// sondear mide un fichero: primero con el lector de WebM del reproductor propio, que
// dice exactamente lo que la app sabe reproducir sin depender de ffprobe; si no es un
// WebM que lea, con ffprobe.
func (t *herramientas) sondear(ctx context.Context, ruta string) (conversor.Info, error) {
	if info, ok := sondearWebM(ruta); ok {
		return info, nil
	}
	h, err := t.Actuales() //nolint:contextcheck // la búsqueda de ffmpeg se hace una vez para toda la app: no la corta quien pregunta primero
	if err != nil {
		return conversor.Info{}, err
	}
	return h.Sondear(ctx, ruta)
}

// sondearWebM abre el fichero con el demuxer del reproductor. Dice ok solo si es un
// WebM/Matroska que se puede reproducir tal cual (VP9 o VP8, y/o Opus); lo que lleve otro códec
// se deja a ffprobe, que sabrá decir cuál.
func sondearWebM(ruta string) (conversor.Info, bool) {
	f, err := os.Open(ruta)
	if err != nil {
		return conversor.Info{}, false
	}
	defer func() { _ = f.Close() }()
	w, err := video.Abrir(f)
	if err != nil {
		return conversor.Info{}, false
	}
	info := conversor.Info{Contenedor: "matroska,webm", Duracion: w.Duracion}
	if v := w.Pista(video.TipoVideo); v != nil {
		switch v.Codec {
		case video.CodecVP9:
			info.Video = "vp9"
		case video.CodecVP8:
			info.Video = "vp8" // CA-07: el reproductor también descodifica VP8
		default:
			return conversor.Info{}, false
		}
		info.Ancho, info.Alto = v.Ancho, v.Alto
	}
	if a := w.Pista(video.TipoAudio); a != nil {
		if a.Codec != video.CodecOpus {
			return conversor.Info{}, false
		}
		info.Audio = "opus"
	}
	return info, info.Nativo()
}
