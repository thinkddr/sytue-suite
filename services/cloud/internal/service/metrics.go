// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Consumo real de un proyecto, para enseñarlo
// ---------------------------------------------------------------------------
//
// Las muestras existen desde la migración 0026 y hasta ahora solo las miraba el
// motor de rightsizing: el panel podía decir «te sobra tamaño» pero no enseñar el
// consumo del que salía esa conclusión. Una recomendación de dinero que no puede
// contrastarse es justo la que nadie aplica.
//
// Va aparte del consejo a propósito: son dos preguntas distintas —«qué hago» y
// «qué está pasando»— y la segunda tiene respuesta aunque la primera sea «aún no
// he medido bastante».

// MetricPoint es una observación del consumo del proyecto en un instante.
type MetricPoint struct {
	At          time.Time `json:"at"`
	CPUMilli    int64     `json:"cpu_milli"`
	MemoryBytes int64     `json:"memory_bytes"`
}

// ProjectMetrics es la serie temporal del consumo con lo que hace falta para
// leerla: el techo contra el que se compara y cada cuánto se muestrea.
type ProjectMetrics struct {
	Points []MetricPoint `json:"points"`
	// MemoryLimitBytes es el techo del tamaño ACTUAL. Sin él, una gráfica de bytes
	// no dice nada: 300 MiB es holgado en `standard` y un OOMKill a punto en
	// `basic`.
	MemoryLimitBytes int64 `json:"memory_limit_bytes"`
	// CPUMilliLimit es el techo de CPU del tamaño actual, en milicores.
	CPUMilliLimit int64  `json:"cpu_milli_limit"`
	InstanceSize  string `json:"instance_size"`
	// BucketSeconds es la separación entre puntos (el cubo de agregación). La
	// gráfica lo necesita para saber si un hueco es «no se midió» o «no había
	// nada corriendo».
	BucketSeconds int `json:"bucket_seconds"`
	WindowHours   int `json:"window_hours"`
}

// maxMetricsWindow acota la ventana a lo que se conserva de verdad: la purga se
// lleva las muestras a los 30 días, así que pedir más solo devolvería un tramo
// vacío que la gráfica tendría que explicar.
const maxMetricsWindow = 7 * 24 * time.Hour

// ProjectResourceMetrics devuelve el consumo observado de un proyecto en las
// últimas `window` horas.
//
// Serie VACÍA no es un error: es lo que hay cuando el proyecto acaba de nacer, o
// cuando el clúster no tiene metrics-server. El panel lo dice con esas palabras
// en vez de pintar una gráfica plana que se leería como «no consume nada».
func (s *Service) ProjectResourceMetrics(ctx context.Context, id uuid.UUID, window time.Duration) (ProjectMetrics, error) {
	if window <= 0 || window > maxMetricsWindow {
		window = 24 * time.Hour
	}
	proj, err := s.store.GetProject(ctx, id)
	if err != nil {
		return ProjectMetrics{}, err
	}
	size := proj.EffectiveInstanceSize()
	out := ProjectMetrics{
		Points:           []MetricPoint{},
		MemoryLimitBytes: int64(size.MemoryGiB * float64(1<<30)),
		CPUMilliLimit:    int64(size.CPULimitMilli()),
		InstanceSize:     size.Name,
		BucketSeconds:    int(sampleBucket.Seconds()),
		WindowHours:      int(window.Hours()),
	}
	ss, ok := s.store.(samplesStore)
	if !ok {
		return out, nil // store sin muestras (dobles de test): serie vacía, no un fallo
	}
	samples, err := ss.ListResourceSamples(ctx, id, time.Now().Add(-window))
	if err != nil {
		return ProjectMetrics{}, err
	}
	for _, m := range samples {
		out.Points = append(out.Points, MetricPoint{
			At: m.At, CPUMilli: m.CPUMilli, MemoryBytes: m.MemoryBytes,
		})
	}
	return out, nil
}

// sampleBucket es la separación real entre muestras. Se declara aquí y no se
// importa de `store` para no arrastrar ese paquete a la respuesta HTTP; que
// coincida lo fija el test.
const sampleBucket = 15 * time.Minute
