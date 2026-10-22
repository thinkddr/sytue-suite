// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// Cohortes: qué tamaño le basta a una app COMO LA TUYA
// ---------------------------------------------------------------------------
//
// El motor de rightsizing decide con el histórico del propio proyecto, y por eso
// no puede decir nada durante los primeros días: es justo cuando el cliente elige
// tamaño a ciegas y casi siempre se pasa. La cohorte cubre ese hueco — «las apps
// como la tuya van sobradas en básico» se puede afirmar el día cero.
//
// INVARIANTE DE PRIVACIDAD, y es lo único que hay que entender de este fichero:
// agregar entre inquilinos es exactamente lo que la RLS existe para impedir, así
// que el umbral de k-anonimato se aplica AL PUBLICAR, no al consultar. Una
// cohorte que no reúna MinCohortTenants inquilinos DISTINTOS no se escribe, y la
// tabla lo hace cumplir además con un CHECK: ni un error de programación puede
// publicar una cohorte de dos clientes. El camino de lectura no ve jamás una fila
// cruda de otro.

// MinCohortTenants es el umbral de k-anonimato. Con 5 inquilinos distintos, ver
// el percentil de la cohorte no permite deducir el consumo de ninguno en
// concreto; con 2 sí, y con 1 la «cohorte» ES el cliente.
//
// Subirlo es gratis y bajarlo NO es una decisión de código: está replicado como
// CHECK en la migración 0029 a propósito, para que cambiarlo obligue a mirar las
// dos mitades.
const MinCohortTenants = 5

// MinCohortSamples es cuánto hay que haber observado un proyecto para que cuente
// en una cohorte. Es EXACTAMENTE el mismo mínimo que exige el motor de
// rightsizing: un proyecto con menos datos que eso no es un dato, y meterlo en la
// media sesgaría el consejo que se le da a los demás.
const MinCohortSamples = minSamples

// CohortKey identifica un grupo de proyectos comparables. Son las tres cosas que
// de verdad predicen el consumo: con qué está hecha la app, qué servicios
// arrastra y qué tamaño tiene pedido.
type CohortKey struct {
	Framework string `json:"framework"`
	// Services son los tipos de servicio gestionado del proyecto, ordenados y
	// unidos por «+» para que la clave sea estable («postgres+storage").
	Services string `json:"services"`
	Size     string `json:"size"`
}

// ProjectUsage es lo observado de UN proyecto, ya reducido a su cifra. Lleva el
// inquilino porque el umbral cuenta inquilinos distintos y no proyectos: diez
// proyectos de la misma empresa siguen siendo una sola empresa.
type ProjectUsage struct {
	Tenant string
	Cohort CohortKey
	// P95MemoryPercent es el percentil 95 de memoria como porcentaje del límite
	// del tamaño que tiene pedido.
	P95MemoryPercent int
}

// Cohort es el agregado PUBLICABLE de un grupo. Solo existe si superó el umbral.
type Cohort struct {
	Key CohortKey `json:"key"`
	// P50/P90 del uso típico de la cohorte, en porcentaje del límite.
	P50MemoryPercent int `json:"p50_memory_percent"`
	P90MemoryPercent int `json:"p90_memory_percent"`
	// Tenants y Projects se guardan para que la publicación sea AUDITABLE: sin
	// ellos nadie puede comprobar a posteriori que el umbral se respetó.
	Tenants  int `json:"tenants"`
	Projects int `json:"projects"`
}

// ServicesKey normaliza la lista de servicios de un proyecto a la forma estable
// de la clave de cohorte. Ordena y deduplica: el orden en que se crearon los
// servicios no puede partir una cohorte en dos.
func ServicesKey(kinds []ServiceKind) string {
	if len(kinds) == 0 {
		return ""
	}
	seen := make(map[string]bool, len(kinds))
	out := make([]string, 0, len(kinds))
	for _, k := range kinds {
		s := string(k)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return strings.Join(out, "+")
}

// UsagePercentOfSize convierte el p95 de memoria observado de un proyecto en el
// porcentaje del límite de SU tamaño, que es la única cifra que llega a
// publicarse: un porcentaje no dice cuánta memoria mueve una empresa, solo cómo
// de ajustado va a lo que pidió.
//
// Se acota a 100 porque el medidor puede leer momentáneamente por encima del
// límite (el cgroup lo permite antes de matar el pod) y la tabla lo rechazaría.
func UsagePercentOfSize(p95Bytes int64, size string) int {
	limit := memoryLimitBytes(InstanceSizeOrDefault(size))
	pct := percentOf(p95Bytes, limit)
	if pct > 100 {
		return 100
	}
	if pct < 0 {
		return 0
	}
	return pct
}

// BuildCohorts agrupa lo observado y devuelve SOLO las cohortes publicables.
//
// Función pura: es lo que permite probar el umbral de verdad, que es la única
// parte de esto que no puede fallar. Las cohortes que no llegan al umbral no se
// devuelven «marcadas»: no se devuelven, y punto — así no hay forma de que una
// capa de arriba las publique por descuido.
func BuildCohorts(usages []ProjectUsage) []Cohort {
	type group struct {
		percents []int
		tenants  map[string]bool
	}
	byKey := make(map[CohortKey]*group)
	for _, u := range usages {
		// Un proyecto sin inquilino o sin tamaño no es agrupable; ignorarlo es
		// preferible a inventarle una cohorte.
		if u.Tenant == "" || u.Cohort.Size == "" {
			continue
		}
		g, ok := byKey[u.Cohort]
		if !ok {
			g = &group{tenants: map[string]bool{}}
			byKey[u.Cohort] = g
		}
		g.percents = append(g.percents, u.P95MemoryPercent)
		g.tenants[u.Tenant] = true
	}

	out := make([]Cohort, 0, len(byKey))
	for key, g := range byKey {
		if len(g.tenants) < MinCohortTenants {
			continue
		}
		out = append(out, Cohort{
			Key:              key,
			P50MemoryPercent: percentileInt(g.percents, 50),
			P90MemoryPercent: percentileInt(g.percents, 90),
			Tenants:          len(g.tenants),
			Projects:         len(g.percents),
		})
	}
	// Orden estable: el resultado se persiste y se compara entre ejecuciones.
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].Key, out[j].Key
		if a.Framework != b.Framework {
			return a.Framework < b.Framework
		}
		if a.Services != b.Services {
			return a.Services < b.Services
		}
		return a.Size < b.Size
	})
	return out
}

// CohortStat es lo que la interfaz enseña de la cohorte junto a la
// recomendación: sin las dos cifras de auditoría, «las apps como la tuya» es una
// afirmación que el cliente no puede contrastar ni creer.
type CohortStat struct {
	P50MemoryPercent int `json:"p50_memory_percent"`
	P90MemoryPercent int `json:"p90_memory_percent"`
	Tenants          int `json:"tenants"`
	Projects         int `json:"projects"`
}

// ApplyCohort completa un consejo que se quedó SIN DATOS PROPIOS con lo que se
// sabe de las apps comparables. Es el hueco que el motor no puede cubrir: los tres
// primeros días, que son justo cuando el cliente eligió tamaño a ciegas.
//
// Tres límites deliberados:
//
//  1. **El histórico propio siempre gana.** Si el consejo ya se decidió con datos
//     del proyecto, esto no lo toca. La cohorte solo rellena el silencio.
//  2. **Nunca propone SUBIR.** Subir por el consumo de terceros es cobrarle más a
//     alguien de quien no sabemos nada. Si su app de verdad se queda corta, el
//     motor lo verá en horas con datos suyos.
//  3. **Baja un escalón, no los que hagan falta.** Con cero datos propios, saltar
//     de `large` a `basic` de un clic es una apuesta; un escalón se corrige solo
//     al día siguiente y el ahorro llega igual.
func ApplyCohort(adv SizingAdvice, replicas int, c Cohort) SizingAdvice {
	if adv.Reason != ReasonInsufficientData || c.Tenants < MinCohortTenants {
		return adv
	}
	adv.Benchmark = &CohortStat{
		P50MemoryPercent: c.P50MemoryPercent,
		P90MemoryPercent: c.P90MemoryPercent,
		Tenants:          c.Tenants,
		Projects:         c.Projects,
	}
	if replicas < 1 {
		replicas = 1
	}
	prev, ok := neighbourSize(adv.From, -1)
	if !ok {
		return adv // ya está en el más pequeño: solo informar
	}
	// Los porcentajes de la cohorte están medidos sobre el límite del tamaño de la
	// cohorte, que se resuelve explícitamente en vez de dar por hecho que coincide
	// con el del proyecto.
	p90Bytes := int64(float64(memoryLimitBytes(InstanceSizeOrDefault(c.Key.Size))) *
		float64(c.P90MemoryPercent) / 100)
	// El mismo margen que exige el motor para bajar con datos propios: nueve de
	// cada diez apps comparables tienen que caber, y con holgura.
	if float64(p90Bytes) > float64(memoryLimitBytes(prev))*downgradePeakCeiling {
		return adv
	}
	adv.Action, adv.Reason, adv.To = SizingDown, ReasonCohortOverProvisioned, prev.Name
	adv.MonthlyDeltaEur = monthlyCostEur(prev, replicas) -
		monthlyCostEur(InstanceSizeOrDefault(adv.From), replicas)
	return adv
}

// percentileInt es el percentil por vecino más cercano sobre enteros. Copia la
// entrada: ordenar la del llamante sería un efecto secundario sorpresa.
func percentileInt(vals []int, p int) int {
	if len(vals) == 0 {
		return 0
	}
	s := append([]int(nil), vals...)
	sort.Ints(s)
	// Redondeo hacia arriba de p/100 * n, con enteros.
	idx := (p*len(s) + 99) / 100
	if idx < 1 {
		idx = 1
	}
	if idx > len(s) {
		idx = len(s)
	}
	return s[idx-1]
}
