// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

// # Las tareas en segundo plano en el escritorio
//
// En el móvil a una app cerrada la despierta el sistema (tareas_fondo.go). En el
// escritorio no hay quien, salvo el Hub, que vive en la bandeja y arranca al iniciar
// sesión: cada PeriodoTareasEnFondo lanza las apps hermanas con trabajo pendiente con
// `escritorio.ArgTareaEnFondo`, y cada una corre su TareaEnFondo —la misma del móvil,
// con la sesión guardada y sin ventana— y sale (TareaEnFondoEscritorio). La primera es
// la copia de seguridad de Photos: con la app cerrada, las fotos nuevas de la carpeta de
// imágenes suben igual. Lo que la app abierta y la tarea comparten va en su registro, así
// que una espera a la otra; si la copia no está encendida, la tarea sale en el acto.

// TareasEnFondoEscritorio son los productos (el esquema sin «sytue-», como
// RegistrarTareaEnFondo) cuya tarea despierta el Hub en el escritorio.
var TareasEnFondoEscritorio = []string{"photos"}

const (
	// PrimeraTareaEnFondo es cuánto espera el Hub tras arrancar: al iniciar sesión el
	// equipo tiene otras cosas que hacer.
	PrimeraTareaEnFondo = 10 * time.Minute
	// PeriodoTareasEnFondo es cada cuánto vuelve a lanzarlas. Más a menudo que el
	// respaldo de seis horas de Android, que además se despierta con cada foto nueva:
	// aquí no hay quien avise de las fotos nuevas de una carpeta.
	PeriodoTareasEnFondo = 2 * time.Hour
)

// lanzarTareaHermana lanza la tarea de la app hermana y espera a que acabe; las pruebas
// la cambian.
var lanzarTareaHermana = func(ctx context.Context, producto string) error {
	return escritorio.LanzarTareaEnFondo(ctx, escritorio.EjecutableDeHermana(producto))
}

// DespertarTareasEnFondo, desde el Hub en la bandeja, lanza las tareas en segundo plano
// de las apps hermanas mientras haya alguien dentro, hasta que `ctx` acabe.
func (s *Suite) DespertarTareasEnFondo(ctx context.Context) {
	go despertarCada(ctx, PrimeraTareaEnFondo, PeriodoTareasEnFondo,
		func() bool { return s.Estado().Dentro() }, TareasEnFondoEscritorio)
}

// despertarCada es el bucle de DespertarTareasEnFondo: una ronda tras `primera` y luego
// cada `cada`, una app detrás de otra. Sin nadie dentro no se lanza nada: la tarea no
// tendría sesión con la que subir.
func despertarCada(ctx context.Context, primera, cada time.Duration, dentro func() bool, productos []string) {
	t := time.NewTimer(primera)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if dentro() {
			for _, p := range productos {
				err := lanzarTareaHermana(ctx, p)
				if err != nil && !errors.Is(err, escritorio.ErrNoDisponible) {
					slog.Info("sesion: la tarea en segundo plano de una app hermana no terminó bien", "producto", p, "error", err)
				}
			}
		}
		t.Reset(cada)
	}
}

// TareaEnFondoEscritorio es el lado de la app: lanzada por el Hub con
// `escritorio.ArgTareaEnFondo`, corre su tarea con la sesión guardada, sin ventana, y
// devuelve con qué código salir (1 si falló y conviene reintentar).
func TareaEnFondoEscritorio(bundle, producto string) int {
	ctx, cancelar := context.WithTimeout(context.Background(), escritorio.PlazoTareaEnFondo)
	defer cancelar()
	if TareaSinVentana(ctx, SinVentana{Bundle: bundle, Producto: producto}) == Fallo {
		return 1
	}
	return 0
}
