// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package transcoder

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/services/cast/internal/domain"
)

// Cómo se llama a ffmpeg. Los argumentos se componen en funciones puras (se
// prueban sin ffmpeg) y una sola función los ejecuta.

// entrada es de dónde lee ffmpeg: un fichero, o un playlist HLS local (la
// grabación de un directo) que necesita abrir sus segmentos.
type entrada struct {
	ruta string
	hls  bool
}

// argsProtocolo son las opciones que van ANTES de `-i`.
//
// Una entrada HLS (la grabación de un directo, que escribe el egress) puede
// abrir ficheros locales —nada de red— con cualquier extensión.
//
// Una SUBIDA no puede abrir nada más que a sí misma: ffmpeg elige el demuxer por
// el CONTENIDO, y un «vídeo» que en realidad era un playlist HLS o un guion de
// `concat` hacía que ffprobe y ffmpeg abrieran lo que nombrara —el vídeo de
// otra organización en el temporal del trabajador, o una URL interna— y lo
// metieran en la salida que luego se descarga (FILE-12). Solo protocolo `file`
// y solo demuxers de contenedores de vídeo, ninguno de los que abren otras
// rutas (hls, concat, image2, tty…).
func (e entrada) argsProtocolo() []string {
	if !e.hls {
		return []string{"-protocol_whitelist", "file", "-format_whitelist", demuxersDeSubida}
	}
	return []string{"-protocol_whitelist", "file,crypto,data", "-allowed_extensions", "ALL"}
}

// demuxersDeSubida son los contenedores de vídeo que se aceptan subidos.
const demuxersDeSubida = "mov,mp4,m4a,3gp,3g2,mj2,matroska,webm,avi,flv,mpegts,mpeg,ogg,asf,mxf,dv,h264,hevc"

// argsComunes abre toda llamada: sin cabecera ni consola, solo errores en stderr
// (que es lo que se enseña si falla), sobrescribir (el directorio es nuevo) y
// los hilos acotados: el chart da 2 CPU y ffmpeg, suelto, los cogería todos.
func argsComunes(hilos int) []string {
	return []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y", "-threads", strconv.Itoa(hilos)}
}

// Segundos por segmento de HLS y su fotograma clave forzado: cada segmento
// empieza en uno, o el reproductor no puede cambiar de calidad ahí.
const segundosSegmento = 4

// argsHLS compone la llamada que produce la escalera entera de una vez: un
// `split` del vídeo decodificado, un `scale` por peldaño, x264 con la tasa de
// cada uno y el mismo AAC en cada variante (audio dentro del vídeo, que es lo
// que todos los reproductores entienden). El master sube un nivel solo:
// ffmpeg pone `master.m3u8` junto a las carpetas `%v` cuando el `%v` está en el
// directorio, que es la disposición de Apple y la que /media resuelve.
//
// Salida bajo `dir`: hls/master.m3u8, hls/<calidad>/index.m3u8,
// hls/<calidad>/init*.mp4 y hls/<calidad>/seg_NNNNN.m4s.
func argsHLS(e entrada, dir string, esc []Calidad, s Sonda, hilos int) []string {
	args := argsComunes(hilos)
	args = append(args, e.argsProtocolo()...)
	args = append(args, "-i", e.ruta)

	// Un split por peldaño y su escalado. `format=yuv420p` porque un original de
	// 10 bits (HEVC de móvil) saldría en High 10 y Safari no lo reproduce.
	partes := make([]string, 0, len(esc)+1)
	etiquetas := make([]string, 0, len(esc))
	for i := range esc {
		etiquetas = append(etiquetas, fmt.Sprintf("[v%d]", i))
	}
	partes = append(partes, fmt.Sprintf("[0:v:0]split=%d%s", len(esc), strings.Join(etiquetas, "")))
	for i, c := range esc {
		partes = append(partes, fmt.Sprintf("[v%d]%s,format=yuv420p[v%do]", i, filtroEscala(c.Lado, s.Vertical()), i))
	}
	args = append(args, "-filter_complex", strings.Join(partes, ";"))

	mapa := make([]string, 0, len(esc))
	for i, c := range esc {
		args = append(args, "-map", fmt.Sprintf("[v%do]", i))
		if s.TieneAudio {
			args = append(args, "-map", "0:a:0")
		}
		args = append(args,
			fmt.Sprintf("-b:v:%d", i), c.Tasa,
			fmt.Sprintf("-maxrate:v:%d", i), c.Maxima,
			fmt.Sprintf("-bufsize:v:%d", i), c.Bufer,
		)
		if s.TieneAudio {
			mapa = append(mapa, fmt.Sprintf("v:%d,a:%d,name:%s", i, i, c.Nombre))
		} else {
			mapa = append(mapa, fmt.Sprintf("v:%d,name:%s", i, c.Nombre))
		}
	}
	args = append(args,
		"-c:v", "libx264", "-preset", "veryfast", "-profile:v", "high",
		// Fotograma clave cada 4 s exactos y ninguno por cambio de escena: los
		// segmentos de todas las calidades caen en el mismo sitio.
		"-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", segundosSegmento),
		"-sc_threshold", "0",
	)
	if s.TieneAudio {
		args = append(args, "-c:a", "aac", "-b:a", kbps(domain.AudioHLSKbps), "-ac", "2")
	}
	args = append(args,
		"-var_stream_map", strings.Join(mapa, " "),
		"-f", "hls",
		"-hls_segment_type", "fmp4",
		"-hls_time", strconv.Itoa(segundosSegmento),
		"-hls_playlist_type", "vod",
		"-hls_list_size", "0",
		"-hls_flags", "independent_segments",
		"-master_pl_name", "master.m3u8",
		"-hls_fmp4_init_filename", "init.mp4",
		"-hls_segment_filename", filepath.Join(dir, "hls", "%v", "seg_%05d.m4s"),
		filepath.Join(dir, "hls", "%v", "index.m3u8"),
	)
	return args
}

// argsWebM compone los WebM VP9/Opus de la app de escritorio, uno por peldaño
// de la escalera (las mismas alturas que el HLS), con una sola decodificación:
// un `split` y una salida por calidad, cada una con los Cues al principio para
// poder buscar sin leer el fichero entero (§7).
//
// Los hilos se acotan también en CADA codificador: ffmpeg da a cada salida
// tantos hilos como núcleos vea si no se le dice, y el chart da 2 CPU.
//
// Salida bajo `dir`: webm/<calidad>.webm (webm/1080p.webm, webm/720p.webm…).
func argsWebM(e entrada, dir string, esc []Calidad, s Sonda, hilos int) []string {
	args := argsComunes(hilos)
	args = append(args, e.argsProtocolo()...)
	args = append(args, "-i", e.ruta)

	partes := make([]string, 0, len(esc)+1)
	etiquetas := make([]string, 0, len(esc))
	for i := range esc {
		etiquetas = append(etiquetas, fmt.Sprintf("[w%d]", i))
	}
	partes = append(partes, fmt.Sprintf("[0:v:0]split=%d%s", len(esc), strings.Join(etiquetas, "")))
	for i, c := range esc {
		partes = append(partes, fmt.Sprintf("[w%d]%s,format=yuv420p[w%do]", i, filtroEscala(c.Lado, s.Vertical()), i))
	}
	args = append(args, "-filter_complex", strings.Join(partes, ";"))

	for i, c := range esc {
		args = append(args, "-map", fmt.Sprintf("[w%do]", i),
			"-c:v", "libvpx-vp9", "-b:v", c.TasaWebM, "-deadline", "good", "-cpu-used", "4", "-row-mt", "1",
			// Un fotograma clave cada 10 s como mucho: a 30 fps son 300; la
			// búsqueda por Cues cae en uno.
			"-g", "300",
			"-threads", strconv.Itoa(hilos),
		)
		if s.TieneAudio {
			args = append(args, "-map", "0:a:0", "-c:a", "libopus", "-b:a", kbps(domain.AudioWebMKbps), "-ac", "2")
		} else {
			args = append(args, "-an")
		}
		args = append(args, "-cues_to_front", "1", "-f", "webm", filepath.Join(dir, "webm", c.Nombre+".webm"))
	}
	return args
}

// filtroPoster deja el póster dentro de 1280×1280 sin agrandar nada: cabe en la
// tarjeta y en la vista previa de un enlace, y un original pequeño se queda
// como está.
const filtroPoster = "scale='min(1280,iw)':'min(1280,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2"

// argsPoster saca un fotograma en `instante` como JPEG (q≈85) y WebP, con una
// sola decodificación. `-ss` antes de `-i` busca en el contenedor, que es rápido.
func argsPoster(e entrada, instante time.Duration, dir string, hilos int) []string {
	args := argsComunes(hilos)
	args = append(args, "-ss", segundos(instante))
	args = append(args, e.argsProtocolo()...)
	args = append(args, "-i", e.ruta,
		"-map", "0:v:0", "-frames:v", "1", "-an", "-vf", filtroPoster, "-q:v", "4", filepath.Join(dir, "poster.jpg"),
		"-map", "0:v:0", "-frames:v", "1", "-an", "-vf", filtroPoster, "-c:v", "libwebp", "-quality", "85", filepath.Join(dir, "poster.webp"),
	)
	return args
}

// instantePoster es en qué momento se saca el póster: el pedido (`at_ms`), o el
// 10 % de la duración, acotado dentro del vídeo (una décima antes del final,
// que en el último fotograma exacto ffmpeg a veces no encuentra nada).
func instantePoster(duracion time.Duration, pedido *time.Duration) time.Duration {
	t := duracion / 10
	if pedido != nil {
		t = *pedido
	}
	if duracion > 0 {
		tope := duracion - 100*time.Millisecond
		if tope < 0 {
			tope = 0
		}
		t = min(t, tope)
	}
	if t < 0 {
		t = 0
	}
	return t
}

// segundos escribe una duración como la entiende ffmpeg («12.345»).
func segundos(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', 3, 64)
}

// errorFfmpeg es un ffmpeg que salió mal, con la última línea que dijo: es lo
// que se guarda como motivo del fallo y lo que la interfaz enseña.
type errorFfmpeg struct {
	paso   string
	motivo string
}

func (e *errorFfmpeg) Error() string { return "ffmpeg (" + e.paso + "): " + e.motivo }

// ejecutar corre ffmpeg con esos argumentos. Si el contexto vence (plazo del
// trabajo) exec mata el proceso y aquí sale el error del contexto, que motivoDe
// traduce a «tiempo agotado».
func (w *trabajador) ejecutar(ctx context.Context, paso string, args []string) error {
	var stderr bytes.Buffer
	// Sin shell y con rutas que fabrica este paquete: nada que interpretar.
	cmd := exec.CommandContext(ctx, w.ffmpeg, args...) //nolint:gosec // G204: ver arriba
	cmd.Stderr = &stderr
	// Con la señal de muerte damos tres segundos a que suelte los ficheros.
	cmd.WaitDelay = 3 * time.Second
	inicio := time.Now()
	err := cmd.Run()
	w.logger.Debug("transcoder: ffmpeg", "paso", paso, "duracion", time.Since(inicio).Round(time.Millisecond).String(), "error", err)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &errorFfmpeg{paso: paso, motivo: ultimaLinea(stderr.String(), err.Error())}
	}
	return nil
}

// maxMotivo es cuánto motivo se guarda: una línea que cabe en la ficha del vídeo.
const maxMotivo = 300

// ultimaLinea es la última línea con contenido de la salida de error, acotada,
// o `porDefecto` si no dijo nada.
func ultimaLinea(salida, porDefecto string) string {
	lineas := strings.Split(strings.TrimSpace(salida), "\n")
	for i := len(lineas) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lineas[i]); l != "" {
			return acotar(l)
		}
	}
	return acotar(porDefecto)
}

// acotar recorta a maxMotivo runas, con puntos suspensivos.
func acotar(s string) string {
	r := []rune(s)
	if len(r) <= maxMotivo {
		return s
	}
	return string(r[:maxMotivo-1]) + "…"
}
