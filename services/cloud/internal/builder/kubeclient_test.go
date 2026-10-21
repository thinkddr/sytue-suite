// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package builder

import (
	"context"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestBuildJobSecurityAndShape(t *testing.T) {
	c := newKubeJobClientFor(fake.NewSimpleClientset(), KubeJobConfig{BuildkitImage: "kaniko:test", CraneImage: "crane:test"})
	job := c.buildJob(JobSpec{
		Name:           "build-abc",
		Namespace:      "sytue-builds",
		Args:           buildctlArgs(KanikoRequest{Destination: "rg/x:tag", Dockerfile: "Dockerfile", RepoURL: "https://github.com/a/b"}, "main"),
		Destination:    "rg/x:tag",
		RegistrySecret: "sytue-registry",
		Labels:         map[string]string{"sytue.eu/tenant": "acme"},
		GitImage:       "git:test",
		RepoURL:        "https://github.com/a/b",
		GitRef:         "main",
	})

	if job.Name != "build-abc" || job.Namespace != "sytue-builds" {
		t.Fatalf("metadatos inesperados: %s/%s", job.Namespace, job.Name)
	}
	if *job.Spec.BackoffLimit != 0 {
		t.Errorf("BackoffLimit debe ser 0, es %d", *job.Spec.BackoffLimit)
	}
	if job.Spec.Template.Spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Errorf("RestartPolicy debe ser Never")
	}
	if job.Spec.Template.Spec.AutomountServiceAccountToken == nil || *job.Spec.Template.Spec.AutomountServiceAccountToken {
		t.Errorf("no debe montar el token de ServiceAccount")
	}

	// Clone y build son initContainers separados; push es el principal.
	clone := containerByName(job.Spec.Template.Spec.InitContainers, "git-clone")
	kaniko := containerByName(job.Spec.Template.Spec.InitContainers, "build")
	push := containerByName(job.Spec.Template.Spec.Containers, "push")
	if clone == nil || kaniko == nil || push == nil {
		t.Fatalf("se esperan initContainers git-clone/kaniko y contenedor push; init=%v main=%v",
			job.Spec.Template.Spec.InitContainers, job.Spec.Template.Spec.Containers)
	}
	if kaniko.Image != "kaniko:test" {
		t.Errorf("imagen de kaniko incorrecta: %s", kaniko.Image)
	}
	if clone.Image != "git:test" {
		t.Errorf("imagen del clonador incorrecta: %s", clone.Image)
	}
	if push.Image != "crane:test" {
		t.Errorf("imagen de crane incorrecta: %s", push.Image)
	}
	// SC restrictivo rootless en AMBOS contenedores.
	assertRestrictedRootless(t, clone.SecurityContext)
	assertRestrictedRootless(t, kaniko.SecurityContext)
	assertRestrictedRootless(t, push.SecurityContext)

	// registry-cred: el build no confiable NO monta la credencial; solo el push.
	if mountByName(kaniko.VolumeMounts, "docker-config") != nil {
		t.Error("el contenedor de build (kaniko) NO debe montar la credencial del registry (docker-config)")
	}
	if m := mountByName(push.VolumeMounts, "docker-config"); m == nil || m.MountPath != pushDockerConfigDir || !m.ReadOnly {
		t.Errorf("el contenedor de push debe montar docker-config en %s (ro): %+v", pushDockerConfigDir, push.VolumeMounts)
	}
	if !hasEnv(push.Env, "DOCKER_CONFIG", pushDockerConfigDir) {
		t.Errorf("el push debe fijar DOCKER_CONFIG=%s: %v", pushDockerConfigDir, push.Env)
	}
	// El build escribe un TARBALL y no empuja: es lo que mantiene la credencial
	// del registry fuera del contenedor que ejecuta el RUN no confiable.
	ka := strings.Join(kaniko.Args, " ")
	if !strings.Contains(ka, "dest="+kanikoTarPath) || strings.Contains(ka, "type=image") {
		t.Errorf("el build debe escribir a tarball, no empujar al registry: %v", kaniko.Args)
	}
	if strings.Contains(ka, "--cache=true") {
		t.Errorf("kaniko no debe usar cache por registry (necesitaría la cred): %v", kaniko.Args)
	}
	if pa := strings.Join(push.Args, " "); !strings.Contains(pa, "push") || !strings.Contains(pa, kanikoTarPath) || !strings.HasSuffix(pa, "rg/x:tag") {
		t.Errorf("crane debe empujar el tarball al destino: %v", push.Args)
	}

	if l := job.Labels["sytue.eu/tenant"]; l != "acme" {
		t.Errorf("label de tenant no propagado: %q", l)
	}
	// El Secret del registry debe existir como volumen docker-config (montado solo en push).
	var found bool
	for _, v := range job.Spec.Template.Spec.Volumes {
		if v.Name == "docker-config" && v.Secret != nil && v.Secret.SecretName == "sytue-registry" {
			found = true
		}
	}
	if !found {
		t.Errorf("el Secret del registry no se declaró como volumen docker-config")
	}
}

func TestSubmitCleanup(t *testing.T) {
	cs := fake.NewSimpleClientset()
	c := newKubeJobClientFor(cs, KubeJobConfig{})
	ctx := context.Background()

	name, err := c.Submit(ctx, JobSpec{Name: "j1", Namespace: "ns"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if name != "j1" {
		t.Fatalf("nombre inesperado: %s", name)
	}
	if _, gerr := cs.BatchV1().Jobs("ns").Get(ctx, "j1", metav1.GetOptions{}); gerr != nil {
		t.Fatalf("el Job no se creó: %v", gerr)
	}
	if err := c.Cleanup(ctx, "ns", "j1"); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	// Cleanup de un Job inexistente no debe error.
	if err := c.Cleanup(ctx, "ns", "nope"); err != nil {
		t.Fatalf("Cleanup de inexistente debe ser nil: %v", err)
	}
}

func TestWaitSucceeded(t *testing.T) {
	cs := fake.NewSimpleClientset(&batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "done", Namespace: "ns"},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
			},
		},
	})
	c := newKubeJobClientFor(cs, KubeJobConfig{PollInterval: 10 * time.Millisecond})
	ok, _, err := c.Wait(context.Background(), "ns", "done", nil)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if !ok {
		t.Fatalf("el Job completado debe reportar éxito")
	}
}

func TestWaitFailed(t *testing.T) {
	cs := fake.NewSimpleClientset(&batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "bad", Namespace: "ns"},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobFailed, Status: corev1.ConditionTrue},
			},
		},
	})
	c := newKubeJobClientFor(cs, KubeJobConfig{PollInterval: 10 * time.Millisecond})
	ok, _, err := c.Wait(context.Background(), "ns", "bad", nil)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if ok {
		t.Fatalf("el Job fallido no debe reportar éxito")
	}
}

func TestJobResultPending(t *testing.T) {
	if _, ok := jobResult(&batchv1.Job{}); ok {
		t.Fatalf("un Job sin condiciones no debe considerarse terminado")
	}
}

// TestBuildJobMountsGeneratedDockerfile verifica que, para frameworks gestionados
// (DockerfileContent != ""), el Job monta el Dockerfile generado (ConfigMap) de
// solo lectura en la ruta que Kaniko espera, y que 'docker' (sin contenido) no lo
// hace.
func TestBuildJobMountsGeneratedDockerfile(t *testing.T) {
	c := newKubeJobClientFor(fake.NewSimpleClientset(), KubeJobConfig{BuildkitImage: "kaniko:test"})

	// Sin contenido (p.ej. framework 'docker'): sin volumen ni montaje extra.
	if hasVolume(c.buildJob(JobSpec{Name: "b1", Namespace: "ns"}), "dockerfile") {
		t.Fatalf("sin DockerfileContent no debe montarse el ConfigMap del Dockerfile")
	}

	// Framework gestionado: se monta el Dockerfile generado en la ruta de Kaniko
	// (el build corre como initContainer 'kaniko').
	job := c.buildJob(JobSpec{Name: "b2", Namespace: "ns", DockerfileContent: "FROM x"})
	if !hasVolume(job, "dockerfile") {
		t.Fatalf("debe existir el volumen ConfigMap 'dockerfile'")
	}
	kaniko := containerByName(job.Spec.Template.Spec.InitContainers, "build")
	if kaniko == nil {
		t.Fatalf("debe existir el initContainer 'kaniko'")
	}
	vm := mountByName(kaniko.VolumeMounts, "dockerfile")
	if vm == nil {
		t.Fatalf("el contenedor de Kaniko debe montar el Dockerfile")
	}
	if vm.MountPath != generatedDockerfilePath || vm.SubPath != generatedDockerfileName || !vm.ReadOnly {
		t.Fatalf("montaje del Dockerfile incorrecto: %+v", *vm)
	}
}

// TestSubmitMaterializesDockerfileConfigMap verifica que Submit crea el ConfigMap
// con el Dockerfile generado y un OwnerReference al Job (para GC), y que no lo crea
// cuando no hay Dockerfile generado ('docker').
func TestSubmitMaterializesDockerfileConfigMap(t *testing.T) {
	cs := fake.NewSimpleClientset()
	c := newKubeJobClientFor(cs, KubeJobConfig{})
	ctx := context.Background()

	spec := JobSpec{Name: "build-df", Namespace: "ns", DockerfileContent: "FROM oven/bun:1\n"}
	name, err := c.Submit(ctx, spec)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	cm, err := cs.CoreV1().ConfigMaps("ns").Get(ctx, configMapNameFor(name), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("el ConfigMap del Dockerfile no se creó: %v", err)
	}
	if cm.Data[generatedDockerfileName] != spec.DockerfileContent {
		t.Fatalf("contenido del Dockerfile no materializado: %q", cm.Data[generatedDockerfileName])
	}
	if len(cm.OwnerReferences) == 0 || cm.OwnerReferences[0].Name != name || cm.OwnerReferences[0].Kind != "Job" {
		t.Fatalf("el ConfigMap debe tener OwnerReference al Job para GC: %+v", cm.OwnerReferences)
	}

	// Sin contenido: no se crea ConfigMap (framework 'docker').
	if _, err := c.Submit(ctx, JobSpec{Name: "build-docker", Namespace: "ns"}); err != nil {
		t.Fatalf("Submit docker: %v", err)
	}
	if _, err := cs.CoreV1().ConfigMaps("ns").Get(ctx, configMapNameFor("build-docker"), metav1.GetOptions{}); err == nil {
		t.Fatalf("no debe crearse ConfigMap cuando no hay Dockerfile generado")
	}
}

// hasVolume indica si el Pod del Job declara un volumen con ese nombre.
func hasVolume(job *batchv1.Job, name string) bool {
	for _, v := range job.Spec.Template.Spec.Volumes {
		if v.Name == name {
			return true
		}
	}
	return false
}
