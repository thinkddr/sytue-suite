// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package transcoder

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"git.sytue.eu/sytue/one/services/cast/internal/config"
)

// El transcodificador no escucha en ningún puerto, así que la sonda de vida no
// puede ser el `healthcheck` del servicio (pide /healthz). En su lugar, el bucle
// refresca un FICHERO DE LATIDO en cada vuelta y, mientras ffmpeg corre, cada
// pocos segundos: quien vigila (el HEALTHCHECK de Dockerfile.transcoder o la
// sonda del chart) mira que exista y que su hora de modificación sea reciente.
// Un bucle colgado —Postgres que no contesta, un ffmpeg zombi fuera de plazo—
// deja de latir y el contenedor se reinicia.

// NombreLatido es el nombre del fichero de latido dentro de TranscoderTmpDir.
// Con el TMPDIR de la imagen (/tmp) queda en /tmp/sytue-cast-transcoder.latido,
// que es la ruta que el Dockerfile comprueba.
const NombreLatido = "sytue-cast-transcoder.latido"

// EdadMaximaLatido es cuánto puede llevar el latido sin refrescarse antes de dar
// el trabajador por colgado. Holgado a propósito frente a las cadencias del bucle
// (3 s sin trabajo, 15 s en trabajo): una pausa de recolección o un nodo cargado
// no son motivo para reiniciar.
const EdadMaximaLatido = 5 * time.Minute

// RutaLatido es dónde late el trabajador con esa configuración.
func RutaLatido(cfg config.Config) string {
	dir := cfg.TranscoderTmpDir
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, NombreLatido)
}

// latir refresca el fichero: escribe la hora, con lo que su mtime es ahora. Un
// fallo al escribir se registra y no para el bucle: el latido es para que nos
// vigilen, no un requisito para trabajar.
func (w *trabajador) latir() {
	if err := escribirLatido(w.latido, time.Now()); err != nil {
		w.logger.Warn("transcoder: no se pudo refrescar el latido", "ruta", w.latido, "error", err)
	}
}

// latirMientras refresca el latido cada cadenciaLatidoEnTrabajo hasta que se
// llame a la función devuelta o el contexto termine.
func (w *trabajador) latirMientras(ctx context.Context) (parar func()) {
	ctx, cancelar := context.WithCancel(ctx)
	hecho := make(chan struct{})
	go func() {
		defer close(hecho)
		t := time.NewTicker(cadenciaLatidoEnTrabajo)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				w.latir()
			}
		}
	}()
	return func() {
		cancelar()
		<-hecho
	}
}

// escribirLatido deja la hora en el fichero (y con ello su mtime).
func escribirLatido(ruta string, ahora time.Time) error {
	if err := os.WriteFile(ruta, []byte(ahora.UTC().Format(time.RFC3339)+"\n"), 0o644); err != nil { //nolint:gosec // G306: es un latido, sin nada que proteger; lo lee la sonda
		return fmt.Errorf("escribir latido: %w", err)
	}
	return nil
}

// ComprobarLatido dice si el trabajador que late en `ruta` sigue vivo: el fichero
// existe y se refrescó hace menos de `maxEdad`. Es lo que un subcomando de
// cmd/cast puede llamar como sonda sin shell; el Dockerfile hace lo mismo con
// `stat` mientras ese subcomando no exista.
func ComprobarLatido(ruta string, maxEdad time.Duration, ahora time.Time) error {
	info, err := os.Stat(ruta)
	if err != nil {
		return fmt.Errorf("latido: %w", err)
	}
	if edad := ahora.Sub(info.ModTime()); edad > maxEdad {
		return fmt.Errorf("latido: el último fue hace %s (tope %s)", edad.Round(time.Second), maxEdad)
	}
	return nil
}
