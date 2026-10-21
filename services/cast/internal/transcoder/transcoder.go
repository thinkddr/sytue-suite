// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package transcoder es el trabajador que convierte los originales de Sytue Cast
// (CAST-ENCARGO §7). Toma trabajos de `transcode_jobs` —uno a la vez por
// réplica, con FOR UPDATE SKIP LOCKED en una transacción de mantenimiento—,
// descarga la entrada a un directorio temporal propio del trabajo, la mide con
// ffprobe, decide la escalera de calidades (nunca hacia arriba), corre ffmpeg y
// sube lo que sale bajo el prefijo del vídeo en el almacén de su organización:
//
//	<video_id>/hls/master.m3u8, <video_id>/hls/<calidad>/…   HLS fMP4 H.264 + AAC
//	<video_id>/webm/<calidad>.webm                            WebM VP9 + Opus (la app de escritorio)
//	<video_id>/poster.jpg, <video_id>/poster.webp             póster
//
// Las dos escaleras llevan las mismas calidades: las que eligió quien lo subió,
// recortadas a lo que el original da (domain.Resolver).
//
// Cuatro clases de trabajo: `upload` (todo lo de arriba desde el original),
// `live_vod` (WebM y póster desde el HLS que grabó el egress, que se queda tal
// cual), `poster` (solo el fotograma pedido) y `qualities` (poner lo generado al
// día con lo elegido: generar lo que falta, borrar lo que sobra). Al acabar deja el vídeo `ready`
// con sus derivadas y declara la cuota a Drive (§8); si falla, la store decide
// si se reintenta (tres veces) o el vídeo queda `failed` con el motivo.
//
// El mismo binario que el servicio, con el subcomando `transcoder`, en una
// imagen con ffmpeg (services/cast/Dockerfile.transcoder). Fuera de aquí nadie
// llama a ffmpeg: si falta, Run falla al arrancar y el pod se cae a la vista.
package transcoder

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cast/internal/config"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

// Deps son las dependencias del trabajador. La firma la fija cmd/cast.
type Deps struct {
	Pool    *pg.Pool
	Objetos *objectstore.Client
	Store   *store.Store
	// Servicio declara la cuota a Drive al terminar cada trabajo (§8). nil = no
	// se declara (solo en pruebas).
	Servicio *service.Servicio
	Cfg      config.Config
	Logger   *slog.Logger
}

// Objetos es lo que el trabajador necesita del almacén: leer la entrada, saber
// cuánto ocupa lo que va a sobrescribir, subir lo que produce y, al quitar una
// calidad, borrarla y medir lo que queda. Lo cumple *objectstore.Client; los
// tests lo cumplen en memoria.
type Objetos interface {
	Get(ctx context.Context, t tenancy.Tenant, key string) (io.ReadCloser, error)
	GetRange(ctx context.Context, t tenancy.Tenant, key, rango string) (*objectstore.Objeto, error)
	Put(ctx context.Context, t tenancy.Tenant, key string, r io.Reader, size int64, opts objectstore.PutOptions) error
	Delete(ctx context.Context, t tenancy.Tenant, key string) error
	DeletePrefix(ctx context.Context, t tenancy.Tenant, prefix string) error
	SizePrefix(ctx context.Context, t tenancy.Tenant, prefix string) (int64, error)
}

// Cola es lo que el trabajador necesita de la cola: lo cumple *store.Store.
type Cola interface {
	Reclamar(ctx context.Context, quien string) (domain.Job, error)
	VideoDelTrabajo(ctx context.Context, j domain.Job) (domain.Video, error)
	Terminar(ctx context.Context, jobID uuid.UUID, resultado store.Estado) (domain.Video, error)
	Fallar(ctx context.Context, jobID uuid.UUID, motivo string) (bool, error)
}

// Errores de arranque: el pod se cae con ellos a la vista, que es mejor que un
// bucle que no puede hacer nada en silencio.
var (
	// ErrSinFfmpeg: la imagen no trae ffmpeg o ffprobe en el PATH.
	ErrSinFfmpeg = errors.New("transcoder: ffmpeg y ffprobe tienen que estar en el PATH")
	// ErrSinDependencias: falta la cola o el almacén.
	ErrSinDependencias = errors.New("transcoder: hacen falta Store y Objetos")
)

// Cadencias del bucle. Constantes y no configuración: a nadie le ha hecho falta
// variarlas todavía, y cada variable de entorno nueva es una más que documentar
// en el chart. `// ponytail: si una réplica sobra o falta, súbelo a Config`.
const (
	// intervaloSondeo es cuánto espera el bucle cuando no hay trabajos.
	intervaloSondeo = 3 * time.Second
	// esperaTrasError es cuánto espera tras un fallo hablando con la base, para
	// no llenar el registro si Postgres está caído.
	esperaTrasError = 10 * time.Second
	// esperaBase acota cada conversación con Postgres, para que un trabajador con
	// la base colgada lo diga (el latido deja de refrescarse) en vez de esperar
	// para siempre.
	esperaBase = 30 * time.Second
	// cadenciaLatidoEnTrabajo es cada cuánto se refresca el latido mientras ffmpeg
	// corre, que puede ser horas.
	cadenciaLatidoEnTrabajo = 15 * time.Second
)

// trabajador es el bucle con sus dependencias resueltas.
type trabajador struct {
	cola     Cola
	objetos  Objetos
	servicio *service.Servicio
	cfg      config.Config
	logger   *slog.Logger
	// quien es con qué nombre reclama (claimed_by): el hostname de la réplica.
	quien string
	// ffmpeg y ffprobe son las rutas resueltas de los binarios.
	ffmpeg, ffprobe string
	// latido es el fichero que refresca el bucle (ver latido.go).
	latido string
}

// Run atiende la cola hasta que el contexto termine. Devuelve el error del
// contexto al apagarse (cmd/cast lo trata como parada limpia) o el de arranque
// si falta algo.
func Run(ctx context.Context, d Deps) error {
	if d.Objetos == nil || d.Store == nil {
		return ErrSinDependencias
	}
	w, err := nuevo(d, d.Store, d.Objetos)
	if err != nil {
		return err
	}
	return w.bucle(ctx)
}

// nuevo resuelve las dependencias y comprueba que ffmpeg está. `cola` y
// `objetos` van aparte de Deps para que el test los cumpla con sus dobles.
func nuevo(d Deps, cola Cola, objetos Objetos) (*trabajador, error) {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSinFfmpeg, err)
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSinFfmpeg, err)
	}
	if d.Cfg.TranscoderTmpDir == "" {
		d.Cfg.TranscoderTmpDir = os.TempDir()
	}
	if err := os.MkdirAll(d.Cfg.TranscoderTmpDir, 0o750); err != nil {
		return nil, fmt.Errorf("transcoder: preparar %s: %w", d.Cfg.TranscoderTmpDir, err)
	}
	if d.Cfg.TranscoderThreads <= 0 {
		d.Cfg.TranscoderThreads = 2
	}
	if d.Cfg.TranscoderMaxMinutes <= 0 {
		d.Cfg.TranscoderMaxMinutes = 120
	}
	quien, err := os.Hostname()
	if err != nil || quien == "" {
		quien = "cast-transcoder"
	}
	return &trabajador{
		cola: cola, objetos: objetos, servicio: d.Servicio, cfg: d.Cfg, logger: d.Logger,
		quien: quien, ffmpeg: ffmpeg, ffprobe: ffprobe, latido: RutaLatido(d.Cfg),
	}, nil
}

// bucle es el ciclo: latir, reclamar, atender o esperar.
func (w *trabajador) bucle(ctx context.Context) error {
	w.logger.Info("transcoder: escuchando la cola",
		"quien", w.quien, "threads", w.cfg.TranscoderThreads, "max_minutos", w.cfg.TranscoderMaxMinutes,
		"tmp", w.cfg.TranscoderTmpDir, "latido", w.latido)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		w.latir()
		hubo, err := w.unaVuelta(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			w.logger.Error("transcoder: no se pudo reclamar de la cola", "error", err)
			esperar(ctx, esperaTrasError)
		case !hubo:
			esperar(ctx, intervaloSondeo)
		}
	}
}

// unaVuelta reclama un trabajo y, si lo hay, lo atiende entero. Devuelve si hubo.
func (w *trabajador) unaVuelta(ctx context.Context) (bool, error) {
	ctxBase, cancel := context.WithTimeout(ctx, esperaBase)
	j, err := w.cola.Reclamar(ctxBase, w.quien)
	cancel()
	switch {
	case errors.Is(err, domain.ErrNoExiste):
		return false, nil
	case err != nil:
		return false, err
	}
	w.atender(ctx, j)
	return true, nil
}

// atender procesa un trabajo reclamado de principio a fin y deja constancia en
// la cola: Terminar si salió, Fallar con un motivo corto si no. Todo el trabajo
// corre bajo un plazo (TranscoderMaxMinutes): al vencer, ffmpeg muere y el
// trabajo falla con «tiempo agotado».
func (w *trabajador) atender(ctx context.Context, j domain.Job) {
	inicio := time.Now()
	registro := w.logger.With("job", j.ID, "video", j.VideoID, "tenant", j.Tenant, "kind", j.Kind, "intento", j.Attempts)
	registro.Info("transcoder: trabajo empezado")

	plazo := time.Duration(w.cfg.TranscoderMaxMinutes) * time.Minute
	ctxJob, cancelar := context.WithTimeout(ctx, plazo)
	defer cancelar()
	pararLatido := w.latirMientras(ctxJob)
	defer pararLatido()

	v, err := w.cola.VideoDelTrabajo(ctxJob, j)
	if err == nil {
		var res resultado
		res, err = w.procesar(ctxJob, j, v)
		if err == nil {
			// Lo que se anota en la cola al acabar no depende de que el proceso se
			// esté apagando: si nos cortan justo aquí, el resultado se anota igual.
			cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), esperaBase)
			defer cancel()
			listo, errT := w.cola.Terminar(cctx, j.ID, res.estado)
			if errT != nil {
				// El vídeo se borró mientras se procesaba, o el trabajo ya no está
				// `running`: no hay a quién apuntarle el resultado.
				registro.Warn("transcoder: el trabajo terminó pero no se pudo anotar", "error", errT)
				return
			}
			if w.servicio != nil {
				w.servicio.DeclararCuota(cctx, v.Tenant)
			}
			registro.Info("transcoder: trabajo terminado",
				"duracion", time.Since(inicio).Round(time.Millisecond).String(),
				"video_ms", derefInt64(listo.DurationMS), "ancho", derefInt32(listo.Width), "alto", derefInt32(listo.Height),
				"bytes_derivadas", res.bytesDerivadas, "bytes_total", listo.BytesTotal)
			return
		}
	}

	// Ídem: un apagado a media conversión deja el trabajo devuelto a la cola.
	motivo := motivoDe(err, plazo)
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), esperaBase)
	defer cancel()
	reintenta, errF := w.cola.Fallar(cctx, j.ID, motivo)
	if errF != nil {
		registro.Error("transcoder: el trabajo falló y no se pudo anotar", "motivo", motivo, "error", errF)
		return
	}
	registro.Warn("transcoder: trabajo fallido", "motivo", motivo, "reintenta", reintenta,
		"duracion", time.Since(inicio).Round(time.Millisecond).String())
}

// esperar duerme `d` o hasta que el contexto termine, lo que llegue antes.
func esperar(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func derefInt32(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}
