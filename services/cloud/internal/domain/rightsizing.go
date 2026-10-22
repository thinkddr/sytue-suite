// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"math"
	"sort"
	"time"
)

// ---------------------------------------------------------------------------
// Ajuste de tamaño (rightsizing): del uso REAL a una recomendación accionable
// ---------------------------------------------------------------------------
//
// La promesa del producto es que el cliente pague lo mínimo con el rendimiento
// óptimo, y que lo resuelva con un clic. Esto es el cerebro de esa promesa: una
// función PURA que recibe muestras de uso y devuelve qué hacer y cuánto cambia la
// factura. Pura a propósito: es lo que permite probar el criterio de verdad, sin
// clúster ni base de datos, que es justo lo que separa esto de un adorno.
//
// Tres principios, y el orden importa:
//
//  1. **Sin datos suficientes NO hay recomendación.** Es un estado explícito
//     (ReasonInsufficientData), no una recomendación tibia. Adivinar aquí es peor
//     que callarse: el cliente actúa sobre lo que le decimos.
//  2. **Bajar exige mucha más evidencia que subir.** Subir protege el servicio;
//     bajar lo arriesga. Una recomendación que provoca un OOMKill destruye la
//     confianza para siempre, y con ella el argumento entero del producto.
//  3. **Percentil 95 para el régimen, MÁXIMO para el riesgo.** La media esconde
//     los picos. Pero el p95 también: por definición ignora el 5% más alto, así
//     que una ráfaga que reventaría el tamaño menor no lo movería. Por eso bajar
//     exige además que el PICO observado quepa.

// ResourceSample es una medición puntual del consumo real de un pod.
type ResourceSample struct {
	At          time.Time
	CPUMilli    int64 // milicores consumidos
	MemoryBytes int64 // memoria residente
}

// SizingAction es qué hacer con el tamaño de instancia.
type SizingAction string

const (
	// SizingKeep: el tamaño actual es el correcto (o no hay con qué decidir).
	SizingKeep SizingAction = "keep"
	// SizingDown: cabe en un tamaño menor. Ahorra dinero.
	SizingDown SizingAction = "down"
	// SizingUp: se está quedando corto. Cuesta más y evita una caída.
	SizingUp SizingAction = "up"
)

// SizingReason explica la decisión en términos que la interfaz pueda traducir a
// una frase corta. No es un código de error: es lo que el usuario va a leer.
type SizingReason string

const (
	// ReasonInsufficientData: aún no hay ventana ni muestras bastantes.
	ReasonInsufficientData SizingReason = "insufficient_data"
	// ReasonWellSized: el consumo encaja en el tamaño actual.
	ReasonWellSized SizingReason = "well_sized"
	// ReasonOverProvisioned: sobra tamaño, y hasta el pico cabe en el menor.
	ReasonOverProvisioned SizingReason = "over_provisioned"
	// ReasonPeakBlocksDowngrade: en régimen sobraría, pero el pico observado NO
	// cabría en el tamaño menor. Se dice, porque es información útil: explica por
	// qué no proponemos el ahorro que el cliente cree estar viendo.
	ReasonPeakBlocksDowngrade SizingReason = "peak_blocks_downgrade"
	// ReasonNearLimit: el consumo roza el techo. Riesgo de OOMKill.
	ReasonNearLimit SizingReason = "near_limit"
	// ReasonAtLargest / ReasonAtSmallest: no hay a dónde mover.
	ReasonAtLargest  SizingReason = "at_largest"
	ReasonAtSmallest SizingReason = "at_smallest"
	// ReasonCohortOverProvisioned: aún no hay datos propios, pero las apps
	// comparables van sobradas en un tamaño menor. Es una razón APARTE de
	// ReasonOverProvisioned a propósito: la interfaz no puede decir «hemos medido
	// tu app» cuando lo que hemos medido son las de otros.
	ReasonCohortOverProvisioned SizingReason = "cohort_over_provisioned"
)

// SizingAdvice es la recomendación completa. Lleva lo observado además de la
// decisión para que la interfaz pueda decir «usas el 12 % de lo que pagas» sin
// recalcular nada ni inventarse el número.
type SizingAdvice struct {
	Action SizingAction `json:"action"`
	Reason SizingReason `json:"reason"`
	// From/To son nombres del catálogo. To == From cuando Action es keep.
	From string `json:"from"`
	To   string `json:"to"`
	// MonthlyDeltaEur es el cambio en la factura mensual: NEGATIVO cuando se
	// ahorra. Cero si no hay cambio.
	MonthlyDeltaEur float64 `json:"monthly_delta_eur"`
	// P95MemoryPercent es el p95 de memoria como porcentaje del límite ACTUAL, que
	// es la cifra honesta de «cuánto de lo que pagas estás usando».
	P95MemoryPercent  int   `json:"p95_memory_percent"`
	PeakMemoryPercent int   `json:"peak_memory_percent"`
	P95CPUMilli       int64 `json:"p95_cpu_milli"`
	// Samples y WindowHours dicen sobre qué se decidió. Sin esto la recomendación
	// no es auditable y el cliente no puede saber si confiar.
	Samples     int `json:"samples"`
	WindowHours int `json:"window_hours"`
	// Benchmark es el agregado anónimo de las apps comparables, cuando lo hay.
	// Nulo mientras no existan bastantes empresas parecidas, que es lo normal al
	// principio: ver domain/benchmark.go.
	Benchmark *CohortStat `json:"benchmark,omitempty"`
}

// Umbrales. Son la política del producto, así que están juntos y nombrados: si
// alguien los cambia, tiene que ver de golpe la asimetría deliberada entre subir
// y bajar.
const (
	// minSamples y minWindowForDowngrade: bajar exige una ventana larga y muestras
	// de sobra. Tres días cubren el ciclo semanal parcialmente y, sobre todo, un
	// despliegue que aún se está calentando.
	minSamples             = 72
	minWindowForDowngrade  = 72 * time.Hour
	minSamplesForDowngrade = 288
	// minWindowForUpgrade: subir se propone con MUCHA menos evidencia, porque el
	// coste de equivocarse es una factura algo mayor y no una caída.
	minWindowForUpgrade = 6 * time.Hour

	// upgradeP95Trigger: en régimen ya se está rozando el techo.
	upgradeP95Trigger = 0.80
	// upgradePeakTrigger: aunque el régimen sea cómodo, un pico a esta altura es
	// un OOMKill esperando a que el tráfico suba un poco.
	upgradePeakTrigger = 0.95

	// downgradeP95Ceiling: el régimen tiene que caber HOLGADO en el tamaño menor,
	// no a ras. El 60 % deja sitio para crecer sin volver a mover nada.
	downgradeP95Ceiling = 0.60
	// downgradePeakCeiling: y el PICO observado tiene que caber también. Esta es
	// la línea que impide que una sola ráfaga acabe en OOMKill tras «ahorrar».
	downgradePeakCeiling = 0.90
)

// MinSamplesForAdvice es el mínimo de muestras por debajo del cual este motor NO
// dice nada (devuelve ReasonInsufficientData sin mirar nada más). Se exporta para
// que el barrido proactivo pueda descartar en SQL los proyectos sobre los que no
// habría consejo, en vez de cargar el histórico de cada uno para acabar callando.
// Es el MISMO número por construcción: dos umbrales separados harían que el
// prefiltro y la decisión discreparan sin que nada fallara.
const MinSamplesForAdvice = minSamples

// SizingAlertPayload es el cuerpo del aviso de tamaño que viaja al webhook. Lleva
// el porqué y el importe además del cambio propuesto: quien lo recibe tiene que
// poder decidir sin abrir el panel.
type SizingAlertPayload struct {
	ProjectID       string       `json:"project_id"`
	Action          SizingAction `json:"action"`
	Reason          SizingReason `json:"reason"`
	FromSize        string       `json:"from_size"`
	ToSize          string       `json:"to_size"`
	MonthlyDeltaEur float64      `json:"monthly_delta_eur"`
	// P95MemoryPercent es la medida sobre la que se decidió, en % del límite del
	// tamaño ACTUAL. Sin ella el aviso no es contrastable.
	P95MemoryPercent int `json:"p95_memory_percent"`
}

// hoursPerMonth es el mes de facturación convencional (365 d / 12).
const hoursPerMonth = 730.0

// RecommendInstanceSize decide qué hacer con el tamaño de una app a partir de su
// consumo real. replicas escala el impacto en la factura, no la decisión: el
// tamaño se decide por el consumo de UN pod, que es lo que se topa contra el
// límite.
//
// Deliberadamente NO recomienda cambiar el número de réplicas ni el tamaño del
// disco. Las réplicas existen sobre todo por disponibilidad —bajarlas mirando
// solo CPU y memoria es un consejo peligroso disfrazado de ahorro— y el disco
// necesita una métrica de ocupación que aquí no se mide. Cuando haya ese dato,
// será otra función; mezclarlo aquí solo produciría consejos con menos evidencia
// que los que este motor exige.
func RecommendInstanceSize(current string, replicas int, samples []ResourceSample) SizingAdvice {
	size := InstanceSizeOrDefault(current)
	adv := SizingAdvice{
		Action: SizingKeep, Reason: ReasonInsufficientData,
		From: size.Name, To: size.Name, Samples: len(samples),
	}
	if replicas < 1 {
		replicas = 1
	}
	limit := memoryLimitBytes(size)
	if limit <= 0 || len(samples) < minSamples {
		return adv
	}

	window := sampleWindow(samples)
	adv.WindowHours = int(window.Hours())
	if window < minWindowForUpgrade {
		return adv
	}

	mems := make([]int64, 0, len(samples))
	cpus := make([]int64, 0, len(samples))
	for _, s := range samples {
		mems = append(mems, s.MemoryBytes)
		cpus = append(cpus, s.CPUMilli)
	}
	p95Mem := percentile(mems, 95)
	peakMem := maxOf(mems)
	adv.P95CPUMilli = percentile(cpus, 95)
	adv.P95MemoryPercent = percentOf(p95Mem, limit)
	adv.PeakMemoryPercent = percentOf(peakMem, limit)

	// --- Subir: poca evidencia basta, porque protege el servicio -------------
	if float64(p95Mem) >= float64(limit)*upgradeP95Trigger ||
		float64(peakMem) >= float64(limit)*upgradePeakTrigger {
		next, ok := neighbourSize(size.Name, +1)
		if !ok {
			adv.Reason = ReasonAtLargest
			return adv
		}
		adv.Action, adv.Reason, adv.To = SizingUp, ReasonNearLimit, next.Name
		adv.MonthlyDeltaEur = monthlyCostEur(next, replicas) - monthlyCostEur(size, replicas)
		return adv
	}

	// --- Bajar: ventana larga, muestras de sobra y que quepa hasta el pico ---
	prev, ok := neighbourSize(size.Name, -1)
	if !ok {
		adv.Action, adv.Reason = SizingKeep, ReasonAtSmallest
		return adv
	}
	if window < minWindowForDowngrade || len(samples) < minSamplesForDowngrade {
		adv.Action, adv.Reason = SizingKeep, ReasonInsufficientData
		return adv
	}
	target := memoryLimitBytes(prev)
	if float64(p95Mem) > float64(target)*downgradeP95Ceiling {
		adv.Action, adv.Reason = SizingKeep, ReasonWellSized
		return adv
	}
	if float64(peakMem) > float64(target)*downgradePeakCeiling {
		// El régimen cabría, el pico no. Se cuenta en vez de callarlo: es la
		// diferencia entre «no puedes ahorrar» y «no puedes ahorrar POR ESTO».
		adv.Action, adv.Reason = SizingKeep, ReasonPeakBlocksDowngrade
		return adv
	}
	adv.Action, adv.Reason, adv.To = SizingDown, ReasonOverProvisioned, prev.Name
	adv.MonthlyDeltaEur = monthlyCostEur(prev, replicas) - monthlyCostEur(size, replicas)
	return adv
}

// monthlyCostEur es lo que cuesta un tamaño al mes con N réplicas, con el MISMO
// precio y la MISMA unidad que factura el medidor (GB·h): si se separasen, el
// ahorro prometido no coincidiría con la factura, que es la peor forma posible de
// perder la confianza del cliente.
func monthlyCostEur(s InstanceSize, replicas int) float64 {
	price, _ := PriceOf("compute_gb_hours")
	return s.MemoryGiB * float64(replicas) * hoursPerMonth * price
}

// memoryLimitBytes es el techo de memoria en bytes. Sale de MemoryGiB —que es el
// LÍMITE y la unidad de facturación— para no parsear cantidades de Kubernetes
// dentro del dominio.
func memoryLimitBytes(s InstanceSize) int64 {
	return int64(s.MemoryGiB * float64(1<<30))
}

// neighbourSize devuelve el tamaño contiguo en el catálogo (+1 mayor, -1 menor).
func neighbourSize(name string, step int) (InstanceSize, bool) {
	for i, s := range InstanceSizes {
		if s.Name != name {
			continue
		}
		j := i + step
		if j < 0 || j >= len(InstanceSizes) {
			return InstanceSize{}, false
		}
		return InstanceSizes[j], true
	}
	return InstanceSize{}, false
}

// sampleWindow es el tiempo cubierto entre la muestra más antigua y la más nueva.
func sampleWindow(samples []ResourceSample) time.Duration {
	if len(samples) < 2 {
		return 0
	}
	first, last := samples[0].At, samples[0].At
	for _, s := range samples[1:] {
		if s.At.Before(first) {
			first = s.At
		}
		if s.At.After(last) {
			last = s.At
		}
	}
	return last.Sub(first)
}

// percentile devuelve el percentil p (0-100) por el método del vecino más
// cercano. Copia la entrada: ordenar la del llamante sería un efecto secundario
// sorpresa en una función que se anuncia como pura.
func percentile(vals []int64, p int) int64 {
	if len(vals) == 0 {
		return 0
	}
	s := append([]int64(nil), vals...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	idx := int(math.Ceil(float64(p)/100*float64(len(s)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(s) {
		idx = len(s) - 1
	}
	return s[idx]
}

func maxOf(vals []int64) int64 {
	var m int64
	for _, v := range vals {
		if v > m {
			m = v
		}
	}
	return m
}

func percentOf(v, total int64) int {
	if total <= 0 {
		return 0
	}
	return int(math.Round(float64(v) / float64(total) * 100))
}
