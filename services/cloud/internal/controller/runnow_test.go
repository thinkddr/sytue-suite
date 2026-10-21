// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

func cronServicio() domain.ManagedService {
	return domain.ManagedService{
		ID: uuid.New(), Tenant: "acme", Kind: domain.ServiceCron,
		Config: map[string]string{"schedule": "0 3 * * *", "image": "img:1"},
	}
}

// La ejecución manual tiene que correr EXACTAMENTE lo que corre la automática:
// si se recompusiera el pod a mano, probar la tarea dejaría de probar nada.
func TestRunServiceNowClonaLaPlantillaDelCron(t *testing.T) {
	t.Parallel()
	svc := cronServicio()
	name := svcResourceName(svc)
	cron := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-cron", Namespace: "tenant-acme"},
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"sytue.eu/service-id": svc.ID.String()}},
				Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "task", Image: "img:1"}},
				}}},
			},
		},
	}
	cs := fake.NewSimpleClientset(cron)
	p := &KubeProvisioner{cs: cs}

	if err := p.RunServiceNow(context.Background(), svc); err != nil {
		t.Fatalf("RunServiceNow: %v", err)
	}
	jobs, err := cs.BatchV1().Jobs("tenant-acme").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("listar jobs: %v", err)
	}
	if len(jobs.Items) != 1 {
		t.Fatalf("jobs creados = %d, quiero 1", len(jobs.Items))
	}
	j := jobs.Items[0]
	if !strings.HasPrefix(j.Name, name+"-run-") {
		t.Errorf("nombre = %q; el sufijo con la hora es lo que lo hace único y legible", j.Name)
	}
	if img := j.Spec.Template.Spec.Containers[0].Image; img != "img:1" {
		t.Errorf("imagen = %q, quiero la de la plantilla del cron", img)
	}
	// La etiqueta del servicio es por donde el visor encuentra el registro de esta
	// ejecución: sin ella, correr la tarea a mano no se puede ni mirar.
	if j.Labels["sytue.eu/service-id"] != svc.ID.String() {
		t.Error("el Job manual no hereda la etiqueta del servicio")
	}
}

// Un Job ya materializado trae selector y etiquetas puestas por el controlador de
// batch. Copiarlas haría que los pods nuevos dijeran pertenecer al Job viejo: el
// API server lo rechaza, y esa es la diferencia entre «ejecutar otra vez» y un
// error que solo aparece con tareas puntuales.
func TestRunServiceNowLimpiaElSelectorDelJob(t *testing.T) {
	t.Parallel()
	svc := cronServicio()
	svc.Kind = domain.ServiceJob
	name := svcResourceName(svc)
	prev := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-job", Namespace: "tenant-acme"},
		Spec: batchv1.JobSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"controller-uid": "vieja"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
					"controller-uid": "vieja", "job-name": name + "-job", "sytue.eu/service-id": svc.ID.String(),
				}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "task", Image: "img:1"}}},
			},
		},
	}
	cs := fake.NewSimpleClientset(prev)
	p := &KubeProvisioner{cs: cs}

	if err := p.RunServiceNow(context.Background(), svc); err != nil {
		t.Fatalf("RunServiceNow: %v", err)
	}
	jobs, _ := cs.BatchV1().Jobs("tenant-acme").List(context.Background(), metav1.ListOptions{})
	var nuevo *batchv1.Job
	for i := range jobs.Items {
		if strings.Contains(jobs.Items[i].Name, "-run-") {
			nuevo = &jobs.Items[i]
		}
	}
	if nuevo == nil {
		t.Fatal("no se creó la ejecución manual")
	}
	if nuevo.Spec.Selector != nil {
		t.Error("el selector del Job anterior viajó al nuevo")
	}
	if _, sigue := nuevo.Spec.Template.Labels["controller-uid"]; sigue {
		t.Error("controller-uid del Job anterior sigue en la plantilla de pod")
	}
	if nuevo.Spec.Template.Labels["sytue.eu/service-id"] != svc.ID.String() {
		t.Error("se perdió la etiqueta del servicio al limpiar")
	}
}

// Un Postgres o un bucket no son tareas: no hay nada que «ejecutar», y la
// respuesta tiene que decirlo en vez de crear un Job vacío.
func TestRunServiceNowRechazaLoQueNoEsTarea(t *testing.T) {
	t.Parallel()
	svc := cronServicio()
	svc.Kind = domain.ServicePostgres
	p := &KubeProvisioner{cs: fake.NewSimpleClientset()}
	if err := p.RunServiceNow(context.Background(), svc); err == nil {
		t.Fatal("un servicio que no es tarea debe rechazarse")
	}
}
