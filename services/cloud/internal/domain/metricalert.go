// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

// Consumo al límite: la señal que avisa ANTES del problema.
//
// El aviso de tamaño (rightsizing.go) responde a «¿estás pagando lo que te
// conviene?» y puede esperar a mañana. Esto responde a otra pregunta: «¿esto se
// cae hoy?». Un proyecto perfectamente dimensionado al que le entra tráfico
// también se queda sin memoria, y ahí el consejo de tamaño dice `keep` mientras
// el contenedor se reinicia.
//
// El cálculo vive aquí y no en el evaluador para poder probarlo sin base de
// datos: es una división y una comparación, pero es la división que decide si
// alguien recibe el aviso a tiempo o no lo recibe.

// MetricUsage es el consumo de UN recurso como porcentaje del límite del tamaño
// contratado.
type MetricUsage struct {
	// Resource es "cpu" o "memory" (los valores que persiste metric_alerts.kind).
	Resource string
	Percent  int
	// Over indica si el consumo alcanza el umbral de aviso.
	Over bool
}

// Nombres de recurso. Coinciden con store.MetricKind*, que es lo que se guarda.
const (
	MetricResourceCPU    = "cpu"
	MetricResourceMemory = "memory"
)

// MetricUsageOf traduce una recomendación de tamaño —que ya trae medidos el p95
// de CPU y el de memoria— al consumo de los dos recursos.
//
// Se usa el p95 y no el pico a propósito: un pico de un minuto lo absorbe
// cualquier aplicación, mientras que un p95 alto significa que el 95 % del tiempo
// se está rozando el techo, que es cuando llegan los reinicios por memoria y el
// estrangulamiento de CPU. El pico ya tiene su papel en la decisión de bajar de
// tamaño (PeakMemoryPercent), donde impedir el cambio sí es lo correcto.
//
// Un tamaño sin límite de CPU declarado da 0 %, no un porcentaje inventado: sin
// techo no hay «cerca del techo», y avisar ahí sería ruido puro.
func MetricUsageOf(adv SizingAdvice) []MetricUsage {
	cpu := 0
	if lim := InstanceSizeOrDefault(adv.From).CPULimitMilli(); lim > 0 {
		cpu = int(adv.P95CPUMilli * 100 / int64(lim))
	}
	return []MetricUsage{
		{Resource: MetricResourceCPU, Percent: cpu, Over: cpu >= MetricAlertPercent},
		{
			Resource: MetricResourceMemory,
			Percent:  adv.P95MemoryPercent,
			Over:     adv.P95MemoryPercent >= MetricAlertPercent,
		},
	}
}
