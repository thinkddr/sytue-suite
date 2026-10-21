// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package runtime

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// Lo que hace que el reinicio sea RODANTE es que la anotación caiga dentro del
// pod template: en `metadata` del Deployment no crearía ReplicaSet nuevo y no
// reiniciaría nada, con la petición devolviendo éxito igual. Es exactamente el
// fallo que un patch mal anidado produce, así que se comprueba dónde aterriza.
func TestRestartDeploymentTocaElPodTemplate(t *testing.T) {
	t.Parallel()
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-abc123",
			Namespace: "tenant-acme",
			Labels:    map[string]string{"sytue.eu/deployment": "d-1"},
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"sytue.eu/deployment": "d-1"},
			}},
		},
	}
	// Un Deployment de OTRO proyecto en el mismo namespace: el selector tiene que
	// dejarlo fuera. Reiniciar de más es tirar producción ajena.
	otro := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name:      "app-zzz999",
		Namespace: "tenant-acme",
		Labels:    map[string]string{"sytue.eu/deployment": "d-2"},
	}}
	cs := fake.NewSimpleClientset(dep, otro)
	k := &KubeLogStreamer{cs: cs}

	n, err := k.RestartDeployment(context.Background(), "tenant-acme", "sytue.eu/deployment=d-1")
	if err != nil {
		t.Fatalf("RestartDeployment: %v", err)
	}
	if n != 1 {
		t.Fatalf("reiniciados = %d, quiero 1 (el otro deployment no es de este proyecto)", n)
	}

	got, err := cs.AppsV1().Deployments("tenant-acme").Get(context.Background(), "app-abc123", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("releer el deployment: %v", err)
	}
	if got.Spec.Template.Annotations["sytue.eu/restarted-at"] == "" {
		t.Error("la anotación no está en el pod template: el rollout no se dispararía")
	}
	if got.Annotations["sytue.eu/restarted-at"] != "" {
		t.Error("la anotación cayó en el Deployment, no en su plantilla de pod")
	}
}

// Sin nada que casar no es un error: un proyecto sin instancias en marcha
// devuelve cero, y esa es la respuesta que el panel enseña tal cual.
func TestRestartDeploymentSinPodsDevuelveCero(t *testing.T) {
	t.Parallel()
	k := &KubeLogStreamer{cs: fake.NewSimpleClientset()}
	n, err := k.RestartDeployment(context.Background(), "tenant-acme", "sytue.eu/deployment=d-1")
	if err != nil {
		t.Fatalf("RestartDeployment: %v", err)
	}
	if n != 0 {
		t.Fatalf("reiniciados = %d, quiero 0", n)
	}
}
