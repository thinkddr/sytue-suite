// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package transcoder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/media"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

// Un trabajo, de la entrada a las derivadas subidas. Cada clase tiene su camino
// pero comparten las piezas: bajar, sondear, ffmpeg, subir, y el Estado que la
// store aplica al vídeo al terminar.

// resultado es lo que un trabajo deja: el Estado para Store.Terminar y, para el
// registro, cuánto pesan las derivadas nuevas.
type resultado struct {
	estado         store.Estado
	bytesDerivadas int64
}

// Errores de un trabajo que no son de ffmpeg.
var (
	// ErrSinEntrada: el vídeo no tiene de dónde leer (sin original ni grabación).
	ErrSinEntrada = errors.New("el vídeo no tiene original ni grabación")
	// ErrClaseDesconocida: una clase de trabajo que este trabajador no conoce.
	ErrClaseDesconocida = errors.New("clase de trabajo desconocida")
)

// cargaPoster es el payload de un trabajo `poster`: el instante pedido.
type cargaPoster struct {
	AtMS *int64 `json:"at_ms"`
}

// Claves relativas de las derivadas bajo el prefijo del vídeo. Son las mismas
// que sirve /media (media.Ruta*); las lee service.claveDe. Los WebM van uno por
// calidad (media.RutaWebMDe).
const (
	claveHLS        = media.RutaHLS
	clavePosterJPG  = media.RutaPoster
	clavePosterWebP = "poster.webp"
	// subdirGrabacion y ficheroEntrada son nombres dentro del temporal del trabajo.
	subdirGrabacion = "grabacion"
	subdirSalida    = "salida"
	ficheroEntrada  = "entrada"
)

// procesar hace el trabajo en un directorio temporal propio, que se borra al
// acabar salga como salga.
func (w *trabajador) procesar(ctx context.Context, j domain.Job, v domain.Video) (resultado, error) {
	dir, err := os.MkdirTemp(w.cfg.TranscoderTmpDir, "cast-"+strings.SplitN(j.ID.String(), "-", 2)[0]+"-")
	if err != nil {
		return resultado{}, fmt.Errorf("crear el temporal del trabajo: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			w.logger.Warn("transcoder: no se pudo borrar el temporal", "dir", dir, "error", err)
		}
	}()
	salida := filepath.Join(dir, subdirSalida)
	if err := os.Mkdir(salida, 0o750); err != nil {
		return resultado{}, fmt.Errorf("crear la salida del trabajo: %w", err)
	}
	t := tenancy.Tenant{Slug: v.Tenant}
	switch j.Kind {
	case domain.JobUpload:
		return w.procesarSubida(ctx, t, v, dir, salida)
	case domain.JobLiveVOD:
		return w.procesarGrabacion(ctx, t, v, dir, salida)
	case domain.JobPoster:
		return w.procesarPoster(ctx, t, j, v, dir, salida)
	case domain.JobQualities:
		return w.procesarCalidades(ctx, t, v, dir, salida)
	default:
		return resultado{}, fmt.Errorf("%w: %q", ErrClaseDesconocida, j.Kind)
	}
}

// procesarSubida es `upload`: del original salen HLS, WebM y póster.
func (w *trabajador) procesarSubida(ctx context.Context, t tenancy.Tenant, v domain.Video, dir, salida string) (resultado, error) {
	if v.OriginalKey == "" {
		return resultado{}, ErrSinEntrada
	}
	e := entrada{ruta: filepath.Join(dir, ficheroEntrada)}
	if _, err := w.descargarA(ctx, t, v.OriginalKey, e.ruta); err != nil {
		return resultado{}, err
	}
	s, err := w.sondear(ctx, e)
	if err != nil {
		return resultado{}, err
	}
	esc, err := escaleraPara(s.Ancho, s.Alto, elegidasDe(v))
	if err != nil {
		return resultado{}, err
	}
	w.logger.Info("transcoder: entrada medida", "video", v.ID, "formato", s.Formato, "codec", s.CodecVideo,
		"audio", s.CodecAudio, "ancho", s.Ancho, "alto", s.Alto, "ms", s.Duracion.Milliseconds(), "escalera", nombres(esc))

	for _, c := range esc {
		if err := os.MkdirAll(filepath.Join(salida, "hls", c.Nombre), 0o750); err != nil {
			return resultado{}, fmt.Errorf("preparar hls/%s: %w", c.Nombre, err)
		}
	}
	if err := w.ejecutar(ctx, "hls", argsHLS(e, salida, esc, s, w.cfg.TranscoderThreads)); err != nil {
		return resultado{}, err
	}
	if err := w.webmYPoster(ctx, e, s, esc, salida); err != nil {
		return resultado{}, err
	}
	derivadas, err := w.subirDirectorio(ctx, t, salida, service.Prefijo(v))
	if err != nil {
		return resultado{}, err
	}
	est := estadoDe(s)
	est.HlsKey = ptr(service.Prefijo(v) + claveHLS)
	est.HlsQualities = domain.AInt32(lados(esc))
	conWebM(&est, v, lados(esc))
	est.PosterKey = posterSalvoPropio(v)
	est.BytesTotal = ptr(v.BytesOriginal + derivadas)
	return resultado{estado: est, bytesDerivadas: derivadas}, nil
}

// procesarGrabacion es `live_vod`: del HLS grabado por el egress salen WebM y
// póster; el HLS se queda como está y sigue siendo lo que la web reproduce. Lo
// que ocupa la grabación cuenta como «original» del vídeo si nadie lo anotó.
func (w *trabajador) procesarGrabacion(ctx context.Context, t tenancy.Tenant, v domain.Video, dir, salida string) (resultado, error) {
	if v.HlsKey == "" {
		return resultado{}, ErrSinEntrada
	}
	g, err := w.descargarGrabacion(ctx, t, v.HlsKey, filepath.Join(dir, subdirGrabacion), 0)
	if err != nil {
		return resultado{}, err
	}
	e := entrada{ruta: g.playlist, hls: true}
	s, err := w.sondear(ctx, e)
	if err != nil {
		return resultado{}, err
	}
	if s.Duracion == 0 && g.duracion > 0 {
		s.Duracion = time.Duration(g.duracion * float64(time.Second))
	}
	esc, err := escaleraPara(s.Ancho, s.Alto, elegidasDe(v))
	if err != nil {
		return resultado{}, err
	}
	w.logger.Info("transcoder: grabación medida", "video", v.ID, "codec", s.CodecVideo, "audio", s.CodecAudio,
		"ancho", s.Ancho, "alto", s.Alto, "ms", s.Duracion.Milliseconds(), "bytes_grabacion", g.bytes, "escalera", nombres(esc))
	if err := w.webmYPoster(ctx, e, s, esc, salida); err != nil {
		return resultado{}, err
	}
	derivadas, err := w.subirDirectorio(ctx, t, salida, service.Prefijo(v))
	if err != nil {
		return resultado{}, err
	}
	est := estadoDe(s)
	conWebM(&est, v, lados(esc))
	est.PosterKey = posterSalvoPropio(v)
	base := v.BytesOriginal
	if base == 0 {
		base = g.bytes
		est.BytesOriginal = ptr(base)
	}
	est.BytesTotal = ptr(base + derivadas)
	return resultado{estado: est, bytesDerivadas: derivadas}, nil
}

// procesarPoster es `poster`: solo el fotograma pedido, del original si lo hay o
// de la grabación (bajando solo hasta ese instante). Sustituye al póster que
// hubiera, propio o no: lo pidió el dueño.
func (w *trabajador) procesarPoster(ctx context.Context, t tenancy.Tenant, j domain.Job, v domain.Video, dir, salida string) (resultado, error) {
	var carga cargaPoster
	if len(j.Payload) > 0 {
		if err := json.Unmarshal(j.Payload, &carga); err != nil {
			return resultado{}, fmt.Errorf("payload del póster ilegible: %w", err)
		}
	}
	var pedido *time.Duration
	if carga.AtMS != nil && *carga.AtMS >= 0 {
		pedido = ptr(time.Duration(*carga.AtMS) * time.Millisecond)
	}
	var e entrada
	switch {
	case v.OriginalKey != "":
		e = entrada{ruta: filepath.Join(dir, ficheroEntrada)}
		if _, err := w.descargarA(ctx, t, v.OriginalKey, e.ruta); err != nil {
			return resultado{}, err
		}
	case v.HlsKey != "":
		hasta := 0.0
		if pedido != nil {
			hasta = pedido.Seconds()
		} else if v.DurationMS != nil {
			hasta = float64(*v.DurationMS) / 1000 / 10
		}
		g, err := w.descargarGrabacion(ctx, t, v.HlsKey, filepath.Join(dir, subdirGrabacion), hasta)
		if err != nil {
			return resultado{}, err
		}
		e = entrada{ruta: g.playlist, hls: true}
	default:
		return resultado{}, ErrSinEntrada
	}
	s, err := w.sondear(ctx, e)
	if err != nil {
		return resultado{}, err
	}
	// La duración de la ficha manda si la entrada no la sabe (un playlist recortado).
	if v.DurationMS != nil && (s.Duracion == 0 || e.hls) {
		s.Duracion = time.Duration(*v.DurationMS) * time.Millisecond
	}
	if err := w.ejecutar(ctx, "poster", argsPoster(e, instantePoster(s.Duracion, pedido), salida, w.cfg.TranscoderThreads)); err != nil {
		return resultado{}, err
	}
	// Lo que se sobrescribe deja de contar; lo nuevo cuenta.
	antes := int64(0)
	for _, k := range []string{clavePosterJPG, clavePosterWebP} {
		n, err := w.tamanoDe(ctx, t, service.Prefijo(v)+k)
		if err != nil {
			return resultado{}, err
		}
		antes += n
	}
	derivadas, err := w.subirDirectorio(ctx, t, salida, service.Prefijo(v))
	if err != nil {
		return resultado{}, err
	}
	est := store.Estado{
		PosterKey:  ptr(service.Prefijo(v) + clavePosterJPG),
		BytesTotal: ptr(max(v.BytesTotal-antes+derivadas, v.BytesOriginal)),
	}
	return resultado{estado: est, bytesDerivadas: derivadas}, nil
}

// webmYPoster produce los WebM de la escalera y el póster (al 10 %) de una
// entrada ya medida.
func (w *trabajador) webmYPoster(ctx context.Context, e entrada, s Sonda, esc []Calidad, salida string) error {
	if err := w.webm(ctx, e, s, esc, salida); err != nil {
		return err
	}
	return w.ejecutar(ctx, "poster", argsPoster(e, instantePoster(s.Duracion, nil), salida, w.cfg.TranscoderThreads))
}

// webm produce los WebM de esos peldaños bajo salida/webm/.
func (w *trabajador) webm(ctx context.Context, e entrada, s Sonda, esc []Calidad, salida string) error {
	if err := os.MkdirAll(filepath.Join(salida, "webm"), 0o750); err != nil {
		return fmt.Errorf("preparar webm/: %w", err)
	}
	return w.ejecutar(ctx, "webm", argsWebM(e, salida, esc, s, w.cfg.TranscoderThreads))
}

// conWebM anota en el Estado los WebM que hay tras el trabajo, cuál va en
// webm_key (el de por defecto) y la elección que el trabajo vio.
func conWebM(est *store.Estado, v domain.Video, calidades []int) {
	est.WebmQualities = domain.AInt32(calidades)
	if q := domain.WebMPorDefecto(calidades); q > 0 {
		est.WebmKey = ptr(service.Prefijo(v) + media.RutaWebMDe(q))
	}
	est.Elegidas = v.Qualities
}

// elegidasDe es la elección guardada del vídeo (nil = la recomendada).
func elegidasDe(v domain.Video) []int {
	if v.Qualities == nil {
		return nil
	}
	out := make([]int, len(v.Qualities))
	for i, q := range v.Qualities {
		out[i] = int(q)
	}
	return out
}

// estadoDe son las medidas del vídeo que toda transcodificación completa anota.
func estadoDe(s Sonda) store.Estado {
	est := store.Estado{}
	if s.Duracion > 0 {
		est.DurationMS = ptr(s.Duracion.Milliseconds())
	}
	if s.Ancho > 0 && s.Alto > 0 {
		est.Width, est.Height = ptr(int32(s.Ancho)), ptr(int32(s.Alto)) //nolint:gosec // G115: dimensiones de un vídeo, caben de sobra
	}
	return est
}

// posterSalvoPropio es la clave del póster generado, salvo que el dueño ya
// hubiera subido uno suyo (service.SubirPoster): ese se respeta. El fichero
// generado se sube igual, por si lo quita.
func posterSalvoPropio(v domain.Video) *string {
	if v.PosterKey != "" && !strings.HasSuffix(v.PosterKey, "/"+clavePosterJPG) {
		return nil
	}
	return ptr(service.Prefijo(v) + clavePosterJPG)
}

// motivoDe es el texto corto que se guarda en el trabajo y, al cuarto fallo, en
// el vídeo: una línea, legible, en español como el resto de motivos internos.
func motivoDe(err error, plazo time.Duration) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Sprintf("tiempo agotado: el trabajo pasó de %s", plazo)
	case errors.Is(err, context.Canceled):
		return "interrumpido: el trabajador se apagó a media conversión"
	}
	return acotar(strings.ReplaceAll(strings.TrimSpace(err.Error()), "\n", " "))
}

func nombres(esc []Calidad) []string {
	out := make([]string, 0, len(esc))
	for _, c := range esc {
		out = append(out, c.Nombre)
	}
	return out
}

func ptr[T any](v T) *T { return &v }
