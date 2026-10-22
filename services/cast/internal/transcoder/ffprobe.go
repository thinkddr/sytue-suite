// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package transcoder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Sonda es lo que el trabajador necesita saber de una entrada antes de decidir
// qué hacer con ella: lo que devuelve `ffprobe -print_format json -show_format
// -show_streams`, reducido a lo que se usa.
type Sonda struct {
	Duracion time.Duration
	// Ancho y Alto son los de PRESENTACIÓN: si el fichero trae una rotación
	// (un móvil grabando en vertical guarda 1920×1080 con «gira 90°»), ya están
	// intercambiados, que es como ffmpeg lo decodifica y como se verá.
	Ancho, Alto int
	CodecVideo  string
	CodecAudio  string
	TieneAudio  bool
	// FPS es la tasa media de fotogramas; 0 si no se sabe.
	FPS float64
	// Formato es `format_name` (p. ej. «mov,mp4,m4a,3gp,3g2,mj2», «matroska,webm», «hls»).
	Formato string
	// Bytes es el tamaño del fichero según ffprobe; 0 si no lo da.
	Bytes int64
}

// Vertical dice si se ve más alto que ancho.
func (s Sonda) Vertical() bool { return s.Alto > s.Ancho }

// LadoCorto es el lado menor de presentación, el que decide la escalera.
func (s Sonda) LadoCorto() int { return min(s.Ancho, s.Alto) }

// Errores de la sonda.
var (
	// ErrSinVideo: la entrada no tiene ninguna pista de vídeo con dimensiones.
	ErrSinVideo = errors.New("ffprobe: la entrada no tiene pista de vídeo")
	// ErrSondaIlegible: ffprobe no devolvió JSON que se pueda leer.
	ErrSondaIlegible = errors.New("ffprobe: salida ilegible")
)

// salidaFfprobe es la forma del JSON de ffprobe, solo los campos que se leen.
// Los nombres son los de ffprobe (snake_case, como pide tagliatelle).
type salidaFfprobe struct {
	Streams []struct {
		CodecName    string `json:"codec_name"`
		CodecType    string `json:"codec_type"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		RFrameRate   string `json:"r_frame_rate"`
		AvgFrameRate string `json:"avg_frame_rate"`
		Duration     string `json:"duration"`
		Tags         struct {
			Rotate string `json:"rotate"`
		} `json:"tags"`
		SideDataList []struct {
			SideDataType string  `json:"side_data_type"`
			Rotation     float64 `json:"rotation"`
		} `json:"side_data_list"`
	} `json:"streams"`
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
		Size       string `json:"size"`
	} `json:"format"`
}

// parsearSonda lee el JSON de ffprobe. La duración sale del contenedor y, si no
// la trae (algunos MKV y los HLS sin ENDLIST), de la pista de vídeo.
func parsearSonda(js []byte) (Sonda, error) {
	var raw salidaFfprobe
	if err := json.Unmarshal(js, &raw); err != nil {
		return Sonda{}, fmt.Errorf("%w: %w", ErrSondaIlegible, err)
	}
	s := Sonda{Formato: raw.Format.FormatName}
	s.Bytes, _ = strconv.ParseInt(raw.Format.Size, 10, 64)
	s.Duracion = segundosADuracion(raw.Format.Duration)
	video := -1
	for i, st := range raw.Streams {
		switch st.CodecType {
		case "video":
			// La primera pista con dimensiones es la principal; una portada
			// incrustada (mjpeg de 1 fotograma) llega como `video` también, pero
			// después.
			if video < 0 && st.Width > 0 && st.Height > 0 {
				video = i
			}
		case "audio":
			if !s.TieneAudio {
				s.TieneAudio, s.CodecAudio = true, st.CodecName
			}
		}
	}
	if video < 0 {
		return Sonda{}, ErrSinVideo
	}
	v := raw.Streams[video]
	s.CodecVideo = v.CodecName
	s.Ancho, s.Alto = v.Width, v.Height
	if s.Duracion == 0 {
		s.Duracion = segundosADuracion(v.Duration)
	}
	s.FPS = tasaDe(v.AvgFrameRate)
	if s.FPS == 0 {
		s.FPS = tasaDe(v.RFrameRate)
	}
	rot := 0.0
	for _, sd := range v.SideDataList {
		if sd.Rotation != 0 {
			rot = sd.Rotation
		}
	}
	if rot == 0 && v.Tags.Rotate != "" {
		rot, _ = strconv.ParseFloat(v.Tags.Rotate, 64)
	}
	if giraNoventa(rot) {
		s.Ancho, s.Alto = s.Alto, s.Ancho
	}
	return s, nil
}

// giraNoventa dice si la rotación deja el fotograma de lado (±90°, 270°…).
func giraNoventa(grados float64) bool {
	g := math.Mod(math.Abs(grados), 180)
	return math.Abs(g-90) < 1
}

// segundosADuracion lee «12.345000» (segundos con decimales) → Duration; 0 si
// no es un número (ffprobe pone «N/A» cuando no lo sabe).
func segundosADuracion(s string) time.Duration {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || f <= 0 || math.IsInf(f, 0) || math.IsNaN(f) {
		return 0
	}
	return time.Duration(f * float64(time.Second))
}

// tasaDe lee «30000/1001» o «25/1» → fps; 0 si no se puede («0/0»).
func tasaDe(s string) float64 {
	num, den, ok := strings.Cut(strings.TrimSpace(s), "/")
	if !ok {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0
		}
		return f
	}
	n, err1 := strconv.ParseFloat(num, 64)
	d, err2 := strconv.ParseFloat(den, 64)
	if err1 != nil || err2 != nil || d == 0 || n <= 0 {
		return 0
	}
	return n / d
}

// sondear corre ffprobe sobre la entrada y lee su salida.
func (w *trabajador) sondear(ctx context.Context, e entrada) (Sonda, error) {
	args := append([]string{"-hide_banner", "-loglevel", "error"}, e.argsProtocolo()...)
	args = append(args, "-print_format", "json", "-show_format", "-show_streams", e.ruta)
	var stdout, stderr bytes.Buffer
	// La ruta la fabrica este paquete dentro de su temporal (nunca viene del
	// usuario) y no hay shell de por medio: exec pasa el vector tal cual.
	cmd := exec.CommandContext(ctx, w.ffprobe, args...) //nolint:gosec // G204: ver arriba
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return Sonda{}, ctx.Err()
		}
		return Sonda{}, fmt.Errorf("ffprobe: %s", ultimaLinea(stderr.String(), err.Error()))
	}
	return parsearSonda(stdout.Bytes())
}
