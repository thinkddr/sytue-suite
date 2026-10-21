// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// metricsJSON arma una respuesta de metrics.k8s.io con los pods dados.
func metricsJSON(pods ...string) []byte {
	out := `{"kind":"PodMetricsList","items":[`
	for i, p := range pods {
		if i > 0 {
			out += ","
		}
		out += p
	}
	return []byte(out + "]}")
}

func pod(labels string, containers ...string) string {
	c := ""
	for i, x := range containers {
		if i > 0 {
			c += ","
		}
		c += x
	}
	return fmt.Sprintf(`{"metadata":{"name":"p","namespace":"tenant-acme","labels":{%s}},"containers":[%s]}`, labels, c)
}

func cont(cpu, mem string) string {
	return fmt.Sprintf(`{"name":"app","usage":{"cpu":%q,"memory":%q}}`, cpu, mem)
}

func etiquetas(projectID string) string {
	return fmt.Sprintf(`"sytue.eu/project-id":%q,"sytue.eu/tenant":"acme","sytue.eu/component":"cloud-app"`, projectID)
}

// TestEntreReplicasSeQuedaElMaximo es la decisión que evita el peor consejo
// posible: el límite de memoria se aplica POR POD, así que promediar escondería
// justo el pod que está a punto de morir por OOM.
func TestEntreReplicasSeQuedaElMaximo(t *testing.T) {
	pid := uuid.New()
	raw := metricsJSON(
		pod(etiquetas(pid.String()), cont("100m", "100Mi")),
		pod(etiquetas(pid.String()), cont("250m", "900Mi")), // la réplica caliente
		pod(etiquetas(pid.String()), cont("120m", "150Mi")),
	)
	got, err := aggregatePodMetrics(raw, time.Now())
	if err != nil {
		t.Fatalf("agregar: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("proyectos = %d; se esperaba 1 (tres réplicas del MISMO proyecto)", len(got))
	}
	if want := int64(900 * 1024 * 1024); got[0].MemoryBytes != want {
		t.Errorf("memoria = %d; se esperaba el máximo %d (ni media ni suma)", got[0].MemoryBytes, want)
	}
	if got[0].CPUMilli != 250 {
		t.Errorf("cpu = %d; se esperaba el máximo 250", got[0].CPUMilli)
	}
	if got[0].Tenant != "acme" || got[0].ProjectID != pid {
		t.Errorf("identidad mal: %s / %s", got[0].Tenant, got[0].ProjectID)
	}
}

// TestLosContenedoresDeUnPodSeSuman: un sidecar consume del MISMO límite de pod,
// así que sumarlos es lo correcto — quedarse con el mayor infravaloraría el uso.
func TestLosContenedoresDeUnPodSeSuman(t *testing.T) {
	pid := uuid.New()
	raw := metricsJSON(pod(etiquetas(pid.String()), cont("100m", "200Mi"), cont("50m", "60Mi")))
	got, err := aggregatePodMetrics(raw, time.Now())
	if err != nil {
		t.Fatalf("agregar: %v", err)
	}
	if want := int64(260 * 1024 * 1024); got[0].MemoryBytes != want {
		t.Errorf("memoria = %d; se esperaba la suma %d", got[0].MemoryBytes, want)
	}
	if got[0].CPUMilli != 150 {
		t.Errorf("cpu = %d; se esperaba la suma 150", got[0].CPUMilli)
	}
}

// TestUnidadesRarasDeKubernetes: la CPU llega en NANOCORES en clústeres reales y
// la memoria en Ki. Leerlas como números planos daría cifras absurdas.
func TestUnidadesRarasDeKubernetes(t *testing.T) {
	pid := uuid.New()
	raw := metricsJSON(pod(etiquetas(pid.String()), cont("1500000n", "131272Ki")))
	got, err := aggregatePodMetrics(raw, time.Now())
	if err != nil {
		t.Fatalf("agregar: %v", err)
	}
	// 1 500 000 n = 1,5 milicores. MilliValue redondea hacia ARRIBA, que además es
	// la dirección prudente en una medida de recursos: sobreestimar el consumo como
	// mucho impide un ahorro; subestimarlo provoca un OOMKill.
	if got[0].CPUMilli != 2 {
		t.Errorf("cpu = %d milicores; 1500000n son 1,5 m → 2 m", got[0].CPUMilli)
	}
	if want := int64(131272 * 1024); got[0].MemoryBytes != want {
		t.Errorf("memoria = %d; 131272Ki son %d bytes", got[0].MemoryBytes, want)
	}
}

// TestPodSinProyectoSeIgnora: un pod que aún no ha pasado por el despliegue que
// añade la etiqueta no puede acabar en un cajón sin dueño.
func TestPodSinProyectoSeIgnora(t *testing.T) {
	pid := uuid.New()
	raw := metricsJSON(
		pod(`"sytue.eu/tenant":"acme"`, cont("100m", "100Mi")),                        // sin project-id
		pod(fmt.Sprintf(`"sytue.eu/project-id":%q`, pid.String()), cont("1m", "1Mi")), // sin tenant
		pod(etiquetas(pid.String()), cont("10m", "10Mi")),
	)
	got, err := aggregatePodMetrics(raw, time.Now())
	if err != nil {
		t.Fatalf("agregar: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("proyectos = %d; se esperaba 1 (los otros dos pods no son identificables)", len(got))
	}
	if got[0].CPUMilli != 10 {
		t.Errorf("se coló un pod sin identificar: cpu = %d", got[0].CPUMilli)
	}
}

// TestRespuestaIlegibleEsError: mejor un error visible que muestras a cero, que
// el motor interpretaría como «esta app no consume nada» y propondría bajarla.
func TestRespuestaIlegibleEsError(t *testing.T) {
	if _, err := aggregatePodMetrics([]byte("no soy json"), time.Now()); err == nil {
		t.Fatal("se esperaba error al leer una respuesta ilegible")
	}
}

// TestElMuestreoSeEspacia: el ciclo corre cada 15 s y la API de métricas no se
// puede llamar a ese ritmo.
func TestElMuestreoSeEspacia(t *testing.T) {
	c := &Controller{}
	base := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)
	if !c.sampleDue(base) {
		t.Fatal("el primer muestreo siempre toca")
	}
	if c.sampleDue(base.Add(15 * time.Second)) {
		t.Error("15 s después NO toca: el ciclo es mucho más rápido que el muestreo")
	}
	if !c.sampleDue(base.Add(sampleEvery + time.Second)) {
		t.Error("pasado el intervalo sí toca")
	}
}

// TestSinCapacidadDeMuestreoEsNoOp: en dev el provisioner es un stub sin clúster,
// y el paso no puede romper el ciclo por eso.
func TestSinCapacidadDeMuestreoEsNoOp(t *testing.T) {
	c := &Controller{provider: &StubProvisioner{}}
	if err := c.reconcileResourceSamples(context.Background()); err != nil {
		t.Fatalf("con un provisioner sin métricas debe ser no-op: %v", err)
	}
}

// TestElPasoDeBenchmarksPublicaLoQueAGRUPA cierra el hueco clásico de este
// repositorio: código correcto que nadie llama. Comprueba que el paso del ciclo
// va de verdad de la lectura cross-tenant a la publicación, y que por el camino
// aplica el umbral de k-anonimato.
func TestElPasoDeBenchmarksPublicaLoQueAGRUPA(t *testing.T) {
	usos := make([]domain.ProjectUsage, 0, domain.MinCohortTenants+1)
	for i := range domain.MinCohortTenants {
		usos = append(usos, domain.ProjectUsage{
			Tenant:           fmt.Sprintf("empresa-%d", i),
			Cohort:           domain.CohortKey{Framework: "go", Size: "standard"},
			P95MemoryPercent: 20,
		})
	}
	// Una cohorte de un solo cliente, que NO puede salir publicada.
	usos = append(usos, domain.ProjectUsage{
		Tenant:           "solitaria",
		Cohort:           domain.CohortKey{Framework: "docker", Size: "pro"},
		P95MemoryPercent: 90,
	})

	fs := &fakeCtrlStore{cohortUsage: usos}
	c := &Controller{store: fs}
	if err := c.reconcileCohorts(context.Background()); err != nil {
		t.Fatalf("reconcileCohorts: %v", err)
	}
	if len(fs.publishedCohorts) != 1 {
		t.Fatalf("publicadas %d cohortes: %+v", len(fs.publishedCohorts), fs.publishedCohorts)
	}
	if got := fs.publishedCohorts[0]; got.Key.Framework != "go" || got.Tenants != domain.MinCohortTenants {
		t.Errorf("cohorte publicada = %+v", got)
	}

	// Y no se recalcula en cada vuelta del ciclo: la consulta recorre todos los
	// inquilinos y es lo más caro que hace el controlador.
	if err := c.reconcileCohorts(context.Background()); err != nil {
		t.Fatalf("segunda vuelta: %v", err)
	}
	if len(fs.publishedCohorts) != 1 {
		t.Errorf("se republicó dentro del intervalo: %d", len(fs.publishedCohorts))
	}
}
