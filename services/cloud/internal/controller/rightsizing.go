// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// ---------------------------------------------------------------------------
// Muestreo del uso REAL de las apps de cliente
// ---------------------------------------------------------------------------
//
// Es el sensor del que depende toda la optimización de coste. Sin él, el medidor
// solo sabe cuánta memoria se PIDIÓ —réplicas × tamaño × horas—, así que decirle
// a alguien «te sobra tamaño» sería inventárselo.
//
// Lee metrics.k8s.io por el cliente REST que ya existe, sin añadir la dependencia
// k8s.io/metrics: go.mod es un fichero compartido y un módulo nuevo por un struct
// de seis campos no compensa.

// sampleEvery espacia las llamadas a la API de métricas. Coincide a propósito con
// store.SampleBucket: muestrear más a menudo solo produciría escrituras que el
// GREATEST del cubo absorbe, y llamadas de más al metrics-server.
const sampleEvery = store.SampleBucket

// sampleRetention es cuánto se conservan las muestras. El motor necesita tres
// días; el resto se guarda solo para poder mirar la evolución.
const sampleRetention = 30 * 24 * time.Hour

// metricsMissingOnce evita repetir en cada ciclo el aviso de que no hay
// metrics-server. Es un no-op esperado en un clúster sin el complemento, no un
// error: llenar el log de ruido cada 15 s escondería los avisos que sí importan.
var metricsMissingOnce sync.Once

// resourceSampler es la capacidad OPCIONAL de leer el uso real del clúster. La
// implementa solo KubeProvisioner; el StubProvisioner de dev no tiene métricas
// que leer. Se descubre por type-assert, igual que postgresRoleRepairer.
type resourceSampler interface {
	SampleResourceUsage(ctx context.Context) ([]store.ResourceSampleInput, error)
}

var _ resourceSampler = (*KubeProvisioner)(nil)

// reconcileResourceSamples es el paso del bucle: muestrea, persiste y purga. Se
// auto-espacia, así que puede vivir en un ciclo de 15 s sin castigar a nadie.
func (c *Controller) reconcileResourceSamples(ctx context.Context) error {
	s, ok := c.provider.(resourceSampler)
	if !ok {
		return nil
	}
	now := time.Now()
	if !c.sampleDue(now) {
		return nil
	}
	rows, err := s.SampleResourceUsage(ctx)
	if err != nil {
		return err
	}
	if err := c.store.RecordResourceSamples(ctx, rows); err != nil {
		return err
	}
	if n, perr := c.store.PurgeResourceSamples(ctx, now.Add(-sampleRetention)); perr != nil {
		return perr
	} else if n > 0 {
		log.From(ctx).Info("muestras de uso purgadas", "filas", n)
	}
	return nil
}

// cohortEvery espacia el recálculo de los benchmarks anónimos. Seis horas: el
// agregado de cientos de proyectos no se mueve en minutos, y la consulta recorre
// TODOS los inquilinos, así que es lo más caro que hace el bucle.
const cohortEvery = 6 * time.Hour

// cohortWindow es el histórico con el que se construyen las cohortes. Una semana
// amortigua el fin de semana, que en apps de negocio hunde el consumo y haría
// oscilar la recomendación según el día en que se recalculó.
const cohortWindow = 7 * 24 * time.Hour

// reconcileCohorts recalcula y republica los benchmarks anónimos.
//
// El umbral de k-anonimato lo aplica domain.BuildCohorts y lo vuelve a exigir el
// CHECK de la tabla: aquí no hay ninguna decisión de privacidad que tomar, y es
// deliberado —este es el sitio con más probabilidad de que alguien «arregle» algo
// deprisa—.
func (c *Controller) reconcileCohorts(ctx context.Context) error {
	now := time.Now()
	if !c.cohortsDue(now) {
		return nil
	}
	usages, err := c.store.CollectCohortUsage(ctx, now.Add(-cohortWindow))
	if err != nil {
		return err
	}
	cohorts := domain.BuildCohorts(usages)
	if err := c.store.PublishCohorts(ctx, cohorts); err != nil {
		return err
	}
	log.From(ctx).Info("benchmarks recalculados",
		"cohortes", len(cohorts), "proyectos_observados", len(usages))
	return nil
}

// cohortsDue dice si toca recalcular. Espaciado POR PROCESO igual que el
// muestreo: dos réplicas recalcularían ambas, y no pasa nada porque la
// publicación reemplaza la tabla entera dentro de una transacción.
func (c *Controller) cohortsDue(now time.Time) bool {
	c.cohortMu.Lock()
	defer c.cohortMu.Unlock()
	if now.Sub(c.lastCohorts) < cohortEvery {
		return false
	}
	c.lastCohorts = now
	return true
}

// sampleDue dice si toca muestrear y anota el instante. El espaciado es POR
// PROCESO, así que dos réplicas del controlador muestrearán ambas; no es un
// problema porque la escritura es idempotente por cubo y se queda con el máximo
// (ver store.RecordResourceSamples).
func (c *Controller) sampleDue(now time.Time) bool {
	c.sampleMu.Lock()
	defer c.sampleMu.Unlock()
	if now.Sub(c.lastSample) < sampleEvery {
		return false
	}
	c.lastSample = now
	return true
}

// SampleResourceUsage lee el consumo de TODOS los pods de app en producción y lo
// agrega por proyecto. Si no hay metrics-server, es un no-op: el complemento no
// es obligatorio en un clúster de Kubernetes y su ausencia no puede tumbar la
// reconciliación de todo lo demás.
func (p *KubeProvisioner) SampleResourceUsage(ctx context.Context) ([]store.ResourceSampleInput, error) {
	raw, err := p.cs.CoreV1().RESTClient().Get().
		AbsPath("/apis/metrics.k8s.io/v1beta1/pods").
		Param("labelSelector", "sytue.eu/component=cloud-app,sytue.eu/target=production").
		DoRaw(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) || apierrors.IsServiceUnavailable(err) {
			metricsMissingOnce.Do(func() {
				log.From(ctx).Info("sin metrics-server: no se muestrea el uso real " +
					"(las recomendaciones de tamaño quedarán en «sin datos suficientes»)")
			})
			return nil, nil
		}
		// Un 403 es lo mismo en la práctica —no se puede medir— pero por una causa
		// que sí se arregla, así que se dice cuál. Va por el mismo camino de una
		// sola vez: repetirlo en cada barrido llenaba el registro de un WARN cada
		// quince minutos, y con `DoRaw` el error del 403 llega sin decodificar y se
		// imprime como «unknown», que no orienta a nadie.
		if apierrors.IsForbidden(err) {
			metricsMissingOnce.Do(func() {
				log.From(ctx).Warn("sin permiso para leer metrics.k8s.io: no se muestrea el uso real " +
					"(falta el apiGroup metrics.k8s.io en el ClusterRole del controlador; " +
					"las recomendaciones de tamaño quedarán en «sin datos suficientes»)")
			})
			return nil, nil
		}
		return nil, fmt.Errorf("controller: leer metrics.k8s.io: %w", err)
	}
	return aggregatePodMetrics(raw, time.Now())
}

// podMetricsList es el subconjunto de metrics.k8s.io/v1beta1 PodMetrics que se
// usa. Struct local a propósito: importar k8s.io/metrics por esto obligaría a
// tocar go.mod, que es un fichero compartido.
type podMetricsList struct {
	Items []struct {
		Metadata struct {
			Name      string            `json:"name"`
			Namespace string            `json:"namespace"`
			Labels    map[string]string `json:"labels"`
		} `json:"metadata"`
		Containers []struct {
			Usage struct {
				CPU    string `json:"cpu"`
				Memory string `json:"memory"`
			} `json:"usage"`
		} `json:"containers"`
	} `json:"items"`
}

// aggregatePodMetrics convierte la respuesta cruda en una observación por
// proyecto, quedándose con el MÁXIMO entre sus pods.
//
// El máximo y no la media: el límite de memoria se aplica POR POD, así que el pod
// que más consume es el que muere por OOM. Promediar escondería exactamente el
// pod del que depende la decisión.
//
// Un pod sin la etiqueta de proyecto se ignora en silencio en vez de agruparse
// aparte: es un pod que aún no ha pasado por el despliegue que la añade, y
// meterlo en un cajón sin dueño solo produciría filas que nadie consulta.
func aggregatePodMetrics(raw []byte, at time.Time) ([]store.ResourceSampleInput, error) {
	var list podMetricsList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("controller: metrics.k8s.io ilegible: %w", err)
	}
	type acc struct {
		tenant string
		cpu    int
		mem    int64
	}
	byProject := make(map[uuid.UUID]*acc, len(list.Items))
	for _, it := range list.Items {
		pid, err := uuid.Parse(it.Metadata.Labels[projectLabel])
		if err != nil {
			continue
		}
		tenant := it.Metadata.Labels["sytue.eu/tenant"]
		if tenant == "" {
			continue
		}
		// Un pod puede llevar varios contenedores (sidecars): lo que se topa contra
		// el límite del pod es la SUMA, así que se suman antes de comparar.
		var cpu int
		var mem int64
		for _, cnt := range it.Containers {
			if q, perr := resource.ParseQuantity(cnt.Usage.CPU); perr == nil {
				cpu += int(q.MilliValue())
			}
			if q, perr := resource.ParseQuantity(cnt.Usage.Memory); perr == nil {
				mem += q.Value()
			}
		}
		cur, ok := byProject[pid]
		if !ok {
			byProject[pid] = &acc{tenant: tenant, cpu: cpu, mem: mem}
			continue
		}
		if cpu > cur.cpu {
			cur.cpu = cpu
		}
		if mem > cur.mem {
			cur.mem = mem
		}
	}
	out := make([]store.ResourceSampleInput, 0, len(byProject))
	for pid, a := range byProject {
		out = append(out, store.ResourceSampleInput{
			Tenant: a.tenant, ProjectID: pid, At: at,
			CPUMilli: a.cpu, MemoryBytes: a.mem,
		})
	}
	return out, nil
}
